package savecore

// Writing the rows a validated manifest names, and proving afterwards that
// nothing outside them moved. Nothing here re-checks what the validators
// already decided: a patch reaching this file has been judged.

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

type byteRange struct{ start, end int }

// applyRows is the shape all three row writers share: open the table, find
// every patched row or refuse by name, write it, and report the bytes that
// moved. Only the table, the name a refusal uses, and what writing one row
// means are ever different, so those are what it takes.
//
// A patch naming a row the table does not have is always a refusal and never a
// skip: the manifest said to write it, and a save that quietly did less than
// the manifest describes is the one outcome an audit artifact cannot survive.
func applyRows(bnd4 []byte, layout regulation.TableLayout, label string, patches []rowPatch,
	write func(row []byte, patch rowPatch) error) ([]byteRange, error) {

	table, err := regulation.OpenTable(bnd4, layout)
	if err != nil {
		return nil, err
	}
	ranges := make([]byteRange, 0, len(patches))
	for _, patch := range patches {
		row, ok := table.ByID[patch.RowID]
		if !ok {
			return nil, fmt.Errorf("%s row %d (%s) was not found: %w", label, patch.RowID, patch.Name, ErrRowNotFound)
		}
		if err := write(table.Data[row.DataOffset:], patch); err != nil {
			return nil, err
		}
		ranges = append(ranges, rowRange(table, row))
	}
	return ranges, nil
}

func applyGoods(bnd4 []byte, patches []rowPatch) ([]byteRange, error) {
	// Read before the walk rather than inside it: the referent check below runs
	// once per patch and the set it checks against does not change.
	_, effects, err := regulation.LoadRowsByID(bnd4, "SpEffectParam.param", "", 0)
	if err != nil {
		return nil, err
	}
	return applyRows(bnd4, goodsLayout, "Goods", patches, func(row []byte, patch rowPatch) error {
		// A price names no other row, so only the trigger has a referent to
		// check. Checked here rather than in the validator because it is the
		// one goods rule that needs the save open.
		if trigger, points := patch.Set["refId_default"]; points {
			if _, exists := effects[int32(trigger)]; !exists {
				return fmt.Errorf("Goods row %d references SpEffect row %d, which does not exist: %w",
					patch.RowID, int32(trigger), ErrRowNotFound)
			}
		}
		writeParamRow(row, goodsFields, patch.Set)
		return nil
	})
}

// applyBuddy writes the BuddyParam patches. wholeRow distinguishes a conversion,
// which only ever clears one flag, from authoring and a reset, which both write
// all four restorable fields of every row they name.
func applyBuddy(bnd4 []byte, patches []rowPatch, wholeRow bool) ([]byteRange, error) {
	return applyRows(bnd4, buddyLayout, "Buddy", patches, func(row []byte, patch rowPatch) error {
		if wholeRow {
			writeBuddyRow(row, patch.Set)
		} else {
			row[buddyAppearOffset] = byte(patch.Set["appearOnAroundSekihi"])
		}
		return nil
	})
}

// writeBuddyRow is the single place BuddyParam field widths are encoded, so a
// conversion and a reset cannot disagree about how a value is stored.
func writeBuddyRow(row []byte, values map[string]float64) {
	writeParamRow(row, buddyFields, values)
}

// writeParamRow writes the named fields and only those. A name the table does
// not have cannot arrive here: every caller is fed by a validator that checks
// the key set against the same map.
func writeParamRow(row []byte, fields map[string]paramField, values map[string]float64) {
	for name, value := range values {
		field := fields[name]
		switch field.kind {
		case "s32":
			binary.LittleEndian.PutUint32(row[field.offset:], uint32(int32(value)))
		case "s16":
			binary.LittleEndian.PutUint16(row[field.offset:], uint16(int16(value)))
		case "u8":
			row[field.offset] = byte(value)
		case "f32":
			binary.LittleEndian.PutUint32(row[field.offset:], math.Float32bits(float32(value)))
		}
	}
}

