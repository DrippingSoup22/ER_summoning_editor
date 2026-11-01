package savecore

// Effect rows of our own: SpEffectParam rows nothing in the world names.
//
// A summon's look and the large Shadow Commoner's marker are effects a clone
// carries in place of one the recipe writes - 295000, the Spirit Summon
// presence, and 297000, the companion marker - a character's body carries free
// casting, a formation row summoning a character climbs the characters'
// ladder, and two creatures carry fixes composed for them (the catalog's
// granted effects, source `own`); each is a COPY of a shipped row at a new id,
// with fields written over it or none. docs/SAFETY.md says such a row needs
// no exception, because nothing in the world can reach it; what keeps that true
// is that the whole surface is below - six rows and nine ladder rungs, each a
// copy of one shipped row with the fields it states - and a manifest naming any
// other row is refused.
//
// The application inserts a row while the written save names it and removes
// it once nothing does, decided at Save over the whole save
// (operationOwnEffects). The structural stage inserts them before any clone
// and proves afterwards that every actor and formation row naming one finds it
// (verifyOwnEffectsNamed), so a clone can never carry a look that is not there.

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// operationOwnEffects inserts and removes effect rows of our own and does
// nothing else. Like the shared write it is decided by what the WHOLE save
// will summon, so the application derives it at Save.
const operationOwnEffects = "own-effects"

// The rows of our own.
const (
	// OwnColoursPresence is 295000 with no phantom overwrite (vfxId -1): the
	// summon drawn in its own colours, every part of its body whole. Seen on
	// the console.
	OwnColoursPresence = int32(295001)
	// QuietMarker is 297000 at an id no battle script tests. The large Shadow
	// Commoner's script disarms it under 297000; under this copy it fights with
	// its own moves and does not follow (seen on the console).
	QuietMarker = int32(297001)
	// FreeCasting is 511060, the Cerulean Hidden Tear, made a built
	// character's for good: every spell and skill costs no FP (a summon needs no FP), lasting for ever, with no glow, no
	// icon and not re-firing its cycled row. Items are used up as before, so
	// the one flask stays one drink. Not yet seen on the console.
	FreeCasting = int32(511063)

	ceruleanHiddenTear = int32(511060)
	// torchMagic is 14707, the magic torch-pole Commoner's magic torch (state
	// 413), at an id its battle script never reads: under 14707 a summon plays
	// the ritual that ends its attacks for good (seen on the console).
	torchMagic, shippedTorchMagic = int32(14709), int32(14707)
	// consortPhaseHold and consortMeteorHold are 5011, a bare permanent marker,
	// re-applying the Promised Consort's phase-2 and meteor markers 20011598
	// and 20011573 every 0.01 s, the shortest interval the game ships, so his
	// first phase holds to the end (seen on the console).
	consortPhaseHold, consortMeteorHold = int32(20011605), int32(20011606)
	// neutralLadderRung is the puppet ladder's +0 row, 290050: an effect
	// that changes nothing, which every rung of the characters' ladder
	// copies.
	neutralLadderRung = int32(290050)
	// firstShareRung is the characters' ladder's 10% rung; the 20% to 90%
	// ones follow it, beside the game's own ladders (2900xx creatures,
	// 29005x and 29015x people, 2902xx the Mimic Tear).
	firstShareRung = int32(290251)
)

// CharacterLadder is the ladder a row summoning one of the player's
// characters carries in place of its Ash's: the
// character as played at +10, a tenth less each level below, and +0 at the
// +1's tenth. A share of the health and of the damage dealt, the stance damage
// with it; defence and resistances stay the character's. +10 is the neutral
// rung itself.
func CharacterLadder() [11]int32 {
	var ladder [11]int32
	for level := range ladder {
		ladder[level] = firstShareRung + int32(max(level, 1)) - 1
	}
	ladder[10] = neutralLadderRung
	return ladder
}

// shareRung is the rung of the characters' ladder giving a share, in tenths.
func shareRung(tenths int) OwnEffect {
	share := float64(tenths) / 10
	set := map[string]float64{"maxHpRate": share, "staminaAttackRate": share}
	for _, element := range []string{"physics", "magic", "fire", "thunder", "dark"} {
		set[element+"AttackPowerRate"] = share
	}
	return OwnEffect{Row: firstShareRung + int32(tenths) - 1, Copy: neutralLadderRung, Set: set}
}

// OwnEffect is one effect row of our own: its id, the shipped row it copies
// and the fields written over the copy.
type OwnEffect struct {
	Row  int32
	Copy int32
	Set  map[string]float64
}

