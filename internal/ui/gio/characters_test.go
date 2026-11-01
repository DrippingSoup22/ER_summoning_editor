package gio

// The Characters view.
//
// The fixture save carries no characters - it is built from the regulation, and
// slots are the half of the file it does not model - so what these can check is
// that the view draws, that an empty save says so rather than breaking, and
// that the rules around granting hold. The grant itself is proved one layer
// down, against a real save, in internal/application and internal/character.

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
)

// Every view has to draw with a save open, including the one whose data the
// fixture cannot provide.
func TestTheCharactersViewDraws(t *testing.T) {
	state := loadedState(t)
	state.view = viewCharacters
	if dims := draw(t, state); dims.Size.X == 0 {
		t.Error("the Characters view did not lay out")
	}
	loaded := state.LoadedSave()
	if len(loaded.Characters) != 10 {
		t.Errorf("%d slots described, want the ten a PS4 save has", len(loaded.Characters))
	}
	// A save opens on its first character, and this one holds nobody, so
	// nobody is chosen and the right half says so rather than drawing an empty
	// list that looks like a character who owns nothing.
	if state.person != noPerson {
		t.Errorf("a character was chosen in a save that holds none: %d", state.person)
	}
}

// A grant the session refuses is reported rather than swallowed.
//
// The fixture save is built from the regulation and holds no characters, so
// every slot refuses - which makes it exactly the right thing to check the
// refusal path against. That a grant succeeds, stages once however many times
// it is asked for, and lands in a written save is proved against a real save in
// internal/application, which is the only place those can be proved at all.
func TestARefusedGrantIsReported(t *testing.T) {
	state := loadedState(t)
	state.view = viewCharacters
	draw(t, state)
	loaded := state.LoadedSave()
	if len(loaded.Ashes) == 0 {
		t.Skip("the fixture delivers no Ashes")
	}
	for _, person := range loaded.Characters {
		if person.Used {
			t.Skip("this fixture holds a character, so nothing here refuses")
		}
	}

	state.grant(characterView{Slot: 0, Name: "nobody"}, loaded.Ashes[0])

	text, bad := state.Notice()
	if !bad {
		t.Errorf("granting into an empty slot reported %q as success", text)
	}
	if state.PendingCount() != 0 {
		t.Errorf("%d changes staged by a grant that was refused", state.PendingCount())
	}
	if state.grantStaged(0, loaded.Ashes[0].GoodsFamily) {
		t.Error("a refused grant reads back as staged")
	}
}

// The level sliders run over every level a Spirit Ash has and no other.
func TestTheLevelSlidersSpanTheElevenLevels(t *testing.T) {
	if ashLevels != character.AshLevels {
		t.Fatalf("the sliders run to +%d, want +%d", ashLevels-1, character.AshLevels-1)
	}
}

// A level slider chooses the level it is let go on, and only then: a drag
// across it chooses nothing on the way, so the levels it crosses stage
// nothing, and the release chooses once.
func TestALevelSliderChoosesWhereItIsLetGo(t *testing.T) {
	var slider levelSlider
	th := newTestState(t, picker{}).Theme()
	router := new(input.Router)
	frame := func() (layout.Dimensions, int) {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(300, 60)), Source: router.Source()}
		dims, chose := slider.Layout(gtx, th, 3, 10)
		router.Frame(gtx.Ops)
		return dims, chose
	}
	dims, _ := frame()
	y := float32(dims.Size.Y) / 2
	at := func(kind pointer.Kind, x float32) pointer.Event {
		return pointer.Event{Kind: kind, Source: pointer.Mouse, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary}
	}
	track := float32(levelTrackWidth)
	router.Queue(at(pointer.Press, track-1))
	if _, chose := frame(); chose != -1 {
		t.Errorf("a press chose +%d before the slider was let go", chose)
	}
	router.Queue(at(pointer.Move, track/2)) // a held button: the router makes it a drag
	if _, chose := frame(); chose != -1 {
		t.Errorf("a drag chose +%d on its way", chose)
	}
	router.Queue(at(pointer.Release, track/2))
	if _, chose := frame(); chose < 4 || chose > 6 {
		t.Errorf("let go at the middle chose %d, want about +5", chose)
	}
	if _, chose := frame(); chose != -1 {
		t.Errorf("the frame after the release chose +%d again", chose)
	}
}

// A row's button says only Add or Remove: what the click does to the
// collection the save will hold, never Undo.
func TestTheRowButtonSaysOnlyAddOrRemove(t *testing.T) {
	for _, want := range []struct {
		name   string
		owned  bool
		staged *application.StagedGrant
		action string
	}{
		{"an Ash they do not have", false, nil, "Add"},
		{"an Ash they have", true, nil, "Remove"},
		{"one staged to be added", false, &application.StagedGrant{Was: -1}, "Remove"},
		{"one staged to be upgraded", true, &application.StagedGrant{Was: 3}, "Remove"},
		{"one staged to be taken away", true, &application.StagedGrant{Was: 3, Take: true}, "Add"},
	} {
		if got := rowAction(want.owned, want.staged); got != want.action {
			t.Errorf("%s says %q, want %q", want.name, got, want.action)
		}
	}
}

// And a row staged to be ADDED keeps its level chooser, because the level it
// will be added at is the one thing about it still worth changing. A row staged
// to be taken away has no level to show.
func TestAStagedAdditionKeepsItsLevelChooser(t *testing.T) {
	for _, want := range []struct {
		name     string
		owned    bool
		staged   *application.StagedGrant
		levelled bool
	}{
		{"an Ash they do not have", false, nil, false},
		{"an Ash they have", true, nil, true},
		{"one staged to be added", false, &application.StagedGrant{Was: -1}, true},
		{"one staged to be upgraded", true, &application.StagedGrant{Was: 3}, true},
		{"one staged to be taken away", true, &application.StagedGrant{Was: 3, Take: true}, false},
	} {
		if got := levelled(want.owned, want.staged); got != want.levelled {
			t.Errorf("%s shows a level chooser: %v, want %v", want.name, got, want.levelled)
		}
	}
}
