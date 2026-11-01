// Package character reads the ten character slots of a decrypted PS4 save.
//
// Everything else in this product edits the regulation. `internal/savecore`
// reaches it by computing the offset of UserData11 - past the header, past all
// ten slots, past UserData10 - and has never looked inside a slot. This package
// is the other half: who the ten characters are, and which Spirit Ashes each of
// them already owns.
//
// See docs/DESIGN.md (The character slots). The layout constants and the parsing order come from
// the GPLv3 EldenRing-SaveForge project, which derived them by diffing native
// saves; this reimplements only the read side it needs. Where a number here
// looks arbitrary it is theirs and is cited, because the alternative is a
// constant nobody can check.
//
// PS4 saves carry no checksum and no encryption over slot data - the MD5 rule
// that makes save editing dangerous is a PC concern - so reading is a matter of
// finding offsets correctly and nothing else.
package character

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// Offsets relative to the magic pattern, from SaveForge's offset_defs.go. They
// are negative: the pattern sits after the fields it anchors.
const (
	offCharacterName = -0x11B // 16 x uint16, UTF-16LE
	offLevel         = -335
	offRunes         = -331

	// minMagicOffset is what the pattern must be found past, or a negative
	// field offset would read before the start of the slot.
	minMagicOffset = 400
	// fallbackMagicBase is used when the pattern is not found at all.
	fallbackMagicBase = 0x15420 + 432
)

// gaHandleTypeMask is the top nibble of an inventory handle, which says what
// class of thing the record refers to. Goods are 0xB.
const (
	gaHandleTypeMask = 0xF0000000
	typeGoods        = 0xB0000000
)

// AshLevels is how many upgrade levels a Spirit Ash has, and so how wide a band
// of goods rows one Ash owns: family + 0 through family + 10.
//
// The upgrade level is IN THE ID. The regulation ships eleven rows per Ash and
// names them - 232000 "Lone Wolf Ashes", 232005 "Lone Wolf Ashes +5" - and an
// inventory record is a handle, a quantity and an acquisition index with no
// GaItem behind it, so there is nowhere else the level could live. See
// docs/DESIGN.md (The character slots).
//
// Ash families are a thousand apart, so a band of eleven cannot reach the next
// family. This constant is about Spirit Ashes and is not a fact about goods in
// general.
const AshLevels = 11

// Inventory, at magic + invStartFromMagic. A fixed area: it never grows, which
// is what will make writing into it safe.
const (
	invStartFromMagic = 505
	commonItemCount   = 0xA80 // 2688
	keyItemCount      = 0x180 // 384
	invRecordLen      = 12
	invKeyCountHeader = 4
)

// The storage box, reached by a chain of offsets from the magic pattern.
//
// Storage matters because ownership has to mean "has it anywhere". An Ash in
// the storage chest that read as not owned would earn its owner a second copy,
// which is the one thing the granting rule exists to prevent.
//
// The chain is a run of constant hops and one dynamic field - the acquired
// projectile count, whose section is count*8+4 bytes - and every constant is
// SaveForge's. It is bounds-checked at each step and sanity-checked at the end,
// because a chain that drifts produces a plausible-looking offset rather than
// an obvious failure.
const (
	dynSpEffect             = 0xD0
	dynEquipedItemIndex     = 0x58
	dynActiveEquipedItems   = 0x1C
	dynEquipedItemsID       = 0x58
	dynActiveEquipedItemsGa = 0x58
	dynInventoryHeld        = 0x9011
	dynEquipedSpells        = 0x74
	dynEquipedItems         = 0x8C
	dynEquipedGestures      = 0x18
	dynEquipedArmaments     = 0x9C
	dynEquipePhysics        = 0x0C
	dynFaceData             = 0x12F

	maxProjCount       = 200000
	storageHeaderSkip  = 4
	storageCommonCount = 0x780 // 1920
	storageKeyCount    = 0x80  // 128
)

