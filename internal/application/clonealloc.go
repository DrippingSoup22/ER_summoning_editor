package application

// Placing creature recipes on clone rows - decided at Save, over the whole
// save.
//
// One creature may be summoned on many recipes - a Guard in one slot, an enemy
// Loner in the next, a Sentinel in another Ash - and each recipe is its own
// clone (formation.CloneBands). Nothing is written until Save, and at Save the
// file is written from the WHOLE configuration - every container, the ones the
// player edited and the ones he did not. So for each creature:
//
//   - one row per recipe, shared by every slot in every Ash that uses it;
//   - a row the save already holds that IS a recipe asked for is reused where
//     it stands - its number is kept, because the game does not care about
//     the number and renumbering would rewrite containers the player never
//     touched;
//   - a creature row this editor inserted that no formation row names any more
//     - the player took the creature out, or changed its recipe - is not
//     written: the prune part removes it (plan/prune.go);
//   - a new recipe takes the LOWEST free band, a band freed by the prune
//     included, so a creature reconfigured on a reopened save stays in band 1
//     rather than walking up the bands. A freed row and the new recipe on its
//     number is a REPLACED row, which the save core allows for the two actor
//     tables alone (savecore/session.go).
//
// FromSoftware's own rows are never pruned (the shipped snapshot names them),
// and neither is any row a formation row outside this save's staged changes
// still names - retired rows included, which is why a row a formation retires
// is written back naming the actor the game ships for it (retiredValues)
// rather than the creature it last summoned. A retired row that still names
// its creature (an older build's) keeps that creature's row until a reset.
//
// Placing it takes two compiles of every staged formation. The PROBE gives
// every recipe a provisional row in a band the save does not hold and asks the
// save (savecore.MatchClones) which of them it already holds, byte for byte and
// strength included, since the role is part of the recipe. Knowing which
// rows are reused, and which rows every other formation row names, is knowing
// which rows the save still uses; the rest of this editor's rows are pruned.
// The FINAL compile then reuses what matched and gives every other recipe the
// lowest band left, in a fixed order - formations by host goods family, slots
// in order - so one staged set always writes the same save, however the
// player got to it.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// cloneAllocator is the formation.Allocator of one compile pass over every
// formation being placed together, so one recipe gets one key however many
// formations name it.
type cloneAllocator struct {
	// held is every row the save will hold before this commit inserts
	// anything: what it holds now, less what the prune removes.
	held map[formation.Table]map[int32]bool
	// reuse is the row a recipe the save already holds is placed on.
	reuse  map[clonePlacement]int32
	placed map[clonePlacement]clonePlace
	taken  map[formation.Table]map[int32]bool
}

type clonePlacement struct {
	table  formation.Table
	source int32
	recipe string
}

type clonePlace struct {
	key    int32
	insert bool
}

func newCloneAllocator(held map[formation.Table]map[int32]bool, reuse map[clonePlacement]int32) *cloneAllocator {
	return &cloneAllocator{
		held:   held,
		reuse:  reuse,
		placed: map[clonePlacement]clonePlace{},
		taken:  map[formation.Table]map[int32]bool{formation.NpcTable: {}, formation.ThinkTable: {}},
	}
}

// Place is formation.Allocator: the recipe's key, and whether it is inserted.
func (allocator *cloneAllocator) Place(table formation.Table, source int32, recipe string) (int32, bool, error) {
	at := clonePlacement{table, source, recipe}
	if done, seen := allocator.placed[at]; seen {
		return done.key, done.insert, nil
	}
	if row, reused := allocator.reuse[at]; reused {
		done := clonePlace{key: row - formation.CloneOffset}
		allocator.placed[at] = done
		return done.key, false, nil
	}
	for _, band := range formation.CloneBands {
		key, err := formation.CloneKey(source, band)
		if err != nil {
			return 0, false, err
		}
		if allocator.held[table][key+formation.CloneOffset] || allocator.taken[table][key] {
			continue
		}
		allocator.taken[table][key] = true
		allocator.placed[at] = clonePlace{key: key, insert: true}
		return key, true, nil
	}
	return 0, false, fmt.Errorf("row %d: %w", source, formation.ErrNoCloneBand)
}

