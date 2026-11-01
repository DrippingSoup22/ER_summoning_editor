package savecore

// The shipped values of FromSoftware's own summon rows.
//
// A Fighter chosen for one overwrites them - it takes the balance package off
// and puts the creature's own health on - and the Helper, or a reset, has to put
// them back. Nothing else can: a save written with a Fighter holds the new
// values in its own bytes, so the old ones are gone unless they were recorded
// before the first write.
//
// The clone rows this editor INSERTS need no record here. They are copied from
// a world actor that is still in the save, so the helper recipe is reproducible
// from the save itself. Only the 115 rows the game ships have shipped values
// that overwriting destroys.
//
// See AGENTS.md, "Summon strength", for why this is not a reset path.

import "fmt"

// npcHPFieldName and npcResidentFieldName spell the field names the snapshot
// records. They are this package's own names rather than NpcParam's, because
// the resident vector is three separate runs in the shipped table and a record
// keyed by spEffectID0..31 would imply a contiguity that is not there.
const npcHPFieldName = "hp"

func npcResidentFieldName(slot int) string { return fmt.Sprintf("resident_%d", slot) }

// NpcRow is one NpcParam row's shipped value for every field the strength
// switch can write: its base health and its 32 resident effect slots.
type NpcRow map[string]int64

// NpcFieldNames returns those names in the order the snapshot stores them:
// health first, then the resident slots in the order the recipe walks them.
func NpcFieldNames() []string {
	names := make([]string, 0, 1+len(npcResidentEffectOffsets))
	names = append(names, npcHPFieldName)
	for slot := range npcResidentEffectOffsets {
		names = append(names, npcResidentFieldName(slot))
	}
	return names
}

func readNpcRow(row []byte) NpcRow {
	values := make(NpcRow, 1+len(npcResidentEffectOffsets))
	values[npcHPFieldName] = int64(readS32(row, npcHPOffset))
	for slot, offset := range npcResidentEffectOffsets {
		values[npcResidentFieldName(slot)] = int64(readS32(row, offset))
	}
	return values
}

// residentEffectsOf is a row's resident effect vector, empty slots dropped.
func residentEffectsOf(row []byte) []int32 {
	effects := make([]int32, 0, len(npcResidentEffectOffsets))
	for _, offset := range npcResidentEffectOffsets {
		if effect := readS32(row, offset); effect > 0 {
			effects = append(effects, effect)
		}
	}
	return effects
}

func holdsEffect(effects []int32, want int32) bool {
	for _, effect := range effects {
		if effect == want {
			return true
		}
	}
	return false
}

// NpcHealthField and NpcResidentFields name the snapshot's columns for callers
// that build row values rather than read them back.
const NpcHealthField = npcHPFieldName

func NpcResidentFields() []string {
	names := make([]string, 0, len(npcResidentEffectOffsets))
	for slot := range npcResidentEffectOffsets {
		names = append(names, npcResidentFieldName(slot))
	}
	return names
}
