package character

import (
	"bytes"
	"slices"
	"testing"
)

// An effect written into a character's record reads back as one it carries,
// changes nothing else in the file, and empties back to the baseline's bytes.
func TestASavedEffectIsWrittenAndEmptied(t *testing.T) {
	file, _ := baseline(t)
	written := bytes.Clone(file)
	if err := SetSavedEffect(written, 4, 7, 9549); err != nil {
		t.Fatal(err)
	}
	loadout, err := ReadLoadout(written, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(loadout.Effects, 9549) {
		t.Errorf("slot 4 carries %v, want 9549", loadout.Effects)
	}
	changed := 0
	for index := range file {
		if file[index] != written[index] {
			changed++
		}
	}
	if changed == 0 || changed > 8 {
		t.Errorf("%d bytes changed, want the record's id and remaining time only", changed)
	}
	if err := SetSavedEffect(written, 4, 7, NoSavedEffect); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(file, written) {
		t.Error("emptying the record does not give the baseline back")
	}
	if SetSavedEffect(written, 4, 13, 9549) == nil || SetSavedEffect(written, 8, 7, 9549) == nil {
		t.Error("a category past 12 or an empty slot is written")
	}
}
