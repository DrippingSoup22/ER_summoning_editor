package gio

// The creature window.

import (
	"image"
	"slices"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// Every choice the creature window offers is staged as asked, with the stance
// it calls for, and the writer takes it: a role that needs a stance of its own,
// a share, the strength, the look, and the enemy switch, which makes the summon
// the Loner on the enemy team.
func TestTheCreatureWindowStagesWhatItOffers(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	state.openAsh(index)
	state.dropAt(ash, application.Placement{X: 2, Z: 3}, payload(aCreature(t, state)))
	slot := ash.Ships // the first free slot

	staged := func() application.AuthoredSlot {
		t.Helper()
		if text, bad := state.Notice(); bad {
			t.Fatalf("refused: %s", text)
		}
		requested := state.session.Staged().Formations[0].Requested
		return requested[len(requested)-1]
	}

	state.setRole(ash, slot, application.PerceptionHunter)
	hunter := staged()
	if want, _ := application.LevelStance(hunter); hunter.Perception != application.PerceptionHunter || hunter.Stance != want {
		t.Errorf("the Hunter is staged as %q on %+v, want %+v", hunter.Perception.Name(), hunter.Stance, want)
	}

	state.setShare(ash, slot, true)
	if shared := staged(); shared.Share != application.ShareOn || shared.Stance.DisablePCTargetShare != 0 {
		t.Errorf("the share is staged as %v on %+v", shared.Share, shared.Stance)
	}

	state.setFighter(ash, slot, true)
	if fighter := staged(); !fighter.Fighter {
		t.Error("the Fighter is staged as a Helper")
	}

	state.setLook(ash, slot, true)
	if own := staged(); own.Look != application.LookOwnColours {
		t.Errorf("the look is staged as %v", own.Look)
	}

	state.setEnemy(ash, slot, true)
	enemy := staged()
	if enemy.Team != application.TeamEnemy || !enemy.WorldBranch || enemy.Perception != application.PerceptionLoner {
		t.Errorf("the enemy is staged on team %v, world branch %t, as %q", enemy.Team, enemy.WorldBranch, enemy.Perception.Name())
	}
}

// A click on the window between its controls leaves it open; a click on the
// dimmed window around it closes it. The panel carries an input layer of its
// own for this, or every blank pixel of it would be the scrim's.
func TestTheCreatureWindowClosesOnlyFromOutside(t *testing.T) {
	state := loadedState(t)
	draw(t, state) // the frame that takes the load in, which closes any Ash
	index, ash := editable(t, state)
	state.view = viewEditor
	state.openAsh(index)
	state.dropAt(ash, application.Placement{X: 2, Z: 3}, payload(aCreature(t, state)))
	state.openCreature(ash.Ships)

	router := new(input.Router)
	size := image.Pt(1280, 800)
	frame := func() {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size), Source: router.Source()}
		state.Layout(gtx, state.Theme())
		router.Frame(gtx.Ops)
	}
	click := func(at f32.Point) {
		frame()
		router.Queue(
			pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
			pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at})
		frame()
	}

	// The panel is centred and creatureWidth wide; a few pixels in from its
	// left edge is its own padding.
	left := float32(size.X-int(creatureWidth)) / 2
	frame()
	if state.creatureSlot == noToken {
		t.Fatal("the window did not open")
	}
	click(f32.Pt(left+6, float32(size.Y)/2))
	if state.creatureSlot == noToken {
		t.Fatal("a click inside the window closed it")
	}
	click(f32.Pt(left/2, float32(size.Y)/2))
	if state.creatureSlot != noToken {
		t.Error("a click outside the window left it open")
	}
}

// Esc lets an explanation kept open go first, and closes the window only on
// the next press: the window is what the player is reading.
func TestEscClosesAnExplanationBeforeTheWindow(t *testing.T) {
	state := loadedState(t)
	draw(t, state) // the frame that takes the load in, which closes any Ash
	index, ash := editable(t, state)
	state.view = viewEditor
	state.openAsh(index)
	state.dropAt(ash, application.Placement{X: 2, Z: 3}, payload(aCreature(t, state)))
	state.openCreature(ash.Ships)
	state.helpPinned = helpShare

	router := new(input.Router)
	frame := func() {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1280, 800)), Source: router.Source()}
		state.Layout(gtx, state.Theme())
		router.Frame(gtx.Ops)
	}
	escape := func() {
		router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
		frame()
	}
	frame()
	escape()
	if state.helpPinned != noHelp || state.creatureSlot == noToken {
		t.Fatalf("after one Esc the explanation is %d and the window on %d; want the explanation gone, the window open",
			state.helpPinned, state.creatureSlot)
	}
	escape()
	if state.creatureSlot != noToken {
		t.Error("a second Esc left the window open")
	}
}

