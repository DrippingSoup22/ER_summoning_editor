package application_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// The same creature in two Ashes (the widest
// freedom the product can give). Its summon-side clone is one row, so the
// first formation inserts it and the second names it - and the save is the one
// a player would get by authoring the two Ashes one after the other. Until
// that day the commit refused it: each formation compiled its own insertion of
// the same row.
func TestTwoAshesSummoningTheSameCreatureShareItsClone(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	session := loadedSession(t, input)
	first, err := session.StageFormation(authoringHost, []application.AuthoredSlot{worldActorSlot(), worldActorSlot()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.StageFormation("Lone Wolf Ashes", []application.AuthoredSlot{worldActorSlot()})
	if err != nil {
		t.Fatal(err)
	}
	commit := mustPrepare(t, session)
	// Each formation alone inserts the actor's two clone rows; together they
	// still insert two, not four.
	alone := func(host string, slots []application.AuthoredSlot) int {
		lone := loadedSession(t, input)
		if _, err := lone.StageFormation(host, slots); err != nil {
			t.Fatal(err)
		}
		return mustPrepare(t, lone).Summary().RowsInserted
	}
	want := alone(authoringHost, []application.AuthoredSlot{worldActorSlot(), worldActorSlot()}) +
		alone("Lone Wolf Ashes", []application.AuthoredSlot{worldActorSlot()}) - 2
	if got := commit.Summary().RowsInserted; got != want {
		t.Errorf("rows inserted = %d, want %d - the shared clone counted once", got, want)
	}

	output := filepath.Join(t.TempDir(), "shared.dat")
	if _, err := session.Commit(context.Background(), commit, output, ""); err != nil {
		t.Fatal(err)
	}
	triggers := []int32{first.Formation.TriggerSpEffectID, second.Formation.TriggerSpEffectID}
	written, err := savecore.ReadFormations(context.Background(), output, triggers)
	if err != nil {
		t.Fatal(err)
	}
	clone := savefixture.CloneableWorldActors[0] + formation.CloneOffset
	if !written.NpcRows[clone] || !written.NpcThinkRows[clone] {
		t.Fatalf("the shared clone %d is not in the save", clone)
	}
	for _, trigger := range triggers {
		for _, row := range written.Blocks[trigger].Rows {
			if row.Npc != clone || row.Think != clone {
				t.Errorf("block %d row %d names %d/%d, want the shared clone %d", trigger, row.RowID, row.Npc, row.Think, clone)
			}
		}
	}
}

// Reopening a save this product wrote: an unchanged creature names the clone
// it already has and inserts nothing, and a creature whose level changed gets
// the new level - on a clone of its own, since the old one is still the old
// recipe, rather than reusing a clone the save holds whatever it was written
// as.
func TestReopeningASaveReusesWhatIsTheSameAndWritesWhatChanged(t *testing.T) {
	t.Parallel()
	first := loadedSession(t, writeVanilla(t))
	if _, err := first.StageFormation(authoringHost, []application.AuthoredSlot{worldActorSlot()}); err != nil {
		t.Fatal(err)
	}
	once := filepath.Join(t.TempDir(), "once.dat")
	if _, err := first.Commit(context.Background(), mustPrepare(t, first), once, ""); err != nil {
		t.Fatal(err)
	}
	actor := savefixture.CloneableWorldActors[0]

	same := loadedSession(t, once)
	staged, err := same.StageFormation(authoringHost, []application.AuthoredSlot{worldActorSlot()})
	if err != nil {
		t.Fatal(err)
	}
	for _, clone := range staged.Formation.Clones {
		if clone.InsertsNpc || clone.InsertsThink {
			t.Errorf("an unchanged creature inserted a clone again: %+v", clone)
		}
	}
	if got := staged.Formation.Slots[0].Npc; got != actor+formation.CloneOffset {
		t.Errorf("the unchanged creature summons %d, want its clone %d", got, actor+formation.CloneOffset)
	}

	// Changed to a Guard, the creature needs a new recipe; the Follower's rows
	// are then named by nothing, so Save removes them and the Guard takes their
	// number (the file is written from the whole configuration).
	changed := loadedSession(t, once)
	twice := reviewCommit(t, changed, "twice.dat", map[string][]application.AuthoredSlot{
		authoringHost: {levelSlot(application.PerceptionGuard)},
	})
	rows := creatureRows(t, twice, actor)
	if fmt.Sprint(rows.npc) != "[1]" || fmt.Sprint(rows.think) != "[1]" {
		t.Errorf("after the change the creature holds npc bands %v and think bands %v, want its one Guard in band 1", rows.npc, rows.think)
	}
	if got := slotRows(t, twice, authoringTrigger); got[0] != [2]int32{bandRow(t, actor, 1), bandRow(t, actor, 1)} {
		t.Errorf("the Guard is summoned through %v, want band 1", got[0])
	}
	// And band 1 now IS the Guard: staged again, it is reused as it stands.
	again := loadedSession(t, twice)
	staged, err = again.StageFormation(authoringHost, []application.AuthoredSlot{levelSlot(application.PerceptionGuard)})
	if err != nil {
		t.Fatal(err)
	}
	for _, clone := range staged.Formation.Clones {
		if clone.InsertsNpc || clone.InsertsThink {
			t.Errorf("the Guard written in band 1 was not recognised: %+v", clone)
		}
	}
}

// A reset takes a higher-band clone back out like any clone this product
// inserted: the shipped record names none in any band.
func TestAResetRemovesAHigherBandClone(t *testing.T) {
	t.Parallel()
	session := loadedSession(t, writeVanilla(t))
	guard := worldActorSlot()
	guard.Perception = application.PerceptionGuard
	guard.Stance, _ = application.LevelStance(guard)
	if _, err := session.StageFormation(authoringHost, []application.AuthoredSlot{worldActorSlot(), guard}); err != nil {
		t.Fatal(err)
	}
	authored := filepath.Join(t.TempDir(), "banded.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), authored, ""); err != nil {
		t.Fatal(err)
	}
	bandTwo, _ := formation.CloneKey(savefixture.CloneableWorldActors[0], 2)
	row := bandTwo + formation.CloneOffset
	before, err := savecore.ReadFormations(context.Background(), authored, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.NpcThinkRows[row] {
		t.Fatalf("the Guard's band-2 think clone %d was not written", row)
	}
	reset := loadedSession(t, authored)
	if _, err := reset.StageReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored.dat")
	if _, err := reset.Commit(context.Background(), mustPrepare(t, reset), restored, ""); err != nil {
		t.Fatal(err)
	}
	after, err := savecore.ReadFormations(context.Background(), restored, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.NpcThinkRows[row] || after.NpcRows[savefixture.CloneableWorldActors[0]+formation.CloneOffset] {
		t.Errorf("the reset left a clone behind (band 2 %v)", after.NpcThinkRows[row])
	}
}
