package savecore

// Person writes: the one place this product writes a CharaInitParam row.
//
// docs/SAFETY.md's first rule says an edit changes the Ash it names and nothing
// else in the world. The NPC review armed
// eleven people the game ships unable to fight - Irina, Fia,
// Hyetta, Roderika, Seluvis, Tanith, Rya, two Finger Maidens, Boc's human row,
// Sage Gowry - and ruled the write IN PLACE, on the shipped row: these people
// never fight in the world and cannot be hit, so no world fight reads what
// changes. Like the shared effect writes, the application writes a field only
// while an Ash in the save holds a person that needs it, and puts the shipped
// value back once none does.
//
// What keeps that exception from widening is that the whole of it is below:
// these fields of these rows, each with exactly two values it may hold - the
// one the game ships and the one the entry needs. A manifest naming any other
// row, field or value is refused. The catalog says which person needs which of
// them (the hierarchy's chara_set column); the application's check holds the
// two lists equal when the service starts.

import (
	"fmt"
	"slices"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// charaInitLayout is CharaInitParam, which this product writes through this
// file only. Its stride and the offsets below are pinned against the real save
// by personwrites_real_test.go (research param-fields reads the same offsets).
var charaInitLayout = regulation.MustLayout("CharaInitParam")

// personFields are the fields a person write may name.
var personFields = map[string]paramField{
	"equip_Wep_Right":   {offset: 16, kind: "s32"},
	"equip_Wep_Left":    {offset: 24, kind: "s32"},
	"equip_Accessory01": {offset: 64, kind: "s32"},
	"equip_Accessory02": {offset: 68, kind: "s32"},
	"equip_Accessory03": {offset: 72, kind: "s32"},
	"equip_Accessory04": {offset: 76, kind: "s32"},
	"equip_Spell_01":    {offset: 96, kind: "s32"},
	"equip_Spell_02":    {offset: 100, kind: "s32"},
	"equip_Spell_03":    {offset: 104, kind: "s32"},
	"equip_Spell_04":    {offset: 108, kind: "s32"},
	"equip_Spell_05":    {offset: 112, kind: "s32"},
	"equip_Spell_06":    {offset: 116, kind: "s32"},
	"soulLv":            {offset: 192, kind: "s16"},
	"baseVit":           {offset: 194, kind: "u8"},
	"baseWil":           {offset: 195, kind: "u8"},
	"baseEnd":           {offset: 196, kind: "u8"},
	"baseStr":           {offset: 197, kind: "u8"},
	"baseDex":           {offset: 198, kind: "u8"},
	"baseMag":           {offset: 199, kind: "u8"},
	"baseFai":           {offset: 200, kind: "u8"},
	"baseLuc":           {offset: 201, kind: "u8"},
}

// PersonWrite is one field a person write may move, and its two values.
type PersonWrite struct {
	Row     int32
	Field   string
	Shipped float64
	Written float64
}

// personWrites is the whole surface, measured against regulation 11711000
// (research catalog_export.py reads each shipped value off the baseline).
var personWrites = []PersonWrite{
	// Tanith, Volcano Manor Proprietress (23000)
	{23000, "equip_Wep_Right", -1, 33050025},
	{23000, "equip_Wep_Left", -1, 34000025},
	{23000, "equip_Spell_01", -1, 53246},
	{23000, "equip_Spell_02", -1, 53720},
	{23000, "soulLv", 18, 60},
	{23000, "baseFai", 11, 14},
	// Preceptor Seluvis (23070)
	{23070, "equip_Wep_Right", -1, 33240010},
	{23070, "equip_Spell_01", -1, 53241},
	{23070, "equip_Spell_02", -1, 4360},
	{23070, "equip_Spell_03", -1, 53801},
	{23070, "equip_Accessory01", -1, 3000},
	{23070, "equip_Accessory02", -1, 3001},
	{23070, "equip_Accessory03", -1, 2000},
	{23070, "equip_Accessory04", -1, 3070},
	{23070, "soulLv", 18, 713},
	{23070, "baseVit", 22, 99},
	{23070, "baseWil", 9, 99},
	{23070, "baseEnd", 11, 99},
	{23070, "baseStr", 16, 99},
	{23070, "baseDex", 14, 99},
	{23070, "baseMag", 9, 99},
	{23070, "baseFai", 8, 99},
	{23070, "baseLuc", 8, 99},
	// Irina of Morne (23100)
	{23100, "equip_Spell_01", -1, 7050},
	{23100, "baseFai", 27, 36},
	// Finger Maiden Hyetta (23101)
	{23101, "equip_Spell_01", -1, 2006400},
	// Rya (23130)
	{23130, "equip_Wep_Right", -1, 33050025},
	{23130, "equip_Wep_Left", -1, 34000025},
	{23130, "equip_Spell_01", -1, 53246},
	{23130, "equip_Spell_02", -1, 53720},
	{23130, "soulLv", 18, 60},
	{23130, "baseMag", 9, 14},
	{23130, "baseFai", 8, 14},
	// Heartbroken Maiden (23200)
	{23200, "equip_Wep_Right", -1, 2550010},
	{23200, "equip_Wep_Left", -1, 34060010},
	{23200, "equip_Spell_01", -1, 2007600},
	{23200, "equip_Spell_02", -1, 2006700},
	{23200, "equip_Accessory01", -1, 3050},
	{23200, "equip_Accessory02", -1, 3040},
	{23200, "equip_Accessory03", -1, 2030},
	{23200, "equip_Accessory04", -1, 3070},
	{23200, "soulLv", 18, 713},
	{23200, "baseVit", 22, 99},
	{23200, "baseWil", 9, 99},
	{23200, "baseEnd", 11, 99},
	{23200, "baseStr", 6, 99},
	{23200, "baseDex", 4, 99},
	{23200, "baseMag", 16, 99},
	{23200, "baseFai", 18, 99},
	{23200, "baseLuc", 11, 99},
	// Fia, Deathbed Companion (23221)
	{23221, "equip_Spell_01", -1, 53291},
	// Vyke's Finger Maiden (23360)
	{23360, "equip_Wep_Right", -1, 34090010},
	{23360, "equip_Spell_01", -1, 53043},
	{23360, "equip_Spell_02", -1, 53044},
	{23360, "equip_Spell_03", -1, 53045},
	{23360, "equip_Spell_04", -1, 53180},
	{23360, "equip_Spell_05", -1, 53181},
	{23360, "equip_Spell_06", -1, 2007300},
	{23360, "equip_Accessory01", -1, 7060},
	{23360, "equip_Accessory02", -1, 3050},
	{23360, "equip_Accessory03", -1, 3040},
	{23360, "equip_Accessory04", -1, 2020},
	{23360, "soulLv", 18, 713},
	{23360, "baseVit", 22, 99},
	{23360, "baseWil", 9, 99},
	{23360, "baseEnd", 11, 99},
	{23360, "baseStr", 6, 99},
	{23360, "baseDex", 4, 99},
	{23360, "baseMag", 5, 99},
	{23360, "baseFai", 8, 99},
	{23360, "baseLuc", 32, 99},
	// Finger Maiden (23380)
	{23380, "equip_Wep_Right", -1, 34000025},
	{23380, "equip_Spell_01", -1, 6431},
	{23380, "equip_Spell_02", -1, 2050081},
	{23380, "equip_Spell_03", -1, 53260},
	{23380, "equip_Spell_04", -1, 6441},
	{23380, "baseFai", 8, 38},
	// Boc the Seamster (23420)
	{23420, "equip_Wep_Right", -1, 1000025},
	// Sage Gowry (23490)
	{23490, "equip_Wep_Right", -1, 34000025},
	{23490, "equip_Spell_01", -1, 53900},
	{23490, "equip_Spell_02", -1, 2007210},
	{23490, "equip_Accessory01", -1, 7010},
	{23490, "soulLv", 18, 100},
	{23490, "baseFai", 32, 40},
}

// PersonWrites is a copy of the surface, for the application's check that the
// catalog asks for exactly these.
func PersonWrites() []PersonWrite { return slices.Clone(personWrites) }

// PersonKey names one field of one CharaInitParam row.
type PersonKey struct {
	Row   int32
	Field string
}

func personWriteOf(row int32, field string) (PersonWrite, bool) {
	for _, write := range personWrites {
		if write.Row == row && write.Field == field {
			return write, true
		}
	}
	return PersonWrite{}, false
}

// validatePersonPatches holds a person write to its surface: only the shared
// write's operation (or a session part of it) may carry one, a row at most
// once, and every field one of the list's with one of its two values.
func validatePersonPatches(plan *manifest) error {
	if len(plan.PersonPatches) == 0 {
		return nil
	}
	if plan.Operation != operationSharedEffects && plan.Operation != operationSession {
		return fmt.Errorf("operation %q may not write CharaInitParam: %w", plan.Operation, ErrManifestInvalid)
	}
	seen := map[int32]bool{}
	for index, patch := range plan.PersonPatches {
		if seen[patch.RowID] {
			return fmt.Errorf("person patch %d: CharaInitParam row %d is written twice: %w", index+1, patch.RowID, ErrManifestInvalid)
		}
		seen[patch.RowID] = true
		if len(patch.Set) == 0 {
			return fmt.Errorf("person patch %d: row %d sets nothing: %w", index+1, patch.RowID, ErrManifestInvalid)
		}
		for field, value := range patch.Set {
			write, known := personWriteOf(patch.RowID, field)
			if !known {
				return fmt.Errorf("person patch %d: %d.%s is not a field this product may write: %w",
					index+1, patch.RowID, field, ErrManifestInvalid)
			}
			if value != write.Shipped && value != write.Written {
				return fmt.Errorf("person patch %d: %d.%s may hold %g or %g, not %g: %w",
					index+1, patch.RowID, field, write.Shipped, write.Written, value, ErrManifestInvalid)
			}
		}
	}
	return nil
}

// applyPersons writes the person fields. Every row and field was bounded by
// validatePersonPatches; the row is looked up in the save, never assumed.
func applyPersons(bnd4 []byte, patches []rowPatch) ([]byteRange, error) {
	if len(patches) == 0 {
		return nil, nil
	}
	table, err := regulation.OpenTable(bnd4, charaInitLayout)
	if err != nil {
		return nil, err
	}
	ranges := make([]byteRange, 0, len(patches))
	for _, patch := range patches {
		row, ok := table.ByID[patch.RowID]
		if !ok {
			return nil, fmt.Errorf("CharaInitParam row %d was not found: %w", patch.RowID, ErrRowNotFound)
		}
		writeParamRow(table.Data[row.DataOffset:], personFields, patch.Set)
		ranges = append(ranges, rowRange(table, row))
	}
	return ranges, nil
}

// readPersonWrites is what the save holds in every field of the surface. A row
// the regulation lacks is left out rather than refused, as readSharedEffects
// does; writing one is refused by applyPersons.
func readPersonWrites(bnd4 []byte) (map[PersonKey]float64, error) {
	table, err := regulation.OpenTable(bnd4, charaInitLayout)
	if err != nil {
		return nil, err
	}
	values := make(map[PersonKey]float64, len(personWrites))
	for _, write := range personWrites {
		row, ok := table.ByID[write.Row]
		if !ok {
			continue
		}
		values[PersonKey{write.Row, write.Field}] = readParamFieldValue(
			table.Data[row.DataOffset:], personFields[write.Field], "CharaInitParam", write.Field)
	}
	return values, nil
}
