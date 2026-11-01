package character

// A built character against the baseline's regulation: the health curve is the
// game's, the rows say what was chosen, and what the game cannot equip is
// refused.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// The curve is CalcCorrectGraphParam row 100, and every character in the
// baseline holds the health it gives for their Vigor.
func TestVigorHealthIsTheGamesCurve(t *testing.T) {
	file, bnd4 := baseline(t)
	data, rows, err := regulation.LoadRowsByID(bnd4, "CalcCorrectGraph.param", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	row := data[rows[100].DataOffset:]
	f32 := func(offset int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(row[offset:])) }
	for index := range 5 {
		if f32(4*index) != vigorCurve.points[index] || f32(20+4*index) != vigorCurve.values[index] ||
			index < 4 && f32(40+4*index) != vigorCurve.exponents[index] {
			t.Errorf("point %d differs from CalcCorrectGraph row 100", index)
		}
	}
	for slot := range 8 {
		loadout, err := ReadLoadout(file, slot)
		if err != nil {
			t.Fatal(err)
		}
		if got := VigorHealth(loadout.Attributes[0]); got != int32(loadout.HP) {
			t.Errorf("slot %d: Vigor %d gives %d, the save holds %d", slot, loadout.Attributes[0], got, loadout.HP)
		}
	}
}

