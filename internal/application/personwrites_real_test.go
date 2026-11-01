package application_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// The NPC review: an armed person's loadout is written
// into its own CharaInitParam row while an Ash holds the person, and the
// shipped values come back once none does - the shared write's rule. Played
// on the real regulation with Irina of Morne (23100, Placidusax's Ruin and
// the Faith it asks) on Nepheli's person shell.
func TestAPersonWriteIsPresentOnlyWhileAnAshHoldsThePerson(t *testing.T) {
	t.Parallel()
	const irina, shell, godrickSoldier = int32(23100), int32(100000020), int32(43110000)
	input := restorableSavePath(t)
	directory := t.TempDir()
	service := newService(t)
	session := service.NewSession()
	if err := session.Load(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	expect := func(path string, armed bool) {
		t.Helper()
		state, err := savecore.ReadState(context.Background(), path, nil)
		if err != nil {
			t.Fatal(err)
		}
		seen := 0
		for _, write := range savecore.PersonWrites() {
			want := write.Shipped
			if armed && write.Row == irina {
				want, seen = write.Written, seen+1
			}
			if got := state.PersonWrites[savecore.PersonKey{Row: write.Row, Field: write.Field}]; got != want {
				t.Errorf("%s: %d.%s = %g, want %g", filepath.Base(path), write.Row, write.Field, got, want)
			}
		}
		if armed && seen == 0 {
			t.Fatal("the surface holds no write for Irina")
		}
	}
	save := func(name string, slots ...application.AuthoredSlot) (string, *application.CommitPlan) {
		t.Helper()
		if _, err := session.StageFormation(authoringHost, slots); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, name)
		commit := mustPrepare(t, session)
		if _, err := session.Commit(context.Background(), commit, path, ""); err != nil {
			t.Fatal(err)
		}
		return path, commit
	}
	person := application.AuthoredSlot{Npc: shell, Think: shell, Person: irina}
	creature := application.AuthoredSlot{Npc: godrickSoldier}
	expect(input, false)

	with, first := save("with.dat", person)
	expect(with, true)
	if !bytes.Contains(first.Manifest(), []byte(`"person_patches"`)) {
		t.Error("the commit that summons Irina carries no person write")
	}
	still, second := save("still.dat", person, creature)
	expect(still, true)
	if bytes.Contains(second.Manifest(), []byte(`"person_patches"`)) {
		t.Error("a save already saying the right thing is written again")
	}
	without, _ := save("without.dat", creature)
	expect(without, false)

	again, _ := save("again.dat", person)
	expect(again, true)
	if _, err := session.StageReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	reset := filepath.Join(directory, "reset.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), reset, ""); err != nil {
		t.Fatal(err)
	}
	expect(reset, false)
}
