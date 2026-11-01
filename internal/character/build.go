package character

// A character made from choices instead of read from a slot: the builder the
// panel's character creator will drive, and the test characters a console
// batch seats.
//
// A built character is the same rows as a slot's (rows.go) - a CharaInitParam
// row, a FaceParam row and a custom weapon per own Ash of War - and the same
// body, with what the save would hold computed instead: the health from Vigor
// on the game's own curve, the level from the attributes as the game counts
// it, and the face copied from one of the character creator's base templates.
// Every row a choice names is checked against the regulation, so a build the
// game cannot equip is refused instead of written.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// The built characters' rows sit in the hundred ids above the slots':
// 2990100 + 10 x n in the three character tables, and 199900100 + 10 x n for
// the body, n from 0 to Builds - 1. Free in every supported regulation like
// the slots' (checked 2026-10-25 on 11701000, 11711000 and 11721000).
const Builds = 10

// BuiltRowOf is the CharaInitParam and FaceParam row of built character n.
func BuiltRowOf(n int) int32 { return RowFloor + rowSpan + rowStride*int32(n) }

// BuiltBodyOf is the NpcParam row of built character n's body.
func BuiltBodyOf(n int) int32 { return BodyFloor + rowSpan + rowStride*int32(n) }

// BuildOf is the built character a row was made for, and whether the row is one.
func BuildOf(row int32) (int, bool) {
	first := BuiltRowOf(0)
	if row < first || row >= first+rowSpan || (row-first)%rowStride != 0 {
		return 0, false
	}
	return int((row - first) / rowStride), true
}

// The character creator's base templates, FaceParam rows: Warrior,
// Truth-Seeker, Aristocrat, Loner, Northerner, Seafarer, Reedlander,
// Draconian, Nightfolk and Numen, from 1000 for body type A and from 2000 for
// body type B. The Warrior is the plain preset.
const (
	TemplateA = 1000
	TemplateB = 2000
	templates = 10
)

// TemplateNames are the base templates' names, in their order from TemplateA
// and from TemplateB.
var TemplateNames = [templates]string{"Warrior", "Truth-Seeker", "Aristocrat", "Loner", "Northerner",
	"Seafarer", "Reedlander", "Draconian", "Nightfolk", "Numen"}

// Affinities are the weapon affinities in the order the regulation numbers
// them: a weapon's row is its Standard row + 100 x affinity (2000100 is the
// Heavy Longsword).
var Affinities = [...]string{"Standard", "Heavy", "Keen", "Quality", "Fire", "Flame Art",
	"Lightning", "Sacred", "Magic", "Cold", "Poison", "Blood", "Occult"}

// Weapon is one hand's choice. A zero Base is an empty hand.
type Weapon struct {
	// Base is the weapon's Standard row at +0, such as 2000000 the Longsword.
	Base     int32
	Affinity int
	Level    int
	// Ash is an Ash of War of its own, an EquipParamGem row; zero fights with
	// the weapon's own art.
	Ash int32
}

// Stack is an item and how many of it a character carries. A zero Row is an
// empty slot.
type Stack struct {
	Row   int32
	Count int
}

// MaxCarried is the most a build carries of one item or one kind of arrow.
const MaxCarried = 99

// Every built character carries one drink, as the game's own people do: NPC
// Flask of Crimson Tears 50203, which heals half the health, in the first
// quick slot - the flask the AI's item judgement knows; the player's flasks
// carry none. The flask limits are the cast's, 3 and 3.
const (
	NPCFlask      = 50203
	npcFlaskLimit = 3
)

// BuildQuickSlots is how many quick items a build chooses: the row's ten,
// less the first, the flask's.
const BuildQuickSlots = quickSlots - 1

