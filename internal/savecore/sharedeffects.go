package savecore

// Shared effect writes: the one place this product writes an effect row the
// world's own fights use.
//
// docs/SAFETY.md's first rule says an edit changes the Ash it names and nothing
// else in the world. Two entries break it and ship anyway: the combined
// Ancestor, whose three attack sets evict each other at the priority the game
// gives them, and Rykard below half health, whose great combo is his arena's
// to hand out. Each needs fields of SHIPPED SpEffectParam rows moved, and
// the Regal Ancestor's and Rykard's own fights read the same rows. So the
// catalog warns, the application writes these fields only while an Ash in
// the save holds such an entry, and it puts the shipped value back once none
// does.
//
// What keeps that exception from widening is that the whole of it is below:
// eight fields of six rows, each with exactly two values it may hold - the one
// the game ships and the one the entry needs. A manifest naming any other row,
// field or value is refused, so "this can reach nothing else" is true by
// construction, as it is for the colour switch's one row. The catalog says
// which entry needs which of them; the
// application's test holds the two lists equal.

import (
	"fmt"
	"slices"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// operationSharedEffects writes shared effect fields and nothing else.
//
// A ninth rule set, because its surface is a seventh table and because it is
// the only operation whose write is decided by what the WHOLE save summons
// rather than by what one staged change asks: the application derives it at
// Save, like the prune.
const operationSharedEffects = "shared-effects"

// effectLayout is SpEffectParam, which this product reads row IDs from and,
// through this file only, writes.
var effectLayout = regulation.MustLayout("SpEffectParam")

// sharedEffectFields are the three fields a shared write may name.
var sharedEffectFields = map[string]paramField{
	"effectEndurance":   {offset: 8, kind: "f32"},
	"replaceSpEffectId": {offset: 292, kind: "s32"},
	"categoryPriority":  {offset: 320, kind: "u8"},
}

// SharedEffectWrite is one field a shared write may move, and its two values.
type SharedEffectWrite struct {
	Row     int32
	Field   string
	Shipped float64
	Written float64
}

// sharedEffectWrites is the whole surface, measured against regulations
// 11701000 and 11711000, which agree on every value here.
var sharedEffectWrites = []SharedEffectWrite{
	// The combined Ancestor: 13615 and 13616 on priorities of their own, so
	// they stop evicting each other and 13614 (all three at 41).
	{13615, "categoryPriority", 41, 42},
	{13616, "categoryPriority", 41, 43},
	// Rykard's great combo, route A: the marker his marked moves add turns
	// into the combo's trigger, the combo's marker into the finisher's, and
	// the permanent triggers become finite, since only the arena removed them.
	{11940, "effectEndurance", 0, 0.1},
	{11940, "replaceSpEffectId", -1, 11941},
	{11941, "effectEndurance", -1, 12},
	{11942, "effectEndurance", 10, 12},
	{11942, "replaceSpEffectId", -1, 11945},
	{11945, "effectEndurance", -1, 12},
}

// SharedEffectWrites is a copy of the surface, for the application's check
// that the catalog asks for exactly these.
func SharedEffectWrites() []SharedEffectWrite { return slices.Clone(sharedEffectWrites) }

// SharedEffectKey names one field of one row.
type SharedEffectKey struct {
	Row   int32
	Field string
}

func sharedWriteOf(row int32, field string) (SharedEffectWrite, bool) {
	for _, write := range sharedEffectWrites {
		if write.Row == row && write.Field == field {
			return write, true
		}
	}
	return SharedEffectWrite{}, false
}

// validateEffectPatches holds a shared write to its surface: only its own
// operation (or a session part of it) may carry one, a row at most once, and
// every field one of the list's with one of its two values.
func validateEffectPatches(plan *manifest) error {
	if len(plan.EffectPatches) == 0 {
		return nil
	}
	if plan.Operation != operationSharedEffects && plan.Operation != operationSession {
		return fmt.Errorf("operation %q may not write SpEffectParam: %w", plan.Operation, ErrManifestInvalid)
	}
	seen := map[int32]bool{}
	for index, patch := range plan.EffectPatches {
		if seen[patch.RowID] {
			return fmt.Errorf("shared effect patch %d: SpEffectParam row %d is written twice: %w", index+1, patch.RowID, ErrManifestInvalid)
		}
		seen[patch.RowID] = true
		if len(patch.Set) == 0 {
			return fmt.Errorf("shared effect patch %d: row %d sets nothing: %w", index+1, patch.RowID, ErrManifestInvalid)
		}
		for field, value := range patch.Set {
			write, known := sharedWriteOf(patch.RowID, field)
			if !known {
				return fmt.Errorf("shared effect patch %d: %d.%s is not a field this product may write: %w",
					index+1, patch.RowID, field, ErrManifestInvalid)
			}
			if value != write.Shipped && value != write.Written {
				return fmt.Errorf("shared effect patch %d: %d.%s may hold %g or %g, not %g: %w",
					index+1, patch.RowID, field, write.Shipped, write.Written, value, ErrManifestInvalid)
			}
		}
	}
	return nil
}

// applyEffects writes the shared fields. Every row and field was bounded by
// validateEffectPatches; the row is looked up in the save, never assumed.
func applyEffects(bnd4 []byte, patches []rowPatch) ([]byteRange, error) {
	if len(patches) == 0 {
		return nil, nil
	}
	table, err := regulation.OpenTable(bnd4, effectLayout)
	if err != nil {
		return nil, err
	}
	ranges := make([]byteRange, 0, len(patches))
	for _, patch := range patches {
		row, ok := table.ByID[patch.RowID]
		if !ok {
			return nil, fmt.Errorf("SpEffectParam row %d was not found: %w", patch.RowID, ErrRowNotFound)
		}
		writeParamRow(table.Data[row.DataOffset:], sharedEffectFields, patch.Set)
		ranges = append(ranges, rowRange(table, row))
	}
	return ranges, nil
}

// readSharedEffects is what the save holds in every field of the surface. A
// row the regulation lacks is left out rather than refused: loading a save
// must not depend on rows only two entries need, and writing one is refused
// by applyEffects.
func readSharedEffects(bnd4 []byte) (map[SharedEffectKey]float64, error) {
	table, err := regulation.OpenTable(bnd4, effectLayout)
	if err != nil {
		return nil, err
	}
	values := make(map[SharedEffectKey]float64, len(sharedEffectWrites))
	for _, write := range sharedEffectWrites {
		row, ok := table.ByID[write.Row]
		if !ok {
			continue
		}
		values[SharedEffectKey{write.Row, write.Field}] = readParamFieldValue(
			table.Data[row.DataOffset:], sharedEffectFields[write.Field], "SpEffectParam", write.Field)
	}
	return values, nil
}
