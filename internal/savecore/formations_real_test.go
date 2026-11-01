package savecore

// The formation reader, held against the regulation it will read.
//
// Two offsets and one grouping rule are all this reader is. None of them can be
// checked against a fixture, because a fixture is written by the same hands
// that read it: what these measure is that the shipped table agrees.
//
// Like every other real-thing test, this needs the private research checkout
// and skips without it.

import (
	"context"
	"testing"
)

// Every shipped block is reachable through the reader's own entry point, with
// the rows in the order the table holds them.
func TestTheReaderFindsAShippedBlockInTableOrder(t *testing.T) {
	state, err := ReadFormations(context.Background(), realSavePath(t), []int32{shippedBlockTrigger})
	if err != nil {
		t.Fatal(err)
	}
	block := state.Blocks[shippedBlockTrigger]
	if len(block.Rows) == 0 {
		t.Fatalf("trigger %d has no rows in the shipped regulation", shippedBlockTrigger)
	}
	for index := 1; index < len(block.Rows); index++ {
		if block.Rows[index].RowID <= block.Rows[index-1].RowID {
			t.Errorf("block rows are out of order: %d follows %d",
				block.Rows[index].RowID, block.Rows[index-1].RowID)
		}
	}
	for _, row := range block.Rows {
		if !state.NpcRows[row.Npc] {
			t.Errorf("row %d summons NpcParam %d, which the table does not hold", row.RowID, row.Npc)
		}
		if !state.NpcThinkRows[row.Think] {
			t.Errorf("row %d thinks with NpcThinkParam %d, which the table does not hold", row.RowID, row.Think)
		}
	}
}

// The Lone Wolf block: three rows, the family every batch has used as the
// ordinary case.
const shippedBlockTrigger = int32(232000)
