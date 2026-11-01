package application_test

// Staging a grant, and the commit that writes it.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
)

// The fixture save is built from the shipped regulation and carries no
// characters, so anything about slots needs the real one.
func realCharacters(t *testing.T) (*application.Session, []application.CharacterState) {
	t.Helper()
	session := newService(t).NewSession()
	if err := session.Load(context.Background(), realSavePath(t)); err != nil {
		t.Fatal(err)
	}
	people, err := session.Characters()
	if err != nil {
		t.Fatal(err)
	}
	return session, people
}

// unowned is an Ash the character does not have, which is the only kind that
// can be staged.
func unowned(t *testing.T, person application.CharacterState) int32 {
	t.Helper()
	for _, family := range []int32{200000, 201000, 202000, 203000, 204000, 205000, 206000, 207000} {
		if !person.Owns[family] {
			return family
		}
	}
	t.Skip("this character owns every Ash the test knows to try")
	return 0
}

// firstUsed is a character to stage against.
func firstUsed(t *testing.T, people []application.CharacterState) application.CharacterState {
	t.Helper()
	for _, person := range people {
		if person.Used {
			return person
		}
	}
	t.Skip("the save holds no character")
	return application.CharacterState{}
}

// The whole point: a grant alone writes a save, the item is in it, and the
// input is untouched. Nothing in the regulation changes, so this also proves
// the copy path a grants-only commit takes.
func TestCommittingAGrantAloneWritesTheItem(t *testing.T) {
	t.Parallel()
	session, people := realCharacters(t)
	person := firstUsed(t, people)
	family := unowned(t, person)
	input := realSavePath(t)
	before, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := session.StageGrant(person.Slot, family, 0); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "granted.dat")
	commit, err := session.PrepareCommit()
	if err != nil {
		t.Fatalf("preparing a grants-only commit: %v", err)
	}
	result, err := session.Commit(context.Background(), commit, output, "")
	if err != nil {
		t.Fatalf("committing: %v", err)
	}
	if result.ItemsGranted != 1 {
		t.Errorf("%d items granted, want 1 (refused: %v)", result.ItemsGranted, result.ItemsRefused)
	}
	if len(result.ItemsRefused) != 0 {
		t.Errorf("the commit refused %v", result.ItemsRefused)
	}

	// The item is in the written save.
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	got, err := character.Read(written)
	if err != nil {
		t.Fatal(err)
	}
	if !got[person.Slot].Owns(uint32(family)) {
		t.Errorf("%s does not own the Ash the commit granted", person.Name)
	}
	// And nobody else was given anything.
	for _, other := range got {
		if other.Slot == person.Slot || !other.Used {
			continue
		}
		if other.Owns(uint32(family)) && !peopleOwned(before, other.Slot, family) {
			t.Errorf("slot %d gained an item the commit did not grant it", other.Slot)
		}
	}

	// The input is byte-identical, which is the rule that outranks everything.
	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("the save that was opened was modified, which this editor must never do")
	}
}

// peopleOwned answers whether a slot owned an Ash in the file as it was.
func peopleOwned(file []byte, slot int, family int32) bool {
	people, err := character.Read(file)
	if err != nil || slot >= len(people) {
		return false
	}
	return people[slot].Owns(uint32(family))
}

