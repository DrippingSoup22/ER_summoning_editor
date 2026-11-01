package savefixture

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// VanillaOptions describes a save carrying every row the embedded shipped-value
// snapshot records: all 84 Ash families across their upgrade levels, all 170
// BuddyParam rows with their shipped formation values, and all 304
// BuddyStoneParam rows with their own distinct values.
//
// Defaults() carries five families and eight stone rows, which is enough to
// exercise the conversion path and fast to build. Reset is different: it reads
// the embedded snapshot and restores every row in it, so a save missing those
// rows fails before any behavior is tested. This builds the save the product
// actually expects to meet.
//
// It reads the snapshot rather than reimplementing it, which is the one thing
// this package does not duplicate on purpose — the snapshot is data, not the
// container format, and two copies of 304 rows of values would be a liability
// rather than an independent check.
func VanillaOptions() (Options, error) {
	snapshot, err := reference.Vanilla()
	if err != nil {
		return Options{}, err
	}

	goodsRows := sortedInt32Keys(snapshot.Goods)
	goods := Table{
		Entry: "EquipParamGoods.param", ParamType: "EQUIP_PARAM_GOODS_ST",
		DataVersion: 3, RowSize: 176, RowIDs: goodsRows,
		Init: func(rowID int32, row []byte) {
			shipped, _ := snapshot.GoodsRow(rowID)
			binary.LittleEndian.PutUint32(row[goodsRefIDDefaultOffset:], uint32(int32(shipped["refId_default"])))
			binary.LittleEndian.PutUint16(row[goodsConsumeMPOffset:], uint16(int16(shipped["consumeMP"])))
			binary.LittleEndian.PutUint16(row[goodsConsumeHPOffset:], uint16(int16(shipped["consumeHP"])))
			row[goodsFlagsOffset] |= goodsEnableMultiMask
		},
	}

	buddyRows := sortedInt32Keys(snapshot.Buddy)
	buddy := Table{
		Entry: "BuddyParam.param", ParamType: "BUDDY_PARAM_ST",
		DataVersion: 2, RowSize: 160, RowIDs: buddyRows,
		Init: func(rowID int32, row []byte) {
			// The canonical series first, then the snapshot over it. The
			// snapshot carries the real ladder now, gaps and all - Giant Rat
			// Ashes ships three blocks with -1 in the levels each does not own
			// - and a fixture that seeded the canonical series everywhere would
			// be a save the shipped-value comparison calls edited.
			for level := 0; level < BuddyDopingLevels; level++ {
				binary.LittleEndian.PutUint32(row[buddyDopingOffset+4*level:], uint32(BuddyDopingBase+int32(level)))
			}
			values, ok := snapshot.BuddyRow(rowID)
			if !ok {
				return
			}
			for name, value := range values {
				field, known := BuddyFieldOffsets[name]
				if !known {
					continue
				}
				switch {
				case BuddyFloatFields[name]:
					binary.LittleEndian.PutUint32(row[field.Offset:], math.Float32bits(float32(value)))
				case field.Bits == 32:
					binary.LittleEndian.PutUint32(row[field.Offset:], uint32(int32(value)))
				case field.Bits == 8:
					row[field.Offset] = byte(value)
				}
			}
		},
	}

	stoneRows := make([]int32, 0, len(snapshot.Stone))
	for rowID := range snapshot.Stone {
		stoneRows = append(stoneRows, rowID)
	}
	slices.Sort(stoneRows)
	stone := Table{
		Entry: "BuddyStoneParam.param", ParamType: "BUDDY_STONE_PARAM_ST",
		DataVersion: 2, RowSize: 64, RowIDs: stoneRows,
		Init: func(rowID int32, row []byte) {
			values, ok := snapshot.StoneRow(rowID)
			if !ok {
				return
			}
			for name, value := range values {
				field, known := StoneFieldOffsets[name]
				if !known {
					continue
				}
				switch field.Bits {
				case 32:
					binary.LittleEndian.PutUint32(row[field.Offset:], uint32(value))
				case 16:
					binary.LittleEndian.PutUint16(row[field.Offset:], uint16(value))
				}
			}
		},
	}

	// Every shipped trigger needs a row, or the product's dangling-reference
	// check refuses the goods write.
	seen := map[int32]bool{}
	effect := Table{
		Entry: "SpEffectParam.param", ParamType: "SP_EFFECT_PARAM_ST",
		DataVersion: 4, RowSize: 912,
	}
	for _, rowID := range goodsRows {
		shipped, _ := snapshot.GoodsRow(rowID)
		trigger := int32(shipped["refId_default"])
		if !seen[trigger] {
			seen[trigger] = true
			effect.RowIDs = append(effect.RowIDs, trigger)
		}
	}
	// The resident effects the summon-side recipe writes onto an inserted
	// clone. They are not goods triggers, so nothing above would have added
	// them, and a clone insertion into this save would fail without them.
	effect.RowIDs = append(effect.RowIDs, SpiritCompanionEffects[:]...)
	slices.Sort(effect.RowIDs)

	// The actor tables carry exactly the clone rows the snapshot recorded, so a
	// save built from the snapshot answers "which clone rows did the game ship"
	// the same way the snapshot does.
	actors := actorTables(snapshot.NpcCloneRows, snapshot.NpcThinkCloneRows)

	options := Defaults()
	options.RegulationVersion = snapshot.RegulationVersion
	options.Tables = append([]Table{goods, buddy, stone, effect, vfxTable(), charaInitTable(), faceTable(), customWeaponTable()}, actors...)
	return options, nil
}

// VanillaGoodsRows lists every goods row the shipped snapshot records, in
// ascending order, so a test can ask the state reader for exactly them.
func VanillaGoodsRows() ([]int32, error) {
	snapshot, err := reference.Vanilla()
	if err != nil {
		return nil, fmt.Errorf("read the shipped snapshot: %w", err)
	}
	return sortedInt32Keys(snapshot.Goods), nil
}

func sortedInt32Keys[V any](values map[int32]V) []int32 {
	return slices.Sorted(maps.Keys(values))
}