// ownEffects is the whole surface: the six rows, and the characters' ladder's
// nine share rungs.
var ownEffects = append([]OwnEffect{
	{Row: OwnColoursPresence, Copy: spiritPresenceEffect, Set: map[string]float64{"vfxId": -1}},
	{Row: QuietMarker, Copy: spiritFollowEffect},
	{Row: FreeCasting, Copy: ceruleanHiddenTear, Set: map[string]float64{
		"effectEndurance": -1, "vfxId": -1, "iconId": -1, "cycleOccurrenceSpEffectId": -1, "goodsConsumptionRate": 1,
	}},
	{Row: torchMagic, Copy: shippedTorchMagic},
	{Row: consortPhaseHold, Copy: worldStayEffect, Set: map[string]float64{"cycleOccurrenceSpEffectId": 20011598, "motionInterval": 0.01}},
	{Row: consortMeteorHold, Copy: worldStayEffect, Set: map[string]float64{"cycleOccurrenceSpEffectId": 20011573, "motionInterval": 0.01}},
}, shareRungs()...)

func shareRungs() []OwnEffect {
	rungs := make([]OwnEffect, 0, 9)
	for tenths := 1; tenths <= 9; tenths++ {
		rungs = append(rungs, shareRung(tenths))
	}
	return rungs
}

// ownEffectFields are the fields an own row may have written over its copy, at
// Paramdex's SpEffectParam offsets.
var ownEffectFields = map[string]paramField{
	"iconId":                    {offset: 0, kind: "s32"},
	"effectEndurance":           {offset: 8, kind: "f32"},
	"motionInterval":            {offset: 12, kind: "f32"},
	"maxHpRate":                 {offset: 16, kind: "f32"},
	"physicsAttackPowerRate":    {offset: 72, kind: "f32"},
	"magicAttackPowerRate":      {offset: 76, kind: "f32"},
	"fireAttackPowerRate":       {offset: 80, kind: "f32"},
	"thunderAttackPowerRate":    {offset: 84, kind: "f32"},
	"staminaAttackRate":         {offset: 200, kind: "f32"},
	"cycleOccurrenceSpEffectId": {offset: 296, kind: "s32"},
	"vfxId":                     {offset: 368, kind: "s32"},
	"darkAttackPowerRate":       {offset: 480, kind: "f32"},
	"goodsConsumptionRate":      {offset: 900, kind: "f32"},
}

// OwnEffects is a copy of the surface, for the application.
func OwnEffects() []OwnEffect {
	out := make([]OwnEffect, len(ownEffects))
	for index, own := range ownEffects {
		out[index] = OwnEffect{Row: own.Row, Copy: own.Copy, Set: maps.Clone(own.Set)}
	}
	return out
}

func ownEffectOf(row int32) (OwnEffect, bool) {
	for _, own := range ownEffects {
		if own.Row == row {
			return own, true
		}
	}
	return OwnEffect{}, false
}

// validateOwnEffects holds the own-effect lists to their operation and their
// surface, before the save is opened.
func validateOwnEffects(plan *manifest) error {
	named := len(plan.OwnEffectInsertions) + len(plan.OwnEffectRemovals)
	if plan.Operation != operationOwnEffects {
		if named != 0 {
			return fmt.Errorf("operation %q may not add or remove an effect row of our own: %w", plan.Operation, ErrManifestInvalid)
		}
		return nil
	}
	if named == 0 {
		return fmt.Errorf("an own-effect write needs the rows it adds or removes")
	}
	if len(plan.GoodsPatches)+len(plan.BuddyPatches)+len(plan.NpcPatches)+len(plan.VfxPatches)+
		len(plan.EffectPatches)+len(plan.PersonPatches)+len(plan.BuddyStonePatches)+len(plan.BuddyStoneRowPatches) != 0 ||
		plan.hasActorOrFormationRowWork() || plan.hasCharacterStructure() {
		return fmt.Errorf("an own-effect write adds and removes effect rows of our own and nothing else")
	}
	seen := map[int32]bool{}
	for _, row := range append(slices.Clone(plan.OwnEffectInsertions), plan.OwnEffectRemovals...) {
		if _, known := ownEffectOf(row); !known {
			return fmt.Errorf("SpEffectParam %d is not an effect row of our own: %w", row, ErrManifestInvalid)
		}
		if seen[row] {
			return fmt.Errorf("SpEffectParam %d is named twice: %w", row, ErrManifestInvalid)
		}
		seen[row] = true
	}
	return nil
}

// hasOwnEffectWork reports whether the manifest adds or removes an own row.
func (plan *manifest) hasOwnEffectWork() bool {
	return len(plan.OwnEffectInsertions)+len(plan.OwnEffectRemovals) > 0
}

