package gio

// Composing a formation, and writing it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// editable is an Ash a formation can actually be staged against, with its
// index.
func editable(t *testing.T, state *State) (int, *ashView) {
	t.Helper()
	for index := range state.LoadedSave().Ashes {
		ash := &state.LoadedSave().Ashes[index]
		if ash.locked() == "" {
			return index, ash
		}
	}
	t.Fatal("no Ash in the fixture can be authored")
	return 0, nil
}

// aSpirit is the first variant of the first family the save can deliver, as a
// drag payload.
func aSpirit(t *testing.T, state *State) (string, variantView) {
	t.Helper()
	families := state.LoadedSave().Families
	if len(families) == 0 || families[0].spirits() == 0 {
		t.Fatal("the fixture delivers no spirits")
	}
	variant := families[0].Entries[0].Variants[0]
	return payloadFor(variant), variant
}

func payloadFor(variant variantView) string {
	return payload(variant)
}

// A drop stages the whole formation, because StageFormation replaces rather
// than edits. The slots the Ash already shipped have to go back in with it.
func TestADropStagesTheWholeFormation(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	state.selected = index
	payload, variant := aSpirit(t, state)

	shipped := ash.Ships
	state.drop(ash, maxSlots-1, state.slotsFor(ash), payload)

	if text, bad := state.Notice(); bad {
		t.Fatalf("the drop was refused: %s", text)
	}
	if state.PendingCount() != 1 {
		t.Fatalf("%d changes are pending after one drop, want 1", state.PendingCount())
	}
	staged := state.session.Staged().Formations
	if len(staged) != 1 {
		t.Fatalf("%d formations are staged, want 1", len(staged))
	}
	if got := len(staged[0].Requested); got != shipped+1 {
		t.Errorf("the staged formation has %d slots, want the shipped %d plus the drop",
			got, shipped)
	}
	if staged[0].HostGoodsFamily != ash.GoodsFamily {
		t.Errorf("the formation was staged against %d, want %s", staged[0].HostGoodsFamily, ash.Name)
	}

	// And the panel shows what was composed rather than what shipped, named
	// the way the hierarchy names it.
	slots := state.slotsFor(ash)
	named, _ := state.LoadedSave().dragged(variant.Npc, variant.Think, variant.Person)
	if !slots[maxSlots-1].Filled || slots[maxSlots-1].Name != named.Name || named.Name == "" {
		t.Errorf("slot 10 shows %+v, want %q", slots[maxSlots-1], named.Name)
	}
	if dims := draw(t, state); dims.Size.X == 0 {
		t.Error("the editor did not lay out after a drop")
	}
}

// warnsAbout is one of the models seen damaging the player who summoned
// it, and the row this save can actually deliver, pointed at it.
//
// The pointing is the fixture's fault rather than the test's: this save
// delivers 54 families and neither the Furnace Golem nor the Merciless Chariot
// is among them, so there is no deliverable row to drop that carries the model
// on its own. What is under test is the path from a label to a warning, and
// that path runs through exactly this map - see drop.
func warnsAbout(t *testing.T, state *State, model string) (variantView, string) {
	t.Helper()
	if _, hurts := application.HurtsSummoner(model); !hurts {
		t.Fatalf("%s does not warn, so this test is testing nothing", model)
	}
	variant := anOrdinarySpirit(t, state)
	// The drop names what it took from the hierarchy the panel drew, so the
	// pointing happens there: the variant's own model becomes the hazardous
	// one, in the view the drop consults.
	loaded := state.LoadedSave()
	for f := range loaded.all {
		for e := range loaded.all[f].Entries {
			for v := range loaded.all[f].Entries[e].Variants {
				if loaded.all[f].Entries[e].Variants[v].Key == variant.Key {
					loaded.all[f].Entries[e].Variants[v].Model = model
				}
			}
		}
	}
	named, _ := loaded.dragged(variant.Npc, variant.Think, variant.Person)
	return variant, named.Name
}

