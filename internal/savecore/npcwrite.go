package savecore

// Writing the values of an NpcParam row that already exists.
//
// Every other actor-table write in this package either INSERTS a row or removes
// one. This one changes a row that was already there, which is what a role
// chosen for FromSoftware's own 115 summon rows needs twice over: a Fighter
// takes the balance package off, and the Helper or a reset restores the
// recorded snapshot.
//
// What the values mean is not decided here. This package holds no catalog and
// no measurement of what a creature is worth, so the caller names fields and
// values and this writes them - the same division goods and BuddyParam patches
// already use. The recipe lives in internal/plan and internal/application, and
// docs/DESIGN.md (Summon strength) carries the reasoning.
//
// The write is bounded to the clone namespace. A row below the offset is a
// world actor, and a switch about summons must never be able to reach the
// creature standing in the world.

import "fmt"

// npcFields are the fields an NpcParam row patch may name: the base health the
// upgrade ladder multiplies, and the 32 resident effect slots.
//
// The names match the snapshot's, so a restore is the snapshot's own record
// handed back unchanged rather than a translation of it.
var npcFields = func() map[string]paramField {
	fields := map[string]paramField{
		npcHPFieldName: {offset: npcHPOffset, kind: "s32"},
	}
	for slot, offset := range npcResidentEffectOffsets {
		fields[npcResidentFieldName(slot)] = paramField{offset: offset, kind: "s32"}
	}
	return fields
}()

// validateNpcPatches holds every NpcParam row patch to the namespace and the
// field set before anything is written.
func validateNpcPatches(plan *manifest) error {
	if len(plan.NpcPatches) == 0 {
		return nil
	}
	if plan.Operation != operationStrength && plan.Operation != operationSession {
		return fmt.Errorf("%s writes no NpcParam row values", plan.Operation)
	}
	seen := make(map[int32]bool, len(plan.NpcPatches))
	for _, patch := range plan.NpcPatches {
		if !inCloneBand(patch.RowID) {
			return fmt.Errorf("npc_patches names %d, which is outside the clone bands (%d + n x %d)",
				patch.RowID, cloneRowOffset, cloneRowOffset)
		}
		if seen[patch.RowID] {
			return fmt.Errorf("npc_patches names row %d twice", patch.RowID)
		}
		seen[patch.RowID] = true
		if len(patch.Set) == 0 {
			return fmt.Errorf("npc_patches row %d sets nothing", patch.RowID)
		}
		for name, value := range patch.Set {
			field, known := npcFields[name]
			if !known {
				return fmt.Errorf("npc_patches row %d names unknown field %q", patch.RowID, name)
			}
			low, high := field.valueRange()
			if value < low || value > high {
				return fmt.Errorf("npc_patches row %d sets %s to %v, outside %v..%v",
					patch.RowID, name, value, low, high)
			}
		}
	}
	return nil
}

// applyNpc writes the named fields of each row and reports the ranges changed.
//
// The empty case returns before the table is opened, which the other two do
// not: NpcParam is the one table an operation that names no row of it must not
// pay to open, because most manifests name none.
func applyNpc(bnd4 []byte, patches []rowPatch) ([]byteRange, error) {
	if len(patches) == 0 {
		return nil, nil
	}
	return applyRows(bnd4, npcLayout, "NpcParam", patches, func(row []byte, patch rowPatch) error {
		writeParamRow(row, npcFields, patch.Set)
		return nil
	})
}
