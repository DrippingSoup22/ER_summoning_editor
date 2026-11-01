package character

// The rows a summoned character is made of. A person summon is a shell (the
// game's own, which supplies the health and the AI) wearing a CharaInitParam
// row; a character from a slot is a row of our own, built here whole from the
// slot, with a FaceParam row for its face and an EquipParamCustomWeapon row for
// each weapon carrying an Ash of War of its own.
//
// Everything the row can say is copied: level, attributes, body and voice, the
// six weapons with their affinity, level and Ash of War, ammunition, armour,
// four talismans, the Physick tears, spells, quick items and pouch with their
// counts, flasks, and the face. What it cannot say is left out rather than
// approximated: spells past the seventh (the row has seven), the fifth
// talisman slot (the game never opens it), the Great Rune and gestures.

import (
	"encoding/binary"
	"math"
)

// The rows' ids: 2990000 + 10 x slot, the same id in all three tables, and a
// custom weapon at its character's id plus its hand slot. The hundred ids from
// 2990000 are free in every supported regulation, and so are the hundred above
// them, the built characters' (build.go); the save core refuses a character
// row outside the two (savecore characters.go).
const (
	RowFloor  = 2990000
	rowStride = 10
	rowSpan   = 100
)

// The three tables, as the save core names them.
const (
	TableCharaInit    = "CharaInitParam"
	TableFace         = "FaceParam"
	TableCustomWeapon = "EquipParamCustomWeapon"
)

// RowOf is the CharaInitParam and FaceParam row of a slot's character.
func RowOf(slot int) int32 { return RowFloor + rowStride*int32(slot) }

// BodyFloor is the first of the NpcParam rows a character's body takes:
// 199900000 + 10 x slot. A body is a copy of a person shell carrying the
// character's own health and active effects, which a person's NpcParam row
// supplies and its CharaInitParam row cannot. The band is cut out of the clone
// bands (formation.InCloneBand), so nothing but the character write reaches it.
const BodyFloor = 199900000

// BodyOf is the NpcParam row of a slot's character's body.
func BodyOf(slot int) int32 { return BodyFloor + rowStride*int32(slot) }

// A character has four bodies side by side, from the one BodyOf and
// BuiltBodyOf name: the Helper in the coat of the Puppet shell it copies and in
// the character's own colours, then the Fighter in each (savecore.CharacterBody).
// The look and the role are both the body a formation names.
const (
	ownColoursBody = 1
	fighterBody    = 2
	// Bodies is how many bodies a character has.
	Bodies = 4
)

// BodyFor is a character's body in a look and a role, from its first body.
func BodyFor(body int32, ownColours, fighter bool) int32 {
	if ownColours {
		body += ownColoursBody
	}
	if fighter {
		body += fighterBody
	}
	return body
}

// BodyAt says which character body an NpcParam row is: the character's first
// body, whether the row draws it in its own colours and whether it is the
// Fighter's, and false for a row that is no character's body.
func BodyAt(row int32) (body int32, ownColours, fighter, ok bool) {
	if row < BodyFloor || row >= BodyFloor+2*rowSpan {
		return 0, false, false, false
	}
	at := (row - BodyFloor) % rowStride
	if at >= Bodies {
		return 0, false, false, false
	}
	return row - at, at&ownColoursBody != 0, at&fighterBody != 0, true
}

// SlotOf is the slot a character row was made for, and whether the row is one.
func SlotOf(row int32) (int, bool) {
	if row < RowFloor || row >= RowFloor+rowSpan || (row-RowFloor)%rowStride != 0 {
		return 0, false
	}
	return int((row - RowFloor) / rowStride), true
}

// Row is one row, whole.
type Row struct {
	Table string
	ID    int32
	Data  []byte
}

// Row sizes, the regulation's (shared/regulation layouts.go).
const (
	charaInitSize    = 320
	faceSize         = 240
	customWeaponSize = 16
)

// charaInitField is where a CharaInitParam field sits and how wide it is.
type charaInitField struct {
	offset int
	kind   string // s32, s16, u16, s8 or u8
}