// A card dropped on the grid fills the Ash's empty slots in its order and
// replaces nobody; the first stands on the cell it was dropped on, and no body
// it adds stands in another, the player's included.
func TestACardFillsTheEmptySlotsAroundTheDropWithoutCrowding(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	state.selected = index
	before := state.slotsFor(ash)
	var many []variantView
	for _, family := range state.LoadedSave().Families {
		for _, entry := range family.Entries {
			many = append(many, entry.Variants...)
		}
	}
	if len(many) <= maxSlots {
		t.Fatalf("the fixture offers %d spirits, too few to overflow ten", len(many))
	}

	state.dropAt(ash, application.Placement{X: 3, Z: 4}, payloadOf(many))

	if text, bad := state.Notice(); bad {
		t.Fatalf("the drop was refused: %s", text)
	}
	after := state.slotsFor(ash)
	places := cellPlaces(after)
	firstNew := -1
	for index := range after {
		if !after[index].Filled {
			t.Fatalf("slot %d is still empty after a card larger than the free slots", index)
		}
		if before[index].Filled && after[index].Npc != before[index].Npc {
			t.Errorf("slot %d held %d and now holds %d: a card replaced a summon", index, before[index].Npc, after[index].Npc)
		}
		if !before[index].Filled && firstNew < 0 {
			firstNew = index
		}
	}
	if firstNew < 0 {
		t.Fatal("the fixture's Ash has no empty slot")
	}
	if got := places[firstNew]; got.X != 3 || got.Z != 4 {
		t.Errorf("the card's first stands at %g, %g, want the dropped-on cell 3, 4", got.X, got.Z)
	}
	bodies := state.LoadedSave().bodies
	everyone := []capsule{onTheGround(personBody, application.Placement{})}
	for index := range after {
		body, known := bodies[after[index].Model]
		if !known {
			body = personBody
		}
		everyone = append(everyone, onTheGround(body, places[index]))
	}
	for index = range after {
		if before[index].Filled {
			continue
		}
		for other, them := range everyone {
			if other != index+1 && everyone[index+1].gap(them) < clearance {
				t.Errorf("the added slot %d stands within %g m of body %d", index, clearance, other)
			}
		}
	}
}

// anOrdinarySpirit is the first variant nothing has been seen against,
// which is very nearly the first variant.
func anOrdinarySpirit(t *testing.T, state *State) variantView {
	t.Helper()
	for _, family := range state.LoadedSave().Families {
		for _, entry := range family.Entries {
			for _, variant := range entry.Variants {
				if _, hurts := application.HurtsSummoner(variant.Model); hurts {
					continue
				}
				return variant
			}
		}
	}
	t.Fatal("every spirit this save delivers warns, which cannot be right")
	return variantView{}
}

// A summon that has been seen to hurt the player who summoned it says so, and
// goes on saying it.
//
// A warning and never a refusal: the drop lands, the formation stages, and the
// player decides. Two are known - the Furnace Golem's burning ground and the
// Merciless Chariot rolling over whoever it turns into - and neither can be
// turned off from a save, which is why this is said rather than fixed. See
// internal/application/hazard.go and curation/TESTING.md section 10.
func TestASummonThatCanHurtYouWarnsAndIsStillSummoned(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	state.selected = index
	dangerous, name := warnsAbout(t, state, "c5170")

	state.drop(ash, maxSlots-1, state.slotsFor(ash), payloadFor(dangerous))

	text, bad := state.Notice()
	if bad {
		t.Fatalf("dropping %s was refused: %s", name, text)
	}
	if !state.Warned() {
		t.Errorf("dropping %s said %q without warning about it", name, text)
	}
	if !strings.Contains(text, "can hurt you") {
		t.Errorf("the warning reads %q, which does not say what it is warning about", text)
	}
	// A warning, not a refusal. The edit is staged like any other.
	if state.PendingCount() != 1 {
		t.Errorf("%d changes are pending after a warned drop, want 1", state.PendingCount())
	}

	// And the slot keeps saying it, because the footer's line is gone as soon
	// as anything else happens and the formation is not.
	slot := state.slotsFor(ash)[maxSlots-1]
	if slot.warning() == "" {
		t.Fatalf("slot 10 holds %s and warns about nothing", slot.Name)
	}
	if hover := slot.tooltip(); !strings.Contains(hover, "Can hurt you") ||
		!strings.Contains(hover, "clear it") {
		t.Errorf("the slot's hover reads %q, which does not say what it is or what to do", hover)
	}
	if dims := draw(t, state); dims.Size.X == 0 {
		t.Error("the editor did not lay out with a warned slot in the formation")
	}
}

