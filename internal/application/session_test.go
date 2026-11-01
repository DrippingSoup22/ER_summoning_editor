package application_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// A second world actor, so a re-authored block can be told from the one it
// replaced. Re-authoring the identical formation writes no bytes at all, which
// is correct and proves nothing.
func otherActorSlots(count int) []application.AuthoredSlot {
	slots := make([]application.AuthoredSlot, 0, count)
	for index := 0; index < count; index++ {
		slots = append(slots, application.AuthoredSlot{Npc: savefixture.CloneableWorldActors[1]})
	}
	return slots
}

func loadedSession(t *testing.T, path string) *application.Session {
	t.Helper()
	session := newService(t).NewSession()
	if err := session.Load(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	return session
}

func mustStageFormation(t *testing.T, session *application.Session, host string, slots int) *application.StagedFormation {
	t.Helper()
	staged, err := session.StageFormation(host, slotsOf(slots))
	if err != nil {
		t.Fatal(err)
	}
	return staged
}

func mustPrepare(t *testing.T, session *application.Session) *application.CommitPlan {
	t.Helper()
	commit, err := session.PrepareCommit()
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

// The workflow G2 exists to make possible: several unrelated edits, one Save,
// one file.
//
// Before this, converting one Ash and authoring another meant two runs of the
// tool and two output files, because each of the three operations refused the
// other two. What the player did was one edit and what the product produced was
// a chain of saves.
func TestOneCommitCarriesSeveralUnrelatedEdits(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	session := loadedSession(t, input)

	if _, err := session.StageConversion("Lone Wolf Ashes", "Giant Rat Ashes"); err != nil {
		t.Fatal(err)
	}
	mustStageFormation(t, session, authoringHost, 5)
	if session.PendingCount() != 2 {
		t.Fatalf("pending = %d, want 2", session.PendingCount())
	}

	commit := mustPrepare(t, session)
	summary := commit.Summary()
	if len(summary.Conversions) != 1 || len(summary.Formations) != 1 {
		t.Fatalf("summary carries %d conversions and %d formations",
			len(summary.Conversions), len(summary.Formations))
	}
	// The conversion's eleven goods rows, and no others: authoring writes no
	// goods row at all, which is what keeps it structurally unable to reach a
	// multiplayer field.
	if summary.GoodsRows != 11 {
		t.Errorf("goods rows = %d, want 11", summary.GoodsRows)
	}
	// Sixteen of the conversion's twenty monument-origin rows, because the four
	// in the 215000 block are written by the authored formation instead, plus
	// that formation's own five rows.
	if summary.BuddyRows != 16+5 {
		t.Errorf("buddy rows = %d, want 21", summary.BuddyRows)
	}
	if summary.RowsInserted == 0 {
		t.Error("a fifth slot in a four-row block inserted nothing")
	}

	output := filepath.Join(t.TempDir(), "committed.dat")
	result, err := session.Commit(context.Background(), commit, output, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.OutputPath != output {
		t.Fatalf("wrote %s, want %s", result.OutputPath, output)
	}

	// Both edits are in the one file. The converted Ash delivers the other
	// family's formation, and the authored Ash's own block summons five.
	state, err := savecore.ReadState(context.Background(), output, []int32{232000})
	if err != nil {
		t.Fatal(err)
	}
	if state.Goods[232000].Trigger == 232000 {
		t.Error("the conversion did not reach the committed save")
	}
	formations, err := savecore.ReadFormations(context.Background(), output, []int32{authoringTrigger})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(formations.Blocks[authoringTrigger].Rows); got != 5 {
		t.Errorf("authored block has %d rows, want 5", got)
	}

	// And a reader that is not savecore agrees, which is the obligation every
	// layer of this product carries: a write proved only against its own
	// encoder is not proved. The row count is the structural half and the
	// trigger is the value half, so one commit's two operations are both
	// visible from outside.
	independent := independentReader(t, output)
	rows, err := independent.RowIDs("BuddyParam.param")
	if err != nil {
		t.Fatal(err)
	}
	inBlock := 0
	for _, rowID := range rows {
		if rowID >= authoringTrigger*100 && rowID < authoringTrigger*100+100 {
			inBlock++
		}
	}
	if inBlock != 5 {
		t.Errorf("an independent reader sees %d rows in the authored block, want 5", inBlock)
	}
	trigger, _, err := independent.GoodsTrigger(232000)
	if err != nil {
		t.Fatal(err)
	}
	if trigger == 232000 {
		t.Error("an independent reader does not see the conversion")
	}
}

// independentReader opens a written save with the fixture's own container
// implementation, which shares no code with the one that wrote it.
func independentReader(t *testing.T, path string) *savefixture.Reader {
	t.Helper()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := savefixture.Open(written)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

// The rule the whole session exists for: staging is not writing.
func TestStagingWritesNothingUntilCommit(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	before, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(input)
	listedBefore, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}

	session := loadedSession(t, input)
	if _, err := session.StageConversion("Lone Wolf Ashes", "Giant Rat Ashes"); err != nil {
		t.Fatal(err)
	}
	mustStageFormation(t, session, authoringHost, 3)
	mustPrepare(t, session)

	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("staging changed the loaded save")
	}
	listedAfter, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(listedAfter) != len(listedBefore) {
		t.Errorf("staging left %d files behind", len(listedAfter)-len(listedBefore))
	}
}

// A reset and an edit are not a contradiction: the reset means "put back
// everything I have not said otherwise about". This is the case the plan calls
// the formation winning where the two meet.
func TestAnEditStagedBesideAResetKeepsWhatItNames(t *testing.T) {
	t.Parallel()
	service := newService(t)
	input := writeVanilla(t)
	directory := t.TempDir()
	authored := filepath.Join(directory, "authored.dat")
	if _, err := service.Author(context.Background(), authoringRequest(input, authored, 6)); err != nil {
		t.Fatal(err)
	}

	session := service.NewSession()
	if err := session.Load(context.Background(), authored); err != nil {
		t.Fatal(err)
	}
	// Reset the save, and in the same breath re-author the very block the reset
	// would take the added rows back out of - with a different actor, so that
	// what survives can be told from what was there.
	if _, err := session.StageReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := session.StageFormation(authoringHost, otherActorSlots(6)); err != nil {
		t.Fatal(err)
	}

	commit := mustPrepare(t, session)
	if commit.Summary().RestorationOverridden == 0 {
		t.Error("the reset gave up nothing to the formation staged beside it")
	}
	// It gave up only what the formation named. The clone the previous
	// formation used is not needed by this one, so the reset still takes it
	// back out - a reset beside an edit is still a reset everywhere else.
	if commit.Summary().RowsRemoved == 0 {
		t.Error("the reset removed nothing at all beside the formation")
	}

	output := filepath.Join(directory, "both.dat")
	if _, err := session.Commit(context.Background(), commit, output, ""); err != nil {
		t.Fatal(err)
	}
	// The block the player re-authored still summons six. A reset on its own
	// would have taken it back to the four the game shipped.
	formations, err := savecore.ReadFormations(context.Background(), output, []int32{authoringTrigger})
	if err != nil {
		t.Fatal(err)
	}
	block := formations.Blocks[authoringTrigger]
	if got := len(block.Rows); got != 6 {
		t.Errorf("authored block has %d rows, want the 6 the formation asked for", got)
	}
	wanted := savefixture.CloneableWorldActors[1] + formation.CloneOffset
	for _, row := range block.Rows {
		if row.Npc != wanted {
			t.Fatalf("row %d summons %d, want the re-authored %d", row.RowID, row.Npc, wanted)
		}
	}
}

// The reason PrepareCommit and Commit are separate calls. A native Save As
// dialog is modal to the operating system and not to the application, so a plan
// built before it opens must not notice anything staged while it is up.
func TestAPreparedCommitDoesNotSeeLaterStaging(t *testing.T) {
	t.Parallel()
	session := loadedSession(t, writeVanilla(t))
	mustStageFormation(t, session, authoringHost, 2)

	commit := mustPrepare(t, session)
	frozen := commit.Manifest()

	if _, err := session.StageConversion("Lone Wolf Ashes", "Giant Rat Ashes"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frozen, commit.Manifest()) {
		t.Error("a prepared plan changed when something else was staged")
	}
	if len(commit.Summary().Conversions) != 0 {
		t.Error("a prepared plan gained the conversion staged after it")
	}
}

// After a save the player is editing the file they just made, not the one they
// started from. Anything else shows unedited data with nothing pending, which
// reads as the edit having been lost.
func TestCommitAdvancesTheSessionToTheOutput(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	session := loadedSession(t, input)
	mustStageFormation(t, session, authoringHost, 5)

	output := filepath.Join(t.TempDir(), "saved.dat")
	result, err := session.Commit(context.Background(), mustPrepare(t, session), output, "")
	if err != nil {
		t.Fatal(err)
	}
	if session.Save().Path != output {
		t.Errorf("session still holds %s", session.Save().Path)
	}
	if session.Save().SHA256 != result.OutputSHA256 {
		t.Error("the session did not adopt the written save")
	}
	if session.PendingCount() != 0 {
		t.Errorf("pending = %d after a successful save", session.PendingCount())
	}

	// And the block it now reads is the one that was just written.
	state, err := session.AshState(authoringHost)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Slots) != 5 {
		t.Errorf("reloaded block has %d rows, want 5", len(state.Slots))
	}
}

// A plan names the save it was made for, so it cannot be applied to another one
// the session has since loaded.
func TestAPlanCannotBeCommittedAgainstADifferentSave(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	first := writeVanilla(t)
	session := loadedSession(t, first)
	mustStageFormation(t, session, authoringHost, 2)
	stale := mustPrepare(t, session)

	second := filepath.Join(directory, "other.dat")
	if _, err := session.Commit(context.Background(), stale, second, ""); err != nil {
		t.Fatal(err)
	}
	// The session now holds the file it just wrote, and the old plan belongs to
	// the one before it.
	third := filepath.Join(directory, "third.dat")
	_, err := session.Commit(context.Background(), stale, third, "")
	if !errors.Is(err, application.ErrSourceMismatch) {
		t.Fatalf("err = %v, want ErrSourceMismatch", err)
	}
	if _, statErr := os.Stat(third); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a refused commit wrote a file anyway")
	}
}

// A formation reopened and staged again as the save holds it - what the
// creature window does when anything in its Ash moves - keeps the clones its
// slots name: Save does not prune them from under it.
func TestAReopenedFormationStagedAsHeldKeepsItsClones(t *testing.T) {
	t.Parallel()
	session := loadedSession(t, writeVanilla(t))
	mustStageFormation(t, session, authoringHost, 2)
	written := filepath.Join(t.TempDir(), "written.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), written, ""); err != nil {
		t.Fatal(err)
	}
	reopened := loadedSession(t, written)
	host, err := reopened.AshState(authoringHost)
	if err != nil {
		t.Fatal(err)
	}
	held := make([]application.AuthoredSlot, 0, len(host.Slots))
	for _, slot := range host.Slots {
		held = append(held, application.AuthoredSlot{Npc: slot.Npc, Think: slot.Think, Stance: slot.Stance,
			Perception: application.PerceptionAsHeld})
	}
	if _, err := reopened.StageFormation(authoringHost, held); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.PrepareCommit(); err != nil {
		t.Errorf("saving the formation as held: %v", err)
	}
}