// magicPattern anchors every field this package reads: four blocks of
// 0xFFFFFFFF followed by twelve zero bytes, the first prefixed with one zero.
var magicPattern = []byte{
	0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// ErrNotASave means the file is not a decrypted PS4 save, or is truncated.
var ErrNotASave = errors.New("not a decrypted PlayStation save")

// Character is one of the ten slots.
//
// An empty slot is a real state rather than an error: a save with three
// characters has seven of them, and a frontend has to draw ten things.
type Character struct {
	// Slot is the character's position, 0 to 9, and is its identity. Names
	// repeat and are player-chosen; the slot is what a write would name.
	Slot int
	// Used reports whether the slot holds a character at all. Everything below
	// is meaningless when it is false.
	Used bool
	Name string
	// Level and Runes are what tells two characters apart when a player has
	// named them alike, which the ten slots of a real save do.
	Level, Runes uint32
	// Class is the starting class, an index into ClassNames.
	Class uint8

	// ReadStorage reports whether the storage box was reachable. When it is
	// false, Owns answers for inventory alone: a true is still true, but a
	// false may be an item sitting in the chest. Anything offering to hand out
	// a copy has to say so rather than imply certainty it does not have.
	ReadStorage bool

	// owned is every goods family this character holds, in inventory and, when
	// it could be read, in storage.
	owned map[uint32]bool
}

// AshLevel is the upgrade level this character holds a Spirit Ash at, and
// whether they hold it at all.
//
// A character holds one row of the family's band, and which row it is IS the
// level. Where a save somehow holds two - which the game cannot produce, since
// every row is maxNum 1, but another editor could - this reports the higher of
// them and Set refuses to write, rather than silently merging two items into
// one.
func (c Character) AshLevel(family uint32) (int, bool) {
	level, owned := 0, false
	for step := 0; step < AshLevels; step++ {
		if c.owned[family+uint32(step)] {
			level, owned = step, true
		}
	}
	return level, owned
}

// Owns reports whether this character already holds a goods item, named by its
// goods family - the number catalog-ashes.csv calls goods_family.
//
// Goods are not in the GaItem array at all, which is the thing worth knowing
// about this format and the thing that cost the most to find out. A weapon has
// a GaItem record and its inventory record holds a reference to it; a goods
// item is handle-encoded, and its inventory record's handle is literally
// 0xB0000000 | family with no GaItem behind it. So ownership is read from the
// containers, and granting one will allocate nothing.
func (c Character) Owns(family uint32) bool { return c.owned[family] }

// Owned is how many distinct goods this character holds. The count's only use
// is a rough sense of whether a slot is a fresh character or a finished one.
func (c Character) Owned() int { return len(c.owned) }

// GoodsHandle is the inventory handle for a goods family. Deterministic, which
// is why granting one needs no allocator.
func GoodsHandle(family uint32) uint32 { return typeGoods | family }

// ClassNames are the starting classes in the order the save numbers them.
var ClassNames = []string{"Vagabond", "Warrior", "Hero", "Bandit", "Astrologer",
	"Prophet", "Confessor", "Samurai", "Prisoner", "Wretch"}

// ClassName is a starting class's name, or empty for a number the game does
// not use.
func (c Character) ClassName() string {
	if int(c.Class) < len(ClassNames) {
		return ClassNames[c.Class]
	}
	return ""
}

// Read parses the ten slots of a decrypted PS4 save.
//
// It takes the whole file rather than a path, so a caller that already holds
// the bytes does not read 26 MB twice, and so this package never opens a file
// it might be tempted to write.
func Read(file []byte) ([]Character, error) {
	if len(file) < regulation.UserData10Offset(regulation.PS4) {
		return nil, fmt.Errorf("file is %d bytes, too small for ten PS4 slots: %w", len(file), ErrNotASave)
	}
	characters := make([]Character, 0, regulation.NumSlots)
	for index := 0; index < regulation.NumSlots; index++ {
		start := regulation.SlotOffset(regulation.PS4, index)
		characters = append(characters, readSlot(index, file[start:start+regulation.SlotSize]))
	}
	return characters, nil
}

// readSlot describes one slot and never fails: a slot it cannot make sense of
// is an unused slot. Ten slots always produce ten answers, because a save with
// one unreadable slot is still a save whose other nine a player can look at.
func readSlot(index int, data []byte) Character {
	character := Character{Slot: index}
	magic, ok := findMagic(data)
	if !ok {
		return character
	}
	name := readName(data, magic+offCharacterName)
	if name == "" {
		// The clearest signal of an empty slot: the game requires a name at
		// creation, so a used slot always carries one.
		return character
	}
	character.Used = true
	character.Name = name
	character.Level = readU32(data, magic+offLevel)
	character.Runes = readU32(data, magic+offRunes)
	character.Class = data[magic+offClass]

	character.owned = make(map[uint32]bool, 128)
	readHeld(data, magic+invStartFromMagic, commonItemCount, keyItemCount, character.owned)
	// Storage is best effort. A save whose chain cannot be walked still gives
	// a correct answer for everything in inventory, and the caller is told
	// which of the two it got rather than left to assume.
	if storage, found := storageOffset(data, magic); found {
		readHeld(data, storage+storageHeaderSkip, storageCommonCount, storageKeyCount, character.owned)
		character.ReadStorage = true
	}
	return character
}

// findMagic locates the pattern the slot's fields are addressed from.
//
// SaveForge falls back to a fixed base when the scan fails. That is kept, but
// only where the fallback could plausibly hold data: a wrong anchor produces a
// convincing name out of unrelated bytes, and an empty slot is a better answer
// than an invented character.
func findMagic(data []byte) (int, bool) {
	if at := bytes.Index(data, magicPattern); at >= minMagicOffset {
		return at, true
	}
	if fallbackMagicBase >= minMagicOffset && fallbackMagicBase+invStartFromMagic < len(data) {
		return fallbackMagicBase, true
	}
	return 0, false
}

// readName reads the sixteen UTF-16 code units of a character name, stopping
// at the first NUL as the game does.
func readName(data []byte, at int) string {
	const units = 16
	if at < 0 || at+units*2 > len(data) {
		return ""
	}
	encoded := make([]uint16, 0, units)
	for i := 0; i < units; i++ {
		unit := binary.LittleEndian.Uint16(data[at+i*2:])
		if unit == 0 {
			break
		}
		encoded = append(encoded, unit)
	}
	name := string(utf16.Decode(encoded))
	// Unprintable text means the anchor is wrong, and reporting that as a
	// character would be worse than reporting an empty slot.
	for _, r := range name {
		if r < 0x20 || r == 0xFFFD {
			return ""
		}
	}
	return name
}

func readU32(data []byte, at int) uint32 {
	if at < 0 || at+4 > len(data) {
		return 0
	}
	return binary.LittleEndian.Uint32(data[at:])
}

// readHeld collects the goods families held by one container.
func readHeld(data []byte, start, common, key int, into map[uint32]bool) {
	eachHeld(data, start, common, key, func(_ int, goods uint32) { into[goods] = true })
}

// eachHeld visits every goods record in one container, with the offset of the
// record as well as what it names - the offset because changing an item's
// upgrade level is a write to that record and nothing else.
//
// A container is a run of common records, a four-byte count header, then a run
// of key records - the same twelve-byte record throughout, and the same shape
// for inventory and for storage, which is what lets one function read both.
func eachHeld(data []byte, start, common, key int, visit func(offset int, goods uint32)) {
	read := func(at, count int) {
		for i := 0; i < count; i++ {
			off := at + i*invRecordLen
			if off < 0 || off+invRecordLen > len(data) {
				return
			}
			handle := binary.LittleEndian.Uint32(data[off:])
			if handle&gaHandleTypeMask == typeGoods {
				visit(off, handle&^uint32(gaHandleTypeMask))
			}
		}
	}
	read(start, common)
	read(start+common*invRecordLen+invKeyCountHeader, key)
}

// eachContainer visits both of a slot's containers, inventory first.
//
// Storage is best effort here for the same reason it is in readSlot: a chain
// that cannot be walked costs the caller the chest, not the answer.
func eachContainer(data []byte, magic int, visit func(offset int, goods uint32)) {
	eachHeld(data, magic+invStartFromMagic, commonItemCount, keyItemCount, visit)
	if storage, found := storageOffset(data, magic); found {
		eachHeld(data, storage+storageHeaderSkip, storageCommonCount, storageKeyCount, visit)
	}
}

// storageOffset walks the chain from the magic pattern to the storage box.
//
// It reports "not found" rather than guessing whenever a step would leave the
// slot or the result fails its sanity check. Storage improves the answer; it is
// never required for one.
func storageOffset(data []byte, magic int) (int, bool) {
	at := magic + dynSpEffect + dynEquipedItemIndex + dynActiveEquipedItems +
		dynEquipedItemsID + dynActiveEquipedItemsGa + dynInventoryHeld +
		dynEquipedSpells + dynEquipedItems + dynEquipedGestures
	if at < 0 || at+4 > len(data) {
		return 0, false
	}
	// The one dynamic field. Everything after it shifts by count*8+4, so a
	// wrong value here puts storage somewhere harmless-looking and wrong.
	projectiles := int(binary.LittleEndian.Uint32(data[at:]))
	if projectiles < 0 || projectiles > maxProjCount {
		return 0, false
	}
	at += projectiles*8 + 4
	at += dynEquipedArmaments + dynEquipePhysics + dynFaceData

	section := storageHeaderSkip + storageCommonCount*invRecordLen +
		invKeyCountHeader + storageKeyCount*invRecordLen
	if at <= magic || at+section > len(data) {
		return 0, false
	}
	return at, true
}
