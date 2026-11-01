package application

// Authoring: the workflow that turns actors a player chose into a formation.
//
// It is the third operation this service exposes, and the first that writes
// something the game never shipped. What makes it safe is not a check placed
// here but the shape of the manifest it builds: an authored plan carries no
// goods patch and no removal, so it can neither re-point an item nor delete a
// row, and the save core refuses one that tries.
//
// The division of labour is the one every operation here follows. This layer
// resolves what the user named and reads what the save says; internal/formation
// decides which rows that becomes; internal/plan writes it down; internal/
// savecore performs and proves it.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// MaxFormationSlots is the most actors one formation can hold.
const MaxFormationSlots = formation.MaxSlots

// AuthoredSlot is one actor a player put in a formation, named by the rows that
// define it.
//
// Think may be left zero, and then the actor thinks with the row that carries
// its own number. That is what the shipped table does for the summons this
// product has read - a block row names the same ID in both fields - but it is a
// default and not a rule, so a caller that knows better may say so.
//
// Both are named as the world row wherever a world row exists. Turning that
// into the summon-side row is internal/formation's arithmetic, and keeping it
// there is what stops any caller naming a destination of its own.
type AuthoredSlot struct {
	Npc   int32
	Think int32
	// Stance is how this actor moves once summoned. The zero value follows,
	// which is what 140 of the 170 shipped rows carry and what a caller that
	// has not thought about it means. See plan.Stance for what is known about
	// the values and what is not.
	Stance Stance
	// Placement is where this actor stands relative to the player, or nil to
	// let the formation lay itself out in a ring. See plan.Placement.
	Placement *Placement
	// Residents are resident effects added to the clone on top of the summon
	// recipe, in its empty slots, whichever its role. This is how a console
	// seat carries an
	// actor's placement-granted effects - the Stormhill knight's Bloody Slash
	// is two effects the map grants and the world row never holds - so the
	// question "does the clone keep the move" can be asked at all.
	//
	// Left empty, the author fills it with the actor's own granted set from
	// the catalog (Variant.Granted): a creature the map grants an effect is
	// delivered with it. BareRow turns that off.
	Residents []int32
	// BareRow summons the row as the game ships it, without the effects its
	// placement grants. A research seat that compares the granted creature
	// against the plain one needs the plain one; a player never does.
	BareRow bool
	// Look is how the summon is drawn: the ghost or its own colours, where
	// its creature lets the player choose (CreatureRules). Staging writes it
	// as the presence.
	Look Look
	// Presence is the effect this creature's clone carries in place of
	// 295000, the Spirit Summon presence - a copy of it with another look
	// (the per-creature spirit look). Zero keeps 295000. It is part of the
	// slot's recipe: two slots of one creature asking two looks are two
	// clones. A research seat names it directly; a frontend chooses a Look.
	Presence int32
	// marker is the companion marker the clone carries in place of 297000,
	// and arrival the animation it arrives on, which staging takes from the
	// creature's rules (CreatureRules).
	marker, arrival int32
	// Perception is how far the summon looks for a target on its own. The
	// zero value is the Follower, the profile the game's own Spirit Ashes
	// carry: near-blind, and fighting what the player fights. See
	// formation.Perception.
	Perception Perception
	// Person is the CharaInitParam row this slot summons instead of a
	// creature, on the person shell Npc names (one of the game's own:
	// 100000010, 100000020, ...). Zero is a creature. See formation.Actor.
	Person int32
	// Fighter summons this creature as itself rather than as a Helper: no
	// balance package, and its own health (strength.go). It is part of the
	// clone's recipe, so a Helper and a Fighter of one creature are two rows.
	// Only a creature's world row is cloned: a slot on a row the save holds -
	// the game's own, a person's shell, a clone kept as held - takes that row's
	// strength (ErrHeldRowStrength; Session.SetRowFighter).
	Fighter bool
	// Team is the team the summon stands on. Zero is the Spirit Summon team
	// every Ash the game ships carries; TeamEnemy and the other named teams
	// make the summon hostile to the player as well. A team on a person
	// copies the
	// shell (formation.SummonCopyRow); a team on a creature rides on its
	// clone. See formation.Team.
	Team Team
	// WorldBranch summons the actor without companion movement (297000), so
	// its logic script runs the world branch: it neither follows nor warps,
	// is not leashed to the player and is fed no target by them - a real
	// enemy delivered by an Ash, or a Loner. See formation.Actor.WorldBranch.
	WorldBranch bool
	// WorldBranchUnheld: the world-branch clone as the engine leaves it - no
	// 5011, no shipped Ash home band - which walks home blind. Research only;
	// formation.Actor.WorldBranchUnheld.
	WorldBranchUnheld bool
	// Share is the box beside the level: whether this creature takes the
	// player's target. The zero value is the level's default
	// - on for the Follower, off for the Guard, the Sentinel and the Hunter -
	// and the Statue, the Loner and every enemy refuse a choice. The slot's
	// Stance must carry it; LevelStance says which stance that is.
	Share Share
}

