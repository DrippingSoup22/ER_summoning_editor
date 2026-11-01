package character

// What a character wears and carries: everything a summon of them is built
// from. Read only when a save holding such a summon is written - opening a save
// reads who the ten characters are (Read), and nothing here.
//
// Nothing is judged. A summon is the character as the save holds them, so what
// is equipped is copied as it stands; a slot that cannot be read is an error,
// never a guessed loadout.
//
// The offsets are SaveForge's (offset_defs.go, equipment_reader.go,
// structures.go), as research's field tickets GC-006 to GC-028 cite them, and
// were read against the 1.17.2 baseline's eight characters (research
// appearance/docs/face-data.md for the face).

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// PlayerGameData fields, relative to the magic pattern. PlayerGameData ends
// one byte into the pattern, so these sit before it.
const (
	offAttributes = -379 // eight u32: vigor, mind, endurance, strength, dexterity, intelligence, faith, arcane
	offBody       = -249 // 1 body type A, 0 type B: CharaInitParam.npcPlayerSex's spelling
	offClass      = -248
	offVoice      = -245
	offFlasks     = -182 // the crimson then the cerulean flask count, one byte each
	offBaseHP     = -415 // health without what equipment adds: the third of current, maximum, base
	offGreatRune  = -184 // 1 while a Great Rune is active, from a Rune Arc until death
)

// The effects a character saves: one
// record per save category, 13 of them, each an effect id (-1 empty) and three
// words, in the first section after PlayerGameData. An effect whose
// saveCategory is -1 is never saved, so a Great Rune's is not here: the game
// keeps the rune's flag (offGreatRune) and the rune in equipment slot 10, and
// applies the effect again on load.
const (
	savedEffects      = 13
	savedEffectRecord = 16
	slotGreatRune     = 10
)

// greatRuneEffects are the effect each restored Great Rune gives while active,
// goods 191 to 196: Godrick's +5 to every attribute (600), Radahn's health,
// focus and stamina (610), Morgott's health (620), Rykard's health on a kill
// (630), Mohg's blessing (640), Malenia's health on a hit (650). Read from the
// regulation (the effects in steps of ten, each the documented bonus); the
// program, not a param, pairs them.
var greatRuneEffects = map[uint32]int32{191: 600, 192: 610, 193: 620, 194: 630, 195: 640, 196: 650}

// The equipment sections. The ones before the held inventory start one byte
// past the magic (PlayerGameData's last byte is the pattern's first); the
// chain's odd 0x9011 takes that byte back, so the spells and everything after
// them are where storageOffset's chain puts them.
const (
	equipSlots     = 22 // left/right hands 1-3, two arrow and bolt pairs, rune, armour, five talismans
	handSlots      = 6  // L1 R1 L2 R2 L3 R3
	spellSlots     = 14
	quickSlots     = 10
	pouchSlots     = 6
	pouchFromQuick = 0x54 // ten quick records, then the active index, then the pouch

	gaItemsStart     = 0x20
	gaItemsOld       = 5118 // a slot of version 81 or below
	gaItemsNew       = 5120
	gaVersionBreak   = 81
	gaWeaponRecord   = 21 // handle, item, two words, the Ash of War's handle, a byte
	gaArmourRecord   = 16
	gaItemRecord     = 8
	gaAshHandleField = 16

	faceRecordSize = 0x120
)

// The slots of the 22 an equipped thing is read from, in the save's order.
const (
	slotArrow1 = 6
	slotBolt1  = 7
	slotArrow2 = 8
	slotBolt2  = 9
	slotHead   = 12
	slotTalis1 = 17
)

// empty is how every equipment table spells a free slot.
const empty = 0xFFFFFFFF

var faceMagic = []byte("FACE")

