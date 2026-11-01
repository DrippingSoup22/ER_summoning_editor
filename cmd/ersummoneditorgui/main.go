// Command ersummoneditorgui is the desktop Spirit Ash editor: a Gio window
// over the same internal/application the command line uses. One binary, every
// catalog embedded, no input beyond the save the player chooses.
//
// The command-line ersummoneditor is unchanged and links none of this.
package main

import (
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/platform/dialogs"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/ui/gio"
)

func main() {
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
	// Full screen unless the player turned it off in Settings, where the
	// switch also changes it while the editor runs. Esc asks to close it.
	window.Option(state.WindowOptions()...)
	state.SetWindow(window)

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
			// No scale multiplier. There was one, as a Settings choice, and it
			// resampled the entire window - every icon, every glyph - to solve
			// a problem the maximised window solves better. What a player
			// actually wanted from it was bigger pictures, and that is now its
			// own setting on the grids that hold them.
			state.Layout(gtx, state.Theme())
			event.Frame(gtx.Ops)
		}
	}
}
