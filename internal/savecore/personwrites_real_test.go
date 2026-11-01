// The person writes rest on a layout read off Paramdex (CHARACTER_INIT_PARAM,
// version 2, 320 bytes) and on shipped values research measured on 11711000.
// This holds both to the real save: the table opens at that stride, every
// field of the surface holds its shipped value, and a write lands in its own
// row and nowhere else. It needs the private research checkout and skips
// without it, like the other real-save tests.
package savecore

import (
	"bytes"
	"testing"
)

func TestAPersonWriteLandsInItsOwnRowOnly(t *testing.T) {
	source := realSave(t)
	before := bytes.Clone(source.BND4)
	write := personWrites[0]
	ranges, err := applyPersons(source.BND4, []rowPatch{{RowID: write.Row, Set: map[string]float64{write.Field: write.Written}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 1 {
		t.Fatalf("one row written, %d ranges declared", len(ranges))
	}
	if err := verifyBoundedMutation(before, source.BND4, ranges); err != nil {
		t.Fatal(err)
	}
	values, err := readPersonWrites(source.BND4)
	if err != nil {
		t.Fatal(err)
	}
	if got := values[PersonKey{write.Row, write.Field}]; got != write.Written {
		t.Fatalf("%d.%s reads %g after the write, not %g", write.Row, write.Field, got, write.Written)
	}
}
