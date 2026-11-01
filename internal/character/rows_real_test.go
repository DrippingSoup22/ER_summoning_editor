package character

// A slot's character read whole from the baseline save, and the rows built
// from it checked against rows the game ships.

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

func baseline(t *testing.T) (file []byte, bnd4 []byte) {
	t.Helper()
	path := os.Getenv("ER_SAVE")
	if path == "" {
		t.Skip("ER_SAVE not set")
	}
	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := regulation.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return file, loaded.BND4
}

func shippedRow(t *testing.T, bnd4 []byte, table string, id int32) []byte {
	t.Helper()
	opened, err := regulation.OpenTable(bnd4, regulation.MustLayout(table))
	if err != nil {
		t.Fatal(err)
	}
	row, ok := opened.ByID[id]
	if !ok {
		t.Fatalf("%s row %d is not in the baseline", table, id)
	}
	return opened.Data[row.DataOffset : row.DataOffset+opened.Layout.RowSize]
}

// Slot 0 is a level 146 Wretch with a Heavy Star Fist +24 carrying Endure,
// Great Arrows x30 and Fan Daggers x40 in its first quick slot - what the
// author's character holds.
func TestASlotReadsAsItsCharacterWears(t *testing.T) {
	file, _ := baseline(t)
	loadout, err := ReadLoadout(file, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loadout.Level != 146 || loadout.Attributes != [8]uint8{60, 10, 40, 70, 15, 10, 10, 10} {
		t.Errorf("level %d attributes %v", loadout.Level, loadout.Attributes)
	}
	if loadout.Equipment[1] != 21080124 || loadout.Ashes[1] < 0 {
		t.Errorf("right hand 1 is %d with Ash of War %d, want 21080124 with its own", loadout.Equipment[1], loadout.Ashes[1])
	}
	if loadout.Ammo[0] != 30 || loadout.Quick[0] != (Item{Goods: 1750, Count: 40}) {
		t.Errorf("arrows x%d, first quick item %+v", loadout.Ammo[0], loadout.Quick[0])
	}
	if loadout.HP != 1900 || len(loadout.Effects) != 0 {
		t.Errorf("health %d, effects %v: want 1900 and none", loadout.HP, loadout.Effects)
	}
	if _, err := ReadLoadout(file, 8); err == nil {
		t.Error("an empty slot read as a character")
	}
}

// What a character has active ships as effects: a saved effect (Lamenter's
// Mask, in its save category's record) and an active Great Rune (Radahn's,
// whose effect is 610), written here into a copy of slot 2.
func TestSavedEffectsAndAnActiveGreatRuneAreEffects(t *testing.T) {
	file, _ := baseline(t)
	file = bytes.Clone(file)
	start := regulation.SlotOffset(regulation.PS4, 2)
	slot := file[start : start+regulation.SlotSize]
	magic, _ := findMagic(slot)
	binary.LittleEndian.PutUint32(slot[magic+1+5*savedEffectRecord:], 19982)
	slot[magic+offGreatRune] = 1
	loadout, err := ReadLoadout(file, 2)
	if err != nil {
		t.Fatal(err)
	}
	projectiles := magic + dynSpEffect + dynEquipedItemIndex + dynActiveEquipedItems + dynEquipedItemsID +
		dynActiveEquipedItemsGa + dynInventoryHeld + dynEquipedSpells + dynEquipedItems + dynEquipedGestures
	armaments := projectiles + 4 + int(readU32(slot, projectiles))*8
	binary.LittleEndian.PutUint32(slot[armaments+4*slotGreatRune:], 0x40000000|192)
	if loadout, err = ReadLoadout(file, 2); err != nil {
		t.Fatal(err)
	}
	if len(loadout.Effects) != 2 || loadout.Effects[0] != 19982 || loadout.Effects[1] != 610 {
		t.Errorf("effects %v, want Lamenter's Mask and Radahn's rune", loadout.Effects)
	}
}

// Slot 3 wears the creator's Seafarer Male face untouched, so its row must be
// FaceParam 1005 byte for byte: every run of the conversion is proved by it.
func TestAnUntouchedTemplateFaceBecomesItsShippedRow(t *testing.T) {
	file, bnd4 := baseline(t)
	loadout, err := ReadLoadout(file, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := loadout.faceRow(), shippedRow(t, bnd4, TableFace, 1005); !bytes.Equal(got, want) {
		t.Errorf("slot 3's face row differs from FaceParam 1005:\n got %x\nwant %x", got, want)
	}
}

// The CharaInitParam offsets read the values research decoded from two
// shipped rows: the Vagabond's class row and Vyke's, which together reach
// every kind of field a character row writes.
func TestTheCharacterRowOffsetsReadShippedRows(t *testing.T) {
	_, bnd4 := baseline(t)
	for id, want := range map[int32]map[string]int64{
		3000: {"equip_Wep_Right": 2000000, "equip_Subwep_Right": 18000000, "equip_Wep_Left": 31330000,
			"equip_Helm": 660000, "equip_Leg": 660300, "soulLv": 9, "baseVit": 15, "npcPlayerSex": 1,
			"secondaryItem_01": 115, "secondaryItemNum_01": 1, "HpEstMax": 3, "MpEstMax": 1, "gestureId0": 0},
		23040: {"wepParamType_Right1": 1, "wepParamType_Left1": 1, "npcPlayerFaceGenId": 23040,
			"voiceType": 118, "item_01": 50203, "itemNum_01": 1, "equip_Spell_01": 53040,
			"equip_Accessory01": 4003, "equip_Accessory02": 4052, "equip_Armer": 950109},
	} {
		row := shippedRow(t, bnd4, TableCharaInit, id)
		for name, value := range want {
			if got := getField(row, charaInitFields[name]); got != value {
				t.Errorf("CharaInitParam %d.%s reads %d, want %d", id, name, got, value)
			}
		}
	}
}

// A saved character's flasks give way to the one NPC flask a built character
// carries; its other quick items keep their order.
func TestASavedCharacterDrinksTheNPCFlask(t *testing.T) {
	var loadout Loadout
	for index := range loadout.Quick {
		loadout.Quick[index] = Item{Goods: -1}
	}
	loadout.Quick[0] = Item{Goods: 1025, Count: 12}
	loadout.Quick[2] = Item{Goods: 1750, Count: 40}
	loadout.Quick[3] = Item{Goods: 1050, Count: 2}
	loadout.Quick[5] = Item{Goods: 300, Count: 5}
	loadout.Flasks = [2]uint8{12, 2}
	init := loadout.Rows(2)[0].Data
	quick := func(step int) (int64, int64) {
		goods, count := charaInitFields["item_01"], charaInitFields["itemNum_01"]
		goods.offset += step * fieldWidth(goods.kind)
		count.offset += step * fieldWidth(count.kind)
		return getField(init, goods), getField(init, count)
	}
	for step, want := range [][2]int64{{NPCFlask, 1}, {1750, 40}, {300, 5}, {-1, 0}} {
		if goods, count := quick(step); goods != want[0] || count != want[1] {
			t.Errorf("quick slot %d holds %d x%d, want %d x%d", step+1, goods, count, want[0], want[1])
		}
	}
	if getField(init, charaInitFields["HpEstMax"]) != npcFlaskLimit {
		t.Error("the flask limit is not the cast's")
	}
}

// A weapon carrying an Ash of War of its own is built as a custom weapon of
// the base, the art and the level, and the hand names it.
func TestAWeaponWithItsOwnAshBecomesACustomWeapon(t *testing.T) {
	var loadout Loadout
	for index := range loadout.Equipment {
		loadout.Equipment[index] = empty
	}
	for hand := range loadout.Ashes {
		loadout.Ashes[hand] = -1
	}
	loadout.Equipment[1], loadout.Ashes[1] = 21080124, 10500
	loadout.Equipment[0] = 34040024
	rows := loadout.Rows(2)
	init, custom := rows[0].Data, rows[len(rows)-1]
	if custom.Table != TableCustomWeapon || custom.ID != RowOf(2)+1 ||
		binary.LittleEndian.Uint32(custom.Data[0:]) != 21080100 ||
		binary.LittleEndian.Uint32(custom.Data[4:]) != 10500 || custom.Data[8] != 24 {
		t.Fatalf("custom weapon %+v", custom)
	}
	if getField(init, charaInitFields["equip_Wep_Right"]) != int64(RowOf(2)+1) ||
		getField(init, charaInitFields["wepParamType_Right1"]) != 1 ||
		getField(init, charaInitFields["equip_Wep_Left"]) != 34040024 ||
		getField(init, charaInitFields["wepParamType_Left1"]) != 0 {
		t.Error("the hands do not name what they carry")
	}
}