// A name is at most MaxNameLength characters, the game's own limit for a
// player's, in nameSize bytes of UTF-8 at nameOffset: CharaInitParam.pad2,
// zero on every shipped row. A shorter name ends at a zero.
// pad2's last byte, classOffset, holds the starting class + 1: zero on a build
// written before there was a class to choose.
const (
	MaxNameLength = 16
	nameOffset    = 238
	nameSize      = 25
	classOffset   = nameOffset + nameSize
)

// The starting classes are CharaInitParam rows 3000 to 3009, in ClassNames'
// order (band class). The attributes a class
// starts with are the least a build of that class has (Choices.Classes).
const classRow = 3000

// Build is one character's choices.
type Build struct {
	// Name is what the player calls the character, empty for none. It is kept
	// in the row (nameOffset) for the editor to show; the game never does.
	Name string
	// Template is the base template the face is copied from; the body type
	// follows it.
	Template int32
	// Class is the starting class, an index into ClassNames, the Vagabond
	// the zero. The builder holds the attributes to its own; the row keeps
	// it for the editor (classOffset), and the game never reads it.
	Class int
	Voice uint8
	// Attributes are vigor, mind, endurance, strength, dexterity,
	// intelligence, faith and arcane, each 1 to 99.
	Attributes [8]uint8
	// Hands are in the save's order: L1 R1 L2 R2 L3 R3.
	Hands [handSlots]Weapon
	// Armour is head, chest, arms and legs, EquipParamProtector rows; zero is
	// bare.
	Armour [4]int32
	// Talismans are EquipParamAccessory rows; zero is an empty slot.
	Talismans [4]int32
	// Ammo is arrow 1, bolt 1, arrow 2 and bolt 2, EquipParamWeapon rows.
	Ammo [4]Stack
	// Spells are Magic rows, packed from the first slot as the row holds
	// them - seven, all CharaInitParam has (equip_Spell_01 to 07); zero is
	// empty.
	Spells [rowSpells]int32
	// Quick are the quick items after the flask, EquipParamGoods rows. The
	// pouch (secondaryItem) is left empty: the AI
	// uses the quick items and not the pouch.
	Quick [BuildQuickSlots]Stack
}

// The hand slots a build names most.
const (
	LeftHand  = 0
	RightHand = 1
)

// Plain is the plain character: the Warrior template of either body type,
// every attribute at 1, nothing equipped.
func Plain(typeA bool) Build {
	build := Build{Template: TemplateB}
	if typeA {
		build.Template = TemplateA
	}
	for index := range build.Attributes {
		build.Attributes[index] = 1
	}
	return build
}

// Level is the character's level as the game counts it - the attributes'
// sum less 79, so the Wretch's ten tens are level 1 - and 1 at the least.
func (b Build) Level() uint32 {
	sum := 0
	for _, value := range b.Attributes {
		sum += int(value)
	}
	return uint32(max(1, sum-79))
}

// HP is the character's health: Vigor on the game's curve.
func (b Build) HP() int32 { return VigorHealth(b.Attributes[0]) }

// vigorCurve is CalcCorrectGraphParam row 100, "HP Scaling - Vigor": health
// at five Vigor points, and the exponent each stretch between two of them
// grows by. A positive exponent rises slowly first, a negative one fast first
// (one less the power of what is left). The game truncates: every character in
// the baseline save holds the health this gives (build_real_test.go).
var vigorCurve = struct {
	points, values [5]float32
	exponents      [4]float32
}{
	points:    [5]float32{1, 25, 40, 60, 99},
	values:    [5]float32{300, 800, 1450, 1900, 2100},
	exponents: [4]float32{1.5, 1.1, -1.2, -1.2},
}

// VigorHealth is the health a character with this much Vigor has.
func VigorHealth(vigor uint8) int32 {
	curve := vigorCurve
	x := min(max(float32(vigor), curve.points[0]), curve.points[4])
	stretch := 0
	for stretch < 3 && x > curve.points[stretch+1] {
		stretch++
	}
	ratio := float64((x - curve.points[stretch]) / (curve.points[stretch+1] - curve.points[stretch]))
	exponent := float64(curve.exponents[stretch])
	growth := math.Pow(ratio, exponent)
	if exponent < 0 {
		growth = 1 - math.Pow(1-ratio, -exponent)
	}
	low, high := float64(curve.values[stretch]), float64(curve.values[stretch+1])
	return int32(low + (high-low)*growth)
}

