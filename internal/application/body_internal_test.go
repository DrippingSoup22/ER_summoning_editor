package application

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
)

// The ladder takes a creature's collision radius for its width and its look
// for its front: Miriel's shell is drawn 3 m wide on the grid, while its sight
// keeps the game's 0.5 m body and reaches from where the shell ends.
func TestTheLadderTakesTheCollisionWidthAndTheLooksFront(t *testing.T) {
	families, err := catalog.LoadFamilies()
	if err != nil {
		t.Fatal(err)
	}
	actor, err := AuthoredSlot{Npc: 60910000, Think: 60910000}.withBody(families)
	if err != nil {
		t.Fatal(err)
	}
	if actor.Body != 0.5 || actor.Front != 3 {
		t.Errorf("Miriel's ladder body is %v wide with its front at %v, want 0.5 and 3", actor.Body, actor.Front)
	}
}