// charaInitFields are the fields a character row writes, at the offsets
// Paramdex's CharaInitParam gives; rows_real_test.go reads shipped rows
// through this table and checks the values research decoded.
var charaInitFields = map[string]charaInitField{
	"equip_Wep_Right": {16, "s32"}, "equip_Subwep_Right": {20, "s32"}, "equip_Subwep_Right3": {264, "s32"},
	"equip_Wep_Left": {24, "s32"}, "equip_Subwep_Left": {28, "s32"}, "equip_Subwep_Left3": {268, "s32"},
	"wepParamType_Right1": {232, "u8"}, "wepParamType_Right2": {233, "u8"}, "wepParamType_Right3": {234, "u8"},
	"wepParamType_Left1": {235, "u8"}, "wepParamType_Left2": {236, "u8"}, "wepParamType_Left3": {237, "u8"},
	"equip_Helm": {32, "s32"}, "equip_Armer": {36, "s32"}, "equip_Gaunt": {40, "s32"}, "equip_Leg": {44, "s32"},
	"equip_Arrow": {48, "s32"}, "equip_Bolt": {52, "s32"}, "equip_SubArrow": {56, "s32"}, "equip_SubBolt": {60, "s32"},
	"arrowNum": {178, "u16"}, "boltNum": {180, "u16"}, "subArrowNum": {182, "u16"}, "subBoltNum": {184, "u16"},
	"equip_Accessory01": {64, "s32"}, "equip_Accessory02": {68, "s32"},
	"equip_Accessory03": {72, "s32"}, "equip_Accessory04": {76, "s32"},
	"elixir_material00": {84, "s32"}, "elixir_material01": {88, "s32"}, "elixir_material02": {92, "s32"},
	"equip_Spell_01": {96, "s32"}, "item_01": {124, "s32"}, "itemNum_01": {204, "u8"},
	"secondaryItem_01": {276, "s32"}, "secondaryItemNum_01": {300, "u8"},
	"gestureId0":         {219, "s8"},
	"npcPlayerFaceGenId": {164, "s32"},
	"soulLv":             {192, "s16"}, "baseVit": {194, "u8"},
	"npcPlayerSex": {228, "u8"}, "HpEstMax": {306, "s8"}, "MpEstMax": {307, "s8"}, "voiceType": {313, "u8"},
}

// rowSpells is how many spells the row holds: equip_Spell_01 to 07.
const rowSpells = 7

// The runs of seven, ten and six: equip_Spell_01..07, item_01..10 and
// itemNum_01..10, secondaryItem_01..06 and secondaryItemNum_01..06, the
// eight attributes from baseVit, the seven gestures from gestureId0. Each
// field of a run follows the one before it by its own width.
const gestureCount = 7

// hands is where each hand slot lands, in the save's order L1 R1 L2 R2 L3 R3:
// the weapon field and its wepParamType.
var hands = [handSlots][2]string{
	{"equip_Wep_Left", "wepParamType_Left1"}, {"equip_Wep_Right", "wepParamType_Right1"},
	{"equip_Subwep_Left", "wepParamType_Left2"}, {"equip_Subwep_Right", "wepParamType_Right2"},
	{"equip_Subwep_Left3", "wepParamType_Left3"}, {"equip_Subwep_Right3", "wepParamType_Right3"},
}

// ammo is where each ammunition slot lands, in Loadout.Ammo's order.
var ammo = [4][2]string{
	{"equip_Arrow", "arrowNum"}, {"equip_Bolt", "boltNum"},
	{"equip_SubArrow", "subArrowNum"}, {"equip_SubBolt", "subBoltNum"},
}

// armour and talismans, in the save's order.
var (
	armour    = [4]string{"equip_Helm", "equip_Armer", "equip_Gaunt", "equip_Leg"}
	talismans = [4]string{"equip_Accessory01", "equip_Accessory02", "equip_Accessory03", "equip_Accessory04"}
)

// Rows builds every row a summon of this character needs. A saved character
// drinks as a built one does: withNPCFlask.
func (l Loadout) Rows(slot int) []Row { return l.withNPCFlask().rows(RowOf(slot), l.faceRow()) }