// Loadout is one character as a summon of them needs them.
type Loadout struct {
	Level uint32
	// Attributes are vigor, mind, endurance, strength, dexterity,
	// intelligence, faith and arcane.
	Attributes [8]uint8
	Body       uint8
	Voice      uint8
	// Flasks are the crimson and cerulean flask counts.
	Flasks [2]uint8
	// HP is the character's health without what its equipment adds - the
	// summon wears the same equipment.
	HP uint32
	// Effects are the effects active on the character: what it saved, and an
	// active Great Rune's.
	Effects []int32
	// Equipment is the 22 equipped slots as the save holds them: typed ids,
	// whose top nibble says weapon (0), armour (1), talisman (2) or goods (4),
	// and 0xFFFFFFFF where nothing is equipped. A weapon's id carries its
	// affinity and upgrade level.
	Equipment [equipSlots]uint32
	// Ashes is the Ash of War each hand slot carries of its own - an
	// EquipParamGem row - or -1 where the weapon fights with its own art.
	Ashes [handSlots]int32
	// Ammo is how many of each equipped arrow and bolt the character holds:
	// arrow 1, bolt 1, arrow 2, bolt 2.
	Ammo [4]uint32
	// Spells are MagicParam rows, -1 where a slot is empty.
	Spells [spellSlots]int32
	Quick  [quickSlots]Item
	Pouch  [pouchSlots]Item
	// Physick is the two tears in the Flask of Wondrous Physick, goods rows or
	// -1.
	Physick [2]int32
	// Face is the slot's appearance record, as the save holds it.
	Face [faceRecordSize]byte
}

// Item is a goods row and how many of it the character holds. Goods is -1
// where the slot is empty.
type Item struct {
	Goods int32
	Count uint32
}

// characterSlot is one slot's bytes, a window into file, and where its magic
// pattern is; a slot holding no character is an error.
func characterSlot(file []byte, slot int) ([]byte, int, error) {
	if slot < 0 || slot >= regulation.NumSlots {
		return nil, 0, fmt.Errorf("slot %d is outside 0..%d", slot, regulation.NumSlots-1)
	}
	if len(file) < regulation.UserData10Offset(regulation.PS4) {
		return nil, 0, fmt.Errorf("file is %d bytes, too small for ten PS4 slots: %w", len(file), ErrNotASave)
	}
	start := regulation.SlotOffset(regulation.PS4, slot)
	data := file[start : start+regulation.SlotSize]
	magic, ok := findMagic(data)
	if !ok || readName(data, magic+offCharacterName) == "" {
		return nil, 0, fmt.Errorf("slot %d holds no character", slot)
	}
	return data, magic, nil
}

// ReadLoadout reads one slot's character whole.
func ReadLoadout(file []byte, slot int) (Loadout, error) {
	data, magic, err := characterSlot(file, slot)
	if err != nil {
		return Loadout{}, err
	}

	var out Loadout
	out.Level = readU32(data, magic+offLevel)
	for index := range out.Attributes {
		out.Attributes[index] = uint8(readU32(data, magic+offAttributes+4*index))
	}
	out.Body, out.Voice = data[magic+offBody], data[magic+offVoice]
	out.Flasks = [2]uint8{data[magic+offFlasks], data[magic+offFlasks+1]}
	out.HP = readU32(data, magic+offBaseHP)
	for record := range savedEffects {
		if effect := int32(readU32(data, magic+1+record*savedEffectRecord)); effect > 0 {
			out.Effects = append(out.Effects, effect)
		}
	}

	handles := magic + 1 + dynSpEffect + dynEquipedItemIndex + dynActiveEquipedItems + dynEquipedItemsID
	spells := magic + dynSpEffect + dynEquipedItemIndex + dynActiveEquipedItems +
		dynEquipedItemsID + dynActiveEquipedItemsGa + dynInventoryHeld
	quick := spells + dynEquipedSpells
	projectiles := quick + dynEquipedItems + dynEquipedGestures
	count := int(readU32(data, projectiles))
	if count < 0 || count > maxProjCount {
		return Loadout{}, fmt.Errorf("slot %d: projectile count %d is not a count", slot, count)
	}
	armaments := projectiles + 4 + count*8
	face := armaments + dynEquipedArmaments + dynEquipePhysics + 4
	if face+faceRecordSize > len(data) || !bytes.Equal(data[face:face+4], faceMagic) {
		return Loadout{}, fmt.Errorf("slot %d: the face record is not where the equipment chain ends", slot)
	}

	items := gaItems(data)
	held := heldCounts(data, magic)
	for index := range out.Equipment {
		out.Equipment[index] = readU32(data, armaments+4*index)
	}
	for hand := range out.Ashes {
		out.Ashes[hand] = -1
		if weapon, known := items[readU32(data, handles+4*hand)]; known {
			if ash, attached := items[weapon.ash]; attached {
				out.Ashes[hand] = int32(ash.item & 0x0FFFFFFF)
			}
		}
	}
	for index, equipped := range []int{slotArrow1, slotBolt1, slotArrow2, slotBolt2} {
		out.Ammo[index] = held[readU32(data, handles+4*equipped)]
	}
	for index := range out.Spells {
		out.Spells[index] = idOrNone(readU32(data, spells+8*index))
	}
	for index := range out.Quick {
		out.Quick[index] = goodsAt(readU32(data, quick+8*index), items, held)
	}
	for index := range out.Pouch {
		out.Pouch[index] = goodsAt(readU32(data, quick+pouchFromQuick+8*index), items, held)
	}
	for index := range out.Physick {
		out.Physick[index] = idOrNone(readU32(data, armaments+dynEquipedArmaments+4*index) & 0x0FFFFFFF)
	}
	copy(out.Face[:], data[face:face+faceRecordSize])
	if data[magic+offGreatRune] != 0 {
		if effect, known := greatRuneEffects[out.Equipment[slotGreatRune]&0x0FFFFFFF]; known {
			out.Effects = append(out.Effects, effect)
		}
	}
	return out, nil
}