func TestABuildIsItsChoicesAndRefusesWhatCannotBeEquipped(t *testing.T) {
	_, bnd4 := baseline(t)
	build := Plain(true)
	build.Attributes[3] = 99
	build.Hands[RightHand] = Weapon{Base: 2000000, Affinity: 1, Level: 25} // Heavy Longsword +25
	build.Armour[0] = 40000                                                // Iron Helmet
	rows, hp, err := build.Rows(2, bnd4)
	if err != nil {
		t.Fatal(err)
	}
	if hp != 300 || len(rows) != 2 || rows[0].ID != 2990120 || rows[1].ID != 2990120 {
		t.Fatalf("health %d, rows %v", hp, rows)
	}
	init := rows[0].Data
	field := func(name string) int64 {
		field := charaInitFields[name]
		switch fieldWidth(field.kind) {
		case 4:
			return int64(int32(binary.LittleEndian.Uint32(init[field.offset:])))
		case 2:
			return int64(int16(binary.LittleEndian.Uint16(init[field.offset:])))
		}
		return int64(init[field.offset])
	}
	if field("equip_Wep_Right") != 2000125 || field("equip_Wep_Left") != -1 || field("equip_Helm") != 40000 ||
		field("equip_Armer") != -1 || field("soulLv") != 27 || field("npcPlayerSex") != 1 ||
		field("npcPlayerFaceGenId") != 2990120 || !bytes.Equal(init[194:202], []byte{1, 1, 1, 99, 1, 1, 1, 1}) ||
		field("item_01") != NPCFlask || field("itemNum_01") != 1 || field("HpEstMax") != 3 || field("MpEstMax") != 3 {
		t.Errorf("the row does not say what was chosen")
	}
	if !bytes.Equal(rows[1].Data, shippedRow(t, bnd4, TableFace, TemplateA)) {
		t.Error("the face is not the template's")
	}

	// What the builder offers is what Rows takes: the Longsword's thirteen
	// affinities to +25, the somber 1010000 at Standard to +10 with no Ash of
	// War, and Thunderbolt on the Longsword at Lightning but not at Fire.
	choices, err := ReadChoices(bnd4)
	if err != nil {
		t.Fatal(err)
	}
	if long, somber := choices.Weapons[2000000], choices.Weapons[1010000]; long.MaxLevel != 25 || !long.Affinities[12] ||
		somber.MaxLevel != 10 || somber.Affinities[1] || !somber.Affinities[0] || somber.Mount >= 0 {
		t.Errorf("the Longsword offers %+v and the somber weapon %+v", long, somber)
	}
	// Each starting class is its row: the level the row states is the one
	// its attributes add up to, and the Wretch starts at ten in everything.
	inits, err := regulation.OpenTable(bnd4, regulation.MustLayout(TableCharaInit))
	if err != nil {
		t.Fatal(err)
	}
	for class, attributes := range choices.Classes {
		row := inits.Data[inits.ByID[classRow+int32(class)].DataOffset:]
		if level := (Build{Attributes: attributes}).Level(); getField(row, charaInitFields["soulLv"]) != int64(level) {
			t.Errorf("the %s starts at %v, level %d, and its row says %d", ClassNames[class], attributes, level,
				getField(row, charaInitFields["soulLv"]))
		}
	}
	if wretch := choices.Classes[len(ClassNames)-1]; wretch != [8]uint8{10, 10, 10, 10, 10, 10, 10, 10} {
		t.Errorf("the Wretch starts at %v", wretch)
	}
	if thunderbolt := choices.Offers(2000000, 21600); !choices.Fits(2000000, 21600) || !thunderbolt[6] || thunderbolt[4] ||
		choices.Fits(2000000, 10500) {
		t.Errorf("Thunderbolt on the Longsword offers %v; Charge Forth goes on it: %t", thunderbolt, choices.Fits(2000000, 10500))
	}

	// Batch 85's sorcerer, at Intelligence 20: the Glintstone Staff's 6 and 10
	// are met; Comet's 52 is not, but Comet is cast as its copy, which needs
	// nothing, and Stars of Ruin, which has none, needs 43.
	sorcerer := Plain(true)
	sorcerer.Attributes[Strength], sorcerer.Attributes[Intelligence] = 10, 20
	sorcerer.Hands[RightHand], sorcerer.Spells[0], sorcerer.Spells[1] = Weapon{Base: 33000000}, 4021, 4220
	if short := choices.Shortfalls(sorcerer); short.Count() != 1 ||
		len(short.Spells[1]) != 1 || short.Spells[1][0] != (Unmet{Attribute: Intelligence, Need: 43}) {
		t.Errorf("the staff falls short by %v and the spells by %v", short.Hands[RightHand], short.Spells)
	}

	refused := map[string]func(*Build){
		"past +25":                 func(b *Build) { b.Hands[RightHand].Level = 26 },
		"a somber weapon +11":      func(b *Build) { b.Hands[RightHand] = Weapon{Base: 1010000, Level: 11} },
		"an affinity it lacks":     func(b *Build) { b.Hands[RightHand] = Weapon{Base: 1010000, Affinity: 1} },
		"an Ash on a unique skill": func(b *Build) { b.Hands[RightHand] = Weapon{Base: 1010000, Ash: 10000} },
		"a spear's Ash":            func(b *Build) { b.Hands[RightHand] = Weapon{Base: 2000000, Ash: 10500} },
		"Fire with Thunderbolt":    func(b *Build) { b.Hands[RightHand] = Weapon{Base: 2000000, Affinity: 4, Ash: 21600} },
		"a name past sixteen":      func(b *Build) { b.Name = "Seventeen letters" },
		"a name past the row":      func(b *Build) { b.Name = strings.Repeat("火", 9) },
		"a name on two lines":      func(b *Build) { b.Name = "Two\nlines" },
		"a chest on the head":      func(b *Build) { b.Armour[0] = 40100 },
		"no template":              func(b *Build) { b.Template = 1010 },
		"attribute 0":              func(b *Build) { b.Attributes[0] = 0 },
		"a bolt for arrows":        func(b *Build) { b.Ammo[0] = Stack{Row: 52000000, Count: 10} },
		"no such spell":            func(b *Build) { b.Spells[0] = 9999999 },
		"no such item":             func(b *Build) { b.Quick[0] = Stack{Row: 299, Count: 1} },
		"none of an item":          func(b *Build) { b.Quick[1] = Stack{Row: 300} },
	}
	for name, change := range refused {
		changed := build
		change(&changed)
		if _, _, err := changed.Rows(2, bnd4); !errors.Is(err, ErrBadBuild) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Everything a build can choose comes back from its rows as it was chosen, so
// a character an earlier session wrote opens in the builder as it was made.
func TestABuildReadsBackFromItsRows(t *testing.T) {
	_, bnd4 := baseline(t)
	build := Plain(false)
	build.Name, build.Template, build.Voice, build.Class = "Ranni's Knight", TemplateB+7, 3, 7
	build.Attributes = [8]uint8{40, 20, 30, 50, 18, 9, 25, 10}
	build.Hands[RightHand] = Weapon{Base: 2000000, Affinity: 4, Level: 20, Ash: 10000}
	build.Hands[LeftHand] = Weapon{Base: 7140000, Affinity: 1, Level: 7}
	build.Hands[5] = Weapon{Base: 11000000}
	build.Armour = [4]int32{40000, 0, 40200, 0}
	build.Talismans = [4]int32{1001, 0, 0, 0}
	build.Ammo = [4]Stack{{Row: 50010000, Count: 30}, {}, {Row: 51000000, Count: 12}, {Row: 52000000, Count: 5}}
	build.Spells = [rowSpells]int32{4000, 6000, 6100}
	build.Quick[0], build.Quick[8] = Stack{Row: 300, Count: 10}, Stack{Row: 820, Count: 3}
	build.Quick[4] = Stack{Row: 830, Count: 99}
	rows, _, err := build.Rows(4, bnd4)
	if err != nil {
		t.Fatal(err)
	}
	held := map[string]map[int32][]byte{}
	for _, row := range rows {
		if held[row.Table] == nil {
			held[row.Table] = map[int32][]byte{}
		}
		held[row.Table][row.ID] = row.Data
	}
	// The row holds Catch Flame as its NPC copy, and the Glintstone Pebble,
	// which has none, as itself.
	spell := charaInitFields["equip_Spell_01"]
	init := held[TableCharaInit][BuiltRowOf(4)]
	if first, second := getField(init, spell), getField(init, charaInitField{spell.offset + 4, "s32"}); first != 4000 ||
		second != 53010 {
		t.Errorf("the row holds the spells %d and %d, want 4000 and Catch Flame's copy 53010", first, second)
	}
	faces, err := TemplateFaces(bnd4)
	if err != nil {
		t.Fatal(err)
	}
	find := func(table string, id int32) []byte { return held[table][id] }
	read, ok, err := ReadBuild(4, find, faces)
	if err != nil || !ok {
		t.Fatalf("reading the build back: %t, %v", ok, err)
	}
	if read != build {
		t.Errorf("read back\n%+v\nwant\n%+v", read, build)
	}
	if _, ok, _ := ReadBuild(5, find, faces); ok {
		t.Error("a build no row holds reads back")
	}
}