// authoredUnit is one formation to place: everything its compile reads.
type authoredUnit struct {
	host  catalog.Ash
	slots []AuthoredSlot
	block savecore.FormationBlock
	// levels are the upgrade levels that reach the block (formation.Block),
	// nil for all of them.
	levels   []int
	strength plan.Strength
}

// placedFormations is the answer: each unit's manifest and description, in
// the units' order, and the creature rows the prune removes.
type placedFormations struct {
	manifests  []*plan.Manifest
	described  []AuthoredFormation
	pruneNpc   []int32
	pruneThink []int32
}

// placeFormations places every unit's recipes together against the save at
// path, as read. With prune, the rows this editor inserted that nothing will
// name are pruned and their bands are free; without it (a staged reset, which
// removes this editor's rows by its own rule, or a formation being previewed
// alone) nothing is freed and nothing is pruned.
func placeFormations(ctx context.Context, path, sourceSHA256 string, read *savecore.FormationState,
	units []authoredUnit, families []catalog.Family, prune bool) (*placedFormations, error) {

	built := make([]formation.Formation, len(units))
	for index, unit := range units {
		authored, err := authoredFormation(unit.slots, read, families)
		if err != nil {
			return nil, err
		}
		built[index] = authored
	}

	// The probe: every recipe on a row the save does not hold.
	saveRows := map[formation.Table]map[int32]bool{formation.NpcTable: read.NpcRows, formation.ThinkTable: read.NpcThinkRows}
	probe := newCloneAllocator(saveRows, nil)
	probed, _, err := compileUnits(sourceSHA256, units, built, read, probe)
	if err != nil {
		return nil, err
	}
	reuse, err := heldRecipes(ctx, path, probed, probe)
	if err != nil {
		return nil, err
	}

	held := saveRows
	var pruneNpc, pruneThink []int32
	if prune {
		if pruneNpc, pruneThink, err = unusedClones(read, probed, reuse); err != nil {
			return nil, err
		}
		held = map[formation.Table]map[int32]bool{
			formation.NpcTable:   without(read.NpcRows, pruneNpc),
			formation.ThinkTable: without(read.NpcThinkRows, pruneThink),
		}
	}

	final := newCloneAllocator(held, reuse)
	manifests, described, err := compileUnits(sourceSHA256, units, built, read, final)
	if err != nil {
		return nil, err
	}
	if err := everyNamedCloneExists(manifests, held); err != nil {
		return nil, err
	}
	return &placedFormations{manifests: manifests, described: described, pruneNpc: pruneNpc, pruneThink: pruneThink}, nil
}

// compileUnits compiles every unit with one allocator, in order.
func compileUnits(sourceSHA256 string, units []authoredUnit, built []formation.Formation,
	read *savecore.FormationState, allocator formation.Allocator) ([]*plan.Manifest, []AuthoredFormation, error) {
	snapshot, err := reference.Vanilla()
	if err != nil {
		return nil, nil, fmt.Errorf("read the embedded shipped-value snapshot: %w", err)
	}
	save := formation.Save{BuddyRows: read.BuddyRows, NpcRows: read.NpcRows, NpcThinkRows: read.NpcThinkRows, Allocator: allocator}
	manifests := make([]*plan.Manifest, len(units))
	described := make([]AuthoredFormation, len(units))
	for index, unit := range units {
		block := blockOf(unit.block)
		block.Levels = unit.levels
		compiled, err := formation.Compile(built[index], block, save)
		if err != nil {
			return nil, nil, err
		}
		manifest, err := plan.BuildAuthor(sourceSHA256, *compiled,
			plan.Vanilla{Buddy: retiredValues(unit.block, snapshot), Ladders: ashLadders(unit.block, snapshot),
				CharacterLadder: savecore.CharacterLadder()},
			unit.strength)
		if err != nil {
			return nil, nil, err
		}
		manifests[index] = manifest
		described[index] = describe(unit.host, *compiled, unit.slots, len(unit.block.Rows))
	}
	return manifests, described, nil
}

