package savecore_test

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

func readFormations(t *testing.T, path string, triggers ...int32) *savecore.FormationState {
	t.Helper()
	state, err := savecore.ReadFormations(context.Background(), path, triggers)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// A block read back after it was authored into shows the inserted row in the
// block, in the position it was inserted at. Anything else would mean the
// reader and the writer disagree about what a formation is, and a second
// authoring pass would be planned against a block that is not there.
func TestAnAuthoredBlockReadsBackWithItsNewRow(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	output := newOutputPath(t)
	before := snapshotOf(t, input)
	encoded := authorManifestFor(t, before.SourceSHA256, fiveOfOneWorldActor(), vanillaOf(t, input, allGoodsRows()))
	if _, err := savecore.Apply(context.Background(), input, output, bytes.NewReader(encoded)); err != nil {
		t.Fatal(err)
	}

	block := readFormations(t, output, authoredTrigger).Blocks[authoredTrigger]
	var ids []int32
	for _, row := range block.Rows {
		ids = append(ids, row.RowID)
	}
	want := []int32{
		authoredBlockHead, authoredBlockHead + 1, authoredBlockHead + 2,
		authoredBlockHead + 3, authoredNewRow,
	}
	if !slices.Equal(ids, want) {
		t.Errorf("the authored block holds %v, want %v", ids, want)
	}
	summon := savefixture.CloneableWorldActors[0] + formation.CloneOffset
	for _, row := range block.Rows {
		if row.Npc != summon || row.Think != summon {
			t.Errorf("row %d names %d/%d after authoring, want %d for both", row.RowID, row.Npc, row.Think, summon)
		}
	}
	// The inserted row inherited the reinforcement of the row it was copied
	// from, which is the whole reason an insertion copies a row instead of
	// starting from zeroes.
	if got, want := block.Rows[4].Doping, block.Rows[3].Doping; got != want {
		t.Errorf("the inserted row carries reinforcement %v, want the copied row's %v", got, want)
	}
}
