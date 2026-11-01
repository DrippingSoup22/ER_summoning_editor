package character_test

// Taking a Spirit Ash back out, and what removal is not allowed to disturb.
//
// These are EldenRing-SaveForge's storage contract restated against this
// package. That project derived the clauses by diffing native saves; they are
// inherited here rather than re-derived, and inherited behaviour with no
// inherited evidence is the version that fails quietly later.

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
)

// A removed record is zeroed where it stands, and nothing around it moves.
//
// Not compacted: shifting the records up would change every acquisition index
// above the hole, which is what the game sorts "recently acquired" by. The
// insert path already looks for a hole, so leaving one is the format's own
// answer to a removal.
func TestRemovingLeavesAHoleAndMovesNothingElse(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tester", Level: 1, Holds: []uint32{232000, 233000, 234000}},
	}})
	before := make([]uint32, 3)
	for record := range before {
		before[record], _, _ = character.FixtureRecord(file, 0, record)
	}

	if err := character.Remove(file, 0, 233000); err != nil {
		t.Fatalf("removing the middle Ash: %v", err)
	}

	if handle, _, _ := character.FixtureRecord(file, 0, 0); handle != before[0] {
		t.Errorf("the record below the hole changed to %#x, want %#x", handle, before[0])
	}
	if handle, _, _ := character.FixtureRecord(file, 0, 2); handle != before[2] {
		t.Errorf("the record above the hole moved: %#x, want %#x", handle, before[2])
	}
	handle, quantity, index := character.FixtureRecord(file, 0, 1)
	if handle != 0 || quantity != 0 {
		t.Errorf("the removed record reads handle %#x quantity %d, want both zero", handle, quantity)
	}
	if index != 1 {
		t.Errorf("the removed record's index is %d, want its own row 1", index)
	}
}

// Adding, removing and adding again reuses the hole and never grows the file.
//
// This is the failure the SaveForge author warned about, checked here rather
// than assumed away. It does not apply to what this editor writes, and the
// reason is worth stating: a Spirit Ash is a GOODS record, which SaveForge
// classifies as addKindStack - "handle-encoded, no serialized GaItem". The
// handle is derived from the item id rather than allocated, so there is no
// second table to leave an orphan in and no placement cursor to exhaust. All of
// that machinery - RepairOrphanedGaItems, the physical-hole capacity counting,
// NextArmamentIndex - exists for weapons, armour and Ashes of War.
//
// What is left for goods is one 12-byte hole in a fixed-size, pre-allocated
// array of 2,688 records. The array does not grow, insert already fills the
// first hole it finds, and the count header is recounted. So the cycle is
// stable rather than merely tolerable.
func TestAddingAndRemovingReusesTheHole(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tester", Level: 1, Holds: []uint32{232000}},
	}})
	size := len(file)
	equipStart, _ := character.FixtureCounters(file, 0)

	// The record the first add lands in, and the acquisition bucket it claims.
	if err := character.Set(file, 0, 233000, 0); err != nil {
		t.Fatal(err)
	}
	handle, _, firstIndex := character.FixtureRecord(file, 0, 1)
	if handle == 0 {
		t.Fatal("the first add did not land in the record after the held one")
	}
	equipAfterAdd, _ := character.FixtureCounters(file, 0)
	if equipAfterAdd != equipStart+1 {
		t.Fatalf("NextEquipIndex went %d -> %d on an add, want one step", equipStart, equipAfterAdd)
	}

	if err := character.Remove(file, 0, 233000); err != nil {
		t.Fatal(err)
	}
	equipAfterRemove, _ := character.FixtureCounters(file, 0)
	if equipAfterRemove != equipAfterAdd {
		t.Errorf("NextEquipIndex moved on a removal: %d -> %d", equipAfterAdd, equipAfterRemove)
	}

	// The second add fills the hole the removal left rather than taking a new
	// record, which is what keeps a long session of adding and removing from
	// walking up the array.
	if err := character.Set(file, 0, 234000, 0); err != nil {
		t.Fatal(err)
	}
	refilled, _, secondIndex := character.FixtureRecord(file, 0, 1)
	if refilled == 0 {
		t.Error("the hole was not reused; the add went somewhere else")
	}
	if len(file) != size {
		t.Errorf("the file grew from %d to %d bytes", size, len(file))
	}

	// And the two adds are in different acquisition buckets. The game keys
	// "order of acquisition" by index>>1, so a reused hole that also reused the
	// index would put two records in one bucket - the collision that reverts on
	// restart.
	if firstIndex>>1 == secondIndex>>1 {
		t.Errorf("both adds claimed acquisition bucket %d", firstIndex>>1)
	}
	if got := character.FixtureHeldCount(file, 0); got != 2 {
		t.Errorf("the count header reads %d after add, remove, add, want 2", got)
	}
}