// ErrBadBuild is a choice the game cannot equip or the rows cannot hold.
var ErrBadBuild = errors.New("not a character the game can build")

// The fields a build is checked by, at Paramdex's offsets.
const (
	weaponReinforceType = 218 // EquipParamWeapon.reinforceTypeId, s16
	weaponType          = 422 // EquipParamWeapon.wepType, u16
	protectorCategory   = 214 // EquipParamProtector.protectorCategory: head, chest, arms, legs
)

// Rows checks the build against a regulation and returns every row built
// character n needs, as a slot's Rows does, and the health its body carries.
func (b Build) Rows(n int, bnd4 []byte) ([]Row, int32, error) {
	if n < 0 || n >= Builds {
		return nil, 0, fmt.Errorf("built character %d: there are %d: %w", n, Builds, ErrBadBuild)
	}
	face, err := b.check(bnd4)
	if err != nil {
		return nil, 0, fmt.Errorf("built character %d: %w", n, err)
	}
	loadout := Loadout{Level: b.Level(), Attributes: b.Attributes, Voice: b.Voice, HP: uint32(b.HP()),
		Flasks: [2]uint8{npcFlaskLimit, npcFlaskLimit}}
	if b.Template < TemplateB {
		loadout.Body = 1
	}
	for slot := range loadout.Equipment {
		loadout.Equipment[slot] = empty
	}
	for hand, weapon := range b.Hands {
		loadout.Ashes[hand] = -1
		if weapon.Base != 0 {
			loadout.Equipment[hand] = uint32(weapon.Base) + 100*uint32(weapon.Affinity) + uint32(weapon.Level)
			if weapon.Ash != 0 {
				loadout.Ashes[hand] = weapon.Ash
			}
		}
	}
	for index, piece := range b.Armour {
		if piece != 0 {
			loadout.Equipment[slotHead+index] = uint32(piece)
		}
	}
	for index, talisman := range b.Talismans {
		if talisman != 0 {
			loadout.Equipment[slotTalis1+index] = uint32(talisman)
		}
	}
	for index, stack := range b.Ammo {
		if stack.Row != 0 {
			loadout.Equipment[slotArrow1+index] = uint32(stack.Row)
			loadout.Ammo[index] = uint32(stack.Count)
		}
	}
	for index := range loadout.Spells {
		loadout.Spells[index] = -1
		if index < rowSpells && b.Spells[index] != 0 {
			loadout.Spells[index] = b.Spells[index]
		}
	}
	goods := func(stack Stack) Item {
		if stack.Row == 0 {
			return Item{Goods: -1}
		}
		return Item{Goods: stack.Row, Count: uint32(stack.Count)}
	}
	loadout.Quick[0] = Item{Goods: NPCFlask, Count: 1}
	for index, stack := range b.Quick {
		loadout.Quick[index+1] = goods(stack)
	}
	for index := range loadout.Pouch {
		loadout.Pouch[index] = Item{Goods: -1}
	}
	loadout.Physick = [2]int32{-1, -1}
	rows := loadout.rows(BuiltRowOf(n), face)
	copy(rows[0].Data[nameOffset:nameOffset+nameSize], b.Name)
	rows[0].Data[classOffset] = byte(b.Class + 1)
	return rows, int32(loadout.HP), nil
}

// checkName refuses a name the row cannot hold or the editor could not show
// on one line: past MaxNameLength characters or nameSize bytes, not UTF-8, a
// character that does not print, or space at either end.
func checkName(name string) error {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxNameLength || len(name) > nameSize ||
		strings.TrimSpace(name) != name || strings.ContainsFunc(name, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return fmt.Errorf("the name %q is not one line of at most %d characters and %d bytes: %w",
			name, MaxNameLength, nameSize, ErrBadBuild)
	}
	return nil
}

