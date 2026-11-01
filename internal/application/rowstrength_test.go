package application_test

// The game's own summon rows: a role chosen for one, written and read back
// through a file, and a reset that puts it back.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// A row the game ships is never cloned, so a role for it is the row's: it is
// staged on the row, written, and read back from the file, and a reset makes it
// a Helper again - here the only thing the reset has to put back. Neither a
// Fighter slot on such a row nor a role for a row the game does not ship is
// taken.
func TestTheGamesOwnRowTakesARoleAndAResetPutsItBack(t *testing.T) {
	t.Parallel()
	snapshot, err := reference.Vanilla()
	if err != nil {
		t.Fatal(err)
	}
	row := snapshot.NpcCloneRows[0]
	session := loadedSession(t, writeVanilla(t))
	if _, err := session.StageFormation(authoringHost, []application.AuthoredSlot{{Npc: row, Fighter: true}}); !errors.Is(err, application.ErrHeldRowStrength) {
		t.Errorf("a Fighter slot on the game's own row: %v, want ErrHeldRowStrength", err)
	}
	if err := session.SetRowFighter(savefixture.CloneableWorldActors[0], true); !errors.Is(err, application.ErrHeldRowStrength) {
		t.Errorf("a role for a world row: %v, want ErrHeldRowStrength", err)
	}
	if fighter, choosable := session.RowFighter(row); fighter || !choosable {
		t.Fatalf("row %d reads Fighter %t, choosable %t; want a Helper the player may change", row, fighter, choosable)
	}

	if err := session.SetRowFighter(row, true); err != nil {
		t.Fatal(err)
	}
	if staged := session.Staged().Strength; staged == nil || staged.Fighters != 1 {
		t.Fatalf("the role staged %+v, want one row to Fighter", staged)
	}
	fighterPath := filepath.Join(t.TempDir(), "fighter.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), fighterPath, ""); err != nil {
		t.Fatalf("writing the Fighter: %v", err)
	}

	reopened := loadedSession(t, fighterPath)
	if fighter, _ := reopened.RowFighter(row); !fighter {
		t.Fatal("the row written as a Fighter reads back as a Helper")
	}
	if count := reopened.PendingCount(); count != 0 {
		t.Errorf("reopening the save staged %d changes", count)
	}
	preview, err := newService(t).PreviewReset(context.Background(), application.Request{InputPath: fighterPath})
	if err != nil || preview.AlreadyVanilla || preview.SummonRows != 1 {
		t.Errorf("the reset preview is %+v (%v), want the one row to put back", preview, err)
	}
	reset, err := reopened.StageReset(context.Background())
	if err != nil {
		t.Fatalf("a reset of a save whose one change is the row: %v", err)
	}
	if reset.SummonRows != 1 {
		t.Errorf("the reset puts back %d summon rows, want 1", reset.SummonRows)
	}
	resetPath := filepath.Join(t.TempDir(), "reset.dat")
	if _, err := reopened.Commit(context.Background(), mustPrepare(t, reopened), resetPath, ""); err != nil {
		t.Fatalf("writing the reset: %v", err)
	}
	if fighter, _ := loadedSession(t, resetPath).RowFighter(row); fighter {
		t.Error("the reset left the row a Fighter")
	}
}
