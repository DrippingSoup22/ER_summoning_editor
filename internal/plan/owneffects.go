package plan

// Effect rows of our own: the presence and marker copies a clone may carry in
// place of the shipped ones, inserted while a clone in the written save names
// one and removed once none does. Which rows exist and what each copies is the
// save core's (savecore/owneffects.go), which refuses any other; the
// application decides at Save which to insert and which to remove, and this
// only spells the decision as a manifest.

import (
	"fmt"
	"slices"
)

// OperationOwnEffects is the operation value an own-effect write carries.
const OperationOwnEffects = "own-effects"

// BuildOwnEffects is the manifest that inserts and removes the given rows, in
// id order, so one decision always writes the same bytes.
func BuildOwnEffects(sourceSHA256 string, insert, remove []int32) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(insert)+len(remove) == 0 {
		return nil, fmt.Errorf("an own-effect write needs at least one row")
	}
	return &Manifest{
		SourceSHA256: sourceSHA256, Operation: OperationOwnEffects,
		OwnEffectInsertions: slices.Sorted(slices.Values(insert)),
		OwnEffectRemovals:   slices.Sorted(slices.Values(remove)),
	}, nil
}