// The save has no way to say "summons nothing", so the last slot cannot be
// emptied. The refusal has to explain itself rather than look like a dead
// control.
func TestTheLastSlotCannotBeEmptied(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	state.selected = index
	for slot := range ash.Slots {
		if ash.Slots[slot].Filled && ash.Ships > 1 {
			t.Skip("this fixture's first editable Ash ships more than one summon")
		}
	}

	state.clearSlot(ash, 0)

	text, bad := state.Notice()
	if !bad || !strings.Contains(text, "summon something") {
		t.Errorf("emptying the last slot said %q (bad=%v), want a refusal that explains itself", text, bad)
	}
	if state.PendingCount() != 0 {
		t.Errorf("%d changes are pending after a refused edit", state.PendingCount())
	}
	if state.slotsFor(ash)[0].Filled != ash.Slots[0].Filled {
		t.Error("a refused edit changed what the panel shows")
	}
}

// The whole point: an edit becomes a file, the input is untouched, and the
// editor carries on with what it just wrote.
func TestSavingWritesTheEditedFileAndCarriesOn(t *testing.T) {
	state := loadedState(t)
	input := state.LoadedSave().Path
	before, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	index, ash := editable(t, state)
	state.selected = index
	payload, _ := aSpirit(t, state)
	state.drop(ash, maxSlots-1, state.slotsFor(ash), payload)

	output := filepath.Join(t.TempDir(), "edited.dat")
	state.dialogs = picker{out: output}
	commit, err := state.session.PrepareCommit()
	if err != nil {
		t.Fatalf("preparing the commit: %v", err)
	}
	if !state.begin("test") {
		t.Fatal("the editor was busy")
	}
	state.saveWorker(commit, output)

	text, bad := state.Notice()
	if bad {
		t.Fatalf("saving failed: %s", text)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("the save was reported as written but is not there: %v", err)
	}
	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the file that was opened was modified, which this editor must never do")
	}
	if state.PendingCount() != 0 {
		t.Errorf("%d changes are still pending after a commit", state.PendingCount())
	}
	if got := state.LoadedSave().Path; got != output {
		t.Errorf("the editor is still on %s, want the file it just wrote", got)
	}

	// The composed formations described the old file, so the frame drops them
	// when it picks the new snapshot up.
	draw(t, state)
	if len(state.edits) != 0 {
		t.Errorf("%d composed formations survived the commit", len(state.edits))
	}
}

// Search is what makes 413 families browsable. It matches the name a player
// would type inside the category chosen, an empty query shows that category
// whole, and a query that matches only elsewhere says where.
func TestSearchNarrowsTheCatalog(t *testing.T) {
	state := loadedState(t)
	loaded := state.LoadedSave()
	category := loaded.Categories[state.kind]
	var inCategory []int
	for index, family := range loaded.Families {
		if family.Category == category {
			inCategory = append(inCategory, index)
		}
	}
	all := len(state.visibleFamilies(loaded))
	if all != len(inCategory) {
		t.Fatalf("an empty search shows %d families, and %s holds %d", all, category, len(inCategory))
	}

	wanted := loaded.Families[inCategory[0]].Name
	state.search.SetText(wanted)
	narrowed := state.visibleFamilies(loaded)
	if len(narrowed) == 0 || len(narrowed) == all {
		t.Fatalf("searching for %q left %d of %d families", wanted, len(narrowed), all)
	}
	for _, index := range narrowed {
		// The query matches the card's whole search text - its name, its
		// types and its variants - so a card whose own name does not hold the
		// word must hold it somewhere inside.
		if !strings.Contains(loaded.Families[index].search, strings.ToLower(wanted)) {
			t.Errorf("%q matched the search for %q", loaded.Families[index].Name, wanted)
		}
	}

	// A card only another category holds is not found here, and the empty
	// catalog names the category that holds it.
	tried := false
	for _, family := range loaded.Families {
		state.search.SetText(family.Name)
		if family.Category == category || len(state.visibleFamilies(loaded)) > 0 {
			continue
		}
		if said := state.nothingHere(loaded); !strings.Contains(said, categoryLabel(family.Category)) {
			t.Errorf("searching %s for %q says %q, not naming %s", category, family.Name, said, family.Category)
		}
		tried = true
		break
	}
	if !tried {
		t.Fatalf("every card of another category also matches one in %s", category)
	}

	state.search.SetText("qqqq no such spirit")
	if got := len(state.visibleFamilies(loaded)); got != 0 {
		t.Errorf("a query matching nothing left %d families", got)
	}
	if state.family != noFamily {
		t.Error("a family stayed open while the grid showed nothing")
	}
}
