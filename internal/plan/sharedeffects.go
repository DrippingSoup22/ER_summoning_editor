package plan

// Shared effect writes: fields of SHIPPED effect rows an entry needs, written
// while an Ash in the save holds that entry and put back once none does.
//
// An entry whose moveset only a write to
// a shipped effect row can give - the combined Ancestor, Rykard below half
// health - ships with a strong warning, because the world's own fight reads
// the same rows. Which entries need which fields is research's
// (shared-effect-writes.csv, through the catalog); which fields may move at
// all, and between which two values, is the save core's, which refuses the
// rest. The application decides at Save which way each field goes; this only
// spells the decision as a manifest.

import (
	"fmt"
	"sort"
)

// OperationSharedEffects is the operation value a shared effect write carries.
const OperationSharedEffects = "shared-effects"

// SharedEffectValue is one field of one shipped SpEffectParam row and the
// value it is to hold.
type SharedEffectValue struct {
	Row   int32
	Field string
	Value float64
	// Why is the manifest's name for the row: the entry that needs it, or
	// that no Ash holds one any more.
	Why string
}

// PersonValue is one field of one shipped CharaInitParam row and the value it
// is to hold: a person write (an armed person's loadout, written into its own
// row while an Ash holds it).
type PersonValue struct {
	Row   int32
	Field string
	Value float64
	// Why is the manifest's name for the row, as for a shared effect.
	Why string
}

// BuildSharedEffects is the manifest that writes the given shared effect and
// person fields, one patch per row in row order, so one decision always
// writes the same bytes. The two ride one operation because both are decided
// at Save over what the whole save summons.
func BuildSharedEffects(sourceSHA256 string, values []SharedEffectValue, persons []PersonValue) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(values)+len(persons) == 0 {
		return nil, fmt.Errorf("a shared effect write needs at least one field")
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationSharedEffects}
	effects := map[int32]*RowPatch{}
	for _, value := range values {
		if err := decide(effects, value.Row, value.Field, value.Value, value.Why, "shared effect"); err != nil {
			return nil, err
		}
	}
	people := map[int32]*RowPatch{}
	for _, value := range persons {
		if err := decide(people, value.Row, value.Field, value.Value, value.Why, "person"); err != nil {
			return nil, err
		}
	}
	manifest.EffectPatches = inRowOrder(effects)
	manifest.PersonPatches = inRowOrder(people)
	return manifest, nil
}

func decide(byRow map[int32]*RowPatch, row int32, field string, value float64, why, what string) error {
	patch := byRow[row]
	if patch == nil {
		patch = &RowPatch{RowID: row, Name: why, Set: map[string]any{}}
		byRow[row] = patch
	}
	if _, twice := patch.Set[field]; twice {
		return fmt.Errorf("%s %d.%s is decided twice", what, row, field)
	}
	patch.Set[field] = value
	return nil
}

func inRowOrder(byRow map[int32]*RowPatch) []RowPatch {
	rows := make([]int32, 0, len(byRow))
	for row := range byRow {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a] < rows[b] })
	out := make([]RowPatch, 0, len(rows))
	for _, row := range rows {
		out = append(out, *byRow[row])
	}
	return out
}