// check refuses what the game cannot equip and returns the template's face.
func (b Build) check(bnd4 []byte) ([]byte, error) {
	if b.Class < 0 || b.Class >= len(ClassNames) {
		return nil, fmt.Errorf("class %d is not a starting class: %w", b.Class, ErrBadBuild)
	}
	if b.Template < TemplateA || b.Template >= TemplateA+templates && b.Template < TemplateB ||
		b.Template >= TemplateB+templates {
		return nil, fmt.Errorf("face %d is not a base template: %w", b.Template, ErrBadBuild)
	}
	faces, err := regulation.OpenTable(bnd4, regulation.MustLayout(TableFace))
	if err != nil {
		return nil, err
	}
	face, ok := faces.ByID[b.Template]
	if !ok {
		return nil, fmt.Errorf("the regulation has no face %d: %w", b.Template, ErrBadBuild)
	}
	for index, value := range b.Attributes {
		if value < 1 || value > 99 {
			return nil, fmt.Errorf("attribute %d is %d, outside 1 to 99: %w", index, value, ErrBadBuild)
		}
	}
	if err := checkName(b.Name); err != nil {
		return nil, err
	}
	if err := b.checkHands(bnd4); err != nil {
		return nil, err
	}
	if err := b.checkCarried(bnd4); err != nil {
		return nil, err
	}
	protectors, err := regulation.OpenTable(bnd4, regulation.MustLayout("EquipParamProtector"))
	if err != nil {
		return nil, err
	}
	for index, piece := range b.Armour {
		if piece == 0 {
			continue
		}
		row, ok := protectors.ByID[piece]
		if !ok || int(protectors.Data[row.DataOffset+protectorCategory]) != index {
			return nil, fmt.Errorf("armour %d is not a piece for slot %d: %w", piece, index, ErrBadBuild)
		}
	}
	accessories, err := regulation.OpenTable(bnd4, regulation.MustLayout("EquipParamAccessory"))
	if err != nil {
		return nil, err
	}
	for _, talisman := range b.Talismans {
		if _, ok := accessories.ByID[talisman]; talisman != 0 && !ok {
			return nil, fmt.Errorf("talisman %d is not in the regulation: %w", talisman, ErrBadBuild)
		}
	}
	return faces.Data[face.DataOffset : face.DataOffset+faceSize], nil
}

// checkHands refuses a weapon the regulation has no row for at that affinity,
// a level past the top of its upgrade ladder - +25, or +10 for a somber
// weapon, read from ReinforceParamWeapon as the run of rows from its type - an
// Ash of War that does not go on the weapon, and an affinity its skill does not
// allow (ashes.go).
func (b Build) checkHands(bnd4 []byte) error {
	weapons, err := regulation.OpenTable(bnd4, regulation.MustLayout("EquipParamWeapon"))
	if err != nil {
		return err
	}
	_, ladders, err := regulation.LoadRowsByID(bnd4, "ReinforceParamWeapon.param", "", 0)
	if err != nil {
		return err
	}
	choices, err := ReadChoices(bnd4)
	if err != nil {
		return err
	}
	for hand, weapon := range b.Hands {
		if weapon.Base == 0 {
			continue
		}
		if weapon.Base < 0 || weapon.Base%10000 != 0 || weapon.Affinity < 0 || weapon.Affinity >= len(Affinities) {
			return fmt.Errorf("hand %d: %d at affinity %d is not a weapon choice: %w", hand, weapon.Base, weapon.Affinity, ErrBadBuild)
		}
		id := weapon.Base + 100*int32(weapon.Affinity)
		row, ok := weapons.ByID[id]
		if !ok {
			return fmt.Errorf("hand %d: weapon %d has no %s row: %w", hand, weapon.Base, Affinities[weapon.Affinity], ErrBadBuild)
		}
		top := ladderTop(weapons, ladders, row)
		if weapon.Level < 0 || weapon.Level > top {
			return fmt.Errorf("hand %d: weapon %d goes to +%d, not +%d: %w", hand, id, top, weapon.Level, ErrBadBuild)
		}
		if weapon.Ash != 0 && !choices.Fits(weapon.Base, weapon.Ash) {
			return fmt.Errorf("hand %d: Ash of War %d does not go on weapon %d: %w", hand, weapon.Ash, weapon.Base, ErrBadBuild)
		}
		if !choices.Offers(weapon.Base, weapon.Ash)[weapon.Affinity] {
			return fmt.Errorf("hand %d: weapon %d cannot be %s with that skill: %w", hand, weapon.Base,
				Affinities[weapon.Affinity], ErrBadBuild)
		}
	}
	return nil
}

