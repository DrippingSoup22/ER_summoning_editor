// Command ersummoneditorgui is the desktop Spirit Ash editor: a Gio window
// over the same internal/application the command line uses. One binary, every
// catalog embedded, no input beyond the save the player chooses.
//
// The command-line ersummoneditor is unchanged and links none of this.
package main

import (
	"log"
	"os"
	"strconv"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/platform/dialogs"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/ui/gio"
)

func main() {
	preparePlatform()
	service, err := application.New()
	if err != nil {
		log.Fatalf("load the catalog: %v", err)
	}
	state := gio.NewState(service, dialogs.NewNative())

	go func() {
		if err := run(state); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(state *gio.State) error {
	window := new(app.Window)
	uiScale := configuredUIScale(os.Getenv("ER_EDITOR_UI_SCALE"), platformUIScale())
	// A window, or full screen if the player turned that on in Settings, where
	// the switch also changes it while the editor runs. Esc asks to close it.
	state.SetWindow(window, uiScale)
	window.Option(state.WindowOptions()...)

	// Development convenience: open a save immediately. Never point this at an
	// original - the editor does not write its input, but a copy is one command
	// and an original is not replaceable.
	if path := os.Getenv("ER_SUMMON_EDITOR_SAVE"); path != "" {
		state.StartLoadSave(path)
	}

	// Asked again on the first frame, in case a driver applies the options
	// above before the window can take them. Gio's Windows driver keeps no
	// record of the mode it last set, so the ask costs nothing when the
	// window is full screen already.
	filled := false

	var ops op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.FrameEvent:
			if !filled {
				filled = true
				if state.FullScreen() {
					window.Option(app.Fullscreen.Option())
				}
			}
			gtx := app.NewContext(&ops, event)
			// The display's own scale, corrected only where the system does
			// not report it (WSL). Bigger pictures are a setting on the grids
			// that hold them, not a scale for the whole window.
			gtx.Metric.PxPerDp *= uiScale
			gtx.Metric.PxPerSp *= uiScale
			state.Layout(gtx, state.Theme())
			event.Frame(gtx.Ops)
		}
	}
}

// configuredUIScale is the platform's correction unless ER_EDITOR_UI_SCALE
// gives a sensible multiplier (0.5 to 3), the same variable the merchant
// editor reads.
func configuredUIScale(raw string, platformDefault float32) float32 {
	if raw == "" {
		return platformDefault
	}
	scale, err := strconv.ParseFloat(raw, 32)
	if err != nil || scale < 0.5 || scale > 3 {
		log.Printf("ignoring invalid ER_EDITOR_UI_SCALE %q (expected 0.5-3)", raw)
		return platformDefault
	}
	return float32(scale)
}