// retiredValues is what a row the formation retires is written back with: what
// the save holds for it (currentBlockValues), except the actor - body, AI,
// person and arrival - which is the one the game ships for that row, or, for a
// row this editor inserted, the one it ships for the block's first row.
//
// A retired row summons nothing (plan.InertTrigger), but it still names an
// actor, and it leaves the block for good: a later formation reads the block
// by its trigger and never sees it again. Had it kept the creature it last
// summoned, that creature's row would be named for ever and never pruned - the
// clone-band review's first test found exactly that - and had it kept a
// character's person, the character could never be deleted. FromSoftware's
// rows are never pruned, so a retired row naming one can neither dangle nor
// pin anything, and a reset writes the same value back.
func retiredValues(block savecore.FormationBlock, snapshot *reference.VanillaSnapshot) map[int32]map[string]float64 {
	values := currentBlockValues(block)
	var fallback map[string]float64
	if len(block.Rows) > 0 {
		fallback, _ = snapshot.BuddyRow(block.Rows[0].RowID)
	}
	for rowID, current := range values {
		shipped, isShipped := snapshot.BuddyRow(rowID)
		if !isShipped {
			shipped = fallback
		}
		if shipped == nil {
			continue
		}
		for _, field := range []string{"npcParamId", "npcThinkParamId", "npcPlayerInitParamId", "generateAnimId"} {
			if value, present := shipped[field]; present {
				current[field] = value
			}
		}
	}
	return values
}

// ashLadders is the ladder each row of a block ships with: a shipped row's own,
// and for a row this editor inserted the one of the block's last shipped row,
// which is the row the first insert copied (formation.Compile) - the Mimic
// Tear's copy carries its own ladder, not the tear's.
func ashLadders(block savecore.FormationBlock, snapshot *reference.VanillaSnapshot) map[int32][formation.DopingLevels]int32 {
	rows := append(append([]savecore.FormationRow(nil), block.Rows...), block.Inert...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].RowID < rows[j].RowID })
	ladders := make(map[int32][formation.DopingLevels]int32, len(rows))
	var template [formation.DopingLevels]int32
	found := false
	for _, row := range rows {
		values, isShipped := snapshot.BuddyRow(row.RowID)
		if !isShipped {
			continue
		}
		var ladder [formation.DopingLevels]int32
		complete := true
		for level := range ladder {
			value, present := values[fmt.Sprintf("dopingSpEffect_lv%d", level)]
			complete = complete && present
			ladder[level] = int32(value)
		}
		if complete {
			ladders[row.RowID], template, found = ladder, ladder, true
		}
	}
	if !found {
		return ladders
	}
	for _, row := range rows {
		if _, isShipped := ladders[row.RowID]; !isShipped {
			ladders[row.RowID] = template
		}
	}
	return ladders
}

