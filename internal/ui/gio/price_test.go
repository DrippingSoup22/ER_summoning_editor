package gio

// The two price fields: when they stage, when they refuse, and when they take
// a change back rather than writing one.

import (
	"strings"
	"testing"
)

// pricedAsh is an Ash whose price is a single number, which is every Ash the
// game ships. The varying case is drawn differently and tested separately.
func pricedAsh(t *testing.T, state *State) *ashView {
	t.Helper()
	for index := range state.LoadedSave().Ashes {
		ash := &state.LoadedSave().Ashes[index]
		if !ash.PriceVaries {
			state.selected = index
			return ash
		}
	}
	t.Skip("the fixture holds no Ash with one price")
	return nil
}

// typePrice puts numbers in the fields the way a player would and applies them.
func typePrice(state *State, ash *ashView, fp, hp string) {
	state.loadPrice(ash)
	state.fpField.SetText(fp)
	state.hpField.SetText(hp)
	state.applyPrice(ash)
}

// The ordinary case: a number goes in, a change is staged, and it says what it
// will cost rather than which rows it will write.
func TestTypingAPriceStagesIt(t *testing.T) {
	state := loadedState(t)
	ash := pricedAsh(t, state)

	typePrice(state, ash, "42", "0")

	if state.priceErr != "" {
		t.Fatalf("a valid price reported %q", state.priceErr)
	}
	if state.PendingCount() != 1 {
		t.Fatalf("%d changes are pending after typing a price, want 1", state.PendingCount())
	}
	costs := state.session.Staged().Costs
	if len(costs) != 1 {
		t.Fatalf("%d prices are staged, want 1", len(costs))
	}
	if costs[0].Price.FP != 42 {
		t.Errorf("the staged price is %d FP, want 42", costs[0].Price.FP)
	}
	if costs[0].Was != ash.Price {
		t.Errorf("the staged price says it was %+v, want the save's %+v", costs[0].Was, ash.Price)
	}
}

// A number the field cannot hold is refused at the field, while the player is
// still looking at the box they typed it into.
func TestAPriceOutsideTheAllowedRangeIsRefusedAtTheField(t *testing.T) {
	state := loadedState(t)
	ash := pricedAsh(t, state)

	typePrice(state, ash, "999999", "0")

	if state.priceErr == "" {
		t.Error("a price past the maximum staged without saying anything")
	}
	if state.PendingCount() != 0 {
		t.Errorf("%d changes are pending after an impossible price", state.PendingCount())
	}

	typePrice(state, ash, "not a number", "0")
	if !strings.Contains(state.priceErr, "whole numbers") {
		t.Errorf("a non-numeric price said %q", state.priceErr)
	}
	if state.PendingCount() != 0 {
		t.Errorf("%d changes are pending after a non-numeric price", state.PendingCount())
	}
}

// Free makes one Ash cost nothing and, unticked, puts it back on its own
// price. All free ticks every box not ticked yet and stages nothing for one
// that is; with all ticked it unticks them all, a price typed for one too.
// And the fields show what an Ash will cost - the price staged for it - when
// the player comes back to it.
func TestFreeAndAllFree(t *testing.T) {
	state := loadedState(t)
	ash := pricedAsh(t, state)
	loaded := state.LoadedSave()
	var other *ashView
	for index := range loaded.Ashes {
		if one := &loaded.Ashes[index]; one != ash && !one.PriceVaries && !one.Price.Free() {
			other = one
			break
		}
	}
	if other == nil || ash.Price.Free() {
		t.Skip("the fixture holds no two Ashes with a price of their own")
	}

	typePrice(state, ash, "42", "0")
	state.loadPrice(other)
	state.loadPrice(ash)
	if got := state.fpField.Text(); got != "42" {
		t.Errorf("back on an Ash with 42 FP staged, the field says %q", got)
	}

	state.setFree(ash, true)
	if price, staged := state.session.StagedPrice(ash.GoodsFamily); !staged || !price.Free() || !state.isFree(ash) {
		t.Errorf("Free staged %+v (%t)", price, staged)
	}
	state.setFree(ash, false)
	if _, staged := state.session.StagedPrice(ash.GoodsFamily); staged {
		t.Error("Free unticked left a price staged")
	}

	state.setFree(ash, true)
	typePrice(state, other, "7", "0")
	notFree := 0
	for index := range loaded.Ashes {
		if !state.isFree(&loaded.Ashes[index]) {
			notFree++
		}
	}
	state.setAllFree(true)
	if !state.allFree() {
		t.Error("All free left an Ash that costs something")
	}
	shipFree := 0
	for index := range loaded.Ashes {
		if one := &loaded.Ashes[index]; !one.PriceVaries && one.Price.Free() {
			if _, staged := state.session.StagedPrice(one.GoodsFamily); staged {
				t.Errorf("All free staged a price for %s, which the game ships free", one.Name)
			}
			shipFree++
		}
	}
	if costs := len(state.session.Staged().Costs); costs != notFree+1 {
		t.Errorf("All free left %d prices staged, want the %d it freed and the one ticked before", costs, notFree)
	}

	state.setAllFree(false)
	if costs := state.session.Staged().Costs; len(costs) != 0 {
		t.Errorf("All free switched off left %d prices staged", len(costs))
	}
	if state.allFree() && shipFree != len(loaded.Ashes) {
		t.Error("All free is still on with every price dropped")
	}
}
