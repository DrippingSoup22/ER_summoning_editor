package application_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

func ownColoursHeld(t *testing.T, path string) bool {
	t.Helper()
	state, err := savecore.ReadState(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return state.OwnEffects[savecore.OwnColoursPresence]
}

// A look is part of a slot's recipe, and its row is ours: two slots of one
// creature in two looks are two clones, Save inserts the own-colours presence
// while a clone names it, and removes it with the last such clone.
func TestALooksRowLivesWhileAClonesNamesIt(t *testing.T) {
	t.Parallel()
	actor := savefixture.CloneableWorldActors[0]
	ghost, own := worldActorSlot(), worldActorSlot()
	own.Presence = savecore.OwnColoursPresence
	path := reviewCommit(t, loadedSession(t, writeVanilla(t)), "both.dat",
		map[string][]application.AuthoredSlot{authoringHost: {ghost, own}})
	if rows := creatureRows(t, path, actor); fmt.Sprint(rows.npc) != "[1 2]" || fmt.Sprint(rows.think) != "[1]" {
		t.Errorf("two looks hold npc bands %v and think bands %v, want two clones and one mind", rows.npc, rows.think)
	}
	if !ownColoursHeld(t, path) {
		t.Fatal("the own-colours presence was not inserted beside the clone that names it")
	}
	path = reviewCommit(t, loadedSession(t, path), "ghost.dat",
		map[string][]application.AuthoredSlot{authoringHost: {ghost}})
	if ownColoursHeld(t, path) {
		t.Error("the own-colours presence outlived the last clone naming it")
	}
}
