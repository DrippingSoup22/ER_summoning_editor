package savecore

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// GoodsState is what one Spirit Ash goods row currently says.
type GoodsState struct {
	// Trigger is refId_default: the SpEffect that delivers the formation.
	Trigger int32
	// ConsumeMP and ConsumeHP are the resource price. -1 is the unset spelling
	// and no shipped row is negative otherwise, so a frontend can show "free"
	// for anything not positive without a second field saying which resource
	// this Ash spends.
	ConsumeMP, ConsumeHP int32
	// EnableMulti is read only so previews and tests can prove the product leaves
	// the session-visible bit unchanged.
	EnableMulti bool
}

// State is the current value of every field this product writes, and nothing
// else. It is deliberately not a general PARAM reader: the writer is bounded to
// three tables and a handful of fields, and the reader is bounded to the same
// ones so the two can be compared.
type State struct {
	SHA256            string
	RegulationVersion uint32
	// Goods holds only the rows the caller asked about. A real regulation has
	// thousands, and which of them are Spirit Ashes is catalog knowledge that
	// does not belong in the save layer.
	Goods map[int32]GoodsState
	// MonumentOriginRows are BuddyParam rows still carrying
	// appearOnAroundSekihi = 1, the half of the shipping recipe that makes a
	// formation spawn at a distant monument.
	MonumentOriginRows []int32
	// StoneRows is how many BuddyStoneParam rows the regulation has.
	StoneRows int
	// ExpandedAreaRows is how many of them already carry the full six-field
	// recipe. Equal to StoneRows means the recipe is already applied.
	ExpandedAreaRows int
	// WideAreaRows and NoRestLimitRows count the rows carrying each half of
	// the monument switch on its own: the five area fields, and
	// summonedEventFlagId at 0.
	WideAreaRows, NoRestLimitRows int
	// WokenMonuments are the sleeping monuments the save holds a row for
	// (monuments.go), ascending.
	WokenMonuments []int32
	// SummonRows is every summon-side actor row's current value for the fields
	// a role writes, keyed by row and named as the shipped-value snapshot names
	// them.
	//
	// A role chosen for one of the game's own rows works by difference - it
	// writes only what does not already say what it should - so it needs the
	// current values and not only a verdict about them. The rows this editor
	// inserted are here too, so a slot can say which role its clone holds.
	SummonRows map[int32]NpcRow
	// PhantomProfile is what SpEffectVfxParam row 57000 currently points at:
	// PhantomParam 200 for the white spirit look, 0 for none. It is the whole
	// state of the colour switch, and it is one field because the switch is.
	//
	// Reported as the number rather than as a verdict so a file holding neither
	// shipped value can be told apart from one holding the white look. plan
	// owns what the numbers mean.
	PhantomProfile int64
	// SharedEffects is what the save holds in every shared effect field
	// (sharedeffects.go), so the application writes one only when it must.
	SharedEffects map[SharedEffectKey]float64
	// PersonWrites is what the save holds in every person-write field
	// (personwrites.go), read the same way.
	PersonWrites map[PersonKey]float64
	// CharacterRows is every row the save holds in the character band
	// (characters.go), so the application rewrites one only when it differs.
	CharacterRows map[CharacterRowKey][]byte
	// PersonShells are the person shell rows, which a character's body copies
	// (CharacterBody), so the application can say what a body would be.
	PersonShells map[int32][]byte
	// OwnEffects are the effect rows of our own the save holds
	// (owneffects.go), and OwnEffectNames which actor rows name each, so the
	// application inserts one only when it is missing and removes one only
	// once nothing will name it.
	OwnEffects     map[int32]bool
	OwnEffectNames map[int32][]int32
}

// ExpandedAreaApplied reports whether every stone row already carries the
// recipe, which is what a re-run of the same conversion would find.
func (state *State) ExpandedAreaApplied() bool {
	return state.StoneRows > 0 && state.ExpandedAreaRows == state.StoneRows
}

