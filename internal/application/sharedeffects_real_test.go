package application_test

import (
	"bytes"
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// The rule for the entries that change the world: the
// write is there while an Ash holds one of them, and gone once none does.
// Played on the real regulation: summon the combined Ancestor, save; save
// again with him still there (nothing to write); take him out, save; put him
// back and reset.
func TestASharedWriteIsPresentOnlyWhileAnAshHoldsItsEntry(t *testing.T) {
	t.Parallel()
	const ancestor, godrickSoldier = int32(46700200), int32(43110000)
	input := restorableSavePath(t)
	directory := t.TempDir()
	service := newService(t)
	session := service.NewSession()
	if err := session.Load(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	fields := func(path string) map[savecore.SharedEffectKey]float64 {
		t.Helper()
		state, err := savecore.ReadState(context.Background(), path, nil)
		if err != nil {
			t.Fatal(err)
		}
		return state.SharedEffects
	}
	expect := func(path string, ancestorWritten bool) {
		t.Helper()
		held := fields(path)
		for _, write := range savecore.SharedEffectWrites() {
			want := write.Shipped
			if ancestorWritten && (write.Row == 13615 || write.Row == 13616) {
				want = write.Written
			}
			got := held[savecore.SharedEffectKey{Row: write.Row, Field: write.Field}]
			if math.Float32bits(float32(got)) != math.Float32bits(float32(want)) {
				t.Errorf("%s: %d.%s = %g, want %g", filepath.Base(path), write.Row, write.Field, got, want)
			}
		}
	}
	host := authoringHost
	save := func(name string, slots ...int32) (string, *application.CommitPlan) {
		t.Helper()
		authored := make([]application.AuthoredSlot, len(slots))
		for index, npc := range slots {
			authored[index] = application.AuthoredSlot{Npc: npc}
		}
		if _, err := session.StageFormation(host, authored); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, name)
		commit := mustPrepare(t, session)
		if _, err := session.Commit(context.Background(), commit, path, ""); err != nil {
			t.Fatal(err)
		}
		return path, commit
	}
	expect(input, false)

	with, first := save("with.dat", ancestor)
	expect(with, true)
	if !bytes.Contains(first.Manifest(), []byte(`"effect_patches"`)) {
		t.Error("the commit that summons the Ancestor carries no shared write")
	}
	// Rykard's fields are not the Ancestor's business.
	still, second := save("still.dat", ancestor, godrickSoldier)
	expect(still, true)
	if bytes.Contains(second.Manifest(), []byte(`"effect_patches"`)) {
		t.Error("a save already saying the right thing is written again")
	}
	without, _ := save("without.dat", godrickSoldier)
	expect(without, false)

	again, _ := save("again.dat", ancestor)
	expect(again, true)
	// Another Ash edited while the Ancestor stays where he is: still written.
	host = "Lone Wolf Ashes"
	other, _ := save("other.dat", godrickSoldier)
	expect(other, true)
	if _, err := session.StageReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	reset := filepath.Join(directory, "reset.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), reset, ""); err != nil {
		t.Fatal(err)
	}
	expect(reset, false)
}
