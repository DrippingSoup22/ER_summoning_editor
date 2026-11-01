package savecore

// The sleeping monuments: 39 characters the maps place running talk 2000, the
// monument's own script, with no BuddyStoneParam row keyed to them on
// regulation 11721000 or 11730034. The script reads its row by the character's entity, so a row
// keyed to one puts a live monument on a tile that holds none: the graces
// Agheel Lake North and Murkwater Cave, Mistwood Outskirts, Artist's Shack,
// Seaside Ruins, Saintsbridge, Altus Highway Junction, a graceless Caelid tile
// and a DLC one.
//
// The monument switch's wide position wakes them and its other position, like
// a reset, puts them back to sleep. Each is a copy of one shipped row keyed to
// its entity, and a manifest naming any other row is refused, so the whole
// surface is the list below.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
	"sort"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// sleepingMonuments are the entities of the 39, ascending.
var sleepingMonuments = []int32{
	12020100, 12020104, 12020106, 12020108,
	13000104, 13000110, 13000112, 13000114, 13000116, 13000118,
	14000108, 15000106, 15000108, 16000102, 16000104, 16000106,
	1039510100, 1041330102, 1041360102, 1041360104, 1042320100, 1042370110,
	1043330100, 1043350102, 1043360102, 1043370104, 1043370106, 1043390102,
	1044340100, 1044340102, 1044370102, 1044380100, 1045380102,
	1049390104, 1049530102, 1051410100,
	2044480100, 2050480100, 2050480102,
}

// wokenTemplate is the Caelid colosseum's stele row: a monument the maps place
// as an invisible character, with no enemy that must die first. A woken row
// copies it and is then keyed to its own entity.
const wokenTemplate = int32(45000100)

// stoneTalkOffset is talkChrEntityId, the character a row belongs to.
const stoneTalkOffset = 4

// SleepingMonuments is a copy of the list, for the application.
func SleepingMonuments() []int32 { return slices.Clone(sleepingMonuments) }

// wokenValues are the fields a woken row is written with over its copy: no
// enemy to defeat and no hand-drawn region, and the once-per-rest flag every
// shipped monument uses, its entity + 2000 (all 39 lie in stored flag blocks).
// The switch's own fields are written afterwards like any other row's.
func wokenValues(entity int32) map[string]float64 {
	return map[string]float64{
		"eliminateTargetEntityId":         0,
		"summonedEventFlagId":             float64(entity + 2000),
		"overwriteActivateRegionEntityId": 0,
		"warnRegionEntityId":              0,
	}
}

// validateMonumentRows holds the woken rows to the switch and to the list,
// before the save is opened. The wide position writes all 39 afresh, so a
// manifest inserting some of them inserts every one; any other position, and a
// reset, may only remove them.
func validateMonumentRows(plan *manifest) error {
	if len(plan.StoneInsertions)+len(plan.StoneRemovals) == 0 {
		return nil
	}
	switch plan.Operation {
	case operationArea, operationConvert:
	case operationReset:
		if len(plan.StoneInsertions) != 0 {
			return fmt.Errorf("a reset undoes additions and may not make any")
		}
	default:
		return fmt.Errorf("operation %q may not wake or put back a monument: %w", plan.Operation, ErrManifestInvalid)
	}
	for _, list := range [][]int32{plan.StoneInsertions, plan.StoneRemovals} {
		seen := map[int32]bool{}
		for _, row := range list {
			if !slices.Contains(sleepingMonuments, row) {
				return fmt.Errorf("BuddyStoneParam %d is not a sleeping monument: %w", row, ErrManifestInvalid)
			}
			if seen[row] {
				return fmt.Errorf("BuddyStoneParam %d is named twice: %w", row, ErrManifestInvalid)
			}
			seen[row] = true
		}
	}
	if len(plan.StoneInsertions) == 0 {
		return nil
	}
	if len(plan.StoneInsertions) != len(sleepingMonuments) {
		return fmt.Errorf("the monument switch wakes all %d sleeping monuments, not %d", len(sleepingMonuments), len(plan.StoneInsertions))
	}
	if !widensArea(plan) {
		return fmt.Errorf("a sleeping monument is woken only beside the wide summoning area")
	}
	return nil
}

// widensArea reports whether the manifest's all-row patch writes the area half
// of the switch.
func widensArea(plan *manifest) bool {
	if len(plan.BuddyStonePatches) != 1 {
		return false
	}
	_, wide := plan.BuddyStonePatches[0].Set["activateRange"]
	return wide
}

func (plan *manifest) hasMonumentWork() bool {
	return len(plan.StoneInsertions)+len(plan.StoneRemovals) > 0
}

// applyMonumentRows removes and inserts the woken rows: removals first, so a
// row woken again is a fresh copy. Each insertion copies the template,
// anchored after the largest id below it, and its own values are written here,
// where the copy is made, proved to stay inside the rows inserted.
func applyMonumentRows(bnd4 []byte, plan *manifest) ([]byte, error) {
	current := bnd4
	var err error
	if len(plan.StoneRemovals) > 0 {
		removals := slices.Sorted(slices.Values(plan.StoneRemovals))
		if current, err = regulation.RemoveRows(current, stoneLayout, removals); err != nil {
			return nil, err
		}
	}
	if len(plan.StoneInsertions) == 0 {
		return current, nil
	}
	table, err := regulation.OpenTable(current, stoneLayout)
	if err != nil {
		return nil, err
	}
	if _, held := table.ByID[wokenTemplate]; !held {
		return nil, fmt.Errorf("BuddyStoneParam %d, which a woken monument copies, was not found: %w", wokenTemplate, ErrRowNotFound)
	}
	present := make([]int32, len(table.Rows))
	for index, row := range table.Rows {
		present[index] = row.ID
	}
	slices.Sort(present)
	rows := slices.Sorted(slices.Values(plan.StoneInsertions))
	insertions := make([]regulation.RowInsertion, 0, len(rows))
	var previous int32
	for _, rowID := range rows {
		if _, held := table.ByID[rowID]; held {
			return nil, fmt.Errorf("BuddyStoneParam %d is already in the save", rowID)
		}
		anchor := present[sort.Search(len(present), func(index int) bool { return present[index] >= rowID })-1]
		if anchor < previous {
			anchor = previous
		}
		insertions = append(insertions, regulation.RowInsertion{RowID: rowID, CopyFrom: wokenTemplate, InsertAfter: &anchor})
		previous = rowID
	}
	grown, err := regulation.InsertRows(current, stoneLayout, insertions)
	if err != nil {
		return nil, err
	}
	copied := bytes.Clone(grown)
	grownTable, err := regulation.OpenTable(grown, stoneLayout)
	if err != nil {
		return nil, err
	}
	ranges := make([]byteRange, 0, len(rows))
	for _, rowID := range rows {
		row := grownTable.ByID[rowID]
		data := grownTable.Data[row.DataOffset:]
		binary.LittleEndian.PutUint32(data[stoneTalkOffset:], uint32(rowID))
		writeStoneRow(data, wokenValues(rowID))
		ranges = append(ranges, rowRange(grownTable, row))
	}
	if err := verifyBoundedMutation(copied, grown, ranges); err != nil {
		return nil, fmt.Errorf("BuddyStoneParam: waking the sleeping monuments: %w", err)
	}
	return grown, nil
}

// readWokenMonuments is which of the 39 the save holds a row for.
func readWokenMonuments(stone *regulation.Table) []int32 {
	var woken []int32
	for _, entity := range sleepingMonuments {
		if _, present := stone.ByID[entity]; present {
			woken = append(woken, entity)
		}
	}
	return woken
}
