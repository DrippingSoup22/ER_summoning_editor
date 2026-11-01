package character

// A built character read back from the rows a save holds, so the builder can
// open what an earlier session wrote: Rows' inverse. The name is read from its
// bytes, the face is matched to the base template it was copied from, a custom
// weapon is read back into its base, affinity, level and Ash of War, and
// everything the build computes - the level and the health - is left to the
// build to compute again.

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// TemplateFaces are the base templates' FaceParam rows, by template, which a
// built character's face is a copy of.
func TemplateFaces(bnd4 []byte) (map[int32][]byte, error) {
	faces, err := regulation.OpenTable(bnd4, regulation.MustLayout(TableFace))
	if err != nil {
		return nil, err
	}
	out := make(map[int32][]byte, 2*templates)
	for _, first := range []int32{TemplateA, TemplateB} {
		for template := first; template < first+templates; template++ {
			if row, ok := faces.ByID[template]; ok {
				out[template] = bytes.Clone(faces.Data[row.DataOffset : row.DataOffset+faceSize])
			}
		}
	}
	return out, nil
}

// ReadBuild reads built character n back. find returns a row the save holds,
// nil where it holds none; faces are TemplateFaces. False when the save holds
// no built character n.
func ReadBuild(n int, find func(table string, id int32) []byte, faces map[int32][]byte) (Build, bool, error) {
	id := BuiltRowOf(n)
	init := find(TableCharaInit, id)
	if init == nil {
		return Build{}, false, nil
	}
	fail := func(why string) (Build, bool, error) {
		return Build{}, true, fmt.Errorf("built character %d %s: %w", n, why, ErrBadBuild)
	}
	if len(init) != charaInitSize {
		return fail("is not a CharaInitParam row")
	}
	var build Build
	face := find(TableFace, id)
	for template, row := range faces {
		if bytes.Equal(row, face) {
			build.Template = template
		}
	}
	if build.Template == 0 {
		return fail("has a face no base template has")
	}
	name := init[nameOffset : nameOffset+nameSize]
	if end := bytes.IndexByte(name, 0); end >= 0 {
		name = name[:end]
	}
	build.Name = string(name)
	if checkName(build.Name) != nil {
		return fail("holds no name the editor wrote")
	}
	// A build written before the class was chosen holds none: the Vagabond's.
	if class := int(init[classOffset]); class >= 1 && class <= len(ClassNames) {
		build.Class = class - 1
	}
	field := func(name string, step int) int64 {
		at := charaInitFields[name]
		at.offset += step * fieldWidth(at.kind)
		return getField(init, at)
	}
	// row is a pointer field, zero where it is empty.
	row := func(name string, step int) int32 { return int32(max(field(name, step), 0)) }

	build.Voice = uint8(field("voiceType", 0))
	for index := range build.Attributes {
		build.Attributes[index] = uint8(field("baseVit", index))
	}
	for hand, fields := range hands {
		weapon, ash := row(fields[0], 0), int32(0)
		if weapon == 0 {
			continue
		}
		if field(fields[1], 0) == 1 {
			custom := find(TableCustomWeapon, weapon)
			if len(custom) != customWeaponSize {
				return fail(fmt.Sprintf("names custom weapon %d, which the save does not hold", weapon))
			}
			weapon = int32(binary.LittleEndian.Uint32(custom[0:])) + int32(custom[8])
			ash = int32(binary.LittleEndian.Uint32(custom[4:]))
		}
		build.Hands[hand] = Weapon{Base: weapon - weapon%10000, Affinity: int(weapon % 10000 / 100),
			Level: int(weapon % 100), Ash: ash}
	}
	for index, fields := range ammo {
		if ammunition := row(fields[0], 0); ammunition != 0 {
			build.Ammo[index] = Stack{Row: ammunition, Count: int(field(fields[1], 0))}
		}
	}
	for index, name := range armour {
		build.Armour[index] = row(name, 0)
	}
	for index, name := range talismans {
		build.Talismans[index] = row(name, 0)
	}
	for index := range build.Spells {
		build.Spells[index] = offeredAs(row("equip_Spell_01", index))
	}
	// The first quick slot is the flask every build carries.
	for index := range build.Quick {
		if goods := row("item_01", index+1); goods != 0 {
			build.Quick[index] = Stack{Row: goods, Count: int(field("itemNum_01", index+1))}
		}
	}
	return build, true, nil
}
