package gio

// The rail and the ten slots.

import (
	"image"
	"testing"

	"gioui.org/layout"
)

// loadedState is an editor with the vanilla fixture open, which is the only
// state in which the Editor has anything to draw.
func loadedState(t testing.TB) *State {
	t.Helper()
	state := newTestState(t, picker{path: vanillaPath(t)})
	state.begin("test")
	state.openWorker()
	if state.LoadedSave() == nil {
		text, _ := state.Notice()
		t.Fatalf("the fixture did not load: %s", text)
	}
	state.view = viewEditor
	return state
}

// Choosing an Ash is what opens the middle panel, and it is frame state: no
// session call, no reload, just an index.
func TestChoosingAnAshOpensItsFormation(t *testing.T) {
	state := loadedState(t)
	if state.selectedAsh(state.LoadedSave()) != nil {
		t.Fatal("a freshly loaded save already had an Ash open")
	}
	if dims := draw(t, state); dims.Size != image.Pt(1280, 800) {
		t.Fatalf("the Editor with no Ash chosen laid out %v", dims.Size)
	}

	state.selected = 3
	chosen := state.selectedAsh(state.LoadedSave())
	if chosen == nil || chosen.Name != state.LoadedSave().Ashes[3].Name {
		t.Fatalf("choosing the fourth Ash opened %v", chosen)
	}
	if dims := draw(t, state); dims.Size != image.Pt(1280, 800) {
		t.Errorf("the Editor with an Ash chosen laid out %v", dims.Size)
	}

	// A stale index must never index the snapshot, whatever leaves one behind.
	state.selected = len(state.LoadedSave().Ashes)
	if state.selectedAsh(state.LoadedSave()) != nil {
		t.Error("an out-of-range selection resolved to an Ash")
	}
	if dims := draw(t, state); dims.Size != image.Pt(1280, 800) {
		t.Errorf("the Editor with a stale selection laid out %v", dims.Size)
	}
}

// The containers are every Ash but the one whose summon the game binds
// itself, which holds nothing to edit.
func TestTheContainersLeaveOutTheAshTheGameFixes(t *testing.T) {
	loaded := loadedState(t).LoadedSave()
	listed := make(map[int32]bool, len(loaded.Containers))
	for _, index := range loaded.Containers {
		listed[loaded.Ashes[index].GoodsFamily] = true
	}
	if listed[256000] || len(listed) != len(loaded.Ashes)-1 {
		t.Errorf("the containers list Kristoff %t and %d of %d Ashes, want all but Kristoff",
			listed[256000], len(listed), len(loaded.Ashes))
	}
}

// A second save has its own Ashes, so the selection cannot survive the load -
// index 40 of one file is a different Ash in another.
func TestLoadingAnotherSaveClearsTheSelection(t *testing.T) {
	state := loadedState(t)
	state.selected = 40

	state.begin("second")
	state.openWorker()
	draw(t, state)

	if state.selected != noAsh {
		t.Errorf("the selection survived a load as %d, want none", state.selected)
	}
}

// A slot a formation holds is named as a drop of the same entry is: a creature
// on a clone by the world rows it copies, a person by its CharaInitParam row
// rather than the shell it stands on, and a person the catalog does not offer
// by that row.
func TestAHeldSlotIsNamedAsADropIs(t *testing.T) {
	state := loadedState(t)
	loaded := state.LoadedSave()
	var person, creature variantView
	for _, family := range loaded.all {
		for _, variant := range everyVariant(family) {
			switch {
			case variant.Person != 0:
				person = variant
			case !variant.Shipped && variant.Think != 0:
				creature = variant
			}
		}
	}
	if person.Person == 0 || creature.Npc == 0 {
		t.Fatal("the fixture offers no person or no creature")
	}
	wantPerson, _ := loaded.dragged(person.Npc, person.Think, person.Person)
	wantCreature, _ := loaded.dragged(creature.Npc, creature.Think, 0)
	slots := &loaded.Ashes[0].Slots
	slots[0] = slotView{Filled: true, Npc: person.Npc, Think: person.Think, Person: person.Person}
	slots[1] = slotView{Filled: true, Npc: person.Npc, Think: person.Think, Person: 2990199}
	// The creature's second clone, in band 2.
	slots[2] = slotView{Filled: true, Npc: creature.Npc + 2*100000000, Think: creature.Think + 2*100000000, Name: "Actor"}
	loaded.nameHeld()
	for at, want := range []string{wantPerson.Name, "Person 2990199", wantCreature.Name} {
		if slots[at].Name != want {
			t.Errorf("held slot %d is named %q, want %q", at, slots[at].Name, want)
		}
	}
}

// A click on an Ash's tile asks for the frame that draws it open at once. The
// click is read after the frame chose what to draw, and without the ask the
// open Ash waited for whatever redrew the window next - the pointer's move.
func TestAClickedAshAsksForTheFrameThatOpensIt(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	h := newHands(t, state)
	// The rail's tile, the larger of the two controls an Ash the game ships
	// a creature of has: the catalog's Ashes card says the same name.
	var tile image.Rectangle
	for bounds, says := range h.controls(false) {
		if says == ash.Name && bounds.Dx()*bounds.Dy() > tile.Dx()*tile.Dy() {
			tile = bounds
		}
	}
	h.clickAt(layout.FPt(tile.Min.Add(tile.Max).Div(2)))
	if state.selected != index {
		t.Fatalf("a click on %s opened Ash %d, want %d", ash.Name, state.selected, index)
	}
	if at, wake := h.router.WakeupTime(); !wake || !at.IsZero() {
		t.Errorf("the click asked for the next frame at %v (asked: %t), want at once", at, wake)
	}
}
