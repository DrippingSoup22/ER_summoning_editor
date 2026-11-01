package character

// Putting a goods item into a character's inventory, and changing the upgrade
// level of one already there.
//
// The whole write is one twelve-byte record and three counters, and it is that
// small for a reason worth stating: a goods item is handle-encoded. A weapon
// needs a GaItem record allocated, a handle chosen from a reserved index zone,
// and a capacity check against a 5,118-entry array. A goods item needs none of
// it - its inventory handle is 0xB0000000 | family, derived rather than
// allocated - which is why this file has no allocator in it.
//
// The counters are the part that matters. Their semantics are SaveForge's,
// derived by diffing native saves, and its code carries a "VERIFIED NATIVE SAVE
// CONTRACT - DO NOT CHANGE WITHOUT NEW NATIVE SAVE EVIDENCE" banner over them.
// They are reproduced here with the same warning, because getting one wrong
// does not fail: it writes an item into the save that the game will not show.

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// invEquipReservedFloor is the lowest acquisition index this writer will use.
//
// SaveForge's InvEquipReservedMax + 2. It is a floor for indices this editor
// creates, not a rule about existing ones: a genuine save holds game-written
// records well below it.
const invEquipReservedFloor = 432 + 2

var (
	// ErrNoSuchCharacter means the slot is out of range or holds no character.
	ErrNoSuchCharacter = errors.New("no character in that slot")
	// ErrAlreadyOwned means the character already holds the item, at the level
	// asked for. Granting a second copy is the one thing this must not do.
	ErrAlreadyOwned = errors.New("this character already has it at that level")
	// ErrNotOwned means there is nothing to take away, which is a refusal
	// rather than a success with no work: a Remove that quietly did nothing
	// would leave the player believing an item is gone.
	ErrNotOwned = errors.New("this character does not have it")
	// ErrInventoryFull means every common inventory record is occupied.
	ErrInventoryFull = errors.New("this character's inventory is full")
	// ErrNoSuchLevel means a level outside +0 to +10, which is the whole of
	// what the regulation ships rows for.
	ErrNoSuchLevel = errors.New("a Spirit Ash goes from +0 to +10")
	// ErrTwoCopies means the band holds records at more than one level, so
	// there is no single level to move. Two records at the SAME level is the
	// ordinary case and not this: a real save holds an Ash in inventory and in
	// the storage box at once, which is one item in two places.
	ErrTwoCopies = errors.New("this character holds it at two different levels")
)

// Set makes a character hold one Spirit Ash at one upgrade level.
//
// One entry point for two writes, because from the player's side they are one
// thing - "this character should have Lone Wolf Ashes at +5" - and which of the
// two it takes is a fact about the save rather than a decision anybody made:
//
//   - not held: a twelve-byte record is inserted at family + level.
//   - held at another level: the record's four-byte handle is rewritten. That
//     is the whole edit - no counter moves and nothing is allocated, because
//     the upgrade level IS the goods id. See AshLevels.
//   - held at this level: refused, because it is not a change.
//
// It writes into file, which must be the caller's own copy: this package never
// opens or writes a path, and the product's rule that an input file is never
// modified is kept by the caller giving it something else. Nothing is written
// unless every check passes, so a refusal leaves the bytes exactly as they were.
func Set(file []byte, slot int, family uint32, level int) error {
	if level < 0 || level >= AshLevels {
		return fmt.Errorf("+%d: %w", level, ErrNoSuchLevel)
	}
	if slot < 0 || slot >= regulation.NumSlots {
		return fmt.Errorf("slot %d: %w", slot, ErrNoSuchCharacter)
	}
	if len(file) < regulation.UserData10Offset(regulation.PS4) {
		return fmt.Errorf("file is %d bytes: %w", len(file), ErrNotASave)
	}
	start := regulation.SlotOffset(regulation.PS4, slot)
	data := file[start : start+regulation.SlotSize]

	current := readSlot(slot, data)
	if !current.Used {
		return fmt.Errorf("slot %d: %w", slot, ErrNoSuchCharacter)
	}
	magic, ok := findMagic(data)
	if !ok {
		return fmt.Errorf("slot %d: %w", slot, ErrNotASave)
	}
	// Read against inventory and storage both. An item in the chest is an item
	// the character has, and handing them a second one is exactly the mistake
	// reading storage exists to prevent.
	held, owns := current.AshLevel(family)
	if !owns {
		return insert(data, magic, GoodsHandle(family+uint32(level)))
	}
	if held == level {
		return fmt.Errorf("%s already has it at +%d: %w", current.Name, level, ErrAlreadyOwned)
	}
	return relevel(data, magic, current.Name, family, level)
}

