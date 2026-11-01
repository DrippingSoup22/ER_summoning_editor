package gio

// The shell's two rules: a worker owns the session while it holds it, and the
// frame only ever renders what a worker handed over.
//
// Layout is exercised headlessly. A layout.Context with no window still maps
// units to pixels and lays widgets out, which is enough to catch a nil
// dereference or an index panic in a panel - the two failures that would
// otherwise only show up in front of the player.

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/platform/dialogs"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

var (
	vanillaOnce  sync.Once
	vanillaSave  []byte
	vanillaError error
)

// vanillaPath writes the synthetic vanilla save the application tests use. It
// is built once: it carries every catalogued goods row, which is what makes a
// load through this package meaningful rather than a smoke test.
func vanillaPath(t testing.TB) string {
	t.Helper()
	vanillaOnce.Do(func() {
		options, err := savefixture.VanillaOptions()
		if err != nil {
			vanillaError = err
			return
		}
		vanillaSave, vanillaError = savefixture.Build(options)
	})
	if vanillaError != nil {
		t.Fatal(vanillaError)
	}
	path := filepath.Join(t.TempDir(), "memory.dat")
	if err := os.WriteFile(path, vanillaSave, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// picker answers the file dialog without opening one. out is where a save
// goes; it is a separate field because the editor must never be able to answer
// "save as" with the file it opened.
type picker struct {
	path string
	out  string
	err  error
}

func (p picker) OpenSave() (string, error) { return p.path, p.err }

func (p picker) SaveAs(string) (string, error) {
	if p.out != "" {
		return p.out, p.err
	}
	return p.path, p.err
}

// newTestState builds an editor whose settings live in a temporary directory.
// A test must never read or write the real config: it would make results
// depend on how the developer has their own editor set up, and could overwrite
// it.
func newTestState(t testing.TB, dial dialogs.Service) *State {
	t.Helper()
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config) // Linux and BSD
	t.Setenv("AppData", config)         // Windows
	service, err := application.New()
	if err != nil {
		t.Fatal(err)
	}
	return NewState(service, dial)
}

// headlessGtx is one frame's context with no window behind it.
//
// The Source matters and is not decoration. A zero input.Source reports
// Enabled() == false, and every material widget draws its DISABLED styling
// when it is - so a headless frame without one renders every button in grey,
// which is wrong in tests and actively misleading in a screenshot.
func headlessGtx() layout.Context {
	router := new(input.Router)
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1280, 800)},
		Source:      router.Source(),
	}
}

// draw lays one frame out and reports what it claimed.
func draw(t *testing.T, state *State) layout.Dimensions {
	t.Helper()
	gtx := headlessGtx()
	return state.Layout(gtx, state.Theme())
}

// The application starts with no save, and that is a state it has to be able
// to draw rather than a gap before the real one.
func TestTheEditorDrawsBeforeASaveIsOpen(t *testing.T) {
	state := newTestState(t, picker{})
	dims := draw(t, state)
	if want := image.Pt(1280, 800); dims.Size != want {
		t.Errorf("the shell laid out %v, want the whole window %v", dims.Size, want)
	}
	if state.LoadedSave() != nil {
		t.Error("something was reported as loaded before anything was opened")
	}
	if state.PendingCount() != 0 {
		t.Error("a fresh editor has staged changes")
	}
}

// The worker reads, snapshots, and hands over; the frame draws the snapshot.
func TestOpeningASaveFillsTheViewTheFrameDraws(t *testing.T) {
	path := vanillaPath(t)
	state := newTestState(t, picker{path: path})

	if !state.begin("test") {
		t.Fatal("a fresh editor was already busy")
	}
	state.openWorker()

	if state.Busy() {
		t.Error("the worker did not hand the session back")
	}
	view := state.LoadedSave()
	if view == nil {
		t.Fatal("the save loaded but the frame was given nothing to draw")
	}
	if view.Name != "memory.dat" {
		t.Errorf("view names %q, want the file that was opened", view.Name)
	}
	if reader, err := savefixture.Open(vanillaSave); err != nil {
		t.Fatal(err)
	} else if view.RegulationVersion != reader.RegulationVersion() {
		t.Errorf("view reports regulation %d, the save holds %d", view.RegulationVersion, reader.RegulationVersion())
	}
	// The counts come from the read models rather than from constants here, so
	// this asserts that both were actually consulted.
	if len(view.Ashes) != 84 {
		t.Errorf("view reports %d Spirit Ashes, want the catalogue's 84", len(view.Ashes))
	}
	if len(view.Families) == 0 || view.Spirits < len(view.Families) {
		t.Errorf("view reports %d families and %d spirits, which cannot both be right",
			len(view.Families), view.Spirits)
	}
	if _, bad := state.Notice(); bad {
		t.Error("a successful load reported a failure")
	}

	if dims := draw(t, state); dims.Size != (image.Pt(1280, 800)) {
		t.Errorf("the loaded editor laid out %v, want the whole window", dims.Size)
	}
}

// A load that fails leaves the editor exactly as usable as it was, holding
// whatever save it already had.
func TestAFailedLoadKeepsTheEditorUsable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-save.dat")
	if err := os.WriteFile(path, []byte("nonsense"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := newTestState(t, picker{path: path})
	state.begin("test")
	state.openWorker()

	text, bad := state.Notice()
	if !bad {
		t.Fatal("opening nonsense was reported as a success")
	}
	if !strings.Contains(text, "not-a-save.dat") {
		t.Errorf("the failure %q does not name the file it was about", text)
	}
	if state.Busy() || state.LoadedSave() != nil {
		t.Error("a failed load left the editor busy or claiming a save")
	}
	if dims := draw(t, state); dims.Size != (image.Pt(1280, 800)) {
		t.Errorf("the editor could not draw after a failed load: %v", dims.Size)
	}
}