// ReadState reports the current value of the fields this product writes.
//
// It exists so a frontend can show what a save says before changing it, so a
// re-run can tell an already-converted family from a fresh one, and so a future
// reset can diff against shipped values. It opens the save read-only and writes
// nothing.
func ReadState(ctx context.Context, path string, goodsRows []int32) (*State, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := loadSave(path)
	if err != nil {
		return nil, err
	}
	state := &State{
		SHA256:            regulation.Sha256Hex(loaded.FileBytes),
		RegulationVersion: loaded.Version,
		Goods:             make(map[int32]GoodsState, len(goodsRows)),
	}

	if len(goodsRows) > 0 {
		goods, err := regulation.OpenTable(loaded.BND4, goodsLayout)
		if err != nil {
			return nil, err
		}
		for _, rowID := range goodsRows {
			row, present := goods.ByID[rowID]
			if !present {
				return nil, fmt.Errorf("Goods row %d was not found: %w", rowID, ErrRowNotFound)
			}
			shipped := readGoodsRow(goods.Data[row.DataOffset:])
			state.Goods[rowID] = GoodsState{
				Trigger:     int32(shipped["refId_default"]),
				ConsumeMP:   int32(shipped["consumeMP"]),
				ConsumeHP:   int32(shipped["consumeHP"]),
				EnableMulti: goods.Data[row.DataOffset+goodsFlagsOffset]&goodsEnableMultiMask != 0,
			}
		}
	}

	buddy, err := regulation.OpenTable(loaded.BND4, buddyLayout)
	if err != nil {
		return nil, err
	}
	for _, row := range buddy.Rows {
		if buddy.Data[row.DataOffset+buddyAppearOffset] != 0 {
			state.MonumentOriginRows = append(state.MonumentOriginRows, row.ID)
		}
	}

	vfx, err := regulation.OpenTable(loaded.BND4, vfxLayout)
	if err != nil {
		return nil, err
	}
	if row, present := vfx.ByID[spiritVfxRow]; present {
		state.PhantomProfile = readParamField(
			vfx.Data[row.DataOffset:], vfxFields["phantomParamOverwriteId"],
			"vfx", "phantomParamOverwriteId")
	} else {
		return nil, fmt.Errorf("SpEffectVfxParam row %d was not found: %w", spiritVfxRow, ErrRowNotFound)
	}

	if state.SharedEffects, err = readSharedEffects(loaded.BND4); err != nil {
		return nil, err
	}
	if state.PersonWrites, err = readPersonWrites(loaded.BND4); err != nil {
		return nil, err
	}
	if state.CharacterRows, err = readCharacterRows(loaded.BND4); err != nil {
		return nil, err
	}
	if state.PersonShells, err = readPersonShells(loaded.BND4); err != nil {
		return nil, err
	}
	if state.OwnEffects, state.OwnEffectNames, err = readOwnEffects(loaded.BND4); err != nil {
		return nil, err
	}

	npc, err := regulation.OpenTable(loaded.BND4, npcLayout)
	if err != nil {
		return nil, err
	}
	state.SummonRows = map[int32]NpcRow{}
	for _, row := range npc.Rows {
		if inCloneBand(row.ID) {
			state.SummonRows[row.ID] = readNpcRow(npc.Data[row.DataOffset:])
		}
	}

	stone, err := regulation.OpenTable(loaded.BND4, stoneLayout)
	if err != nil {
		return nil, err
	}
	state.StoneRows = len(stone.Rows)
	for _, row := range stone.Rows {
		data := stone.Data[row.DataOffset:]
		area, rest := stoneRowCarries(data, areaHalf), stoneRowCarries(data, []string{restField})
		if area {
			state.WideAreaRows++
		}
		if rest {
			state.NoRestLimitRows++
		}
		if area && rest {
			state.ExpandedAreaRows++
		}
	}
	state.WokenMonuments = readWokenMonuments(stone)
	return state, nil
}

// restField is the once-per-rest half of the monument switch, and areaHalf
// the other five fields: where a summon may be called.
const restField = "summonedEventFlagId"

var areaHalf = func() []string {
	var names []string
	for _, name := range fieldNames(stoneFields) {
		if name != restField {
			names = append(names, name)
		}
	}
	return names
}()

// stoneRowCarries reports whether every named expanded-area field holds its
// recipe value. All or none: a half with some of its fields set is a partially
// applied recipe, which is a state no version of this product produces and
// must not be reported as applied.
func stoneRowCarries(row []byte, names []string) bool {
	for _, name := range names {
		field := stoneFields[name]
		var current float64
		switch field.kind {
		case "u32":
			current = float64(binary.LittleEndian.Uint32(row[field.offset:]))
		case "u16":
			current = float64(binary.LittleEndian.Uint16(row[field.offset:]))
		case "s16":
			current = float64(int16(binary.LittleEndian.Uint16(row[field.offset:])))
		default:
			panic("unreachable: unknown stone field kind " + field.kind + " for " + name)
		}
		if current != field.want {
			return false
		}
	}
	return true
}
