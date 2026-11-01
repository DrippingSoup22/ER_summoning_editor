package gio

// Writing the edited save.
//
// The order here is the one rule worth stating twice: the plan is frozen
// before the dialog opens. A native file dialog is modal to the operating
// system and not to this application, so a plan assembled after it opened
// could silently include or omit an edit made while it was up. PrepareCommit
// runs in the frame, where nothing else can be happening; the dialog and the
// write run in a worker.
//
// The loaded file is never the output. That is enforced a layer down, and it
// is why this asks where to save on every commit instead of remembering an
// answer.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/platform/dialogs"
)

// StartSaveAs freezes what is staged and asks where to write it.
func (s *State) StartSaveAs() {
	loaded := s.LoadedSave()
	if loaded == nil || s.Busy() {
		return
	}
	// In the frame, and before the dialog: see this file's own note on why the
	// order is the rule. Calling it here is allowed because no worker holds the
	// session - which the Busy check above is what guarantees.
	commit, err := s.session.PrepareCommit()
	if err != nil {
		s.setNotice(notice{text: describeCommitError(err), bad: true})
		s.invalidate()
		return
	}
	if !s.begin("Choosing where to save…") {
		return
	}
	go s.saveWorker(commit, suggestedOutput(loaded.Path))
}

func (s *State) saveWorker(commit *application.CommitPlan, suggested string) {
	path, err := s.dialogs.SaveAs(suggested)
	if err != nil {
		if errors.Is(err, dialogs.ErrCanceled) {
			s.finish(nil, notice{})
			return
		}
		s.finish(nil, notice{text: "Could not open the save dialog: " + err.Error(), bad: true})
		return
	}
	s.progress("Writing the save…")

	// No audit manifest: an empty path tells the commit layer not to write
	// one. It stays on the command line (-manifest-out).
	result, err := s.session.Commit(context.Background(), commit, path, "")
	switch {
	case result != nil && err != nil:
		// The save was written and is correct; only the session's own reload
		// of it failed. Saying so is the difference between a file the player
		// can use and one they think they lost.
		s.finish(nil, notice{
			text: fmt.Sprintf("%s was written, but the editor could not reopen it: %v",
				filepath.Base(path), err),
			bad: true,
		})
		return
	case err != nil:
		s.finish(nil, notice{text: describeCommitError(err), bad: true})
		return
	}

	// The session has advanced to the file it just wrote, so the snapshot is
	// rebuilt from it: the player carries on editing what they made rather
	// than the file they started from.
	view, readErr := s.readView()
	if readErr != nil {
		s.finish(nil, notice{
			text: fmt.Sprintf("%s was written, but could not be described: %v",
				filepath.Base(result.OutputPath), readErr),
			bad: true,
		})
		return
	}
	s.finishLoad(view, true, notice{text: fmt.Sprintf("Saved %s.", filepath.Base(result.OutputPath))})
}

// suggestedOutput is the name the dialog opens on. Beside the input, and never
// the input itself.
func suggestedOutput(path string) string {
	name := filepath.Base(path)
	extension := filepath.Ext(name)
	return filepath.Join(filepath.Dir(path),
		strings.TrimSuffix(name, extension)+"-edited"+extension)
}

// describeCommitError turns a refusal into a sentence. Only the two a player
// can actually cause are named; everything else is shown as it came, because a
// friendlier wording for an unknown failure hides it.
func describeCommitError(err error) string {
	switch {
	case errors.Is(err, application.ErrNothingStaged):
		return "There is nothing to save yet."
	case errors.Is(err, application.ErrNoSaveLoaded):
		return "Open a save first."
	default:
		return err.Error()
	}
}
