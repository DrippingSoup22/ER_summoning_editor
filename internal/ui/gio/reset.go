package gio

// Reset to vanilla, which stages rather than acts.
//
// This is the sharpest departure from the command line, where Reset is a thing
// that happens. Here it joins the pending list and can be dropped like any
// other change: a player can reset, keep editing, change their mind, and none
// of it is real until they save. The footer is where that becomes visible.
//
// It runs in a worker because it is not a memory operation. StageReset re-reads
// the loaded file - a reset is a diff, and there is nothing in memory to diff
// against, since Load reads what this product writes and a reset restores rows
// outside that. Reading 26 MB inside a frame would drop it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// StartReset stages a return to shipped behaviour for the whole save.
func (s *State) StartReset() {
	if s.LoadedSave() == nil {
		return
	}
	if !s.begin("Comparing the save against vanilla…") {
		return
	}
	go s.resetWorker()
}

func (s *State) resetWorker() {
	staged, err := s.session.StageReset(context.Background())
	if err != nil {
		s.finish(nil, notice{text: describeResetError(err), bad: true})
		return
	}
	restored := staged.GoodsRows + staged.BuddyRows + staged.StoneRows
	message := fmt.Sprintf("Reset staged: %d rows back to vanilla", restored)
	if staged.RowsRemoved > 0 {
		message += fmt.Sprintf(", %d added rows out", staged.RowsRemoved)
	}
	s.finish(nil, notice{text: message + "."})
}

// dropReset drops a staged reset. Unlike staging one this is a memory
// operation, so it happens in the frame.
func (s *State) dropReset() {
	if s.Busy() {
		return
	}
	if s.session.UnstageReset() {
		s.setNotice(notice{text: "Reset dropped."})
	}
}

// resetStaged reports whether a reset is pending, which is what turns the
// footer's button around.
func (s *State) resetStaged() bool {
	if s.Busy() || !s.session.Loaded() {
		return false
	}
	return s.session.Staged().Restoration != nil
}

// describeResetError turns a refusal into a sentence a player can act on.
//
// The already-vanilla case is the one worth naming: it is not a failure, it is
// the answer to the question the button asks, and reporting it as an error
// would tell a player something went wrong when nothing did.
func describeResetError(err error) string {
	var unsupported *application.UnsupportedRegulationError
	switch {
	case errors.Is(err, application.ErrAlreadyVanilla):
		return "Already vanilla: nothing to reset."
	case errors.As(err, &unsupported):
		return fmt.Sprintf(
			"Regulation %d: this editor knows the shipped values of %s only.",
			unsupported.Found, application.SupportedRegulations())
	case errors.Is(err, application.ErrSourceMismatch):
		return "The save changed on disk: open it again."
	default:
		return "Could not compare against vanilla: " + err.Error()
	}
}
