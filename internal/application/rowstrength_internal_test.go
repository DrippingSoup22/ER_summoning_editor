package application

// What a creature's clones carry beyond their recipe, and the ladder an
// inserted row carries.

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// A creature's granted set is the creature's, added once however many slots
// summon it (two Tibia Mariners would write 15349 twice on one clone),
// and a composite's removal rides the same entry - once, and never on a bare
// row, which is the creature as the game ships it.
func TestAGrantIsAddedOncePerCreatureAndARemovalRidesTheEntry(t *testing.T) {
	t.Parallel()
	service := &Service{families: []catalog.Family{
		{Model: "c4950", Name: "Tibia Mariner",
			Variants: []catalog.Variant{{Npc: 49500050, Think: 49500050, Granted: []int32{15349}}}},
		{Model: "c4710", Name: "Rykard",
			Variants: []catalog.Variant{{Npc: 47100038, Think: 47100000, Removed: []int32{11937}}}},
	}}
	twice := []AuthoredSlot{{Npc: 49500050, Think: 49500050}, {Npc: 49500050, Think: 49500050, Perception: PerceptionGuard}}
	if got := service.authoredStrength(twice)[49500050].Residents; len(got) != 1 || got[0] != 15349 {
		t.Errorf("two slots of one creature carry residents %v, want 15349 once", got)
	}
	serpent := []AuthoredSlot{{Npc: 47100038, Think: 47100000}, {Npc: 47100038, Think: 47100000}}
	entry, held := service.authoredStrength(serpent)[47100038]
	if !held || len(entry.Residents) != 0 {
		t.Fatalf("the Serpent's entry is %+v (held %v), want a removal-only entry", entry, held)
	}
	if got := entry.Removes; len(got) != 1 || got[0] != 11937 {
		t.Errorf("removes = %v, want 11937 once", got)
	}
	if bare := service.authoredStrength([]AuthoredSlot{{Npc: 47100038, Think: 47100000, BareRow: true}}); len(bare[47100038].Removes) != 0 {
		t.Errorf("a bare row still carried the removal: %v", bare)
	}
}

// A row this editor inserted carries the ladder of the block's last shipped
// row, the one the first insert copied: in the Mimic Tear's block that is the
// copy's own ladder (290200 on), not the tear's (290000 on).
func TestAnInsertedRowCarriesTheLadderItWasCopiedFrom(t *testing.T) {
	t.Parallel()
	snapshot, err := reference.Vanilla()
	if err != nil {
		t.Fatal(err)
	}
	block := savecore.FormationBlock{Trigger: 207000, Rows: []savecore.FormationRow{
		{RowID: 20700000}, {RowID: 20700001}, {RowID: 20700002}}}
	ladders := ashLadders(block, snapshot)
	for row, first := range map[int32]int32{20700000: 290000, 20700001: 290200, 20700002: 290200} {
		ladder, known := ladders[row]
		if !known || ladder[0] != first || ladder[10] != first+10 {
			t.Errorf("row %d carries %v (known %v), want %d to %d", row, ladder, known, first, first+10)
		}
	}
}
