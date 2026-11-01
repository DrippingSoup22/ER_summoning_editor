package character

// A synthetic save, for tests.
//
// Every other test in this product stands on internal/savefixture, which builds
// a regulation. Nothing has ever read a character slot, so there was nothing to
// test slot code against - and testing it against the maintainer's own save is
// not an option: it is a private input, it is never written, and a test that
// needs it skips everywhere else.
//
// So this builds the smallest thing shaped like a save that the reader accepts:
// the ten slots, a magic pattern where the reader will look for one, a name,
// and an inventory holding whatever the test wants it to hold. It is enough
// because the reader's whole job is offsets, and an offset is either right or
// wrong against a layout a test controls exactly.

import (
	"encoding/binary"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// Fixture describes the save to build.
type Fixture struct {
	// Characters is what to put in the slots, in order. Fewer than ten leaves
	// the rest empty, which is the ordinary shape of a real save.
	Characters []FixtureCharacter
}

// FixtureCharacter is one slot's contents.
type FixtureCharacter struct {
	Name         string
	Level, Runes uint32
	// Holds is the goods families to place in common inventory, and Stores the
	// ones to place in the storage box. The two are separate so a test can put
	// an item only in the chest, which is the case that decides whether
	// ownership really covers both.
	Holds, Stores []uint32
}

// BuildSave assembles the file. It is 26 MB of mostly zeros, which is what a PS4
// save is.
func BuildSave(fixture Fixture) []byte {
	file := make([]byte, regulation.UserData10Offset(regulation.PS4)+0x60000)
	for index, character := range fixture.Characters {
		if index >= regulation.NumSlots {
			break
		}
		start := regulation.SlotOffset(regulation.PS4, index)
		writeSlot(file[start:start+regulation.SlotSize], character)
	}
	return file
}

// FixtureMagic is where BuildSave puts the magic pattern, exported so a test can
// address the same fields the reader does without repeating the arithmetic.
const FixtureMagic = 0x20000

func writeSlot(data []byte, character FixtureCharacter) {
	copy(data[FixtureMagic:], magicPattern)

	name := []rune(character.Name)
	for i := 0; i < 16 && i < len(name); i++ {
		binary.LittleEndian.PutUint16(data[FixtureMagic+offCharacterName+i*2:], uint16(name[i]))
	}
	binary.LittleEndian.PutUint32(data[FixtureMagic+offLevel:], character.Level)
	binary.LittleEndian.PutUint32(data[FixtureMagic+offRunes:], character.Runes)

	invStart := FixtureMagic + invStartFromMagic
	for i, family := range character.Holds {
		if i >= commonItemCount {
			break
		}
		at := invStart + i*invRecordLen
		binary.LittleEndian.PutUint32(data[at:], GoodsHandle(family))
		binary.LittleEndian.PutUint32(data[at+4:], 1)
		binary.LittleEndian.PutUint32(data[at+8:], uint32(100+i*2))
	}
	binary.LittleEndian.PutUint32(data[invStart-4:], uint32(len(character.Holds)))

	// The storage box, at wherever the reader's own chain says it is. Built by
	// asking the reader rather than by repeating its arithmetic, so a fixture
	// can never drift from the thing it exists to test.
	if storage, ok := storageOffset(data, FixtureMagic); ok {
		at := storage + storageHeaderSkip
		for i, family := range character.Stores {
			if i >= storageCommonCount {
				break
			}
			off := at + i*invRecordLen
			binary.LittleEndian.PutUint32(data[off:], GoodsHandle(family))
			binary.LittleEndian.PutUint32(data[off+4:], 1)
			binary.LittleEndian.PutUint32(data[off+8:], uint32(200+i*2))
		}
		binary.LittleEndian.PutUint32(data[storage:], uint32(len(character.Stores)))
	}
}

// FixtureCounters reads the two counters a grant has to move, so a test can
// assert on them without reaching into the package's offset arithmetic.
func FixtureCounters(file []byte, slot int) (equipIndex, acquisition uint32) {
	start := regulation.SlotOffset(regulation.PS4, slot)
	data := file[start : start+regulation.SlotSize]
	equipOff := FixtureMagic + invStartFromMagic + commonItemCount*invRecordLen +
		invKeyCountHeader + keyItemCount*invRecordLen
	return binary.LittleEndian.Uint32(data[equipOff:]), binary.LittleEndian.Uint32(data[equipOff+4:])
}

// FixtureRecord reads one common-inventory record, for the same reason.
func FixtureRecord(file []byte, slot, record int) (handle, quantity, index uint32) {
	start := regulation.SlotOffset(regulation.PS4, slot)
	data := file[start : start+regulation.SlotSize]
	at := FixtureMagic + invStartFromMagic + record*invRecordLen
	return binary.LittleEndian.Uint32(data[at:]),
		binary.LittleEndian.Uint32(data[at+4:]),
		binary.LittleEndian.Uint32(data[at+8:])
}

// FixtureHeldCount reads the count header a grant has to advance.
func FixtureHeldCount(file []byte, slot int) uint32 {
	start := regulation.SlotOffset(regulation.PS4, slot)
	data := file[start : start+regulation.SlotSize]
	return binary.LittleEndian.Uint32(data[FixtureMagic+invStartFromMagic-4:])
}

// FixtureStoredLevels is the upgrade level of every record the storage box
// holds in one Ash's band, so a test can check the chest as well as the belt.
func FixtureStoredLevels(file []byte, slot int, family uint32) []int {
	start := regulation.SlotOffset(regulation.PS4, slot)
	data := file[start : start+regulation.SlotSize]
	magic, ok := findMagic(data)
	if !ok {
		return nil
	}
	storage, found := storageOffset(data, magic)
	if !found {
		return nil
	}
	var levels []int
	eachHeld(data, storage+storageHeaderSkip, storageCommonCount, storageKeyCount,
		func(_ int, goods uint32) {
			if goods >= family && goods < family+AshLevels {
				levels = append(levels, int(goods-family))
			}
		})
	return levels
}

// FixtureGrantRegions is every byte range a grant to one slot is allowed to
// touch: the record it fills, and the three counters it moves.
//
// A test asserting containment against this is stronger than counting changed
// bytes, because a wrong offset writes the right number of bytes in the wrong
// place - which is precisely the failure this package has to make loud.
func FixtureGrantRegions(slot, record int) [][2]int {
	start := regulation.SlotOffset(regulation.PS4, slot)
	invStart := start + FixtureMagic + invStartFromMagic
	equipOff := invStart + commonItemCount*invRecordLen + invKeyCountHeader + keyItemCount*invRecordLen
	return [][2]int{
		{invStart + record*invRecordLen, invRecordLen}, // the record
		{equipOff, 4},     // NextEquipIndex
		{equipOff + 4, 4}, // NextAcquisitionSortId
		{invStart - 4, 4}, // the count header
	}
}
