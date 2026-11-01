package savecore

import (
	"slices"
	"testing"
)

// A presence copy is written where the recipe writes 295000 and a marker copy
// where it writes 297000, and nothing else moves (the per-creature spirit
// look, the quiet marker).
func TestAPresenceAndAMarkerAreWrittenInPlaceOfTheirs(t *testing.T) {
	last := 0
	for _, offset := range npcResidentEffectOffsets {
		last = max(last, offset+4)
	}
	row := make([]byte, last)
	for _, offset := range npcResidentEffectOffsets {
		writeS32(row, offset, -1)
	}
	plain, err := summonResidentEffects(row, false, false, false, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := summonResidentEffects(row, false, false, false, nil, nil, OwnColoursPresence, QuietMarker)
	if err != nil {
		t.Fatal(err)
	}
	swapped := map[int32]int32{spiritPresenceEffect: OwnColoursPresence, spiritFollowEffect: QuietMarker}
	for from, to := range swapped {
		if !slices.Contains(plain, from) || slices.Contains(copied, from) || !slices.Contains(copied, to) {
			t.Fatalf("plain %v, with the copies %v", plain, copied)
		}
	}
	for index := range plain {
		if plain[index] != copied[index] && swapped[plain[index]] != copied[index] {
			t.Errorf("slot %d: %d became %d", index, plain[index], copied[index])
		}
	}
}
