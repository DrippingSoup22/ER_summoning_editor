package plan

import (
	"testing"
)

const tintSHA = "116e8ecd89a4671df2264807b47d58dfe23dc13613081732c1f671cb39458ec0"

// Both positions write a shipped value into the one row, and neither writes
// anything else. The row is the whole safety argument: PhantomParam 200 has a
// second caller, vfx 51220, so a switch that could name another vfx row could
// change something that has nothing to do with summons.
func TestTheSwitchWritesOneShippedValueIntoOneRow(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		tint  Tint
		value int64
	}{
		{"the white spirit look", SpiritTint, SpiritPhantomProfile},
		// 410 rather than 0: clearing the field also lets the
		// role's red name plate through, and 410 is a profile that multiplies
		// nothing while still being an overwrite.
		{"the creature's own colours", OriginalColours, NeutralPhantomProfile},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			manifest, err := BuildTint(tintSHA, testCase.tint)
			if err != nil {
				t.Fatal(err)
			}
			if len(manifest.VfxPatches) != 1 {
				t.Fatalf("wrote %d vfx patches, want 1", len(manifest.VfxPatches))
			}
			patch := manifest.VfxPatches[0]
			if patch.RowID != SpiritVfxRow {
				t.Errorf("wrote row %d, want %d", patch.RowID, SpiritVfxRow)
			}
			if got := patch.Set["phantomParamOverwriteId"]; got != testCase.value {
				t.Errorf("phantomParamOverwriteId = %v, want %v", got, testCase.value)
			}
			if len(patch.Set) != 1 {
				t.Errorf("set %d fields, want the one the switch is", len(patch.Set))
			}
			if n := len(manifest.GoodsPatches) + len(manifest.BuddyPatches) +
				len(manifest.NpcPatches) + len(manifest.BuddyStoneRowPatches); n != 0 {
				t.Errorf("wrote %d rows outside SpEffectVfxParam", n)
			}
		})
	}
}