// Share is re-exported from the formation layer for the reason Stance is.
type Share = formation.Share

const (
	ShareDefault = formation.ShareDefault
	ShareOn      = formation.ShareOn
	ShareOff     = formation.ShareOff
)

// LevelShare is formation.LevelShare: a level's default share, whether the
// writer takes a choice on it, and whether the level has a box at all.
func LevelShare(p Perception) (defaultOn, choosable, known bool) { return formation.LevelShare(p) }

// ShareOffered is the share box a frontend offers on a level: its default,
// and whether the player chooses it (a frontend greys the box out when not).
// It is LevelShare less the Follower, which always shares: by itself it
// notices only two metres past its body, so off it would fight nothing. The
// writer still takes a Follower off, because 28 of the game's own Ashes are
// shipped that way and arrive on the Follower as they are.
func ShareOffered(p Perception) (defaultOn, choosable bool) {
	defaultOn, choosable, _ = formation.LevelShare(p)
	return defaultOn, choosable && p != PerceptionFollower
}

// LevelStance is the stance a slot must carry for its level and share: the
// level's follow type where it fixes one, the product's follow otherwise, and
// the share the slot resolves to. False off the ladder, where the stance is
// taken as given. A frontend fills the slot's stance from it rather than
// having the slot refused.
func LevelStance(slot AuthoredSlot) (Stance, bool) { return formation.LevelStance(slot.resolve()) }

// CloneSource is formation.CloneSource: the world row a summon-side clone in
// any band was copied from. A display that labels what a save holds needs it:
// a creature on a second recipe is summoned through a clone in a higher band
// (formation.CloneBands), which no label map names directly.
func CloneSource(row int32) (int32, bool) { return formation.CloneSource(row) }

// Team is re-exported from the formation layer for the reason Perception is.
type Team = formation.Team

var (
	TeamSummon     = formation.TeamSummon
	TeamEnemy      = formation.TeamEnemy
	TeamBeast      = formation.TeamBeast
	TeamHostileNPC = formation.TeamHostileNPC
	TeamArchEnemy  = formation.TeamArchEnemy
)

// Perception is re-exported from the formation layer for the reason Stance
// is.
type Perception = formation.Perception

var (
	// The ladder the product offers (docs/DESIGN.md, the ladder).
	PerceptionFollower = formation.PerceptionFollower
	PerceptionStatue   = formation.PerceptionStatue
	PerceptionGuard    = formation.PerceptionGuard
	PerceptionSentinel = formation.PerceptionSentinel
	PerceptionHunter   = formation.PerceptionHunter
	PerceptionLoner    = formation.PerceptionLoner

	// Research levels, for the author's console test tool.
	PerceptionEyes  = formation.PerceptionEyes
	PerceptionBlind = formation.PerceptionBlind
	PerceptionWorld = formation.PerceptionWorld

	// PerceptionAsHeld marks a slot re-staged as the save holds it.
	PerceptionAsHeld = formation.PerceptionAsHeld

	// Ladder is the levels the product offers, the default first.
	Ladder = formation.Ladder
)

// Placement is re-exported for the reason Stance is: a frontend that already
// holds AuthoredSlot should not need a second import to say where an actor
// stands.
type Placement = plan.Placement

// RingPlacement is where a slot with no placement of its own stands: index of
// count, evenly round the player. A frontend drawing a layout draws such a slot
// where the writer will put it.
func RingPlacement(index, count int) Placement { return formation.RingPlacement(index, count) }