// A grant rides along with a regulation edit rather than replacing it: one
// save, both kinds of change, and the counts for each reported separately.
func TestAGrantCommitsBesideARegulationEdit(t *testing.T) {
	t.Parallel()
	session, people := realCharacters(t)
	person := firstUsed(t, people)
	family := unowned(t, person)

	if _, err := session.StageGrant(person.Slot, family, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := session.StageCost(authoringHost, plan.Price{FP: 42}); err != nil {
		t.Fatal(err)
	}
	if session.PendingCount() != 2 {
		t.Fatalf("%d changes pending, want a grant and a price", session.PendingCount())
	}

	output := filepath.Join(t.TempDir(), "both.dat")
	commit, err := session.PrepareCommit()
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.Commit(context.Background(), commit, output, "")
	if err != nil {
		t.Fatalf("committing both: %v", err)
	}
	if result.ItemsGranted != 1 {
		t.Errorf("%d items granted alongside the price change, want 1 (refused: %v)",
			result.ItemsGranted, result.ItemsRefused)
	}
	if result.GoodsRows == 0 {
		t.Error("the price change wrote no goods rows, so the regulation half did not happen")
	}
	written, _ := os.ReadFile(output)
	got, _ := character.Read(written)
	if !got[person.Slot].Owns(uint32(family)) {
		t.Error("the item is missing from a save whose regulation edit did land")
	}
}

// ownedAsh is an Ash the character does have, which is the only kind whose
// level can be changed.
func ownedAsh(t *testing.T, person application.CharacterState) int32 {
	t.Helper()
	for family := range person.Owns {
		return family
	}
	t.Skip("this character owns no Ash at all")
	return 0
}

// The write side: an upgrade committed alone moves the level in the written
// save, leaves the input alone, and does not hand out a second copy.
func TestCommittingAnUpgradeMovesTheLevel(t *testing.T) {
	t.Parallel()
	session, people := realCharacters(t)
	person := firstUsed(t, people)
	family := ownedAsh(t, person)
	was := person.Levels[family]
	want := (was + 7) % character.AshLevels
	input := realSavePath(t)
	before, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := session.StageGrant(person.Slot, family, want); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "upgraded.dat")
	commit, err := session.PrepareCommit()
	if err != nil {
		t.Fatalf("preparing an upgrade-only commit: %v", err)
	}
	result, err := session.Commit(context.Background(), commit, output, "")
	if err != nil {
		t.Fatalf("committing: %v", err)
	}
	if result.ItemsGranted != 1 || len(result.ItemsRefused) != 0 {
		t.Fatalf("%d written, refused %v", result.ItemsGranted, result.ItemsRefused)
	}

	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	got, err := character.Read(written)
	if err != nil {
		t.Fatal(err)
	}
	level, owned := got[person.Slot].AshLevel(uint32(family))
	if !owned || level != want {
		t.Errorf("%s holds it at +%d (owned=%v), want +%d", person.Name, level, owned, want)
	}
	// One item, not two: the old row is gone rather than left beside the new
	// one, which is the whole difference between an upgrade and a grant.
	if got[person.Slot].Owns(uint32(family)+uint32(was)) && was != want {
		t.Errorf("%s still holds the +%d row as well", person.Name, was)
	}

	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("the save that was opened was modified, which this editor must never do")
	}
}

// A removal stages, appears in what is staged, and can be dropped - and it
// shares its key with a grant, because one character cannot both be given an
// Ash and have it taken away.
func TestARemovalStagesAndReplacesAGrant(t *testing.T) {
	t.Parallel()
	session, people := realCharacters(t)
	person := firstUsed(t, people)
	var owned int32
	for family := range person.Owns {
		if owned == 0 || family < owned {
			owned = family
		}
	}
	if owned == 0 {
		t.Skip("the character owns no Ash to remove")
	}

	staged, err := session.StageRemoval(person.Slot, owned)
	if err != nil {
		t.Fatalf("staging a removal: %v", err)
	}
	if !staged.Removal() {
		t.Error("it staged as a grant")
	}
	if staged.Was != person.Levels[owned] {
		t.Errorf("it records +%d as the level they hold, want +%d", staged.Was, person.Levels[owned])
	}
	if session.PendingCount() != 1 {
		t.Errorf("%d changes pending after one removal", session.PendingCount())
	}

	// Staging a grant for the same Ash replaces it rather than queueing both.
	if _, err := session.StageGrant(person.Slot, owned, 10); err != nil {
		t.Fatalf("staging a level over a removal: %v", err)
	}
	if session.PendingCount() != 1 {
		t.Errorf("a grant over a removal left %d changes pending, want 1", session.PendingCount())
	}
	for _, grant := range session.Staged().Grants {
		if grant.GoodsFamily == owned && grant.Removal() {
			t.Error("the removal survived being replaced by a grant")
		}
	}

	if !session.UnstageGrant(person.Slot, owned) {
		t.Error("the staged change could not be dropped")
	}
	if session.PendingCount() != 0 {
		t.Errorf("%d changes survived being dropped", session.PendingCount())
	}
}
