package application_test

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// A FORMATION SHRUNK, SAVED AND GROWN AGAIN. A formation
// smaller than its block retires the surplus rows - triggerSpEffectId -1 - so
// they leave the block and stay where they were, directly after it; growing
// again used to collide with the first of them, so a player who took a
// creature out of a container and saved could never add one back there. The
// block now re-fills the rows it retired (savecore.FormationBlock.Inert).

// blockRowIDs is the live rows and the inert rows of one block, as the save
// holds them.
func blockRowIDs(t *testing.T, path string, trigger int32) (live, inert []int32) {
	t.Helper()
	state, err := savecore.ReadFormations(context.Background(), path, []int32{trigger})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range state.Blocks[trigger].Rows {
		live = append(live, row.RowID)
	}
	for _, row := range state.Blocks[trigger].Inert {
		inert = append(inert, row.RowID)
	}
	return live, inert
}

func rowRun(first int32, count int) []int32 {
	run := make([]int32, 0, count)
	for index := 0; index < count; index++ {
		run = append(run, first+int32(index))
	}
	return run
}

// authorCount writes the host's formation with count slots over path.
func authorCount(t *testing.T, path string, count int, name string) (string, application.CommitSummary) {
	t.Helper()
	session := loadedSession(t, path)
	if _, err := session.StageFormation(authoringHost, slotsOf(count)); err != nil {
		t.Fatalf("%d slots: %v", count, err)
	}
	commit := mustPrepare(t, session)
	out := filepath.Join(t.TempDir(), name)
	if _, err := session.Commit(context.Background(), commit, out, ""); err != nil {
		t.Fatalf("%d slots: %v", count, err)
	}
	return out, commit.Summary()
}

// A reset after all of it is the shipped save again: the formation rows the
// editor inserted go, the shipped ones - re-filled or still retired - are put
// back.
func TestAResetAfterAShrinkAndAGrowIsTheShippedSave(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	path := input
	for step, count := range []int{2, 1, 6, 1} {
		path, _ = authorCount(t, path, count, fmt.Sprintf("step-%d.dat", step))
	}
	session := loadedSession(t, path)
	if _, err := session.StageReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), restored, ""); err != nil {
		t.Fatal(err)
	}
	want, err := savecore.ReadFormations(context.Background(), input, []int32{authoringTrigger})
	if err != nil {
		t.Fatal(err)
	}
	got, err := savecore.ReadFormations(context.Background(), restored, []int32{authoringTrigger})
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got.BuddyRows, want.BuddyRows) {
		t.Errorf("the reset left %d BuddyParam rows, the shipped save has %d", len(got.BuddyRows), len(want.BuddyRows))
	}
	for id, actors := range want.Actors {
		if got.Actors[id] != actors {
			t.Errorf("formation row %d names %+v after the reset, shipped %+v", id, got.Actors[id], actors)
		}
	}
	if live, inert := blockRowIDs(t, restored, authoringTrigger); !slices.Equal(live, rowRun(authoringTrigger*100, authoringRows)) || len(inert) != 0 {
		t.Errorf("after the reset the block is %v with retired rows %v, want the shipped four and none", live, inert)
	}
}