// PlacementLimit and FacingLimit are how far a placement may go.
//
// Far wider than the shipped envelope of x -1.8..1.8 and z -1.5..1.5 on
// purpose: a large creature inside that envelope arrives on top of the player.
// Actors placed at up to 25 metres arrive where they are put, and nothing has
// been seen failing at any distance, so the bound is wide enough to be found
// by testing and narrow enough to refuse a typo.
const (
	PlacementLimit = 1000.0
	FacingLimit    = 180.0
)

// ValidPlacement refuses a placement the write layer would refuse anyway, so a
// frontend hears about it while the player is still typing rather than at the
// commit.
func ValidPlacement(place Placement) error {
	for name, value := range map[string]float64{"x": place.X, "z": place.Z} {
		if value < -PlacementLimit || value > PlacementLimit {
			return fmt.Errorf("%s offset %g is outside %g..%g metres",
				name, value, -PlacementLimit, PlacementLimit)
		}
	}
	if place.Angle < -FacingLimit || place.Angle > FacingLimit {
		return fmt.Errorf("facing %g is outside %g..%g degrees",
			place.Angle, -FacingLimit, FacingLimit)
	}
	return nil
}

// Stance is re-exported from the plan layer for the reason Price is: a frontend
// staging a formation already holds AuthoredSlot, and naming a stance must not
// cost it another import.
type Stance = plan.Stance

var (
	StanceFollow      = plan.StanceFollow
	StanceWander      = plan.StanceWander
	StanceHold        = plan.StanceHold
	StanceIndependent = formation.StanceIndependent
	StanceStill       = plan.StanceStill
)

// slot is the formation-layer form of an authored slot: the actor, how it
// moves, and where it stands.
func (slot AuthoredSlot) slot(families []catalog.Family) (formation.Slot, error) {
	actor, err := slot.withBody(families)
	if err != nil {
		return formation.Slot{}, err
	}
	return formation.Slot{Actor: actor, Stance: slot.Stance, Placement: slot.Placement}, nil
}

// resolve fills in the default half. It does not validate; the save says which
// rows exist and this type has not seen it.

func (slot AuthoredSlot) resolve() formation.Actor {
	think := slot.Think
	if think == 0 {
		think = slot.Npc
	}
	return formation.Actor{Npc: slot.Npc, Think: think, Perception: slot.Perception, Person: slot.Person, Team: slot.Team, WorldBranch: slot.WorldBranch, WorldBranchUnheld: slot.WorldBranchUnheld, Share: slot.Share, Presence: slot.Presence, Marker: slot.marker, Fighter: slot.Fighter, Arrival: slot.arrival}
}

// withBody is the actor with its family's body (shapeOf) - the collision
// radius and the front the ladder measures its reach from - and whether its
// battle script can turn. A family the catalog does not know leaves both
// zero, and the eyes level then moves the nose alone.
func (slot AuthoredSlot) withBody(families []catalog.Family) (formation.Actor, error) {
	actor := slot.resolve()
	model := fmt.Sprintf("c%04d", slot.Npc/10000)
	for _, family := range families {
		if family.Model == model {
			shape, err := shapeOf(family)
			if err != nil {
				return formation.Actor{}, err
			}
			actor.Body, actor.Front = shape.Radius, shape.Front
			break
		}
	}
	never, err := catalog.NeverTurns(actor.Think)
	if err != nil {
		return formation.Actor{}, err
	}
	actor.NoTurn = never
	return actor, nil
}

// AuthoringRequest names the Ash whose formation is being rewritten and what to
// put in it. Host is an exact case-insensitive name or a numeric goods-family
// ID, exactly as a conversion names one.
type AuthoringRequest struct {
	Host  string
	Slots []AuthoredSlot
}

// PlannedSlot is one slot of the formation as it will be written.
type PlannedSlot struct {
	// Slot is the position in the formation, from 1.
	Slot int
	// RowID is the BuddyParam row that carries it.
	RowID int32
	// Requested is what the caller named; Npc and Think are the summon-side
	// rows the game will actually read, which differ whenever the requested
	// actor is a world row that has to be cloned first.
	Requested  AuthoredSlot
	Npc, Think int32
	// Inserted means this row does not exist yet and will be added to the block.
	Inserted bool
	// Place is where this actor will stand: what the caller asked for, or the
	// ring the formation laid out for it. Reported because "unstated" now has
	// an answer, and a player who did not choose one should still be able to
	// see what they got.
	Place Placement
}

