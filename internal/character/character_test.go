package character_test

// Reading the slots, and granting an item into one.

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
)

// A save with two characters has two characters and eight empty slots, and the
// empty ones are a state to draw rather than an error to report. A slot whose
// name reads as unprintable text is one whose anchor is wrong, and is empty too.
func TestASaveDescribesAllTenSlots(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tarnished", Level: 42, Runes: 1234, Holds: []uint32{200000}},
		{Name: "Granny", Level: 7},
		{Name: "garbled", Level: 9},
	}})
	people, err := character.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 10 {
		t.Fatalf("%d slots described, want 10", len(people))
	}
	if !people[0].Used || people[0].Name != "Tarnished" || people[0].Level != 42 || people[0].Runes != 1234 {
		t.Errorf("slot 0 read as %+v", people[0])
	}
	if !people[1].Used || people[1].Name != "Granny" {
		t.Errorf("slot 1 read as %+v", people[1])
	}
	for _, empty := range people[2:] {
		if empty.Used {
			t.Errorf("slot %d reports a character that was never written", empty.Slot)
		}
	}
	if !people[0].Owns(200000) {
		t.Error("the item put in slot 0's inventory did not read back")
	}
	if people[1].Owns(200000) {
		t.Error("slot 1 owns an item only slot 0 was given")
	}
}

// The case the whole storage read exists for. An Ash in the chest is an Ash the
// character has, and reading inventory alone would offer them a second copy.
func TestAnItemInStorageCountsAsOwned(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tarnished", Holds: []uint32{200000}, Stores: []uint32{201000}},
	}})
	people, _ := character.Read(file)
	if !people[0].ReadStorage {
		t.Fatal("the storage box was not reachable, so this proves nothing")
	}
	if !people[0].Owns(201000) {
		t.Error("an item in the storage box read as not owned")
	}
	if people[0].Owns(202000) {
		t.Error("an item in neither container read as owned")
	}
}

// A grant is one record and three counters. The counters are the part that has
// no second chance: NextEquipIndex advances by exactly one, and a wrong value
// there writes an item the game will not show.
func TestGrantingWritesTheRecordAndMovesTheCounters(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tarnished", Holds: []uint32{200000, 201000}},
	}})
	equipBefore, acquisitionBefore := character.FixtureCounters(file, 0)
	countBefore := character.FixtureHeldCount(file, 0)

	if err := character.Set(file, 0, 202000, 0); err != nil {
		t.Fatalf("granting: %v", err)
	}

	// The record lands in the first empty slot, which is the third here.
	handle, quantity, index := character.FixtureRecord(file, 0, 2)
	if handle != character.GoodsHandle(202000) {
		t.Errorf("record handle is 0x%08X, want 0x%08X", handle, character.GoodsHandle(202000))
	}
	if quantity != 1 {
		t.Errorf("quantity is %d, want 1 - this editor never grants a stack", quantity)
	}

	equipAfter, acquisitionAfter := character.FixtureCounters(file, 0)
	if equipAfter != equipBefore+1 {
		t.Errorf("NextEquipIndex went %d -> %d, want exactly one more: it is the visibility gate",
			equipBefore, equipAfter)
	}
	if acquisitionAfter != index+1 {
		t.Errorf("NextAcquisitionSortId is %d with a record index of %d, want %d",
			acquisitionAfter, index, index+1)
	}
	if acquisitionAfter <= acquisitionBefore {
		t.Errorf("the acquisition counter did not advance: %d -> %d", acquisitionBefore, acquisitionAfter)
	}
	if got := character.FixtureHeldCount(file, 0); got != countBefore+1 {
		t.Errorf("the count header went %d -> %d, want one more", countBefore, got)
	}

	// And it reads back as owned, which is what stops a second grant.
	people, _ := character.Read(file)
	if !people[0].Owns(202000) {
		t.Error("the granted item does not read back as owned")
	}
}

// A refusal writes nothing at all, which matters because the caller's copy is
// the only copy: there is no input to restore a half-written record from.
func TestARefusedGrantWritesNothing(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tarnished"},
	}})
	before := append([]byte(nil), file...)

	for _, bad := range []struct {
		slot int
		want error
	}{
		{slot: 1, want: character.ErrNoSuchCharacter},  // an empty slot
		{slot: 10, want: character.ErrNoSuchCharacter}, // past the ten
		{slot: -1, want: character.ErrNoSuchCharacter},
	} {
		if err := character.Set(file, bad.slot, 200000, 0); !errors.Is(err, bad.want) {
			t.Errorf("granting into slot %d returned %v, want %v", bad.slot, err, bad.want)
		}
	}
	if !bytes.Equal(before, file) {
		t.Error("a refused grant changed the file")
	}
}

// A grant touches one record and three counters and nothing else. This is the
// check that a bad offset shows up as a test failure rather than as a save the
// game refuses.
func TestAGrantChangesOnlyWhatItMeansTo(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tarnished", Holds: []uint32{200000}},
		{Name: "Granny", Holds: []uint32{201000}},
	}})
	before := append([]byte(nil), file...)
	if err := character.Set(file, 0, 202000, 0); err != nil {
		t.Fatal(err)
	}

	allowed := character.FixtureGrantRegions(0, 1)
	inside := func(at int) bool {
		for _, region := range allowed {
			if at >= region[0] && at < region[0]+region[1] {
				return true
			}
		}
		return false
	}
	changed := 0
	for i := range file {
		if file[i] == before[i] {
			continue
		}
		changed++
		if !inside(i) {
			t.Fatalf("a grant changed byte 0x%X, which is outside the record and the three counters", i)
		}
	}
	if changed == 0 {
		t.Error("a grant changed nothing at all")
	}

	// And the other character is untouched.
	people, _ := character.Read(file)
	if people[1].Owns(202000) || !people[1].Owns(201000) {
		t.Error("granting to slot 0 changed what slot 1 owns")
	}
}

