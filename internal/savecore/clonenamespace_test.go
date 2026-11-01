package savecore

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// The clone bands are the model's rule too, written twice and checked once.
func TestTheCloneBandsAgreeWithTheAuthoringModel(t *testing.T) {
	if len(cloneBands) != len(formation.CloneBands) {
		t.Fatalf("savecore knows %d bands, formation %d", len(cloneBands), len(formation.CloneBands))
	}
	for index, band := range formation.CloneBands {
		if cloneBands[index] != band {
			t.Errorf("band %d: savecore %d, formation %d", index, cloneBands[index], band)
		}
	}
	for _, row := range []int32{100000000, 140700000, 199999999, 240700000, 440700000, 540700000, 640700000, 2040700000, 200000000, 99999999, -1} {
		if inCloneBand(row) != formation.InCloneBand(row) {
			t.Errorf("row %d: savecore says %v, formation %v", row, inCloneBand(row), formation.InCloneBand(row))
		}
	}
}