// withNPCFlask is the loadout with the player's flasks of crimson and cerulean
// tears, which the AI never drinks, out of its quick slots, the other items
// moved up in order, and one NPC flask first, at the cast's limits - what every
// built character carries. A tenth item has no slot left.
func (l Loadout) withNPCFlask() Loadout {
	quick := []Item{{Goods: NPCFlask, Count: 1}}
	for _, item := range l.Quick {
		if item.Goods >= 0 && !playerFlask(item.Goods) {
			quick = append(quick, item)
		}
	}
	for index := range l.Quick {
		l.Quick[index] = Item{Goods: -1}
		if index < len(quick) {
			l.Quick[index] = quick[index]
		}
	}
	l.Flasks = [2]uint8{npcFlaskLimit, npcFlaskLimit}
	return l
}

// playerFlask is a Flask of Crimson or Cerulean Tears, full or empty, +0 to
// +12.
func playerFlask(goods int32) bool {
	return goods >= 1000 && goods <= 1025 || goods >= 1050 && goods <= 1075
}

// rows builds the rows at a character's id, with its FaceParam row.
func (l Loadout) rows(id int32, face []byte) []Row {
	init := make([]byte, charaInitSize)
	set := func(name string, value int64) { putField(init, charaInitFields[name], value) }
	setAt := func(name string, step int, value int64) {
		field := charaInitFields[name]
		field.offset += step * fieldWidth(field.kind)
		putField(init, field, value)
	}
	// Every pointer starts empty, as Paramdex defaults it: a row of our own
	// says nothing it was not told.
	for _, name := range []string{"equip_Wep_Right", "equip_Subwep_Right", "equip_Subwep_Right3",
		"equip_Wep_Left", "equip_Subwep_Left", "equip_Subwep_Left3", "equip_Helm", "equip_Armer", "equip_Gaunt",
		"equip_Leg", "equip_Arrow", "equip_Bolt", "equip_SubArrow", "equip_SubBolt", "equip_Accessory01",
		"equip_Accessory02", "equip_Accessory03", "equip_Accessory04", "elixir_material00",
		"elixir_material01", "elixir_material02"} {
		set(name, -1)
	}
	for step := range rowSpells {
		setAt("equip_Spell_01", step, -1)
	}
	for step := range quickSlots {
		setAt("item_01", step, -1)
	}
	for step := range pouchSlots {
		setAt("secondaryItem_01", step, -1)
	}
	for step := range gestureCount {
		setAt("gestureId0", step, -1)
	}

	rows := []Row{}
	for hand, fields := range hands {
		weapon := l.Equipment[hand]
		if weapon == empty {
			continue
		}
		if l.Ashes[hand] < 0 {
			set(fields[0], int64(weapon))
			continue
		}
		// An Ash of War of its own makes the weapon a custom weapon: the base
		// with its affinity, the art, and the level, which the plain id
		// carries in its last two digits.
		custom := make([]byte, customWeaponSize)
		binary.LittleEndian.PutUint32(custom[0:], weapon-weapon%100)
		binary.LittleEndian.PutUint32(custom[4:], uint32(l.Ashes[hand]))
		custom[8] = uint8(weapon % 100)
		rows = append(rows, Row{Table: TableCustomWeapon, ID: id + int32(hand), Data: custom})
		set(fields[0], int64(id)+int64(hand))
		set(fields[1], 1)
	}
	for index, fields := range ammo {
		if item := l.Equipment[slotArrow1+index]; item != empty {
			set(fields[0], int64(item))
			set(fields[1], int64(min(l.Ammo[index], math.MaxUint16)))
		}
	}
	for index, name := range armour {
		if item := l.Equipment[slotHead+index]; item != empty {
			set(name, int64(item&0x0FFFFFFF))
		}
	}
	for index, name := range talismans {
		if item := l.Equipment[slotTalis1+index]; item != empty {
			set(name, int64(item&0x0FFFFFFF))
		}
	}
	for index, tear := range l.Physick {
		setAt("elixir_material00", index, int64(tear))
	}
	step := 0
	for _, spell := range l.Spells {
		if spell >= 0 && step < rowSpells {
			setAt("equip_Spell_01", step, int64(CastAs(spell)))
			step++
		}
	}
	for index, item := range l.Quick {
		setAt("item_01", index, int64(item.Goods))
		setAt("itemNum_01", index, int64(min(item.Count, math.MaxUint8)))
	}
	for index, item := range l.Pouch {
		setAt("secondaryItem_01", index, int64(item.Goods))
		setAt("secondaryItemNum_01", index, int64(min(item.Count, math.MaxUint8)))
	}
	set("soulLv", int64(min(l.Level, math.MaxInt16)))
	for index, value := range l.Attributes {
		setAt("baseVit", index, int64(value))
	}
	set("npcPlayerSex", int64(l.Body))
	set("voiceType", int64(l.Voice))
	set("HpEstMax", int64(min(l.Flasks[0], math.MaxInt8)))
	set("MpEstMax", int64(min(l.Flasks[1], math.MaxInt8)))
	set("npcPlayerFaceGenId", int64(id))

	return append([]Row{
		{Table: TableCharaInit, ID: id, Data: init},
		{Table: TableFace, ID: id, Data: face},
	}, rows...)
}

