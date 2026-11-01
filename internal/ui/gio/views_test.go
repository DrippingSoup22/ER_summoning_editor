package gio

// The three views, and the settings behind them.

import (
	"image"
	"slices"
	"strings"
	"testing"

	"gioui.org/io/key"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// A setting takes effect and is stored in one gesture, and what was stored is
// what comes back.
func TestASettingAppliesAndSurvivesARestart(t *testing.T) {
	state := newTestState(t, picker{})
	state.settings.Font = "cinzel"
	state.settings.EditorIconSize = 120
	state.applySettings()

	if state.Theme().Face != displayFace {
		t.Errorf("the theme face is %q, want the chosen %q", state.Theme().Face, displayFace)
	}
	if state.EditorIconSize() != 120 {
		t.Errorf("the rail draws at %v, want the chosen 120", state.EditorIconSize())
	}
	if text, bad := state.Notice(); bad {
		t.Errorf("storing settings failed: %s", text)
	}

	stored := loadSettings()
	if stored.Font != "cinzel" || stored.EditorIconSize != 120 {
		t.Errorf("stored settings read back as %+v, want cinzel with a 120dp rail", stored)
	}
}

// A config written by an older build, hand-edited, or truncated by a crash
// must leave the editor usable rather than odd.
func TestUnusableSettingsFallBackQuietly(t *testing.T) {
	repaired := Settings{Font: "papyrus"}.normalize()
	if repaired.Font != "lora" {
		t.Errorf("an unknown font became %q, want lora", repaired.Font)
	}
	if zero := (Settings{}).normalize(); zero != defaultSettings() {
		t.Errorf("an empty config became %+v, want the defaults", zero)
	}
}

// Every view has to draw at any time, including with no save open. The Editor
// with nothing loaded explains itself rather than being unreachable.
func TestEveryViewDrawsWithAndWithoutASave(t *testing.T) {
	loadedState := newTestState(t, picker{path: vanillaPath(t)})
	loadedState.begin("test")
	loadedState.openWorker()

	for _, state := range []*State{newTestState(t, picker{}), loadedState} {
		for view := viewCharacters; int(view) < viewCount; view++ {
			state.view = view
			if dims := draw(t, state); dims.Size != (image.Pt(1280, 800)) {
				t.Errorf("view %s laid out %v, want the whole window", viewNames[view], dims.Size)
			}
		}
	}
}

// Esc asks whether to close the editor once nothing else takes it, and a
// chosen summon does not. The dialog says what closing loses, if anything;
// Cancel closes it, and Close closes the editor. Opening another save over
// staged work asks the same question first.
func TestEscAsksBeforeClosingTheEditor(t *testing.T) {
	state := loadedState(t)
	closed := false
	state.quit = func() { closed = true }
	h := newHands(t, state)
	says := func(text string) bool {
		return slices.ContainsFunc(h.screen(), func(line string) bool { return strings.HasSuffix(line, " "+text) })
	}

	h.press(key.NameEscape)
	if !state.quitOpen || !says("Close the editor?") || !says("Close") {
		t.Fatalf("Esc left the dialog open %t", state.quitOpen)
	}
	h.click("Cancel")
	if state.quitOpen {
		t.Fatal("Cancel left the dialog open")
	}

	index, ash := editable(t, state)
	state.openAsh(index)
	payload, _ := aSpirit(t, state)
	state.dropAt(ash, application.Placement{X: 3, Z: 0}, payload) // chooses the summon
	h.frame()
	h.press(key.NameEscape)
	if !state.quitOpen || state.chosenSlot == noToken {
		t.Fatalf("Esc over a chosen summon opened the dialog %t and left the summon chosen %t",
			state.quitOpen, state.chosenSlot != noToken)
	}
	h.click("Cancel")
	// Opening another save over the staged change asks the same way.
	opened := false
	state.askBeforeOpening(func() { opened = true })
	h.frame()
	if opened || !says("Open another save?") || !says("1 change staged and not saved.") {
		t.Fatalf("opening over a staged change did not ask first (opened %t)", opened)
	}
	h.click("Open without saving")
	if !opened {
		t.Fatal("Open without saving did not open")
	}

	h.press(key.NameEscape)
	if !says("1 change staged and not saved.") {
		t.Fatal("the dialog does not say the staged change is lost")
	}
	h.click("Close without saving")
	if !closed {
		t.Error("Close did not close the editor")
	}
}

// A theme chosen in Settings recolours the window at once and is stored.
func TestTheThemeIsChosenInSettingsAndStored(t *testing.T) {
	t.Cleanup(func() { usePalette("spirit") })
	state := newTestState(t, picker{})
	state.view = viewSettings
	h := newHands(t, state)
	h.click("Spirit " + arrowGlyph(false))
	h.click("Elden Ring")
	if colorBg != palettes["elden"].bg || loadSettings().Theme != "elden" {
		t.Fatalf("after choosing Elden Ring the ground is %v, stored theme %q", colorBg, loadSettings().Theme)
	}
	if repaired := (Settings{Theme: "neon"}).normalize(); repaired.Theme != "spirit" {
		t.Errorf("an unknown theme became %q, want spirit", repaired.Theme)
	}
}

// An explanation left open on a view the player has left lets go of itself,
// so the next Esc is not spent closing a bubble nobody can see.
func TestAnExplanationOutOfSightLetsEscThrough(t *testing.T) {
	state := newTestState(t, picker{})
	state.view = viewSettings
	h := newHands(t, state)
	h.click("?")
	if state.helpPinned == noHelp {
		t.Fatal("a click on a Settings ? opened nothing")
	}
	h.click("Characters")
	h.press(key.NameEscape)
	if !state.quitOpen {
		t.Error("Esc on another view closed an unseen explanation instead of asking to close")
	}
}

// The message line: news fades on its own, and a warning stays until the next
// press, wherever it lands.
func TestNewsFadesAndAWarningWaitsForThePress(t *testing.T) {
	state := newTestState(t, picker{})
	h := newHands(t, state)
	shows := func(text string) bool {
		return slices.ContainsFunc(h.screen(), func(line string) bool { return strings.HasSuffix(line, " "+text) })
	}
	state.setNotice(notice{text: "Saved a copy."})
	h.frame()
	if !shows("Saved a copy.") {
		t.Fatal("the news is not on the line")
	}
	h.wait(noticeLife)
	if shows("Saved a copy.") {
		t.Error("the news is still on the line after its time")
	}

	state.setNotice(notice{text: "It can hurt you.", warn: true})
	h.frame()
	h.wait(3 * noticeLife)
	if !shows("It can hurt you.") {
		t.Fatal("the warning faded before anything was pressed")
	}
	h.click("Settings")
	if shows("It can hurt you.") {
		t.Error("the warning outlived the next press")
	}
}

// A window is the default, and the full-screen switch takes effect and is
// stored.
func TestTheFullScreenSwitchIsStored(t *testing.T) {
	state := newTestState(t, picker{})
	if state.FullScreen() {
		t.Fatal("the editor opens full screen by default")
	}
	state.view = viewSettings
	h := newHands(t, state)
	toggle := func() {
		t.Helper()
		at, ok := h.described("Full screen")
		if !ok {
			t.Fatal("Settings shows no Full screen switch")
		}
		h.clickAt(at)
	}
	toggle()
	if !state.FullScreen() || !loadSettings().FullScreen {
		t.Fatalf("after the switch the editor is full screen %t, stored %t",
			state.FullScreen(), loadSettings().FullScreen)
	}
	toggle()
	if state.FullScreen() || loadSettings().FullScreen {
		t.Errorf("after the second switch the editor is full screen %t, stored %t",
			state.FullScreen(), loadSettings().FullScreen)
	}
}