// The ammunition each ammo slot takes, by wepType (research character_row.py
// WEP_TYPE): arrows and greatarrows in the arrow slots, bolts and greatbolts in
// the bolt slots.
var (
	arrowTypes = map[uint16]bool{81: true, 83: true}
	boltTypes  = map[uint16]bool{85: true, 86: true}
)

// checkCarried refuses ammunition that is not the slot's kind, a spell or an
// item the regulation has no row for, a count outside 1 to MaxCarried, and
// more flasks than a player holds.
func (b Build) checkCarried(bnd4 []byte) error {
	weapons, err := regulation.OpenTable(bnd4, regulation.MustLayout("EquipParamWeapon"))
	if err != nil {
		return err
	}
	for index, stack := range b.Ammo {
		if stack.Row == 0 {
			continue
		}
		kinds := arrowTypes
		if index%2 == 1 {
			kinds = boltTypes
		}
		row, ok := weapons.ByID[stack.Row]
		if !ok || !kinds[binary.LittleEndian.Uint16(weapons.Data[row.DataOffset+weaponType:])] {
			return fmt.Errorf("ammunition slot %d cannot take %d: %w", index, stack.Row, ErrBadBuild)
		}
	}
	_, spells, err := regulation.LoadRowsByID(bnd4, "Magic.param", "", 0)
	if err != nil {
		return err
	}
	for _, spell := range b.Spells {
		if _, ok := spells[spell]; spell != 0 && !ok {
			return fmt.Errorf("%d is not a spell: %w", spell, ErrBadBuild)
		}
	}
	goods, err := regulation.OpenTable(bnd4, regulation.MustLayout("EquipParamGoods"))
	if err != nil {
		return err
	}
	for _, stack := range append(append([]Stack{}, b.Ammo[:]...), b.Quick[:]...) {
		if stack.Row != 0 && (stack.Count < 1 || stack.Count > MaxCarried) {
			return fmt.Errorf("%d of %d is not 1 to %d: %w", stack.Count, stack.Row, MaxCarried, ErrBadBuild)
		}
	}
	for _, stack := range b.Quick {
		if _, ok := goods.ByID[stack.Row]; stack.Row != 0 && !ok {
			return fmt.Errorf("%d is not an item: %w", stack.Row, ErrBadBuild)
		}
	}
	if _, ok := goods.ByID[NPCFlask]; !ok {
		return fmt.Errorf("the regulation has no NPC flask %d", NPCFlask)
	}
	return nil
}

// ladderTop is the highest level a weapon row upgrades to: the run of
// ReinforceParamWeapon rows from its reinforce type.
func ladderTop(weapons *regulation.Table, ladders map[int32]regulation.ParamRow, row regulation.ParamRow) int {
	ladder := int32(int16(binary.LittleEndian.Uint16(weapons.Data[row.DataOffset+weaponReinforceType:])))
	top := 0
	for top < 99 {
		if _, ok := ladders[ladder+int32(top)+1]; !ok {
			break
		}
		top++
	}
	return top
}