// aCreature is a catalog creature of the player's own: not a person, and not
// a creature of the game's own Ashes, whose row takes no other role.
func aCreature(t *testing.T, state *State) variantView {
	t.Helper()
	for _, family := range state.LoadedSave().Families {
		for _, entry := range family.Entries {
			for _, variant := range entry.Variants {
				if variant.Person == 0 && !variant.Shipped {
					return variant
				}
			}
		}
	}
	t.Fatal("the fixture delivers no creature")
	return variantView{}
}

// The window as a player's hands meet it. Pointing at a control moves nothing
// in the window, and a click moves only what is under the line it changes, so
// a click lands where the player aimed; a control the window does not offer is
// drawn with no input at all, so no click reaches it; and an explanation opens
// on a click on its "?", never under the pointer.
func TestTheCreatureWindowTakesClicksOnlyWhereItOffers(t *testing.T) {
	state := loadedState(t)
	draw(t, state) // the frame that takes the load in, which closes any Ash
	index, ash := editable(t, state)
	state.view = viewEditor
	state.openAsh(index)
	state.dropAt(ash, application.Placement{X: 2, Z: 3}, payload(aCreature(t, state)))
	slot := ash.Ships
	state.openCreature(slot)
	h := newHands(t, state)

	still := h.screen()
	for _, name := range []string{"Statue", "Hunter", "Fighter", "?"} {
		h.point(name)
		if now := h.screen(); !slices.Equal(now, still) {
			t.Fatalf("pointing at %s changed what the window shows", name)
		}
	}
	if state.helpPinned != noHelp {
		t.Error("pointing at a ? opened its explanation")
	}
	h.click("?")
	if state.helpPinned == noHelp {
		t.Error("a click on a ? opened nothing")
	}
	h.press(key.NameEscape)

	// The Follower always shares the player's aggro: its switch is not one.
	if _, offered := h.find("Share aggro"); offered {
		t.Error("the Follower's Share aggro takes a click")
	}
	// Fighter's line is shorter than Helper's: what is above it stays put.
	statue, _ := h.find("Statue")
	h.click("Fighter")
	if moved, _ := h.find("Statue"); moved != statue {
		t.Errorf("choosing the Fighter moved the roles from %v to %v", statue, moved)
	}
	h.click("Guard")
	h.click("Share aggro")
	if got := state.slotsFor(ash)[slot]; !got.Fighter || got.Level != application.PerceptionGuard || got.Share != application.ShareOn {
		t.Errorf("after Fighter, Guard and Share aggro the slot is fighter %t, %q, share %v",
			got.Fighter, got.Level.Name(), got.Share)
	}
	if text, bad := state.Notice(); bad {
		t.Errorf("refused: %s", text)
	}

	// The facing, set by the pad.
	h.click("Face back-left")
	if got := state.placeOf(state.slotsFor(ash), slot).Angle; got != -135 {
		t.Errorf("Face back-left left the summon facing %g, want -135", got)
	}

	// A creature of the game's own Ashes keeps its role: no role is a button.
	shipped, found := aShippedCreature(state)
	if !found {
		t.Skip("the fixture delivers no creature of the game's own Ashes")
	}
	state.dropAt(ash, application.Placement{X: -2, Z: 3}, payload(shipped))
	state.openCreature(state.chosenSlot)
	h.frame()
	for _, role := range roles {
		if _, offered := h.find(role.name); offered {
			t.Errorf("the game's own Ash creature offers the %s", role.name)
		}
	}
}

// aShippedCreature is a creature of the game's own Ashes, as its card offers
// it.
func aShippedCreature(state *State) (variantView, bool) {
	for _, family := range state.LoadedSave().Families {
		for _, entry := range family.Entries {
			for _, variant := range entry.Variants {
				if variant.Shipped && variant.Person == 0 {
					return variant, true
				}
			}
		}
	}
	return variantView{}, false
}
