package savecore

import (
	"fmt"
	"sort"
	"testing"
)

// TestEveryBuddyFieldIsAccountedFor is the guard the write set never had.
//
// A BuddyParam row is 160 bytes. Every one of them must be claimed either by
// buddyFields, which is what the product writes and a reset restores, or by
// buddyInheritedFields, which is what an authored formation deliberately leaves
// alone and why. A byte in neither is a field nobody has decided about, and a
// field nobody has decided about is how the host leaked into three authored
// formations already: the mount pair, the stance pair, and npcPlayerInitParamId.
//
// So this is a coverage claim over the row rather than a list of names to keep
// in step. Adding a field to buddyFields without removing the bytes it occupies
// from the inherited table fails here, and so does the reverse, which is what
// makes "decide" the only way through.
func TestEveryBuddyFieldIsAccountedFor(t *testing.T) {
	claimed := make(map[int]string, buddyLayout.RowSize)

	claim := func(name string, offset, width int) {
		t.Helper()
		if offset < 0 || offset+width > buddyLayout.RowSize {
			t.Fatalf("%s occupies %d..%d, outside the %d-byte row",
				name, offset, offset+width-1, buddyLayout.RowSize)
		}
		for at := offset; at < offset+width; at++ {
			if other, taken := claimed[at]; taken {
				t.Errorf("byte %d is claimed by both %s and %s", at, other, name)
				continue
			}
			claimed[at] = name
		}
	}

	for name, field := range buddyFields {
		claim(name, field.offset, fieldWidth(t, field.kind))
	}
	for name, field := range buddyInheritedFields {
		claim(name, field.offset, field.width)
		if field.why == "" {
			t.Errorf("%s is inherited with no reason recorded; write one", name)
		}
	}

	var unclaimed []int
	for at := 0; at < buddyLayout.RowSize; at++ {
		if _, taken := claimed[at]; !taken {
			unclaimed = append(unclaimed, at)
		}
	}
	if len(unclaimed) > 0 {
		sort.Ints(unclaimed)
		t.Fatalf("%d bytes of the BuddyParam row are in neither buddyFields nor "+
			"buddyInheritedFields: %v\nAdd them to one, with the reason if inherited.",
			len(unclaimed), unclaimed)
	}
}

func fieldWidth(t *testing.T, kind string) int {
	t.Helper()
	switch kind {
	case "s32", "u32":
		return 4
	case "s16", "u16":
		return 2
	case "u8":
		return 1
	case "f32":
		return 4
	}
	t.Fatal(fmt.Errorf("no width for field kind %q", kind))
	return 0
}
