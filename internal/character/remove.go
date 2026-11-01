package character

// Taking a Spirit Ash back out.
//
// The inverse of Set, and a much smaller operation than it looks. Adding a
// record has to place it and move three counters, and getting the counters
// wrong is the mistake with a name - the save the game refuses. Removing one
// touches none of them:
//
//   - the record is zeroed IN PLACE, leaving a hole. Nothing shifts up, so no
//     other record's acquisition index moves and no other record changes at
//     all. The insert path already scans for a hole, so a hole is a slot the
//     format expects.
//   - the count header is RECOUNTED rather than decremented. A decrement is
//     right only if the header and the records already agreed; a recount is
//     right either way, and the header is what the game reads as its insertion
//     point.
//   - NextEquipIndex and NextAcquisitionSortId are left alone. They are
//     monotonic. Winding either back would hand a future record an index that
//     has already been used, which is the collision the even-stride policy in
//     insert exists to avoid.
//
// The contract is inherited rather than derived here: EldenRing-SaveForge
// established it by diffing native saves and asserts every clause of it in
// storage_contract_test.go, and the tests beside this file are that contract
// restated against this package. Both projects are GPL-3.0-or-later and
// THIRD_PARTY_NOTICES.md records the borrowing.

import (
	"encoding/binary"
	"fmt"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// Remove takes one Spirit Ash out of a character's inventory and storage box.
//
// Every record of the family goes, at whatever upgrade level it is held at and
// in whichever container: an Ash the player has one of in the chest and another
// of in their pack is one Ash to them, and leaving half of it behind would be a
// Remove that did not remove it.
//
// It writes into file, which must be the caller's own copy. Nothing is written
// unless the character actually holds the Ash, so a refusal leaves the bytes
// exactly as they were.
func Remove(file []byte, slot int, family uint32) error {
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
	if _, owns := current.AshLevel(family); !owns {
		return fmt.Errorf("%s does not have it: %w", current.Name, ErrNotOwned)
	}

	// Collected before anything is written, so the walk cannot be confused by
	// its own edits and so a run that finds nothing writes nothing.
	var records []int
	eachContainer(data, magic, func(offset int, goods uint32) {
		if goods >= family && goods < family+uint32(AshLevels) {
			records = append(records, offset)
		}
	})
	if len(records) == 0 {
		return fmt.Errorf("%s does not have it: %w", current.Name, ErrNotOwned)
	}

	invStart := magic + invStartFromMagic
	for _, offset := range records {
		binary.LittleEndian.PutUint32(data[offset:], 0)
		binary.LittleEndian.PutUint32(data[offset+4:], 0)
		// The index field keeps the record's own position in the held block and
		// is zeroed in storage, which is the shape the reference writes and the
		// only difference between the two containers.
		if row, held := heldRow(offset, invStart); held {
			binary.LittleEndian.PutUint32(data[offset+8:], uint32(row))
			continue
		}
		binary.LittleEndian.PutUint32(data[offset+8:], 0)
	}
	reconcileHeld(data, invStart)
	if storage, found := storageOffset(data, magic); found {
		reconcileStorage(data, storage)
	}
	return nil
}

// heldRow is the record's index within the held common block, or false for one
// that is not in it - a key item, or a record in the storage box.
func heldRow(offset, invStart int) (int, bool) {
	if offset < invStart {
		return 0, false
	}
	row := (offset - invStart) / invRecordLen
	if row >= commonItemCount || (offset-invStart)%invRecordLen != 0 {
		return 0, false
	}
	return row, true
}

// reconcileHeld rewrites the held common count to the number of records that
// are actually there. See the note above on why this is a recount.
func reconcileHeld(data []byte, invStart int) {
	countOff := invStart - 4
	if countOff < 0 || countOff+4 > len(data) {
		return
	}
	binary.LittleEndian.PutUint32(data[countOff:], occupied(data, invStart, commonItemCount))
}

// reconcileStorage rewrites the storage box's own distinct-item count.
func reconcileStorage(data []byte, storage int) {
	if storage < 0 || storage+4 > len(data) {
		return
	}
	binary.LittleEndian.PutUint32(data[storage:],
		occupied(data, storage+storageHeaderSkip, storageCommonCount))
}

// occupied counts the records of a common block that hold something.
//
// Empty is spelled both ways the shipped data spells it, which is the same rule
// insert uses to find a free slot: a block where the two disagreed would be one
// where a record could be both free to write into and counted as present.
func occupied(data []byte, start, count int) uint32 {
	total := uint32(0)
	for i := 0; i < count; i++ {
		off := start + i*invRecordLen
		if off < 0 || off+invRecordLen > len(data) {
			break
		}
		if handle := binary.LittleEndian.Uint32(data[off:]); handle != 0 && handle != 0xFFFFFFFF {
			total++
		}
	}
	return total
}