// The real save, when it is there: the reader has to work on the thing it was
// written for, not only on the fixture that was built to suit it.
func TestTheShippedSaveReadsAsCharacters(t *testing.T) {
	path := os.Getenv("ER_SAVE")
	if path == "" {
		t.Skip("ER_SAVE not set")
	}
	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	people, err := character.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	used := 0
	for _, person := range people {
		if !person.Used {
			continue
		}
		used++
		if person.Name == "" || person.Level == 0 {
			t.Errorf("slot %d is used but reads as %q level %d", person.Slot, person.Name, person.Level)
		}
		if !person.ReadStorage {
			t.Errorf("slot %d: the storage box was not reachable", person.Slot)
		}
		if person.Owned() == 0 {
			t.Errorf("slot %d holds no goods at all, which no real character does", person.Slot)
		}
	}
	if used == 0 {
		t.Fatal("the shipped save read as ten empty slots")
	}
}

// The upgrade level is the goods id, so changing one is a four-byte write.
//
// The regulation ships eleven rows per Ash and names them - 232000 "Lone Wolf
// Ashes", 232005 "Lone Wolf Ashes +5" - and an inventory record is a handle, a
// quantity and an acquisition index with no GaItem behind it, so the level has
// nowhere else to live. See docs/DESIGN.md (The character slots).
func TestChangingTheLevelRewritesTheHandleAndNothingElse(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tester", Level: 40, Holds: []uint32{200000, 232000, 240000}},
	}})
	before := append([]byte(nil), file...)

	if err := character.Set(file, 0, 232000, 5); err != nil {
		t.Fatalf("moving Lone Wolf Ashes to +5: %v", err)
	}

	people, err := character.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	level, owned := people[0].AshLevel(232000)
	if !owned || level != 5 {
		t.Errorf("after the write the character holds it at +%d (owned=%v), want +5", level, owned)
	}
	// And nothing else moved. A grant advances three counters and fills a
	// record; a level change writes one handle and touches nothing else, so
	// every differing byte has to be inside those four.
	//
	// Containment rather than a count: 232000 to 232005 differs in one byte of
	// the four, and a test that counted would be asserting which level it
	// picked rather than where the write landed.
	handleAt := character.FixtureGrantRegions(0, 1)[0][0]
	for _, at := range differences(before, file) {
		if at < handleAt || at >= handleAt+4 {
			t.Errorf("byte %d changed, which is outside the record's handle at %d", at, handleAt)
		}
	}
	if len(differences(before, file)) == 0 {
		t.Error("nothing changed at all")
	}
	if equip, acquisition := character.FixtureCounters(file, 0); equip != 0 || acquisition != 0 {
		t.Errorf("a level change moved the counters to %d and %d", equip, acquisition)
	}
	if count := character.FixtureHeldCount(file, 0); count != 3 {
		t.Errorf("the held count is %d, want the 3 it started at", count)
	}
	// The record keeps its quantity and its acquisition index: it is the same
	// item, at a different level.
	handle, quantity, index := character.FixtureRecord(file, 0, 1)
	if handle != character.GoodsHandle(232005) || quantity != 1 || index != 102 {
		t.Errorf("the record reads %08X, quantity %d, index %d", handle, quantity, index)
	}
}

// differences is every offset at which two files of the same length differ.
func differences(before, after []byte) []int {
	var at []int
	for index := range before {
		if index >= len(after) {
			break
		}
		if before[index] != after[index] {
			at = append(at, index)
		}
	}
	return at
}

// A real save holds an Ash in inventory AND in the storage box at once - both
// used slots of the maintainer's save do - so a level change has to move both.
// Leaving one behind turns one item into two with different names.
func TestAnAshHeldInBothPlacesMovesInBoth(t *testing.T) {
	file := character.BuildSave(character.Fixture{Characters: []character.FixtureCharacter{
		{Name: "Tester", Level: 40, Holds: []uint32{232000}, Stores: []uint32{232000}},
	}})
	if err := character.Set(file, 0, 232000, 8); err != nil {
		t.Fatalf("moving an Ash held in both places: %v", err)
	}
	people, err := character.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	if level, owned := people[0].AshLevel(232000); !owned || level != 8 {
		t.Errorf("the Ash reads as +%d (owned=%v), want +8", level, owned)
	}
	// The inventory copy specifically, since AshLevel would be satisfied by
	// either of them.
	if handle, _, _ := character.FixtureRecord(file, 0, 0); handle != character.GoodsHandle(232008) {
		t.Errorf("the inventory record reads %08X, want %08X", handle, character.GoodsHandle(232008))
	}
	// And the stored copy, read back through the same walk the writer used.
	stored := 0
	for _, level := range character.FixtureStoredLevels(file, 0, 232000) {
		stored++
		if level != 8 {
			t.Errorf("a stored record reads +%d, want +8", level)
		}
	}
	if stored != 1 {
		t.Errorf("%d stored records, want 1", stored)
	}
}