// heldRecipes asks the save which recipes the probe placed on a new row it
// already holds, and answers with the row each of them reuses.
//
// One question for all of them, so the save is read once however many Ashes
// are staged - Save is pressed on the frontend's own frame. One allocator
// placed every manifest, so a key means one recipe wherever it appears, and
// the manifests' clone lists merge into one without two entries for a key.
func heldRecipes(ctx context.Context, path string, probed []*plan.Manifest, probe *cloneAllocator) (map[clonePlacement]int32, error) {
	merged := mergedClones(probed)
	if merged == nil {
		return map[clonePlacement]int32{}, nil
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	matches, err := savecore.MatchClones(ctx, path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	reuse := map[clonePlacement]int32{}
	for at, done := range probe.placed {
		if !done.insert {
			continue
		}
		found := matches.Npc
		if at.table == formation.ThinkTable {
			found = matches.Think
		}
		if row, held := found[done.key]; held {
			reuse[at] = row
		}
	}
	return reuse, nil
}

// mergedClones is one manifest carrying every probed manifest's clone
// insertions and the lists that say how each is written, each key once; nil
// when none inserts anything. It is a question for MatchClones, never a plan
// to apply.
func mergedClones(probed []*plan.Manifest) *plan.Manifest {
	var merged *plan.Manifest
	seen := map[string]bool{}
	first := func(list string, key int32) bool {
		id := fmt.Sprintf("%s/%d", list, key)
		if seen[id] {
			return false
		}
		seen[id] = true
		return true
	}
	for _, manifest := range probed {
		if len(manifest.NpcCloneSources)+len(manifest.NpcThinkCloneSources) == 0 {
			continue
		}
		if merged == nil {
			merged = &plan.Manifest{SourceSHA256: manifest.SourceSHA256, Operation: manifest.Operation}
		}
		// The formation rows ride along because MatchClones reads the question
		// as the authoring manifest it is shaped like, and one of those writes
		// at least one row. Every staged formation writes its own block, so the
		// union of their rows names no row twice.
		merged.BuddyPatches = append(merged.BuddyPatches, manifest.BuddyPatches...)
		merged.BuddyRowInsertions = append(merged.BuddyRowInsertions, manifest.BuddyRowInsertions...)
		for _, key := range manifest.NpcCloneSources {
			if first("npc", key) {
				merged.NpcCloneSources = append(merged.NpcCloneSources, key)
			}
		}
		for _, entry := range manifest.NpcCloneStrength {
			if first("strength", entry.Source) {
				merged.NpcCloneStrength = append(merged.NpcCloneStrength, entry)
			}
		}
		for _, entry := range manifest.NpcCloneTeams {
			if first("team", entry.Source) {
				merged.NpcCloneTeams = append(merged.NpcCloneTeams, entry)
			}
		}
		for _, key := range manifest.NpcCloneWorldBranch {
			if first("world", key) {
				merged.NpcCloneWorldBranch = append(merged.NpcCloneWorldBranch, key)
			}
		}
		for _, key := range manifest.NpcCloneWorldBranchUnheld {
			if first("unheld", key) {
				merged.NpcCloneWorldBranchUnheld = append(merged.NpcCloneWorldBranchUnheld, key)
			}
		}
		for _, entry := range manifest.NpcCloneWeaponOut {
			if first("weapon", entry.Source) {
				merged.NpcCloneWeaponOut = append(merged.NpcCloneWeaponOut, entry)
			}
		}
		for _, key := range manifest.NpcThinkCloneSources {
			if first("think", key) {
				merged.NpcThinkCloneSources = append(merged.NpcThinkCloneSources, key)
			}
		}
		for _, entry := range manifest.NpcThinkPerception {
			if first("perception", entry.Source) {
				merged.NpcThinkPerception = append(merged.NpcThinkPerception, entry)
			}
		}
	}
	return merged
}

// unusedClones is every creature row this editor inserted that nothing will
// name once the staged formations are written: not a formation row this
// commit leaves alone, and not a recipe a staged formation reuses. A row
// FromSoftware ships is never one of them.
func unusedClones(read *savecore.FormationState, probed []*plan.Manifest, reuse map[clonePlacement]int32) (npc, think []int32, err error) {
	snapshot, err := reference.Vanilla()
	if err != nil {
		return nil, nil, fmt.Errorf("read the embedded shipped-value snapshot: %w", err)
	}
	// Every formation row a staged formation writes - filled, inserted or
	// retired - is written whole, so what it names after the commit is what
	// the final compile says: a reused row, a new one, or a row the save holds
	// that a slot names as it is - a slot kept as held, a shipped row. The
	// probe puts every new recipe on a row the save does not hold, so a row it
	// names that the save holds is one of the last kind.
	rewritten := map[int32]bool{}
	named := map[formation.Table]map[int32]bool{formation.NpcTable: {}, formation.ThinkTable: {}}
	tables := map[string]formation.Table{"npcParamId": formation.NpcTable, "npcThinkParamId": formation.ThinkTable}
	held := map[formation.Table]map[int32]bool{formation.NpcTable: read.NpcRows, formation.ThinkTable: read.NpcThinkRows}
	for _, manifest := range probed {
		for _, patch := range manifest.BuddyPatches {
			rewritten[patch.RowID] = true
			for field, table := range tables {
				if row, ok := asRowID(patch.Set[field]); ok && held[table][row] {
					named[table][row] = true
				}
			}
		}
	}
	for rowID, actors := range read.Actors {
		if rewritten[rowID] {
			continue
		}
		named[formation.NpcTable][actors.Npc] = true
		named[formation.ThinkTable][actors.Think] = true
	}
	for at, row := range reuse {
		named[at.table][row] = true
	}
	unused := func(rows map[int32]bool, shipped []int32, named map[int32]bool) []int32 {
		var out []int32
		for row := range rows {
			if formation.InCloneBand(row) && !slices.Contains(shipped, row) && !named[row] {
				out = append(out, row)
			}
		}
		slices.Sort(out)
		return out
	}
	return unused(read.NpcRows, snapshot.NpcCloneRows, named[formation.NpcTable]),
		unused(read.NpcThinkRows, snapshot.NpcThinkCloneRows, named[formation.ThinkTable]), nil
}

// everyNamedCloneExists is the placement's own proof, run before anything is
// written: every summon-side row a staged formation row names is a row the
// save will still hold, or one a staged formation inserts. A failure here is a
// defect in this file, never a player's choice.
func everyNamedCloneExists(manifests []*plan.Manifest, held map[formation.Table]map[int32]bool) error {
	inserted := map[formation.Table]map[int32]bool{formation.NpcTable: {}, formation.ThinkTable: {}}
	for _, manifest := range manifests {
		for _, key := range manifest.NpcCloneSources {
			inserted[formation.NpcTable][key+formation.CloneOffset] = true
		}
		for _, key := range manifest.NpcThinkCloneSources {
			inserted[formation.ThinkTable][key+formation.CloneOffset] = true
		}
		for _, shell := range manifest.NpcSummonCopies {
			if row, err := formation.SummonCopyRow(shell); err == nil {
				inserted[formation.NpcTable][row] = true
			}
		}
	}
	for _, manifest := range manifests {
		for _, patch := range manifest.BuddyPatches {
			for field, table := range map[string]formation.Table{"npcParamId": formation.NpcTable, "npcThinkParamId": formation.ThinkTable} {
				value, named := patch.Set[field]
				if !named {
					continue
				}
				row, ok := asRowID(value)
				// A character's body is the character write's, which builds
				// it in the same commit (characters.go).
				if !ok || row < formation.CloneOffset || formation.IsCharacterBody(row) {
					continue
				}
				if !held[table][row] && !inserted[table][row] {
					return fmt.Errorf("formation row %d names %s row %d, which this save would not hold: %w",
						patch.RowID, field, row, ErrStagedChangesCannotCombine)
				}
			}
		}
	}
	return nil
}

func asRowID(value any) (int32, bool) {
	switch number := value.(type) {
	case int64:
		return int32(number), true
	case float64:
		return int32(number), true
	case int32:
		return number, true
	case int:
		return int32(number), true
	}
	return 0, false
}

func without(rows map[int32]bool, gone []int32) map[int32]bool {
	kept := make(map[int32]bool, len(rows))
	for row := range rows {
		kept[row] = true
	}
	for _, row := range gone {
		delete(kept, row)
	}
	return kept
}