// gaItem is one record of the slot's item registry: what a weapon, armour or
// Ash of War handle stands for.
type gaItem struct {
	item uint32
	ash  uint32
}

// gaItems reads the registry. Its records are variable-sized - a weapon's is
// 21 bytes, armour's 16, anything else 8 - and the size follows the item id's
// top nibble (SaveForge structures.go, GaItemRecordSize).
func gaItems(data []byte) map[uint32]gaItem {
	count := gaItemsNew
	if version := binary.LittleEndian.Uint32(data); version > 0 && version <= gaVersionBreak {
		count = gaItemsOld
	}
	out := make(map[uint32]gaItem, count)
	at := gaItemsStart
	for range count {
		if at+gaItemRecord > len(data) {
			break
		}
		handle, item := readU32(data, at), readU32(data, at+4)
		size := gaItemRecord
		if item != 0 && item != empty {
			switch item >> 28 {
			case 0:
				size = gaWeaponRecord
			case 1:
				size = gaArmourRecord
			}
		}
		record := gaItem{item: item, ash: empty}
		if size == gaWeaponRecord && at+size <= len(data) {
			record.ash = readU32(data, at+gaAshHandleField)
		}
		if handle != 0 && handle != empty {
			out[handle] = record
		}
		at += size
	}
	return out
}

// heldCounts is how many of each handle the held inventory holds.
func heldCounts(data []byte, magic int) map[uint32]uint32 {
	out := map[uint32]uint32{}
	at := magic + invStartFromMagic
	for index := 0; index < commonItemCount+keyItemCount; index++ {
		record := at + index*invRecordLen
		if index >= commonItemCount {
			record += invKeyCountHeader
		}
		if record+invRecordLen > len(data) {
			break
		}
		if handle := readU32(data, record); handle != 0 && handle != empty {
			out[handle] += readU32(data, record+4)
		}
	}
	return out
}

// goodsAt reads a quick or pouch handle as the goods row it names and how
// many are held. A goods handle is the id itself; anything else goes through
// the registry, and a slot holding something that is not goods is empty to a
// summon, whose item slots take goods only.
func goodsAt(handle uint32, items map[uint32]gaItem, held map[uint32]uint32) Item {
	switch {
	case handle == 0 || handle == empty:
		return Item{Goods: -1}
	case handle&gaHandleTypeMask == typeGoods:
		return Item{Goods: int32(handle &^ gaHandleTypeMask), Count: held[handle]}
	}
	if record, known := items[handle]; known && record.item>>28 == 4 {
		return Item{Goods: int32(record.item & 0x0FFFFFFF), Count: held[handle]}
	}
	return Item{Goods: -1}
}

// idOrNone reads a row id the save spells empty as 0 or all ones.
func idOrNone(raw uint32) int32 {
	if raw == 0 || raw == empty || raw == empty&0x0FFFFFFF {
		return -1
	}
	return int32(raw)
}
