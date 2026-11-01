package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// THE GAME'S OWN ASHES, AS SHIPPED: an Ashes card put in
// another Ash's container summons FromSoftware's own rows - its creature and
// think row, a person on its own shell with its record - on the stance its Ash
// ships, and writes no actor row at all. FromSoftware's row IS the Follower
// (by the author's choice), so the card is staged on it. Written to a real save and read
// back.
func TestAnAshesCardIsWrittenAsTheGameShipsIt(t *testing.T) {
	t.Parallel()
	input := realSavePath(t)
	service := newService(t)
	session := service.NewSession()
	if err := session.Load(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	before, err := savecore.ReadFormations(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Latenna's archer holds its ground (follow type 2); Dolores is a person on
	// her own shell, 100000050, whose think row is her behaviour.
	// The Putrid Corpse wanders on its own senses (1/1): its Follower shares
	// nothing, as its Ash ships it.
	latenna := application.AuthoredSlot{Npc: 131700000, Think: 131700000, Stance: application.Stance{PcFollowType: 2}}
	corpse := application.AuthoredSlot{Npc: 136610000, Think: 136610000, Share: application.ShareOff,
		Stance: application.Stance{PcFollowType: 1, DisablePCTargetShare: 1}}
	dolores := application.AuthoredSlot{Npc: 100000050, Think: 100000050, Person: 23640}
	staged, err := session.StageFormation("Lone Wolf Ashes", []application.AuthoredSlot{latenna, corpse, dolores})
	if err != nil {
		t.Fatal(err)
	}
	if clones := staged.Formation.Clones; len(clones) != 0 {
		t.Errorf("an Ashes card planned %d clones, want none: %+v", len(clones), clones)
	}
	out := filepath.Join(t.TempDir(), "ashes.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), out, ""); err != nil {
		t.Fatal(err)
	}

	after, err := savecore.ReadFormations(context.Background(), out, nil)
	if err != nil {
		t.Fatal(err)
	}
	for row := range after.NpcRows {
		if !before.NpcRows[row] && formation.InCloneBand(row) {
			t.Errorf("NpcParam gained row %d; an Ashes card writes no actor row", row)
		}
	}
	reopened := service.NewSession()
	if err := reopened.Load(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	host, err := reopened.AshState("Lone Wolf Ashes")
	if err != nil {
		t.Fatal(err)
	}
	if len(host.Slots) != 3 {
		t.Fatalf("Lone Wolf Ashes summons %d, want the three cards", len(host.Slots))
	}
	for at, want := range []application.AuthoredSlot{latenna, corpse, dolores} {
		got := host.Slots[at]
		if got.Npc != want.Npc || got.Think != want.Think || got.Person != want.Person || got.Stance != want.Stance {
			t.Errorf("slot %d reads back %d/%d person %d on %+v, want %d/%d person %d on %+v",
				at+1, got.Npc, got.Think, got.Person, got.Stance, want.Npc, want.Think, want.Person, want.Stance)
		}
	}
}

// Another level on a creature of the game's own Ashes is refused, not written
// silently as the Follower: FromSoftware's row is never written, so a Guard
// needs a second row of the creature, which is not built yet. The row as the
// file holds it (PerceptionAsHeld) is not a level and is staged.
func TestAnotherLevelOnAnAshCreatureIsRefused(t *testing.T) {
	t.Parallel()
	session := newService(t).NewSession()
	if err := session.Load(context.Background(), realSavePath(t)); err != nil {
		t.Fatal(err)
	}
	guard := application.AuthoredSlot{Npc: 131700000, Think: 131700000, Perception: application.PerceptionGuard}
	guard.Stance, _ = application.LevelStance(guard)
	if _, err := session.StageFormation("Lone Wolf Ashes", []application.AuthoredSlot{guard}); !errors.Is(err, application.ErrAshLevelOwed) {
		t.Fatalf("a Guard on Latenna's archer staged with %v, want ErrAshLevelOwed", err)
	}
	asHeld := application.AuthoredSlot{Npc: 131700000, Think: 131700000, Perception: application.PerceptionAsHeld,
		Stance: application.Stance{PcFollowType: 2}}
	if _, err := session.StageFormation("Lone Wolf Ashes", []application.AuthoredSlot{asHeld}); err != nil {
		t.Fatalf("the row as the file holds it was refused: %v", err)
	}
}
