package gio

// The character builder's slots.

import (
	"image"
	"slices"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/layout"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// What the builder does that the session does not check: spells stay packed
// from the first slot, as the row holds them, a stack starts with a count, a
// new weapon keeps the level the hand had as far as it goes, an Ash of War
// moves the affinity to one it allows and is dropped by a weapon it does not go
// on, and the catalog offers a slot only what it takes.
func TestTheBuilderFillsSlotsAsTheRowHoldsThem(t *testing.T) {
	state := newTestState(t, picker{})
	if err := state.loadItems(); err != nil {
		t.Fatal(err)
	}
	every, lightning := [len(application.Affinities)]bool{}, [len(application.Affinities)]bool{}
	for affinity := range every {
		every[affinity] = true
		lightning[affinity] = affinity <= 3 || affinity == 6 || affinity == 7
	}
	loaded := &savedView{choices: &application.BuildChoices{
		Weapons: map[int32]application.WeaponChoice{
			2000000: {MaxLevel: 25, Mount: 1, Affinities: every, Skill: every},
			1010000: {MaxLevel: 10, Mount: -1, Affinities: [len(application.Affinities)]bool{true}, Skill: every},
		},
		Ashes: map[int32]application.AshChoice{
			21600: {Mounts: 1 << 1, Affinities: lightning, Default: 6}, // Thunderbolt
			10500: {Mounts: 1 << 15, Affinities: every, Default: 3},    // Charge Forth, a spear's
		},
	}}
	open := &builder{draft: application.PlainBuild(true)}
	put := func(slot builderSlot, key string) {
		t.Helper()
		item, ok := state.itemNamed(key)
		if !ok {
			t.Fatalf("no item %s", key)
		}
		open.slot = slot
		state.put(open, item, loaded)
	}

	put(builderSlot{spellSlot, 4}, "magic-4000")
	put(builderSlot{spellSlot, 6}, "magic-6000")
	if open.draft.Spells[0] != 4000 || open.draft.Spells[1] != 6000 || open.slot.index != 1 {
		t.Errorf("spells went to %v, the slot to %d", open.draft.Spells, open.slot.index)
	}
	open.slot = builderSlot{spellSlot, 0}
	open.clear(loaded.choices)
	if open.draft.Spells[0] != 6000 || open.draft.Spells[1] != 0 {
		t.Errorf("after taking the first out the spells are %v", open.draft.Spells)
	}

	put(builderSlot{ammoSlot, 1}, "weapon-52000000")
	put(builderSlot{quickSlot, 8}, "goods-300")
	if open.draft.Ammo[1].Count != newAmmo || open.draft.Quick[8].Count != newGoods {
		t.Errorf("new stacks hold %d bolts and %d pots", open.draft.Ammo[1].Count, open.draft.Quick[8].Count)
	}

	put(builderSlot{handSlot, application.RightHand}, "weapon-2000000")
	open.draft.Hands[application.RightHand].Level, open.draft.Hands[application.RightHand].Affinity = 25, 4
	ash := builderSlot{ashSlot, application.RightHand}
	open.slot = ash
	for _, item := range state.items {
		if open.offer(loaded).takes(item) && item.Key() != "gem-21600" {
			t.Errorf("the Longsword is offered %s", item.Name)
		}
	}
	put(ash, "gem-21600")
	if hand := open.draft.Hands[application.RightHand]; hand.Ash != 21600 || hand.Affinity != 6 || open.slot != ash.hand() {
		t.Errorf("Thunderbolt on a Fire Longsword gives %+v, and the slot %+v", hand, open.slot)
	}
	put(builderSlot{handSlot, application.RightHand}, "weapon-1010000")
	if hand := open.draft.Hands[application.RightHand]; hand.Base != 1010000 || hand.Level != 10 || hand.Ash != 0 || hand.Affinity != 0 {
		t.Errorf("the somber weapon came in as %+v", hand)
	}

	arrows := builderSlot{ammoSlot, 0}
	categories, subcategories := arrows.takes()
	for _, item := range state.items {
		if takesItem(item, categories, subcategories) && item.Subcategory != "Arrows" && item.Subcategory != "Greatarrows" {
			t.Errorf("the arrow slot is offered %s, a %s", item.Name, item.Subcategory)
		}
	}
}

// A click on an equipment cell turns the item catalog to that slot in the
// frame that reads it, rather than at the pointer's next move.
func TestAnEquipmentCellTurnsTheCatalogAtOnce(t *testing.T) {
	state := loadedState(t)
	state.LoadedSave().choices = &application.BuildChoices{}
	h := newHands(t, state) // its first frame takes the load in
	state.openBuilder(noBuild)
	if state.builder == nil {
		text, _ := state.Notice()
		t.Fatalf("the builder did not open: %s", text)
	}
	h.frame()
	h.click("Talisman 2")
	if !slices.ContainsFunc(h.screen(), func(line string) bool { return strings.HasSuffix(line, " ITEMS FOR TALISMAN 2") }) {
		t.Error("the frame that read the click still shows another slot's items")
	}
}

// A new character starts a Vagabond at the Vagabond's attributes, and no
// attribute takes a value under its class's: one typed under it is not taken
// and the field shows the attribute again once the keys leave. Another class
// moves untouched attributes to its own and raises edited ones only where
// they fall under it.
func TestTheStartingClassHoldsTheAttributes(t *testing.T) {
	state := loadedState(t)
	vagabond := [8]uint8{15, 10, 11, 14, 13, 9, 9, 7}
	astrologer := [8]uint8{9, 15, 9, 8, 12, 16, 7, 9}
	state.LoadedSave().choices = &application.BuildChoices{Classes: [][8]uint8{vagabond, {}, {}, {}, astrologer}}
	h := newHands(t, state) // its first frame takes the load in
	state.openBuilder(noBuild)
	open := state.builder
	if open == nil || open.draft.Class != 0 || open.draft.Attributes != vagabond {
		t.Fatalf("a new character starts as %+v", open)
	}
	h.frame()
	// The attribute fields are the column's small ones, Vigor the first.
	vigor := func() f32.Point {
		t.Helper()
		var first image.Rectangle
		for bounds := range h.controls(true) {
			if bounds.Dx() < 100 && bounds.Max.X < 400 && (first.Empty() || bounds.Min.Y < first.Min.Y) {
				first = bounds
			}
		}
		if first.Empty() {
			t.Fatal("the builder shows no attribute field")
		}
		return layout.FPt(first.Min.Add(first.Max).Div(2))
	}
	h.clickAt(vigor())
	h.type_("5")
	h.click("Body type B") // the keys leave the field
	if got := open.attributes[0].Text(); open.draft.Attributes[0] != 15 || got != "15" {
		t.Errorf("Vigor 5 under the Vagabond's 15 left the attribute %d and the field %q", open.draft.Attributes[0], got)
	}
	h.clickAt(vigor())
	h.type_("30")
	if open.draft.Attributes[0] != 30 {
		t.Fatalf("Vigor 30 left the attribute %d", open.draft.Attributes[0])
	}

	open.chooseClass(state.LoadedSave(), 4)
	want := vagabond
	want[0] = 30
	for index := range want {
		want[index] = max(want[index], astrologer[index])
	}
	if open.draft.Attributes != want {
		t.Errorf("an edited Vagabond made an Astrologer is %v, want %v", open.draft.Attributes, want)
	}
	open.draft.Attributes = astrologer
	open.chooseClass(state.LoadedSave(), 0)
	if open.draft.Attributes != vagabond {
		t.Errorf("an untouched Astrologer made a Vagabond is %v, want %v", open.draft.Attributes, vagabond)
	}
}

// The item catalog offers a dropdown only where it is a choice: a talisman's
// or a helm's one category is none, a hand's three are, and so are its
// subcategories; every category offered holds an item the slot takes, and a
// hand opens on what it holds - melee armaments in the right, shields in the
// left.
func TestTheItemFiltersAreOfferedOnlyAsAChoice(t *testing.T) {
	state := newTestState(t, picker{})
	if err := state.loadItems(); err != nil {
		t.Fatal(err)
	}
	open := &builder{draft: application.PlainBuild(true)}
	for _, want := range []struct {
		slot        builderSlot
		kinds, subs bool
		opens       string
	}{
		{builderSlot{talismanSlot, 0}, false, false, "talismans"},
		{builderSlot{armourSlot, 0}, false, false, "head"},
		{builderSlot{handSlot, application.RightHand}, true, true, "melee_armaments"},
		{builderSlot{handSlot, application.LeftHand}, true, true, "shields"},
	} {
		open.slot, open.kind, open.sub = want.slot, openingCategory, 0
		takes := open.offer(nil)
		filters := state.itemFiltersFor(open, takes)
		if filters.choiceOfKinds != want.kinds || filters.choiceOfSubs != want.subs {
			t.Errorf("%s offers the categories %t and the subcategories %t, want %t and %t",
				want.slot.name(), filters.choiceOfKinds, filters.choiceOfSubs, want.kinds, want.subs)
		}
		if opened := filters.kinds[open.kind]; opened != want.opens {
			t.Errorf("%s opens on %s, want %s", want.slot.name(), opened, want.opens)
		}
		for index, kind := range filters.kinds {
			if len(state.offeredItems(open, takes, kind, allSubcategories)) == 0 {
				t.Errorf("%s offers the category %d, %s, which holds nothing it takes", want.slot.name(), index, kind)
			}
		}
	}
}
