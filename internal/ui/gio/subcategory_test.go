package gio

// The subcategory filter beside the category, and the broken switch that moved
// from there to Settings.
//
// Like spirits_test.go these hold the BEHAVIOUR - a subcategory narrows its
// category, choosing a category resets it, the switch is stored - and not the
// vocabulary, which is research's. The one word they name is the order the
// author gave the boss subcategories in.

import (
	"testing"
)

// Every subcategory of every category narrows the grid to its own variants,
// on the real hierarchy: a card holding two subcategories keeps only the
// chosen one's entries, and "All subcategories" gives the category back whole.
func TestASubcategoryNarrowsItsCategory(t *testing.T) {
	state := loadedState(t)
	loaded := state.LoadedSave()
	tried := 0
	for kind, category := range loaded.Categories {
		state.chooseKind(kind)
		whole := len(state.visibleFamilies(loaded))
		subs := loaded.subcategoriesOf(category)
		for sub := 1; sub < len(subs); sub++ {
			state.sub = sub
			visible := state.visibleFamilies(loaded)
			// A subcategory of broken rows alone shows nothing with the
			// switch off, which is the switch working, not the filter failing.
			if len(visible) == 0 && subs[sub] == "broken" {
				continue
			}
			if len(visible) == 0 {
				t.Errorf("%s / %s shows no card", category, subs[sub])
			}
			for _, index := range visible {
				family := loaded.Families[index]
				for _, variant := range everyVariant(family) {
					if variant.Subcategory != subs[sub] {
						t.Errorf("%s / %s shows %s, which is %q", category, subs[sub], variant.Key, variant.Subcategory)
					}
				}
			}
			tried++
		}
		state.sub = 0
		if back := len(state.visibleFamilies(loaded)); back != whole {
			t.Errorf("%s: All subcategories shows %d cards, the category alone %d", category, back, whole)
		}
	}
	if tried == 0 {
		t.Fatal("the hierarchy holds no subcategory to filter by")
	}
}

// The subcategory dropdown is drawn only where the category has one to choose:
// a dropdown offering "All subcategories" alone is no choice.
func TestTheSubcategoryDropdownShowsOnlyWithAChoice(t *testing.T) {
	state := loadedState(t)
	h := newHands(t, state)
	offered := 0
	for kind, category := range state.LoadedSave().Categories {
		state.chooseKind(kind)
		h.frame()
		_, shown := h.find(allSubcategories + " ▾")
		want := len(state.LoadedSave().subcategoriesOf(category)) > 1
		if shown != want {
			t.Errorf("%s: the subcategory dropdown is shown %t, want %t", category, shown, want)
		}
		if want {
			offered++
		}
	}
	if offered == 0 || offered == len(state.LoadedSave().Categories) {
		t.Fatalf("%d of %d categories have subcategories, so the test tells nothing apart",
			offered, len(state.LoadedSave().Categories))
	}
}
