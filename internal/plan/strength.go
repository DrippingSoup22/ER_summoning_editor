package plan

// The manifest a role chosen for FromSoftware's own summon rows produces.
//
// It is a fifth operation because its write surface is a fifth. It writes the
// values of summon rows that already exist, and it adds and removes nothing -
// so a validator permissive enough to describe it alongside authoring would
// stop rejecting either one's mistakes. That is the same argument the other
// four rest on.
//
// The values are decided a layer up, in internal/application, which is where
// the catalog lives. This file assembles them into a manifest and checks the
// shape.

import "fmt"

// OperationStrength is the operation value this manifest carries.
const OperationStrength = "strength"

// RowValues is one NpcParam row's field values, keyed by the names the
// shipped-value snapshot uses.
type RowValues struct {
	RowID int32
	Set   map[string]int64
}

// BuildStrength assembles one strength change: at least one row, or the
// change decided nothing and a manifest that writes nothing should not exist.
func BuildStrength(sourceSHA256 string, rows []RowValues, why string) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("a strength change needs at least one row to write")
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationStrength}
	for _, row := range rows {
		if len(row.Set) == 0 {
			return nil, fmt.Errorf("summon row %d sets nothing", row.RowID)
		}
		set := make(map[string]any, len(row.Set))
		for name, value := range row.Set {
			set[name] = value
		}
		manifest.NpcPatches = append(manifest.NpcPatches, RowPatch{
			RowID: row.RowID, Name: why + ": summon row", Set: set,
		})
	}
	return manifest, nil
}