// PlannedClone is one actor that cannot be summoned until a row is inserted for
// it. Either half may already exist, so both are reported separately.
type PlannedClone struct {
	SourceNpc, SourceThink   int32
	Npc, Think               int32
	InsertsNpc, InsertsThink bool
}

// AuthoredFormation is the complete description of what authoring will write.
type AuthoredFormation struct {
	HostGoodsFamily int32
	HostName        string
	// TriggerSpEffectID is the block being rewritten. It is the host's own
	// shipped trigger, which every upgrade level of the family resolves, so one
	// edit reaches all eleven levels.
	TriggerSpEffectID int32
	// ShippedRows is how many rows the block holds before authoring.
	ShippedRows int
	Slots       []PlannedSlot
	// Retired are block rows the formation does not need. They lose their
	// trigger, which is how the game spells an inert row, and name the actor
	// the game ships for that row (retiredValues), so no retired row keeps an
	// unused creature row in the save.
	Retired []int32
	Clones  []PlannedClone
}

// AuthoringPreview describes an authored formation that has not been written.
type AuthoringPreview struct {
	Save      *SaveInfo
	Formation AuthoredFormation
	// BuddyRows is how many rows would be written, RowsInserted how many rows
	// do not exist yet, counting the actor rows a clone needs.
	BuddyRows    int
	RowsInserted int
	OutputPath   string
	// Manifest is exactly the audit artifact Author would write.
	Manifest []byte
}

// PreviewAuthoring reports exactly what Author would do, and writes nothing.
func (service *Service) PreviewAuthoring(ctx context.Context, request Request) (*AuthoringPreview, error) {
	prepared, err := service.prepareAuthoring(ctx, request, optionalOutputPath)
	if err != nil {
		return nil, err
	}
	return prepared.preview, nil
}

// Author writes a formation the player composed into the host Ash's own block.
//
// It never touches the input save, and it writes no goods row: the host keeps
// pointing at its own formation and every upgrade level of it keeps working,
// because what changed is the formation and not what resolves it.
func (service *Service) Author(ctx context.Context, request Request) (*Result, error) {
	// The refusals that belong to this request shape rather than to staging:
	// a request naming both an authored formation and a conversion, and one
	// naming no slots at all.
	if err := checkPaths(request, requireOutputPath); err != nil {
		return nil, err
	}
	if len(request.Conversions) > 0 {
		return nil, ErrAuthoringTakesNoConversions
	}
	if request.Authoring == nil || len(request.Authoring.Slots) == 0 {
		return nil, ErrNoSlots
	}
	if err := ashLevelsBuilt(request.Authoring.Slots); err != nil {
		return nil, err
	}
	return service.oneShot(ctx, request, func(session *Session) error {
		_, err := session.StageFormation(request.Authoring.Host, request.Authoring.Slots)
		return err
	})
}

type authoringPreparation struct {
	preview *AuthoringPreview
}

