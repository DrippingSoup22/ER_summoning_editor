// These tests hold the authoring model against the shipped catalog rather than
// against a fixture written to agree with it. A model that cannot reproduce
// what the game already does is not a model of the game.
package formation_test

import (
	"encoding/csv"
	"strconv"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// shippedFormation is one family's block as the provenance-locked catalog
// records it: the BuddyParam rows it owns and the distinct actors they name.
//
// The catalog records distinct actors rather than one per row, so which row
// names which actor is not recoverable from it. That is not what these tests
// are about: the arithmetic under test is how many rows a formation occupies
// and where the next one goes.
type shippedFormation struct {
	name   string
	rows   []int32
	actors []int32
}

func shippedFormations(t *testing.T) []shippedFormation {
	t.Helper()
	records, err := csv.NewReader(reference.AshCatalog()).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	columns := map[string]int{}
	for index, name := range records[0] {
		columns[name] = index
	}
	var formations []shippedFormation
	for _, record := range records[1:] {
		formations = append(formations, shippedFormation{
			name:   record[columns["name"]],
			rows:   splitIDs(t, record[columns["formation_rows"]]),
			actors: splitIDs(t, record[columns["npc_ids"]]),
		})
	}
	if len(formations) != 84 {
		t.Fatalf("read %d families from the catalog, want 84", len(formations))
	}
	return formations
}

func splitIDs(t *testing.T, column string) []int32 {
	t.Helper()
	var ids []int32
	for _, text := range strings.Split(column, "|") {
		value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 32)
		if err != nil {
			t.Fatalf("catalog holds %q, which is not an ID: %v", text, err)
		}
		ids = append(ids, int32(value))
	}
	return ids
}

// Re-authoring a shipped formation at its own size must land on exactly its own
// rows: every row reused, nothing inserted, nothing retired and no clone asked
// for. Anything else means the model does not describe the data it claims to.
//
// This covers the two shapes a hand-written fixture would miss. Thirteen
// families mix distinct actors across their rows, and Jolán and Anna's two rows
// sit a hundred thousand apart — so the block being one contiguous run is a
// convention the model follows when it extends, not an assumption it makes when
// it reads.
func TestEveryShippedFormationReAuthorsOntoItsOwnRows(t *testing.T) {
	for _, shipped := range shippedFormations(t) {
		t.Run(shipped.name, func(t *testing.T) {
			block := formation.Block{Trigger: 1, Rows: make([]formation.BlockRow, 0, len(shipped.rows))}
			save := formation.Save{
				BuddyRows:    map[int32]bool{},
				NpcRows:      map[int32]bool{},
				NpcThinkRows: map[int32]bool{},
			}
			for _, rowID := range shipped.rows {
				block.Rows = append(block.Rows, formation.BlockRow{RowID: rowID, Doping: canonicalDoping()})
				save.BuddyRows[rowID] = true
			}

			authored := formation.Formation{Name: shipped.name}
			for index := range shipped.rows {
				actor := shipped.actors[index%len(shipped.actors)]
				authored.Slots = append(authored.Slots, formation.Slot{
					Actor: formation.Actor{Npc: actor, Think: actor},
				})
			}

			compiled, err := formation.Compile(authored, block, save)
			if err != nil {
				t.Fatal(err)
			}
			if len(compiled.Edits) != len(shipped.rows) {
				t.Fatalf("%d edits for %d rows", len(compiled.Edits), len(shipped.rows))
			}
			for index, edit := range compiled.Edits {
				if edit.RowID != shipped.rows[index] {
					t.Errorf("slot %d writes row %d, want the shipped %d", index+1, edit.RowID, shipped.rows[index])
				}
			}
			if len(compiled.Inserts) != 0 || len(compiled.Retired) != 0 || len(compiled.Clones) != 0 {
				t.Errorf("reproducing a shipped formation asked for %d inserts, %d retirements and %d clones",
					len(compiled.Inserts), len(compiled.Retired), len(compiled.Clones))
			}
		})
	}
}

// Filling every shipped family to ten is the operation v2 exists for. It must
// succeed for all of them, extend the block by exactly the shortfall, and still
// name no world row — the actors are the family's own.
func TestEveryShippedFamilyCanBeFilledToTenSlots(t *testing.T) {
	for _, shipped := range shippedFormations(t) {
		t.Run(shipped.name, func(t *testing.T) {
			block := formation.Block{Trigger: 1}
			save := formation.Save{
				BuddyRows:    map[int32]bool{},
				NpcRows:      map[int32]bool{},
				NpcThinkRows: map[int32]bool{},
			}
			for _, rowID := range shipped.rows {
				block.Rows = append(block.Rows, formation.BlockRow{RowID: rowID, Doping: canonicalDoping()})
				save.BuddyRows[rowID] = true
			}

			authored := formation.Formation{Name: shipped.name}
			for index := 0; index < formation.MaxSlots; index++ {
				actor := shipped.actors[index%len(shipped.actors)]
				authored.Slots = append(authored.Slots, formation.Slot{
					Actor: formation.Actor{Npc: actor, Think: actor},
				})
			}

			compiled, err := formation.Compile(authored, block, save)
			if err != nil {
				t.Fatal(err)
			}
			if want := formation.MaxSlots - len(shipped.rows); len(compiled.Inserts) != want {
				t.Errorf("%d inserts to reach ten from %d rows, want %d",
					len(compiled.Inserts), len(shipped.rows), want)
			}
			if len(compiled.Clones) != 0 {
				t.Errorf("filling a family with its own actors asked for %d clones", len(compiled.Clones))
			}
			last := shipped.rows[len(shipped.rows)-1]
			for _, insert := range compiled.Inserts {
				if insert.InsertAfter != last {
					t.Errorf("row %d anchors on %d, want the running end of the block %d",
						insert.RowID, insert.InsertAfter, last)
				}
				last = insert.RowID
			}
		})
	}
}
