package savecore

import (
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// Apply's guards defend against something other than a wrong manifest, so the
// writers before them being correct means none can be tripped through it. Two
// are handed a false claim directly here. The other two - the input file
// changed mid-operation, and an output that fails to reopen as intended - are
// inline in Apply; they are load-bearing and deliberately uncovered, so do not
// remove one because coverage says it is idle.
func TestTheWriteGuardsRejectFalseClaims(t *testing.T) {
	bnd4 := loadFixture(t, fixtureSave(t)).BND4

	moved := append([]byte(nil), bnd4...)
	moved[100]++
	if err := verifyBoundedMutation(bnd4, moved, []byteRange{{start: 200, end: 300}}); err == nil {
		t.Error("a byte outside every declared range moved and the bounded-mutation check passed it")
	}
	if err := verifyBoundedMutation(bnd4, moved, []byteRange{{start: 100, end: 101}}); err != nil {
		t.Errorf("a byte inside its declared range was refused: %v", err)
	}

	table, err := regulation.OpenTable(bnd4, buddyLayout)
	if err != nil {
		t.Fatal(err)
	}
	shrunk, err := regulation.RemoveRows(bnd4, buddyLayout, []int32{table.Rows[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyStructuralStage(bnd4, shrunk, &manifest{}); err == nil {
		t.Error("a row vanished with no removal declared and the structural check passed it")
	}
	last := table.Rows[len(table.Rows)-1].ID
	grown, err := regulation.InsertRows(bnd4, buddyLayout, []regulation.RowInsertion{{RowID: last + 1, CopyFrom: last}})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyStructuralStage(bnd4, grown, &manifest{}); err == nil {
		t.Error("a row appeared with no insertion declared and the structural check passed it")
	}
}