// applyOwnEffects removes and inserts the own rows. An insertion copies its
// shipped row, anchored after the largest id below it, and the fields are
// written over the copy here, where the copy is made - proved to stay inside
// the rows inserted, as a clone's recipe is.
func applyOwnEffects(bnd4 []byte, plan *manifest) ([]byte, error) {
	current := bnd4
	var err error
	if len(plan.OwnEffectRemovals) > 0 {
		removals := slices.Sorted(slices.Values(plan.OwnEffectRemovals))
		if current, err = regulation.RemoveRows(current, effectLayout, removals); err != nil {
			return nil, err
		}
	}
	if len(plan.OwnEffectInsertions) == 0 {
		return current, nil
	}
	table, err := regulation.OpenTable(current, effectLayout)
	if err != nil {
		return nil, err
	}
	present := make([]int32, len(table.Rows))
	for index, row := range table.Rows {
		present[index] = row.ID
	}
	slices.Sort(present)
	rows := slices.Sorted(slices.Values(plan.OwnEffectInsertions))
	insertions := make([]regulation.RowInsertion, 0, len(rows))
	var previous int32
	for _, rowID := range rows {
		own, _ := ownEffectOf(rowID) // validateOwnEffects knows every one
		if _, held := table.ByID[rowID]; held {
			return nil, fmt.Errorf("SpEffectParam %d is already in the save", rowID)
		}
		if _, held := table.ByID[own.Copy]; !held {
			return nil, fmt.Errorf("SpEffectParam %d, which %d copies, was not found: %w", own.Copy, rowID, ErrRowNotFound)
		}
		anchor := present[sort.Search(len(present), func(index int) bool { return present[index] >= rowID })-1]
		if anchor < previous {
			anchor = previous
		}
		insertions = append(insertions, regulation.RowInsertion{RowID: rowID, CopyFrom: own.Copy, InsertAfter: &anchor})
		previous = rowID
	}
	grown, err := regulation.InsertRows(current, effectLayout, insertions)
	if err != nil {
		return nil, err
	}
	copied := bytes.Clone(grown)
	grownTable, err := regulation.OpenTable(grown, effectLayout)
	if err != nil {
		return nil, err
	}
	ranges := make([]byteRange, 0, len(rows))
	for _, rowID := range rows {
		own, _ := ownEffectOf(rowID)
		row := grownTable.ByID[rowID]
		writeParamRow(grownTable.Data[row.DataOffset:], ownEffectFields, own.Set)
		ranges = append(ranges, rowRange(grownTable, row))
	}
	if err := verifyBoundedMutation(copied, grown, ranges); err != nil {
		return nil, fmt.Errorf("SpEffectParam: writing the rows of our own: %w", err)
	}
	return grown, nil
}

// verifyOwnEffectsNamed proves the stage's composition from the other side:
// every actor row that names an own row as a resident, and every formation row
// that names one as a ladder rung, finds it in the save. A clone naming one
// this commit removes, or one nothing inserted, would carry a look or a marker
// that is not there - the presence missing is no spirit coat at all, the
// marker missing a summon that neither follows nor fights - and a formation
// row a rung that changes nothing.
func verifyOwnEffectsNamed(bnd4 []byte) error {
	effects, err := regulation.OpenTable(bnd4, effectLayout)
	if err != nil {
		return err
	}
	missing := func(effect int32) bool {
		_, own := ownEffectOf(effect)
		_, present := effects.ByID[effect]
		return own && !present
	}
	npc, err := regulation.OpenTable(bnd4, npcLayout)
	if err != nil {
		return err
	}
	for _, row := range npc.Rows {
		for _, effect := range residentEffectsOf(npc.Data[row.DataOffset:]) {
			if missing(effect) {
				return fmt.Errorf("NpcParam row %d names SpEffectParam %d, which the save would not hold", row.ID, effect)
			}
		}
	}
	buddy, err := regulation.OpenTable(bnd4, buddyLayout)
	if err != nil {
		return err
	}
	for _, row := range buddy.Rows {
		for level := range dopingLevels {
			if rung := readS32(buddy.Data[row.DataOffset:], buddyDopingOffset+4*level); missing(rung) {
				return fmt.Errorf("BuddyParam row %d names SpEffectParam %d at +%d, which the save would not hold", row.ID, rung, level)
			}
		}
	}
	return nil
}

// readOwnEffects is which own rows the save holds, and which actor rows name
// each of them.
func readOwnEffects(bnd4 []byte) (map[int32]bool, map[int32][]int32, error) {
	effects, err := regulation.OpenTable(bnd4, effectLayout)
	if err != nil {
		return nil, nil, err
	}
	held := map[int32]bool{}
	for _, own := range ownEffects {
		if _, present := effects.ByID[own.Row]; present {
			held[own.Row] = true
		}
	}
	npc, err := regulation.OpenTable(bnd4, npcLayout)
	if err != nil {
		return nil, nil, err
	}
	names := map[int32][]int32{}
	for _, row := range npc.Rows {
		for _, effect := range residentEffectsOf(npc.Data[row.DataOffset:]) {
			if _, own := ownEffectOf(effect); own && !slices.Contains(names[row.ID], effect) {
				names[row.ID] = append(names[row.ID], effect)
			}
		}
	}
	return held, names, nil
}
