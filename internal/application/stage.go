package application

// Staging: what the player has changed, before any of it is written.
//
// Everything here is an overlay on the loaded save and nothing here touches a
// file. The one exception proves the rule: staging a reset has to read the save
// again to learn what differs from shipped values, and it refuses if the file
// changed since Load, because a restoration computed against a different save
// is not a restoration of this one.
//
// A staged change is keyed by what it targets, so editing the same Ash twice
// replaces rather than accumulates. That is what lets a pending count be a
// count of real differences and an undo be a deletion rather than an inverse
// edit somebody has to compute.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// StagedFormation is one Ash's formation as the player has composed it.
//
// It carries the plan it would contribute to a commit, built when it was
// staged. Building it now rather than at Save is what makes a frontend able to
// show the consequence of a drop immediately - which rows are filled, which are
// retired, which actors need a clone inserted - and what makes a formation that
// cannot be written fail while the player is still looking at it.
type StagedFormation struct {
	HostGoodsFamily int32
	HostName        string
	// Requested is exactly what the player asked for, in slot order.
	Requested []AuthoredSlot
	// Formation is what would be written, resolved against the loaded save.
	// It is this formation placed by itself; what Save writes is placed over
	// every staged formation together (PrepareCommit, clonealloc.go), so the
	// clone rows a commit's summary names can differ from these - a row freed
	// by the prune, or one another staged formation placed first.
	Formation AuthoredFormation

	// manifests are what it writes, one a block, and units what they are
	// compiled from, kept so Save can place them again beside every other
	// staged formation. Every Ash has one block but the Giant Rat, which has
	// one for each range of levels (unitsFor).
	manifests []*plan.Manifest
	units     []authoredUnit
}

// StagedConversion is one Ash re-pointed at another family's formation.
//
// Unlike a formation it carries no plan of its own. Conversions are not
// independent of one another: they share the one shipping recipe that makes any
// of them work, so all of the staged ones become a single part of the commit
// rather than one part each.
type StagedConversion struct {
	PlannedConversion

	// conversion is the per-level plan this one contributes. It is kept rather
	// than recomputed so that what a frontend was shown when the player staged
	// it is what the commit writes.
	conversion plan.Conversion
}

// StagedRestoration is a staged reset: the whole save returned to shipped
// behaviour, held as pending changes rather than performed.
//
// This is the departure from what the command line does, and the reason the
// session exists. A reset stops being an operation that excludes the others and
// becomes a source of staged changes, so a player can reset, keep editing,
// change their mind, and have none of it be real until they save.
type StagedRestoration struct {
	// GoodsRows, BuddyRows and StoneRows are how many rows differ from shipped
	// values, before anything else staged is allowed to override them.
	GoodsRows, BuddyRows, StoneRows int
	// RowsRemoved is how many rows this editor added that the reset would take
	// back out.
	RowsRemoved int
	// SummonRows is how many of the game's own summon rows an earlier Fighter
	// left, which the reset makes Helpers again (rowstrength.go), and Colours
	// whether the colour switch goes back to the game's spirit look (tint.go).
	SummonRows int
	Colours    bool
	// Characters is how many custom characters the save holds, which the
	// reset deletes (builds.go).
	Characters int
	// StoneRecipeApplied reports the expanded-area recipe sitting on every
	// stone row, which is what a conversion by this product leaves behind.
	StoneRecipeApplied bool

	// manifest is nil when the summon rows are all there is to put back.
	manifest *plan.Manifest
}

// StageFormation records a formation the player composed for one Ash.
//
// The host is an exact case-insensitive name or a numeric goods-family ID, and
// the slots name actors by their world rows exactly as the command line does. A
// second call for the same Ash replaces the first.
func (session *Session) StageFormation(host string, slots []AuthoredSlot) (*StagedFormation, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	if len(slots) == 0 {
		return nil, ErrNoSlots
	}
	ash, err := catalog.Resolve(session.service.ashes, host)
	if err != nil {
		return nil, &UnknownAshError{Role: "host", Selector: host, cause: err}
	}
	if err := session.canAuthor(ash.GoodsFamily, ash.Name, ash.TriggerFamilies[0]); err != nil {
		return nil, err
	}
	if err := ashLevelsBuilt(slots); err != nil {
		return nil, err
	}
	ruled, err := session.service.applyRules(slots)
	if err != nil {
		return nil, err
	}
	units, err := session.unitsFor(ash, ruled)
	if err != nil {
		return nil, err
	}
	manifests, described, err := compileAuthored(context.Background(), session.loaded.info.Path,
		session.loaded.info.SHA256, units, session.loaded.formations, session.service.families)
	if err != nil {
		return nil, err
	}

	staged := &StagedFormation{
		HostGoodsFamily: ash.GoodsFamily,
		HostName:        ash.Name,
		Requested:       slices.Clone(slots),
		Formation:       described,
		manifests:       manifests,
		units:           units,
	}
	session.formations[ash.GoodsFamily] = staged
	return staged, nil
}

