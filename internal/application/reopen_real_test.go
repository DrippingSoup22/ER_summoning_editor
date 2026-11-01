package application_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// A save edited every way the product writes, the way a player comes back to
// it: saved, reopened, edited again, and reset. Reopened it holds nothing
// pending and reads back as written; a second edit leaves the first one's
// rows as they are; and the reset gives back the baseline's regulation byte
// for byte. The edits: a conversion, a price, a role for one of the game's own
// rows, summoning anywhere, and one formation of ten -
// Helper, Fighter, a Guard sharing aggro, an enemy, a creature in its own
// colours (an effect row of our own), the Commoner on the quiet marker, the
// combined Ancestor (a shared effect write), Irina armed (a person write), a
// character from a slot and a built one.
func TestASaveEditedEveryWayReopensEditsAgainAndResetsToTheBaseline(t *testing.T) {
	t.Parallel()
	const godrickSoldier, irina, shell = int32(43110000), int32(23100), int32(100000020)
	input := restorableSavePath(t)
	directory := t.TempDir()
	service := newService(t)
	ctx := context.Background()
	snapshot, err := reference.Vanilla()
	if err != nil {
		t.Fatal(err)
	}
	shippedRow := snapshot.NpcCloneRows[0]

	session := service.NewSession()
	if err := session.Load(ctx, input); err != nil {
		t.Fatal(err)
	}
	categories, err := session.Hierarchy()
	if err != nil {
		t.Fatal(err)
	}
	var characters []application.HierarchyEntry
	for _, category := range categories {
		for _, card := range category.Cards {
			if card.Card == "characters" {
				characters = card.Types[0].Entries
			}
		}
	}
	if len(characters) < 4 {
		t.Fatalf("the baseline offers %d characters", len(characters))
	}
	body, think, _, err := application.PersonShell(characters[3])
	if err != nil {
		t.Fatal(err)
	}
	staged := func(slot application.AuthoredSlot) application.AuthoredSlot {
		slot.Stance, _ = application.LevelStance(slot)
		return slot
	}
	slots := []application.AuthoredSlot{
		{Npc: godrickSoldier},
		{Npc: godrickSoldier, Fighter: true},
		staged(application.AuthoredSlot{Npc: godrickSoldier, Perception: application.PerceptionGuard, Share: application.ShareOn}),
		staged(application.AuthoredSlot{Npc: godrickSoldier, Perception: application.PerceptionLoner,
			Team: application.TeamEnemy, WorldBranch: true}),
		{Npc: 22000078, Think: 22000000},
		staged(application.AuthoredSlot{Npc: 52410088, Think: 52410000, Perception: application.PerceptionHunter}),
		{Npc: 46700200},
		{Npc: shell, Think: shell, Person: irina},
		{Npc: body, Think: think, Person: characters[3].Ships},
		{Npc: character.BuiltBodyOf(3), Think: shell, Person: character.BuiltRowOf(3)},
	}
	if _, err := session.StageConversion("Lone Wolf Ashes", "Bloodfiend Hexer's Ashes"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.StageCost("Godrick Soldier Ashes", plan.Price{FP: 3}); err != nil {
		t.Fatal(err)
	}
	if err := session.SetRowFighter(shippedRow, true); err != nil {
		t.Fatal(err)
	}
	if err := session.SetWideArea(true); err != nil {
		t.Fatal(err)
	}
	if err := session.SetNoRestLimit(true); err != nil {
		t.Fatal(err)
	}
	if _, err := session.StageBuild(3, character.Plain(false)); err != nil {
		t.Fatal(err)
	}
	if _, err := session.StageFormation(authoringHost, slots); err != nil {
		t.Fatal(err)
	}
	commit := func(session *application.Session, name string) string {
		t.Helper()
		path := filepath.Join(directory, name)
		if _, err := session.Commit(ctx, mustPrepare(t, session), path, ""); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
		return path
	}
	edited := commit(session, "edited.dat")

	// Reopened, it reads back as written and stages nothing.
	reopened := loadedSession(t, edited)
	if count := reopened.PendingCount(); count != 0 {
		t.Errorf("reopening the edited save staged %d changes", count)
	}
	states, err := reopened.AshStates()
	if err != nil {
		t.Fatal(err)
	}
	var held []application.SlotState
	for _, state := range states {
		switch state.Name {
		case "Lone Wolf Ashes":
			if !state.RePointed {
				t.Error("the conversion does not read back")
			}
		case "Godrick Soldier Ashes":
			if state.Price.FP != 3 {
				t.Errorf("the price reads back as %+v", state.Price)
			}
		case authoringHost:
			held = state.Slots
		}
	}
	if len(held) != len(slots) {
		t.Errorf("the formation reads back with %d slots, want %d", len(held), len(slots))
	}
	if fighter, _ := reopened.RowFighter(shippedRow); !fighter {
		t.Error("the game's own row reads back as a Helper")
	}

	// Edited again: the formation re-staged as held, as the window does when
	// anything in its Ash moves, and a price on another Ash. Only the price is
	// new; every row the first save wrote stays as it is.
	again := make([]application.AuthoredSlot, len(held))
	for index, slot := range held {
		place := slot.Place
		again[index] = application.AuthoredSlot{Npc: slot.Npc, Think: slot.Think, Person: slot.Person,
			Stance: slot.Stance, Placement: &place, Perception: application.PerceptionAsHeld}
	}
	if _, err := reopened.StageFormation(authoringHost, again); err != nil {
		t.Fatalf("re-staging the held formation: %v", err)
	}
	if _, err := reopened.StageCost("Banished Knight Oleg", plan.Price{FP: 7}); err != nil {
		t.Fatal(err)
	}
	second := commit(reopened, "second.dat")
	for _, entry := range differingEntries(t, edited, second) {
		if entry != "EquipParamGoods.param" {
			t.Errorf("the second edit changed %s", entry)
		}
	}

	// The reset gives back the baseline.
	final := loadedSession(t, second)
	if _, err := final.StageReset(ctx); err != nil {
		t.Fatal(err)
	}
	reset := commit(final, "reset.dat")
	for _, entry := range differingEntries(t, input, reset) {
		t.Errorf("after the reset %s differs from the baseline", entry)
	}
	if _, err := loadedSession(t, reset).StageReset(ctx); !errors.Is(err, application.ErrAlreadyVanilla) {
		t.Errorf("a reset save still has something to reset: %v", err)
	}
}

// differingEntries names the regulation files two saves hold differently.
func differingEntries(t *testing.T, first, second string) []string {
	t.Helper()
	entries := func(path string) map[string][]byte {
		t.Helper()
		loaded, err := regulation.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		listed, err := regulation.ListBND4Entries(loaded.BND4)
		if err != nil {
			t.Fatal(err)
		}
		files := make(map[string][]byte, len(listed))
		for _, entry := range listed {
			files[entry.Name] = loaded.BND4[entry.Offset : entry.Offset+entry.Size]
		}
		return files
	}
	a, b := entries(first), entries(second)
	var differ []string
	for name, data := range a {
		if !bytes.Equal(data, b[name]) {
			differ = append(differ, filepath.Base(name))
		}
	}
	for name := range b {
		if _, held := a[name]; !held {
			differ = append(differ, filepath.Base(name))
		}
	}
	return differ
}
