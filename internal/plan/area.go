package plan

// The monument switch, on its own.
//
// The expanded-area recipe - summoning away from a Rebirth Monument - has a
// manifest of its own, because authoring writes the host's own block and has
// nothing to say to the monument table. It writes two tables and no others:
// every BuddyStoneParam row, and the twenty BuddyParam rows that ship
// appearOnAroundSekihi = 1. Both halves move together in both directions - see
// plan.Area for why off has to undo exactly what on did.

import "fmt"

// OperationArea is the manifest's name for a monument-switch change.
const OperationArea = "area"

// BuildArea emits the switch and nothing else.
//
// It carries no goods row, no actor row and no formation. That is the whole
// point of it being its own operation rather than a relaxed conversion: a
// validator that had to express both would stop rejecting either one's
// mistakes, which is the argument the operation constants in savecore already
// make four times over.
func BuildArea(sourceSHA256 string, recipe Recipe) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(recipe.MonumentOriginRows) == 0 {
		return nil, fmt.Errorf("the monument switch needs the rows shipping appearOnAroundSekihi = 1")
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationArea}
	if err := writeArea(manifest, recipe); err != nil {
		return nil, err
	}
	return manifest, nil
}