// unitsFor is one formation as the blocks it is written into: the Ash's one
// block, or the Giant Rat's three, each with the levels that reach it, so the
// summons the player placed are the ones every level summons.
func (session *Session) unitsFor(ash catalog.Ash, slots []AuthoredSlot) ([]authoredUnit, error) {
	levels, err := levelBlocks(ash)
	if err != nil {
		return nil, err
	}
	strength := session.service.authoredStrength(slots)
	units := make([]authoredUnit, 0, len(ash.TriggerFamilies))
	for _, trigger := range ash.TriggerFamilies {
		block, err := session.blockFor(ash, trigger)
		if err != nil {
			return nil, err
		}
		units = append(units, authoredUnit{host: ash, slots: slots, block: block, levels: levels[trigger], strength: strength})
	}
	return units, nil
}

// levelBlocks is which upgrade levels reach each of an Ash's blocks, as the
// game ships its goods rows: a row's trigger less its own level is its block
// (Giant Rat +7 is 233107, in 233100). Nil for an Ash with one block, which
// every level reaches.
func levelBlocks(ash catalog.Ash) (map[int32][]int, error) {
	if len(ash.TriggerFamilies) == 1 {
		return nil, nil
	}
	snapshot, err := reference.Vanilla()
	if err != nil {
		return nil, fmt.Errorf("read the embedded shipped-value snapshot: %w", err)
	}
	levels := make(map[int32][]int, len(ash.TriggerFamilies))
	for level := 0; level < ash.UpgradeLevels; level++ {
		row, shipped := snapshot.GoodsRow(ash.GoodsFamily + int32(level))
		block := int32(row["refId_default"]) - int32(level)
		if !shipped || !slices.Contains(ash.TriggerFamilies, block) {
			return nil, fmt.Errorf("%s +%d reaches no block of its own: %w", ash.Name, level, ErrRowNotFound)
		}
		levels[block] = append(levels[block], level)
	}
	return levels, nil
}

// StageConversion records one Ash re-pointed at another family's formation. A
// second call for the same host replaces the first.
func (session *Session) StageConversion(host, destination string) (*StagedConversion, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	pairs, err := session.service.resolvePairs([]ConversionRequest{{Host: host, Destination: destination}})
	if err != nil {
		return nil, err
	}
	if authored, clash := session.formations[pairs[0].host.GoodsFamily]; clash {
		return nil, &HostStagedTwiceError{
			Name: authored.HostName, GoodsFamily: authored.HostGoodsFamily, Destination: destination,
		}
	}
	// Planned now, against the loaded save, so a destination an earlier edit
	// re-pointed is refused while the player is still choosing rather than at
	// Save. What it produces is also what a frontend shows: which formation
	// each upgrade level would deliver, and whether that is already true.
	conversions, resolved, err := planConversions(pairs, session.loaded.state)
	if err != nil {
		return nil, err
	}
	planned := describeConversion(conversions[0], resolved[0], session.loaded.state)

	staged := &StagedConversion{PlannedConversion: planned, conversion: conversions[0]}
	session.conversions[planned.HostGoodsFamily] = staged
	return staged, nil
}