// relevel rewrites the handle of every record the character holds this Ash in.
//
// Every record and not the first one, because a real save holds an Ash in
// inventory AND in the storage box at once - both slots of the maintainer's own
// save do, for Lone Wolf Ashes among others. That is one item in two places and
// both have to end up at the level the player asked for; leaving one behind
// would give them "Lone Wolf Ashes" on their belt and "Lone Wolf Ashes +5" in
// the chest, which is two items where there was one.
//
// Records at DIFFERENT levels are refused instead. The game cannot produce that
// - every row is maxNum 1 per container - but another editor could, and there
// is no single level to move.
//
// The records are found again here rather than carried from the read, because
// the check and the write have to see the same bytes: this runs on a file the
// caller may have written into since.
func relevel(data []byte, magic int, name string, family uint32, level int) error {
	var found []int
	levels := make(map[uint32]bool, 2)
	eachContainer(data, magic, func(offset int, goods uint32) {
		if goods >= family && goods < family+AshLevels {
			found = append(found, offset)
			levels[goods] = true
		}
	})
	switch {
	case len(found) == 0:
		// Not reachable through Set, which has just read the same slot. It is
		// here because a silent no-op is the worse of the two outcomes.
		return fmt.Errorf("%s no longer holds %d: %w", name, family, ErrNoSuchCharacter)
	case len(levels) > 1:
		return fmt.Errorf("%s: %d levels in the band of %d: %w",
			name, len(levels), family, ErrTwoCopies)
	}
	for _, at := range found {
		binary.LittleEndian.PutUint32(data[at:], GoodsHandle(family+uint32(level)))
	}
	return nil
}

// insert writes the record and moves the counters, in that order.
//
// Every offset is bounds-checked before anything is written, so the function
// either does all of it or none of it. A half-written record is the one outcome
// with no recovery: the file is not the input, so there is nothing to restore
// it from.
func insert(data []byte, magic int, handle uint32) error {
	invStart := magic + invStartFromMagic
	countOff := invStart - 4
	equipOff := invStart + commonItemCount*invRecordLen + invKeyCountHeader + keyItemCount*invRecordLen
	acqOff := equipOff + 4

	if countOff < 0 || acqOff+4 > len(data) {
		return fmt.Errorf("inventory does not fit the slot: %w", ErrNotASave)
	}

	empty := -1
	for i := 0; i < commonItemCount; i++ {
		at := invStart + i*invRecordLen
		held := binary.LittleEndian.Uint32(data[at:])
		if held == 0 || held == 0xFFFFFFFF {
			empty = i
			break
		}
	}
	if empty < 0 {
		return ErrInventoryFull
	}

	// The acquisition index. Written on an even stride above a floor, which is
	// SaveForge's write policy rather than a rule of the format: the game holds
	// only that no two records share a bucket, and an even stride guarantees
	// that. In-game acquisition order is keyed by index >> 1.
	acquisition := binary.LittleEndian.Uint32(data[acqOff:])
	if acquisition < invEquipReservedFloor {
		acquisition = invEquipReservedFloor
	}
	if acquisition%2 != 0 {
		acquisition++
	}
	index := acquisition + 1

	at := invStart + empty*invRecordLen
	binary.LittleEndian.PutUint32(data[at:], handle)
	binary.LittleEndian.PutUint32(data[at+4:], 1)
	binary.LittleEndian.PutUint32(data[at+8:], index)

	binary.LittleEndian.PutUint32(data[acqOff:], index+1)

	// VERIFIED NATIVE SAVE CONTRACT - DO NOT CHANGE WITHOUT NEW NATIVE SAVE
	// EVIDENCE AND A REGRESSION TEST. A genuinely new common-inventory record
	// advances NextEquipIndex by exactly one. It must never be reconciled to
	// the acquisition counter, and never set to the record's own index: that
	// mistake is SaveForge's CE-108255-1, and the symptom is a save the game
	// refuses. Evidence there is native captures T050 and T210.
	binary.LittleEndian.PutUint32(data[equipOff:], binary.LittleEndian.Uint32(data[equipOff:])+1)

	// The count header the game reads as its insertion point, and as the
	// full check when it equals the container's capacity.
	binary.LittleEndian.PutUint32(data[countOff:], binary.LittleEndian.Uint32(data[countOff:])+1)
	return nil
}
