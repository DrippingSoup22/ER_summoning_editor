package plan

import (
	"testing"
)

func mustBuildCost(t *testing.T, costs ...Cost) *Manifest {
	t.Helper()
	manifest, err := BuildCost(testSHA, costs)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func loneWolfCost(price Price) Cost {
	return Cost{HostGoodsFamily: 232000, HostName: "Lone Wolf Ashes", UpgradeLevels: 11, Price: price}
}

// The player means "this Ash costs less", and eleven goods rows carry that.
func TestACostReachesEveryUpgradeLevel(t *testing.T) {
	manifest := mustBuildCost(t, loneWolfCost(Price{FP: 40}))

	if manifest.Operation != OperationCost {
		t.Errorf("operation = %q, want %q", manifest.Operation, OperationCost)
	}
	if len(manifest.GoodsPatches) != 11 {
		t.Fatalf("cost writes %d rows, want the family's 11", len(manifest.GoodsPatches))
	}
	for level, patch := range manifest.GoodsPatches {
		if want := int32(232000 + level); patch.RowID != want {
			t.Errorf("patch %d writes row %d, want %d", level, patch.RowID, want)
		}
		if len(patch.Set) != 2 {
			t.Errorf("patch %d sets %d fields, want the two price fields", level, len(patch.Set))
		}
		if patch.Set["consumeMP"] != int32(40) || patch.Set["consumeHP"] != int32(0) {
			t.Errorf("patch %d sets %v, want 40 FP and no HP", level, patch.Set)
		}
	}
	// Nothing else. A price that could reach the formation trigger would be an
	// operation able to do a conversion's work without being one.
	if len(manifest.BuddyPatches)+len(manifest.BuddyStonePatches)+len(manifest.BuddyStoneRowPatches) != 0 {
		t.Error("a cost edit wrote a table other than EquipParamGoods")
	}
}