// StageReset stages a return to shipped behaviour for the whole save.
//
// It writes nothing. What it produces is the same restoration Reset would
// perform, held as pending changes: the rows whose values differ from the ones
// the game shipped, and the rows this editor added that the game did not.
//
// It re-reads the loaded save, because a reset is a diff and there is nothing
// in memory to diff against - Load reads what this product writes, and a reset
// restores rows outside that. A file that changed since Load is refused rather
// than diffed, since a restoration computed against a different save is not a
// restoration of this one.
func (session *Session) StageReset(ctx context.Context) (*StagedRestoration, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	// A loaded save is of a supported version, and every supported version is
	// row-for-row identical to the snapshot's, so its shipped values apply.
	shipped, err := shippedVanilla()
	if err != nil {
		return nil, err
	}
	current, snapshot, err := currentVanilla(ctx, session.loaded.info.Path, shipped)
	if err != nil {
		return nil, err
	}
	if snapshot.SourceSHA256 != session.loaded.info.SHA256 {
		return nil, fmt.Errorf("%s changed since it was loaded: %w",
			session.loaded.info.Path, savecore.ErrSourceMismatch)
	}

	manifest, err := plan.BuildReset(session.loaded.info.SHA256, shipped, current)
	// The game's own summon rows an earlier Fighter left go back too, the
	// colour switch, and the custom characters go; they are written at Save beside whatever the player chose
	// for them (commit.go).
	summons, colours, characters := len(session.helpersBack()), session.coloursChanged(), session.heldBuilds()
	if errors.Is(err, plan.ErrAlreadyVanilla) && (summons > 0 || colours || characters > 0) {
		manifest, err = nil, nil
	}
	if err != nil {
		// Nothing differs from shipped values, so there is no reset to stage.
		// A frontend should say the save is unedited rather than report a
		// failure the player cannot act on.
		return nil, err
	}
	// A reset invalidates nothing else that is staged. It only makes authoring
	// into a re-pointed Ash legitimate, so it needs no check of its own.
	staged := &StagedRestoration{SummonRows: summons, Colours: colours, Characters: characters, StoneRecipeApplied: stoneRecipeApplied(snapshot), manifest: manifest}
	if manifest != nil {
		staged.GoodsRows = len(manifest.GoodsPatches)
		staged.BuddyRows = len(manifest.BuddyPatches)
		staged.StoneRows = len(manifest.BuddyStoneRowPatches)
		staged.RowsRemoved = len(manifest.BuddyRowRemovals) + len(manifest.NpcCloneRemovals) + len(manifest.NpcThinkCloneRemovals)
	}
	session.restoration = staged
	return staged, nil
}

// Unstage drops whatever is staged against one Ash - a formation, a conversion,
// or both - and reports whether anything was there.
func (session *Session) Unstage(goodsFamily int32) bool {
	_, hadFormation := session.formations[goodsFamily]
	_, hadConversion := session.conversions[goodsFamily]
	delete(session.formations, goodsFamily)
	delete(session.conversions, goodsFamily)
	return hadFormation || hadConversion
}

// UnstageReset drops a staged reset and reports whether one was there.
func (session *Session) UnstageReset() bool {
	had := session.restoration != nil
	session.restoration = nil
	return had
}

// ClearStaged discards every pending change. The loaded save is untouched: it
// was never written to.
func (session *Session) ClearStaged() {
	session.formations = make(map[int32]*StagedFormation)
	session.conversions = make(map[int32]*StagedConversion)
	session.costs = make(map[int32]*StagedCost)
	session.restoration = nil
	session.grants = nil
	session.builds = nil
	session.dropped = nil
	session.wideArea, session.noRestLimit = nil, nil
	session.DropStrength()
}

// PendingCount is how many staged changes there are, counting a reset as one.
//
// It counts intents rather than rows, because that is what the player made and
// what they can drop one at a time. How many rows those intents write is the
// commit's business and is reported there.
func (session *Session) PendingCount() int {
	count := len(session.formations) + len(session.conversions) + len(session.costs) + len(session.grants) +
		len(session.builds) + len(session.dropped)
	if session.restoration != nil {
		count++
	}
	if session.strength != nil {
		count++
	}
	if session.wideArea != nil {
		count++
	}
	if session.noRestLimit != nil {
		count++
	}
	return count
}

// Staged is an ordered snapshot of everything pending, for display. The lists
// are sorted by host goods family so a frontend renders the same order twice.
type Staged struct {
	Formations  []*StagedFormation
	Conversions []*StagedConversion
	Costs       []*StagedCost
	Restoration *StagedRestoration
	// Grants are the Ashes going into a character's inventory, ordered by slot
	// and then by Ash.
	Grants []*StagedGrant
	// Strength is the roles chosen for the game's own summon rows, when the save
	// does not already hold them. It is one entry however many rows it writes.
	Strength *StagedStrength
	// Area is the monument switches, present only when one was moved.
	Area *StagedArea
	// Builds are the characters built from choices, by number, and
	// DroppedBuilds the numbers of those the save holds that are being
	// deleted.
	Builds        []*StagedBuild
	DroppedBuilds []int
}