// applyStoneChanges routes to the writer the operation calls for. Both go
// through the same declared-range accounting, so a reset is an ordinary
// bounded edit with an unusual target rather than a second write path.
func applyStoneChanges(bnd4 []byte, plan *manifest) ([]byteRange, int, error) {
	switch plan.Operation {
	case operationReset:
		return applyStoneRows(bnd4, plan.BuddyStoneRowPatches)
	case operationAuthor, operationCost, operationStrength, operationTint, operationPrune, operationSharedEffects, operationCharacters, operationOwnEffects:
		// Authoring edits the host's own formation block and reuses its shipped
		// trigger; a cost edit changes a number in a goods row; the colour
		// switch moves one pointer in SpEffectVfxParam; a prune removes
		// creature rows nothing names. None has anything to
		// say to the monument table, and writing the expanded-area recipe for
		// any of them would be a global edit smuggled in under another intent.
		return nil, 0, nil
	case operationArea, operationConvert, operationSession:
		// The switch, in whichever positions, alone or riding on a
		// conversion; a flattened session holds whichever its parts asked for.
		// The half that is on is the all-row patch and the half that is off the
		// shipped rows, which write different fields, so the order between
		// them is only tidiness. Named rather than reached by falling through:
		// an operation that arrived by default would index an empty slice, and
		// the fourth operation added did exactly that.
		var ranges []byteRange
		rows := 0
		if len(plan.BuddyStonePatches) == 1 {
			tableRanges, count, err := applyStone(bnd4, plan.BuddyStonePatches[0])
			if err != nil {
				return nil, 0, err
			}
			ranges, rows = tableRanges, count
		}
		restored, count, err := applyStoneRows(bnd4, plan.BuddyStoneRowPatches)
		if err != nil {
			return nil, 0, err
		}
		if len(plan.BuddyStonePatches) == 0 {
			rows = count
		}
		return append(ranges, restored...), rows, nil
	}
	return nil, 0, fmt.Errorf("operation %q has no BuddyStoneParam rule: %w", plan.Operation, ErrManifestInvalid)
}

// applyStoneRows restores one recorded value set per row.
//
// Unlike the recipe it undoes, this refuses a row the save does not have. The
// recipe writes whatever rows are present; a reset is restoring one specific
// recorded regulation, and a missing row means the snapshot describes a
// different one.
func applyStoneRows(bnd4 []byte, patches []rowPatch) ([]byteRange, int, error) {
	table, err := regulation.OpenTable(bnd4, stoneLayout)
	if err != nil {
		return nil, 0, err
	}
	ranges := make([]byteRange, 0, len(patches))
	for _, patch := range patches {
		row, ok := table.ByID[patch.RowID]
		if !ok {
			return nil, 0, fmt.Errorf("BuddyStone row %d was not found: %w", patch.RowID, ErrRowNotFound)
		}
		writeStoneRow(table.Data[row.DataOffset:], patch.Set)
		ranges = append(ranges, rowRange(table, row))
	}
	return ranges, len(patches), nil
}

func applyStone(bnd4 []byte, patch tablePatch) ([]byteRange, int, error) {
	table, err := regulation.OpenTable(bnd4, stoneLayout)
	if err != nil {
		return nil, 0, err
	}
	if len(table.Rows) == 0 {
		return nil, 0, fmt.Errorf("BuddyStoneParam has no rows: %w", ErrUnsupportedRegulation)
	}
	ranges := make([]byteRange, 0, len(table.Rows))
	for _, row := range table.Rows {
		writeStoneRow(table.Data[row.DataOffset:], patch.Set)
		ranges = append(ranges, rowRange(table, row))
	}
	return ranges, len(table.Rows), nil
}

// writeStoneRow is the single place stone field widths are encoded, so a
// conversion and a reset cannot disagree about how a value is stored.
func writeStoneRow(row []byte, values map[string]float64) {
	for name, value := range values {
		field := stoneFields[name]
		switch field.kind {
		case "u32":
			binary.LittleEndian.PutUint32(row[field.offset:], uint32(value))
		case "u16":
			binary.LittleEndian.PutUint16(row[field.offset:], uint16(value))
		case "s16":
			binary.LittleEndian.PutUint16(row[field.offset:], uint16(int16(value)))
		}
	}
}

func rowRange(table *regulation.Table, row regulation.ParamRow) byteRange {
	start := table.Entry.Offset + row.DataOffset
	return byteRange{start: start, end: start + table.Layout.RowSize}
}

func verifyBoundedMutation(before, after []byte, ranges []byteRange) error {
	if len(before) != len(after) {
		return fmt.Errorf("BND4 size changed from %d to %d", len(before), len(after))
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	for index := range before {
		if before[index] == after[index] {
			continue
		}
		position := sort.Search(len(ranges), func(rangeIndex int) bool { return ranges[rangeIndex].end > index })
		if position == len(ranges) || index < ranges[position].start {
			return fmt.Errorf("byte %d changed outside every declared row", index)
		}
	}
	return nil
}

// applyVfx moves the colour switch.
//
// One row, one field, and the narrowest write in the product. The row is not
// taken from the patch: validateVfxPatches has already refused any row but
// spiritVfxRow, and OpenTable is asked for that constant here as well, so the
// two agree by construction rather than by the manifest being trusted twice.
func applyVfx(bnd4 []byte, patches []rowPatch) ([]byteRange, error) {
	if len(patches) == 0 {
		return nil, nil
	}
	table, err := regulation.OpenTable(bnd4, vfxLayout)
	if err != nil {
		return nil, err
	}
	row, ok := table.ByID[spiritVfxRow]
	if !ok {
		return nil, fmt.Errorf("SpEffectVfxParam row %d was not found: %w", spiritVfxRow, ErrRowNotFound)
	}
	writeParamRow(table.Data[row.DataOffset:], vfxFields, patches[0].Set)
	return []byteRange{rowRange(table, row)}, nil
}