// prepareAuthoring is to Author what prepare is to Apply: one resolution and
// one plan, shared, so a preview cannot promise something the write would not
// do.
func (service *Service) prepareAuthoring(ctx context.Context, request Request, needOutput bool) (*authoringPreparation, error) {
	if err := checkPaths(request, needOutput); err != nil {
		return nil, err
	}
	// A conversion re-points the host at another family's formation while
	// authoring rewrites the host's own. Asked for together they describe two
	// different formations for one item, and there is no order that satisfies
	// both.
	if len(request.Conversions) > 0 {
		return nil, ErrAuthoringTakesNoConversions
	}
	if request.Authoring == nil || len(request.Authoring.Slots) == 0 {
		return nil, ErrNoSlots
	}
	if err := ashLevelsBuilt(request.Authoring.Slots); err != nil {
		return nil, err
	}

	host, err := catalog.Resolve(service.ashes, request.Authoring.Host)
	if err != nil {
		return nil, &UnknownAshError{Role: "host", Selector: request.Authoring.Host, cause: err}
	}
	if err := refuseFixed(host.GoodsFamily, host.Name); err != nil {
		return nil, err
	}
	trigger := host.TriggerFamilies[0]

	goodsRows := make([]int32, 0, host.UpgradeLevels)
	for level := 0; level < host.UpgradeLevels; level++ {
		goodsRows = append(goodsRows, host.GoodsFamily+int32(level))
	}
	state, err := service.readState(ctx, request.InputPath, goodsRows)
	if err != nil {
		return nil, err
	}
	save := &SaveInfo{
		Path: request.InputPath, SHA256: state.SHA256,
		RegulationVersion: state.RegulationVersion,
		Supported:         RegulationSupported(state.RegulationVersion),
	}
	if !save.Supported {
		return nil, &UnsupportedRegulationError{Found: save.RegulationVersion}
	}
	// An Ash converted by an earlier edit no longer delivers its own formation,
	// so authoring into its block would change an item the user did not name
	// and leave the one they did name unchanged. A reset restores it.
	if current := state.Goods[host.GoodsFamily].Trigger; current != trigger {
		return nil, &HostRePointedError{Name: host.Name, Shipped: trigger, Found: current}
	}

	read, err := savecore.ReadFormations(ctx, request.InputPath, host.TriggerFamilies)
	if err != nil {
		return nil, fmt.Errorf("read the host formation: %w", err)
	}
	for _, block := range host.TriggerFamilies {
		if len(read.Blocks[block].Rows) == 0 {
			return nil, fmt.Errorf("%s delivers formation %d, which this save holds no rows for: %w",
				host.Name, block, ErrRowNotFound)
		}
	}

	// The plan itself is the session's, exactly as Author makes it (oneShot):
	// what a formation writes is placed at Save over the whole save - a creature row nothing names any more is pruned, and a freed band
	// is reused - so a preview compiled apart from that would promise a
	// different manifest from the one Author writes.
	session := service.NewSession()
	if err := session.Load(ctx, request.InputPath); err != nil {
		return nil, err
	}
	if _, err := session.StageFormation(request.Authoring.Host, request.Authoring.Slots); err != nil {
		return nil, err
	}
	commit, err := session.PrepareCommit()
	if err != nil {
		return nil, err
	}
	staged := commit.summary.Formations[0]
	preview := &AuthoringPreview{
		Save: save, Formation: staged.Formation,
		OutputPath: request.OutputPath, Manifest: commit.Manifest(),
	}
	for _, manifest := range staged.manifests {
		preview.BuddyRows += len(manifest.BuddyPatches)
		preview.RowsInserted += len(manifest.BuddyRowInsertions) +
			len(manifest.NpcCloneSources) + len(manifest.NpcThinkCloneSources)
	}
	return &authoringPreparation{preview: preview}, nil
}

// compileAuthored places one formation by itself against the save at path, as
// read: what the player asked for becomes the rows it writes, a manifest a
// block, with no prune and nothing freed. It is what staging shows while the
// player is still composing; what Save writes is placed over every staged
// formation together (placeFormations), which is where a creature row nothing
// uses goes. The formation described is its +0 block's.
func compileAuthored(ctx context.Context, path, sourceSHA256 string, units []authoredUnit,
	read *savecore.FormationState, families []catalog.Family) ([]*plan.Manifest, AuthoredFormation, error) {
	placed, err := placeFormations(ctx, path, sourceSHA256, read, units, families, false)
	if err != nil {
		return nil, AuthoredFormation{}, err
	}
	return placed.manifests, placed.described[0], nil
}

// authoredFormation turns the request's slots into the model's, refusing an
// actor this save has no row for.
//
// The check belongs here rather than in internal/formation, which is asked
// which rows a formation needs and answers that a missing clone must be
// inserted. A missing source is a different thing: there is nothing to copy,
// and the clone insertion would fail deep inside the save write with a row ID
// the user never typed.
func authoredFormation(slots []AuthoredSlot, read *savecore.FormationState, families []catalog.Family) (formation.Formation, error) {
	if err := oneStrengthPerCreature(slots); err != nil {
		return formation.Formation{}, err
	}
	authored := formation.Formation{Slots: make([]formation.Slot, 0, len(slots))}
	for index, slot := range slots {
		actor := slot.resolve()
		if !read.NpcRows[actor.Npc] && !isOwnBody(actor) {
			return formation.Formation{}, &UnknownActorError{Slot: index + 1, Table: "NpcParam", RowID: actor.Npc}
		}
		if !read.NpcThinkRows[actor.Think] {
			return formation.Formation{}, &UnknownActorError{Slot: index + 1, Table: "NpcThinkParam", RowID: actor.Think}
		}
		built, err := slot.slot(families)
		if err != nil {
			return formation.Formation{}, err
		}
		authored.Slots = append(authored.Slots, built)
	}
	return authored, nil
}

