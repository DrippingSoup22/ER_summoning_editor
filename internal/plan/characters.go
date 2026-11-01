package plan

// The player's own characters, summoned as people: the rows a character from a
// save slot is made of, written whole in the character band of three tables
// (savecore characters.go). The application decides at Save which rows the
// written save needs - a row for every character a formation will name, built
// from the slot as it is now - and this only spells the decision.

import (
	"encoding/hex"
	"fmt"
	"sort"
)

// OperationCharacters is the operation value a character write carries.
const OperationCharacters = "characters"

// CharacterRow is one character row, whole. Insert says the save does not
// hold it yet.
type CharacterRow struct {
	Table  string `json:"table"`
	RowID  int32  `json:"row_id"`
	Insert bool   `json:"insert,omitempty"`
	Data   string `json:"data"`
}

// CharacterBody is a character's body: a copy of a person shell carrying the
// character's health and active effects. The save core copies the shell as
// the commit leaves it, so the manifest names it rather than carrying bytes.
type CharacterBody struct {
	RowID   int32   `json:"row_id"`
	Shell   int32   `json:"shell"`
	Insert  bool    `json:"insert,omitempty"`
	HP      int32   `json:"hp"`
	Effects []int32 `json:"effects,omitempty"`
	// OwnColours draws the body in the character's own colours instead of
	// its Puppet shell's coat, and Fighter writes it without the balance
	// package.
	OwnColours bool `json:"own_colours,omitempty"`
	Fighter    bool `json:"fighter,omitempty"`
}

// CharacterRowKey names one character row, for a removal; a body's table is
// NpcParam.
type CharacterRowKey struct {
	Table string `json:"table"`
	RowID int32  `json:"row_id"`
}

// CharacterWrite is one row to write: its table, id, bytes, and whether the
// save holds it already.
type CharacterWrite struct {
	Table  string
	RowID  int32
	Data   []byte
	Insert bool
}

// BuildCharacters is the manifest that writes and removes the given character
// rows, in table and row order, so one decision always writes the same bytes.
func BuildCharacters(sourceSHA256 string, writes []CharacterWrite, bodies []CharacterBody, removals []CharacterRowKey) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(writes)+len(bodies)+len(removals) == 0 {
		return nil, fmt.Errorf("a character write needs at least one row")
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationCharacters}
	for _, write := range writes {
		manifest.CharacterRows = append(manifest.CharacterRows, CharacterRow{
			Table: write.Table, RowID: write.RowID, Insert: write.Insert, Data: hex.EncodeToString(write.Data)})
	}
	sort.Slice(manifest.CharacterRows, func(a, b int) bool {
		one, other := manifest.CharacterRows[a], manifest.CharacterRows[b]
		return one.Table < other.Table || one.Table == other.Table && one.RowID < other.RowID
	})
	manifest.CharacterBodies = append([]CharacterBody(nil), bodies...)
	sort.Slice(manifest.CharacterBodies, func(a, b int) bool {
		return manifest.CharacterBodies[a].RowID < manifest.CharacterBodies[b].RowID
	})
	manifest.CharacterRowRemovals = append([]CharacterRowKey(nil), removals...)
	sort.Slice(manifest.CharacterRowRemovals, func(a, b int) bool {
		one, other := manifest.CharacterRowRemovals[a], manifest.CharacterRowRemovals[b]
		return one.Table < other.Table || one.Table == other.Table && one.RowID < other.RowID
	})
	return manifest, nil
}