// Staged returns what is pending. The slices are fresh, so a frontend holding
// one cannot change what the session will write.
func (session *Session) Staged() Staged {
	staged := Staged{Restoration: session.restoration, Strength: session.strength, Area: session.stagedArea()}
	for _, family := range sortedFamilies(session.formations) {
		staged.Formations = append(staged.Formations, session.formations[family])
	}
	for _, family := range sortedFamilies(session.conversions) {
		staged.Conversions = append(staged.Conversions, session.conversions[family])
	}
	for _, family := range sortedFamilies(session.costs) {
		staged.Costs = append(staged.Costs, session.costs[family])
	}
	staged.Grants = session.stagedGrants()
	staged.Builds = session.stagedBuilds()
	staged.DroppedBuilds = slices.Sorted(maps.Keys(session.dropped))
	return staged
}

// sortedFamilies is every key of a staging map, in order, so the same staged
// session always produces the same manifest.
func sortedFamilies[V any](staged map[int32]V) []int32 {
	return slices.Sorted(maps.Keys(staged))
}

// canAuthor reports whether one Ash may have a formation staged against it.
//
// Two things stop it. A conversion re-points the host at another family's
// formation while an authored formation rewrites the host's own, so together
// they name two different formations for one item. And an Ash an earlier edit
// re-pointed no longer delivers its own formation, so authoring into its block
// would rewrite an item the player did not name - unless a reset is staged,
// which puts it back, and then the block the formation is composed against is
// the one the Ash will deliver.
//
// It takes the three facts it needs rather than a catalog entry, so that the
// commit-time re-check below can ask it about a formation that is already
// staged without resolving the Ash a second time.
func (session *Session) canAuthor(goodsFamily int32, name string, shipped int32) error {
	if err := refuseFixed(goodsFamily, name); err != nil {
		return err
	}
	if conversion, clash := session.conversions[goodsFamily]; clash {
		return &HostStagedTwiceError{
			Name: name, GoodsFamily: goodsFamily, Destination: conversion.DestinationName,
		}
	}
	if current, known := session.loaded.state.Goods[goodsFamily]; known &&
		current.Trigger != shipped && session.restoration == nil {
		return &HostRePointedError{Name: name, Shipped: shipped, Found: current.Trigger}
	}
	return nil
}

// validateStaged re-checks the staged set when a commit is prepared.
//
// Staging refuses what it can at the moment it happens, but *dropping* a change
// can invalidate one that is still staged: a formation authored into a
// re-pointed Ash is legitimate only while the reset that restores it is there
// too. Re-judging every staged change on every drop would be the alternative,
// and asking once at the commit is both simpler and impossible to forget.
func (session *Session) validateStaged() error {
	for family, staged := range session.formations {
		// The trigger the formation was composed against is the Ash's own, by
		// construction: StageFormation authors into TriggerFamilies[0].
		if err := session.canAuthor(family, staged.HostName, staged.Formation.TriggerSpEffectID); err != nil {
			return err
		}
	}
	// A formation staged after a deletion may name the character again.
	named := session.namedBuilds()
	for n := range session.dropped {
		if named[n] {
			return fmt.Errorf("custom character %d is being deleted: %w", n+1, ErrBuildNamed)
		}
	}
	return nil
}

// describeConversion is what a frontend shows about one staged conversion: the
// formation each upgrade level would deliver, and whether the save already says
// so.
func describeConversion(conversion plan.Conversion, resolved ResolvedConversion, state *savecore.State) PlannedConversion {
	applied := true
	for level := 0; level < conversion.UpgradeLevels; level++ {
		if state.Goods[conversion.HostGoodsFamily+int32(level)].Trigger != conversion.LevelTriggers[level] {
			applied = false
			break
		}
	}
	formations := levelFormations(conversion.LevelTriggers)
	return PlannedConversion{
		ResolvedConversion: resolved,
		UpgradeLevels:      conversion.UpgradeLevels,
		CurrentTrigger:     state.Goods[conversion.HostGoodsFamily].Trigger,
		LevelTriggers:      conversion.LevelTriggers,
		LevelFormations:    formations,
		VariesByLevel:      variesByLevel(formations),
		AlreadyApplied:     applied,
	}
}