// oneStrengthPerCreature refuses a formation whose slots name one creature
// with different research-seat knobs (ErrOneCreatureTwoStrengths): the knobs
// are recorded per creature (plan.Strength), so the slots would share one
// answer silently rather than get what each asked for. The role is not one of
// them: it is the clone's.
func oneStrengthPerCreature(slots []AuthoredSlot) error {
	type knobs struct {
		bare      bool
		residents string
	}
	seen := map[int32]knobs{}
	for index, slot := range slots {
		residents := slices.Clone(slot.Residents)
		slices.Sort(residents)
		asked := knobs{bare: slot.BareRow, residents: fmt.Sprint(residents)}
		creature := slot.resolve().Npc % formation.CloneOffset
		if earlier, named := seen[creature]; named && earlier != asked {
			return fmt.Errorf("slot %d names creature %d with other strength knobs than an earlier slot: %w",
				index+1, creature, ErrOneCreatureTwoStrengths)
		}
		seen[creature] = asked
	}
	return nil
}

func blockOf(read savecore.FormationBlock) formation.Block {
	block := formation.Block{Trigger: read.Trigger, Rows: make([]formation.BlockRow, 0, len(read.Rows))}
	for _, row := range read.Rows {
		block.Rows = append(block.Rows, formation.BlockRow{RowID: row.RowID, Doping: row.Doping})
	}
	// The rows the block retired, which a larger formation re-fills.
	for _, row := range read.Inert {
		block.Inert = append(block.Inert, formation.BlockRow{RowID: row.RowID, Doping: row.Doping})
	}
	return block
}

// currentBlockValues is what the save holds for every row of the block, which
// is what a retired row is written back with. Only the block is needed: a
// formation can only retire a row it was going to use.
func currentBlockValues(read savecore.FormationBlock) map[int32]map[string]float64 {
	values := make(map[int32]map[string]float64, len(read.Rows))
	for _, row := range read.Rows {
		values[row.RowID] = row.Values(read.Trigger)
	}
	return values
}

func describe(host catalog.Ash, compiled formation.Compiled, requested []AuthoredSlot, shippedRows int) AuthoredFormation {
	described := AuthoredFormation{
		HostGoodsFamily:   host.GoodsFamily,
		HostName:          host.Name,
		TriggerSpEffectID: compiled.Trigger,
		ShippedRows:       shippedRows,
		Retired:           compiled.Retired,
	}
	for _, edit := range compiled.Edits {
		described.Slots = append(described.Slots, PlannedSlot{
			Slot: edit.Slot, RowID: edit.RowID, Requested: requested[edit.Slot-1],
			Npc: edit.Actor.Npc, Think: edit.Actor.Think, Place: edit.Place,
		})
	}
	for _, insert := range compiled.Inserts {
		described.Slots = append(described.Slots, PlannedSlot{
			Slot: insert.Slot, RowID: insert.RowID, Requested: requested[insert.Slot-1],
			Npc: insert.Actor.Npc, Think: insert.Actor.Think, Inserted: true,
			Place: insert.Place,
		})
	}
	for _, clone := range compiled.Clones {
		planned := PlannedClone{SourceNpc: clone.Source.Npc, SourceThink: clone.Source.Think}
		if clone.Npc != nil {
			planned.Npc, planned.InsertsNpc = clone.Npc.RowID, true
		}
		if clone.Think != nil {
			planned.Think, planned.InsertsThink = clone.Think.RowID, true
		}
		described.Clones = append(described.Clones, planned)
	}
	return described
}

// checkPaths is the path contract every operation shares: an input, an output
// that is a different file, and a manifest that is neither.
func checkPaths(request Request, needOutput bool) error {
	if strings.TrimSpace(request.InputPath) == "" {
		return ErrPathRequired
	}
	if needOutput && strings.TrimSpace(request.OutputPath) == "" {
		return ErrPathRequired
	}
	if request.OutputPath != "" && samePath(request.InputPath, request.OutputPath) {
		return fmt.Errorf("output path must differ from input path: %w", ErrPathCollision)
	}
	if request.ManifestPath != "" && (samePath(request.ManifestPath, request.InputPath) ||
		(request.OutputPath != "" && samePath(request.ManifestPath, request.OutputPath))) {
		return fmt.Errorf("manifest path must differ from the input and output save paths: %w", ErrPathCollision)
	}
	return nil
}