// faceRuns carry the save's FACE record onto FaceParam: where each run lands
// in the row, where it starts in the record, and how long it is. Research
// appearance/docs/face-data.md (The record onto FaceParam) has the evidence:
// five of the baseline's eight faces convert to a shipped row byte for byte.
var faceRuns = [][3]int{
	{1, 0xB3, 30},    // skin, the face's cosmetics, lips
	{90, 0xD1, 9},    // the tattoo's place, angle, scale, colour, gloss, mirror
	{31, 0xDA, 4},    // body hair and its colour
	{36, 0xDE, 24},   // the right eye, then the left
	{61, 0xF6, 6},    // hair colour, shine, roots, white
	{68, 0xFC, 6},    // the beard's
	{75, 0x102, 6},   // the eyebrows'
	{82, 0x108, 3},   // eyelash colour
	{86, 0x10B, 3},   // eyepatch colour
	{99, 0xAC, 7},    // the body scales
	{113, 0x2C, 127}, // age, gender, caricature, and the FaceGen data
}

// faceParts are where the record's eight selections land, in its order: face,
// hair, eyes, eyebrows, beard, eyepatch, tattoo, eyelashes.
var faceParts = [8]int{0, 60, 35, 74, 67, 85, 89, 81}

// faceRow is the character's FaceParam row. The burn scars and the eye
// override flags (bytes 106 and 107) stay zero, the creator's own value.
func (l Loadout) faceRow() []byte {
	row := make([]byte, faceSize)
	for _, run := range faceRuns {
		copy(row[run[0]:run[0]+run[2]], l.Face[run[1]:run[1]+run[2]])
	}
	for index, at := range faceParts {
		row[at] = l.Face[0x0C+4*index]
	}
	return row
}

func fieldWidth(kind string) int {
	switch kind {
	case "s32":
		return 4
	case "s16", "u16":
		return 2
	}
	return 1
}

// getField is putField's inverse.
func getField(row []byte, field charaInitField) int64 {
	switch field.kind {
	case "s32":
		return int64(int32(binary.LittleEndian.Uint32(row[field.offset:])))
	case "s16":
		return int64(int16(binary.LittleEndian.Uint16(row[field.offset:])))
	case "u16":
		return int64(binary.LittleEndian.Uint16(row[field.offset:]))
	case "s8":
		return int64(int8(row[field.offset]))
	}
	return int64(row[field.offset])
}

func putField(row []byte, field charaInitField, value int64) {
	switch fieldWidth(field.kind) {
	case 4:
		binary.LittleEndian.PutUint32(row[field.offset:], uint32(int32(value)))
	case 2:
		binary.LittleEndian.PutUint16(row[field.offset:], uint16(value))
	default:
		row[field.offset] = byte(value)
	}
}
