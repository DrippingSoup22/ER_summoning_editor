package character

// What a weapon can be built as, and what an Ash of War goes on.
//
// A weapon takes an Ash of War when its EquipParamWeapon.gemMountType is 2, and
// an Ash goes on the weapon types whose EquipParamGem.canMountWep bit it sets.
// The Ash then decides the affinities: its own list, the physical four and its
// element's, so a lightning Ash allows Lightning and Sacred and no Fire. A
// weapon fighting with its own skill has that skill's list - the one every Ash
// carrying the same art allows - and every affinity where no Ash carries it.
// Standard is always allowed: it is the weapon as it ships.

import (
	"encoding/binary"
	"fmt"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// WeaponChoice is what a weapon can be built as: the affinities the
// regulation has a row for, the highest level its Standard row reaches, the
// canMountWep bit of its type - -1 for a weapon that takes no Ash of War - the
// affinities its own skill allows, and what each affinity's row requires.
type WeaponChoice struct {
	Affinities [len(Affinities)]bool
	MaxLevel   int
	Mount      int
	Skill      [len(Affinities)]bool
	Needs      [len(Affinities)]Needs
}

// AshChoice is what an Ash of War allows: the weapon types it goes on, as
// canMountWep bits, the affinities a weapon carrying it may have, and the one
// it comes with.
type AshChoice struct {
	Mounts     uint64
	Affinities [len(Affinities)]bool
	Default    int
}

// Choices are every weapon's choice, by its Standard row at +0, every Ash of
// War's, by its EquipParamGem row, what every spell requires, by its Magic
// row, and the attributes every starting class begins with, by ClassNames'
// index.
type Choices struct {
	Weapons map[int32]WeaponChoice
	Ashes   map[int32]AshChoice
	Spells  map[int32]Needs
	Classes [][8]uint8
}

// Fits reports whether an Ash of War goes on a weapon.
func (c Choices) Fits(base, ash int32) bool {
	weapon, isWeapon := c.Weapons[base]
	gem, isAsh := c.Ashes[ash]
	return isWeapon && isAsh && weapon.Mount >= 0 && gem.Mounts&(1<<weapon.Mount) != 0
}

// Offers is the affinities a weapon may be built at with an Ash of War, zero
// for its own skill: those it has a row for that the skill allows.
func (c Choices) Offers(base, ash int32) [len(Affinities)]bool {
	weapon := c.Weapons[base]
	allows := weapon.Skill
	if ash != 0 {
		allows = c.Ashes[ash].Affinities
	}
	var out [len(Affinities)]bool
	for affinity := range out {
		out[affinity] = weapon.Affinities[affinity] && allows[affinity]
	}
	return out
}

// mountBits are each weapon type's canMountWep bit, by EquipParamWeapon.wepType:
// Paramdex's names for the base game's types, and the DLC's eight on the bits
// it calls reserved.
var mountBits = map[uint16]int{
	1: 0, 3: 1, 5: 2, 7: 3, 9: 4, 11: 5, 13: 6, 14: 7, 15: 8, 16: 9, 17: 10, 19: 11, 21: 12, 23: 13,
	24: 14, 25: 15, 28: 17, 29: 18, 31: 19, 35: 20, 37: 21, 39: 22, 41: 23, 50: 24, 51: 25, 53: 26,
	55: 27, 56: 28, 57: 29, 61: 31, 65: 32, 67: 33, 69: 34, 87: 35,
	88: 36, 89: 37, 90: 38, 91: 39, 92: 40, 93: 41, 94: 42, 95: 43,
}

// The fields the choices are read from, at Paramdex's offsets.
const (
	weaponArt      = 408 // EquipParamWeapon.swordArtsParamId, s32
	weaponGemMount = 577 // EquipParamWeapon.gemMountType, u8: 2 takes an Ash of War
	gemArt         = 24  // EquipParamGem.swordArtsParamId, s32
	// configurableWepAttr00-15 are the bits of the two bytes at 48, and 16-23
	// those of the byte at 51; bit n is affinity n.
	gemAffinities      = 48
	gemDefaultAffinity = 53 // EquipParamGem.defaultWepAttr, u8
	gemMounts          = 56 // EquipParamGem.canMountWep, 64 bits
	// properStrength, properAgility, properMagic and properFaith, a byte each,
	// then properLuck far after them.
	weaponNeeds  = 242
	weaponArcane = 405
	// Magic.requirementIntellect then requirementFaith, and requirementLuck.
	spellIntelligence = 34
	spellArcane       = 14
)

// ReadChoices reads every weapon's and Ash of War's choice from a regulation.
func ReadChoices(bnd4 []byte) (Choices, error) {
	weapons, err := regulation.OpenTable(bnd4, regulation.MustLayout("EquipParamWeapon"))
	if err != nil {
		return Choices{}, err
	}
	_, ladders, err := regulation.LoadRowsByID(bnd4, "ReinforceParamWeapon.param", "", 0)
	if err != nil {
		return Choices{}, err
	}
	gemData, gems, err := regulation.LoadRowsByID(bnd4, "EquipParamGem.param", "", 0)
	if err != nil {
		return Choices{}, err
	}
	spellData, spells, err := regulation.LoadRowsByID(bnd4, "Magic.param", "", 0)
	if err != nil {
		return Choices{}, err
	}
	out := Choices{Weapons: map[int32]WeaponChoice{}, Ashes: make(map[int32]AshChoice, len(gems)),
		Spells: make(map[int32]Needs, len(spells))}
	for id, row := range spells {
		spell := spellData[row.DataOffset:]
		var needs Needs
		needs[Intelligence], needs[Faith] = spell[spellIntelligence], spell[spellIntelligence+1]
		needs[Arcane] = spell[spellArcane]
		out.Spells[id] = needs
	}
	// A spell needs what its copy needs, since the row holds the copy.
	for spell := range out.Spells {
		out.Spells[spell] = out.Spells[CastAs(spell)]
	}
	// skills are the affinities each art allows: what every Ash carrying it does.
	skills := map[int32][len(Affinities)]bool{}
	for id, row := range gems {
		gem := gemData[row.DataOffset:]
		listed := uint32(gem[gemAffinities]) | uint32(gem[gemAffinities+1])<<8 | uint32(gem[gemAffinities+3])<<16
		ash := AshChoice{Mounts: binary.LittleEndian.Uint64(gem[gemMounts:]), Default: int(gem[gemDefaultAffinity])}
		for affinity := range ash.Affinities {
			ash.Affinities[affinity] = listed&(1<<affinity) != 0
		}
		out.Ashes[id] = ash
		art := int32(binary.LittleEndian.Uint32(gem[gemArt:]))
		allowed, seen := skills[art]
		for affinity := range allowed {
			allowed[affinity] = ash.Affinities[affinity] && (!seen || allowed[affinity])
		}
		skills[art] = allowed
	}
	for _, row := range weapons.Rows {
		affinity := int(row.ID % 10000 / 100)
		if row.ID <= 0 || row.ID%100 != 0 || affinity >= len(Affinities) {
			continue
		}
		base := row.ID - row.ID%10000
		choice, seen := out.Weapons[base]
		if !seen {
			choice.Mount = -1
		}
		choice.Affinities[affinity] = true
		weapon := weapons.Data[row.DataOffset:]
		copy(choice.Needs[affinity][Strength:Faith+1], weapon[weaponNeeds:weaponNeeds+4])
		choice.Needs[affinity][Arcane] = weapon[weaponArcane]
		if affinity == 0 {
			choice.MaxLevel = ladderTop(weapons, ladders, row)
			if bit, known := mountBits[binary.LittleEndian.Uint16(weapon[weaponType:])]; known && weapon[weaponGemMount] == 2 {
				choice.Mount = bit
			}
			skill, carried := skills[int32(binary.LittleEndian.Uint32(weapon[weaponArt:]))]
			for index := range choice.Skill {
				choice.Skill[index] = !carried || skill[index]
			}
			choice.Skill[0] = true
		}
		out.Weapons[base] = choice
	}
	inits, err := regulation.OpenTable(bnd4, regulation.MustLayout(TableCharaInit))
	if err != nil {
		return Choices{}, err
	}
	out.Classes = make([][8]uint8, len(ClassNames))
	for class := range out.Classes {
		row, ok := inits.ByID[classRow+int32(class)]
		if !ok {
			return Choices{}, fmt.Errorf("the regulation has no class row %d", classRow+class)
		}
		at := row.DataOffset + charaInitFields["baseVit"].offset
		copy(out.Classes[class][:], inits.Data[at:at+8])
	}
	return out, nil
}
