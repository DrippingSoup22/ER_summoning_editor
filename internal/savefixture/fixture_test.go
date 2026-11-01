// A fixture nobody checks is a second thing that can be wrong. These tests
// prove the synthetic save is well formed and carries the seeded values before
// any product code is pointed at it.
package savefixture

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

func TestBuiltSaveReadsBackThroughItsOwnReader(t *testing.T) {
	save, err := Build(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := Open(save)
	if err != nil {
		t.Fatal(err)
	}
	if got := reader.RegulationVersion(); got != 11701000 {
		t.Fatalf("regulation version = %d, want 11701000", got)
	}
	for _, entry := range []string{
		"EquipParamGoods.param", "BuddyParam.param",
		"BuddyStoneParam.param", "SpEffectParam.param",
	} {
		ids, err := reader.RowIDs(entry)
		if err != nil {
			t.Fatalf("%s: %v", entry, err)
		}
		if len(ids) < 2 {
			t.Fatalf("%s has %d rows, too few for a stride", entry, len(ids))
		}
	}
}

// VanillaOptions is the fixture reset needs — every row the shipped snapshot
// records, rather than the handful Defaults carries. It had no test in this
// package because only other packages call it, which meant a fault in it would
// have surfaced as a confusing failure somewhere else.
func TestVanillaOptionsCarryEveryShippedRow(t *testing.T) {
	options, err := VanillaOptions()
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int, len(options.Tables))
	for _, table := range options.Tables {
		counts[table.Entry] = len(table.RowIDs)
	}
	for entry, want := range map[string]int{
		"EquipParamGoods.param": 924,
		// All 170 BuddyParam rows, not the 20 carrying the monument flag: a
		// reset restores four fields on every row, so a fixture missing 150 of
		// them would fail at the writer rather than testing anything.
		"BuddyParam.param":      170,
		"BuddyStoneParam.param": 304,
	} {
		if counts[entry] != want {
			t.Errorf("%s carries %d rows, want %d", entry, counts[entry], want)
		}
	}

	// The actor tables are checked by containment rather than by count. Every
	// clone row the snapshot records has to be present, but the world sources
	// beside them are not one-to-one: the clone at exactly the offset has no
	// source below it, and a count would have to encode that arithmetic instead
	// of the rule.
	snapshot, err := reference.Vanilla()
	if err != nil {
		t.Fatal(err)
	}
	rowsOf := func(entry string) map[int32]bool {
		present := map[int32]bool{}
		for _, table := range options.Tables {
			if table.Entry == entry {
				for _, rowID := range table.RowIDs {
					present[rowID] = true
				}
			}
		}
		return present
	}
	for entry, clones := range map[string][]int32{
		"NpcParam.param":      snapshot.NpcCloneRows,
		"NpcThinkParam.param": snapshot.NpcThinkCloneRows,
	} {
		present := rowsOf(entry)
		if len(clones) == 0 {
			t.Fatalf("the snapshot records no clone rows for %s", entry)
		}
		for _, rowID := range clones {
			if !present[rowID] {
				t.Errorf("%s is missing shipped clone row %d", entry, rowID)
			}
		}
	}

	save, err := Build(options)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := Open(save)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := VanillaGoodsRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 924 {
		t.Fatalf("VanillaGoodsRows returned %d rows, want 924", len(rows))
	}
	for _, rowID := range rows {
		if _, _, err := reader.GoodsTrigger(rowID); err != nil {
			t.Fatalf("goods row %d is missing from the vanilla fixture: %v", rowID, err)
		}
	}
}
