package application_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// THE CLONE ROWS ARE DECIDED AT SAVE: the file is
// written from the whole configuration - one row per recipe of a creature,
// shared by every slot that uses it; a row the save holds that is a recipe
// asked for kept where it stands; a row this editor inserted that nothing
// names removed; a new recipe on the lowest free band. One test per case the
// clone-band review of that day found.

// levelSlot is the fixture's first world actor on a level, on the stance the
// level requires.
func levelSlot(level application.Perception) application.AuthoredSlot {
	slot := worldActorSlot()
	slot.Perception = level
	slot.Stance, _ = application.LevelStance(slot)
	return slot
}

// reviewCommit stages the named Ashes on session and writes the result.
func reviewCommit(t *testing.T, session *application.Session, name string, ashes map[string][]application.AuthoredSlot) string {
	t.Helper()
	for host, slots := range ashes {
		if _, err := session.StageFormation(host, slots); err != nil {
			t.Fatalf("%s: %v", host, err)
		}
	}
	out := filepath.Join(t.TempDir(), name)
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), out, ""); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

type bandRows struct{ npc, think []int32 }

// creatureRows is the clone bands a creature holds a row in, per table.
func creatureRows(t *testing.T, path string, actor int32) bandRows {
	t.Helper()
	state, err := savecore.ReadFormations(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	var rows bandRows
	for _, band := range formation.CloneBands {
		key, _ := formation.CloneKey(actor, band)
		if state.NpcRows[key+formation.CloneOffset] {
			rows.npc = append(rows.npc, band)
		}
		if state.NpcThinkRows[key+formation.CloneOffset] {
			rows.think = append(rows.think, band)
		}
	}
	return rows
}

// slotRows is what each row of a block summons, in order.
func slotRows(t *testing.T, path string, trigger int32) [][2]int32 {
	t.Helper()
	state, err := savecore.ReadFormations(context.Background(), path, []int32{trigger})
	if err != nil {
		t.Fatal(err)
	}
	var rows [][2]int32
	for _, row := range state.Blocks[trigger].Rows {
		rows = append(rows, [2]int32{row.Npc, row.Think})
	}
	return rows
}

func bandRow(t *testing.T, actor, band int32) int32 {
	t.Helper()
	key, err := formation.CloneKey(actor, band)
	if err != nil {
		t.Fatal(err)
	}
	return key + formation.CloneOffset
}

// Reconfigured again and again on reopened saves, a creature holds exactly the
// rows its slots use - and going back to an earlier recipe never leaves the
// later one behind.
func TestAReconfiguredCreatureHoldsOnlyTheRowsItUses(t *testing.T) {
	t.Parallel()
	path := writeVanilla(t)
	actor := savefixture.CloneableWorldActors[0]
	for index, level := range []application.Perception{
		application.PerceptionFollower, application.PerceptionGuard, application.PerceptionSentinel,
		application.PerceptionFollower, application.PerceptionGuard,
	} {
		path = reviewCommit(t, loadedSession(t, path), fmt.Sprintf("step-%d.dat", index),
			map[string][]application.AuthoredSlot{authoringHost: {levelSlot(level)}})
		rows := creatureRows(t, path, actor)
		if fmt.Sprint(rows.npc) != "[1]" || fmt.Sprint(rows.think) != "[1]" {
			t.Errorf("step %d (%s): the creature holds npc bands %v and think bands %v, want one row each in band 1",
				index, level.Name(), rows.npc, rows.think)
		}
	}
}

// Taken out of the one Ash that summoned it, a creature leaves no row - and no
// formation row, the ones the smaller formation retires included, names one
// of this editor's rows the save does not hold. (Rows naming FromSoftware's
// own summons are not the question: the fixture carries a subset of those.)
func TestACreatureTakenOutOfItsAshLeavesNoRow(t *testing.T) {
	t.Parallel()
	gone, kept := savefixture.CloneableWorldActors[0], savefixture.CloneableWorldActors[1]
	path := reviewCommit(t, loadedSession(t, writeVanilla(t)), "both.dat",
		map[string][]application.AuthoredSlot{authoringHost: {{Npc: gone}, {Npc: kept}}})
	if rows := creatureRows(t, path, gone); len(rows.npc) != 1 {
		t.Fatalf("the creature was not written to begin with: %+v", rows)
	}
	path = reviewCommit(t, loadedSession(t, path), "one.dat",
		map[string][]application.AuthoredSlot{authoringHost: {{Npc: kept}}})
	if rows := creatureRows(t, path, gone); len(rows.npc)+len(rows.think) != 0 {
		t.Errorf("the creature taken out still holds npc bands %v and think bands %v", rows.npc, rows.think)
	}
	if rows := creatureRows(t, path, kept); fmt.Sprint(rows.npc) != "[1]" || fmt.Sprint(rows.think) != "[1]" {
		t.Errorf("the creature kept holds npc bands %v and think bands %v, want its one row each", rows.npc, rows.think)
	}
	state, err := savecore.ReadFormations(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	ours := func(row int32) bool {
		return row >= formation.CloneOffset && (row%formation.CloneOffset == gone || row%formation.CloneOffset == kept)
	}
	for rowID, actors := range state.Actors {
		if ours(actors.Npc) && !state.NpcRows[actors.Npc] {
			t.Errorf("formation row %d names NpcParam row %d, which the save does not hold", rowID, actors.Npc)
		}
		if ours(actors.Think) && !state.NpcThinkRows[actors.Think] {
			t.Errorf("formation row %d names NpcThinkParam row %d, which the save does not hold", rowID, actors.Think)
		}
	}
}

// Save never removes a row the save was loaded with that this editor did not
// insert: the game's own summon rows stay whatever names them.
func TestSaveNeverRemovesARowTheGameShips(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	before, err := savecore.ReadFormations(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := reviewCommit(t, loadedSession(t, input), "authored.dat",
		map[string][]application.AuthoredSlot{authoringHost: {worldActorSlot()}})
	after, err := savecore.ReadFormations(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for row := range before.NpcRows {
		if !after.NpcRows[row] {
			t.Errorf("NpcParam row %d the save was loaded with is gone", row)
		}
	}
	for row := range before.NpcThinkRows {
		if !after.NpcThinkRows[row] {
			t.Errorf("NpcThinkParam row %d the save was loaded with is gone", row)
		}
	}
}
