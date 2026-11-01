package application_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// The fixture proves the machinery; this proves the product. The shipped
// regulation has 924 goods rows, 170 formation rows and 304 stone rows, and a
// commit that carries two operations at once has to be right about all of them.
func realSavePath(t *testing.T) string {
	t.Helper()
	path := os.Getenv("ER_SAVE")
	if path == "" {
		t.Skip("ER_SAVE not set")
	}
	return path
}

// restorableSavePath is the real save for a test that resets it.
func restorableSavePath(t *testing.T) string {
	t.Helper()
	return realSavePath(t)
}

// Reset restores the snapshot's values on every supported version, so the
// snapshot itself must be of one.
func TestTheVanillaSnapshotIsOfASupportedVersion(t *testing.T) {
	snapshot, err := reference.Vanilla()
	if err != nil {
		t.Fatal(err)
	}
	if !application.RegulationSupported(snapshot.RegulationVersion) {
		t.Fatalf("the snapshot is regulation %d; this build supports %s", snapshot.RegulationVersion, application.SupportedRegulations())
	}
}

// A real editing sitting, start to finish: open a save, make three unrelated
// changes, save once - then open the result, ask for a reset, and save again.
// One of them is the Giant Rat, whose levels reach three blocks: what the
// player placed must be what every level summons, and the reset must take all
// three back.
//
// The two halves are one test because the second is what proves the first did
// not leave anything behind. A commit that carried two operations has to be
// undoable by one reset, or the editor can produce a save it cannot take back.
func TestASessionEditsAndRestoresTheShippedRegulation(t *testing.T) {
	t.Parallel()
	input := restorableSavePath(t)
	directory := t.TempDir()
	service := newService(t)

	session := service.NewSession()
	if err := session.Load(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if !session.Save().Supported {
		t.Fatalf("regulation %d is not the supported one", session.Save().RegulationVersion)
	}
	// Godrick Soldier: an ordinary world actor with no summon-side twin, so
	// authoring it exercises the clone path rather than only rewriting values.
	// Research seen this actor summoned through an inserted clone.
	const godrickSoldier = int32(43110000)

	if _, err := session.StageConversion("Lone Wolf Ashes", "Bloodfiend Hexer's Ashes"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.StageFormation(authoringHost, []application.AuthoredSlot{
		{Npc: godrickSoldier}, {Npc: godrickSoldier}, {Npc: godrickSoldier},
		{Npc: godrickSoldier}, {Npc: godrickSoldier},
	}); err != nil {
		t.Fatal(err)
	}
	const rats = 6
	ratSlots := make([]application.AuthoredSlot, rats)
	for index := range ratSlots {
		ratSlots[index] = application.AuthoredSlot{Npc: godrickSoldier}
	}
	if _, err := session.StageFormation("Giant Rat Ashes", ratSlots); err != nil {
		t.Fatal(err)
	}
	ratLevels := map[int32][]int{233000: {0, 1, 2, 3, 4}, 233100: {5, 6, 7, 8, 9}, 233200: {10}}

	edited := filepath.Join(directory, "edited.dat")
	commit := mustPrepare(t, session)
	if _, err := session.Commit(context.Background(), commit, edited, ""); err != nil {
		t.Fatal(err)
	}
	// The input is what it was. Every other guarantee is worth nothing without
	// this one.
	if after, err := savecore.Inspect(context.Background(), input); err != nil {
		t.Fatal(err)
	} else if after.SHA256 != commit.Summary().Save.SHA256 {
		t.Fatal("the input save changed")
	}

	// Both edits are in the one file: the converted Ash delivers another
	// family's formation, and the authored Ash's own block grew to five.
	state, err := savecore.ReadState(context.Background(), edited, []int32{232000})
	if err != nil {
		t.Fatal(err)
	}
	if state.Goods[232000].Trigger == 232000 {
		t.Error("the conversion did not reach the committed save")
	}
	formations, err := savecore.ReadFormations(context.Background(), edited, []int32{authoringTrigger})
	if err != nil {
		t.Fatal(err)
	}
	block := formations.Blocks[authoringTrigger]
	if len(block.Rows) != 5 {
		t.Fatalf("authored block has %d rows, want 5", len(block.Rows))
	}
	for _, row := range block.Rows {
		if row.Npc != godrickSoldier+100000000 {
			t.Fatalf("row %d summons %d, want the inserted clone", row.RowID, row.Npc)
		}
	}
	ratBlocks, err := savecore.ReadFormations(context.Background(), edited, []int32{233000, 233100, 233200})
	if err != nil {
		t.Fatal(err)
	}
	for trigger, levels := range ratLevels {
		block := ratBlocks.Blocks[trigger]
		if len(block.Rows) != rats {
			t.Errorf("Giant Rat block %d has %d rows, want the %d placed", trigger, len(block.Rows), rats)
		}
		for _, row := range block.Rows {
			if row.Npc != godrickSoldier+100000000 {
				t.Errorf("Giant Rat row %d summons %d, want the inserted clone", row.RowID, row.Npc)
			}
			for _, level := range levels {
				if row.Doping[level] <= 0 {
					t.Errorf("Giant Rat row %d has no reinforcement at +%d, a level that reaches it", row.RowID, level)
				}
			}
		}
	}

	// The session is now editing the file it wrote, so the reset below is
	// staged against that rather than against the original.
	if session.Save().Path != edited {
		t.Fatalf("session holds %s after the commit", session.Save().Path)
	}
	restoration, err := session.StageReset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if restoration.RowsRemoved == 0 {
		t.Error("the reset would remove none of the rows the commit added")
	}
	restored := filepath.Join(directory, "restored.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), restored, ""); err != nil {
		t.Fatal(err)
	}

	// A save this editor has taken back to shipped behaviour has nothing left
	// to restore, which is the only claim a reset can be checked against
	// without a second copy of the regulation.
	back := service.NewSession()
	if err := back.Load(context.Background(), restored); err != nil {
		t.Fatal(err)
	}
	if _, err := back.StageReset(context.Background()); err == nil {
		t.Fatal("a restored save still had something to reset")
	}
	final, err := savecore.ReadFormations(context.Background(), restored, []int32{authoringTrigger})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(final.Blocks[authoringTrigger].Rows); got != authoringRows {
		t.Errorf("restored block has %d rows, want the %d the game ships", got, authoringRows)
	}
}
