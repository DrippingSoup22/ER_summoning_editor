package gio

// Reset to vanilla.

import (
	"path/filepath"
	"strings"
	"testing"
)

// The branch that matters: a save this editor wrote does differ from vanilla,
// so the reset has something to stage. It stages rather than acting - it joins
// the pending list and can be dropped - which is the whole departure from the
// command line, where Reset is a thing that happens.
//
// Getting here needs an edited save, so this makes one: an edit, a commit, and
// then the reset against the file that commit produced.
func TestResettingAnEditedSaveStagesRatherThanActing(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	state.selected = index
	payload, _ := aSpirit(t, state)
	state.drop(ash, maxSlots-1, state.slotsFor(ash), payload)

	output := filepath.Join(t.TempDir(), "edited.dat")
	state.dialogs = picker{out: output}
	commit, err := state.session.PrepareCommit()
	if err != nil {
		t.Fatalf("preparing the commit: %v", err)
	}
	if !state.begin("test") {
		t.Fatal("the editor was busy")
	}
	state.saveWorker(commit, output)
	if text, bad := state.Notice(); bad {
		t.Fatalf("writing the save to reset failed: %s", text)
	}
	if state.PendingCount() != 0 {
		t.Fatalf("%d changes survived the commit", state.PendingCount())
	}

	state.resetWorker()

	text, bad := state.Notice()
	if bad {
		t.Fatalf("staging a reset on an edited save failed: %s", text)
	}
	if !state.resetStaged() {
		t.Fatal("an edited save staged no reset")
	}
	if !strings.Contains(text, "staged") {
		t.Errorf("the reset reported %q, which does not say it only staged", text)
	}
	if state.PendingCount() != 1 {
		t.Fatalf("%d changes are pending after staging a reset, want 1", state.PendingCount())
	}

	// It appears in the list as itself, and the ✕ drops it like anything else.
	entries := allEntries(state.pendingSections())
	if len(entries) != 1 || !strings.Contains(entries[0].Label, "Reset") {
		t.Fatalf("the pending list shows %+v, want the reset", entries)
	}
	entries[0].Drop()
	if state.resetStaged() || state.PendingCount() != 0 {
		t.Error("the ✕ did not drop the staged reset")
	}
}
