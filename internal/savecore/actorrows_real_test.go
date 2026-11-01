// What the clone namespace actually is, measured on the shipped regulation.
//
// Two claims this product rests on were written down as prose and cited to
// research rather than checked here. Both are measurable against the save the
// snapshot was taken from, so both are checked. Like every other real-thing
// test this needs the private research checkout and skips without it.
package savecore

import (
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// rawTable reads one table's row headers without a TableLayout, so that a
// measurement of what the shipped regulation holds cannot be turned green by
// the layout it is measuring. regulation.OpenTable refuses a table whose row size or data
// version disagrees with npcLayout, which is right for a write and wrong for a
// count.
func rawTable(t *testing.T, bnd4 []byte, entryName string) ([]regulation.ParamRow, []byte) {
	t.Helper()
	_, data, _, rows, err := regulation.LoadParamRows(bnd4, entryName)
	if err != nil {
		t.Fatal(err)
	}
	return rows, data
}

// The clone namespace is exactly the Spirit Summon team, in both directions.
//
// docs/SAFETY.md rule 1 and the snapshot both treat "is this row summon-side?"
// as a question about the row ID. That only holds if FromSoftware's ID
// convention and its own teamType agree, and research measured 115 rows tagged
// [Spirit Summon] independently of any ID arithmetic. This is the two counts
// meeting: not 115 of each, but the same 115 rows.
//
// The offset and the team number are the recipe's own, from clone.go. Reading
// them here is what ties the rule an inserted clone is written to to the rule
// the shipped data already follows.
func TestTheCloneNamespaceIsExactlyTheSpiritSummonTeam(t *testing.T) {
	source := realSave(t)
	rows, data := rawTable(t, source.BND4, npcLayout.Entry)
	stride, err := regulation.UniformRowSize(data, rows)
	if err != nil {
		t.Fatal(err)
	}
	if npcTeamTypeOffset >= stride {
		t.Fatalf("teamType is at %d but the row is %d bytes", npcTeamTypeOffset, stride)
	}
	var matched, inBandNotTeam, teamNotInBand int
	for _, row := range rows {
		inBand := row.ID >= cloneRowOffset && row.ID < cloneRowCeiling
		isTeam := data[row.DataOffset+npcTeamTypeOffset] == spiritSummonTeam
		switch {
		case inBand && isTeam:
			matched++
		case inBand:
			inBandNotTeam++
			t.Errorf("row %d is in the clone namespace but is team %d, not Spirit Summon",
				row.ID, data[row.DataOffset+npcTeamTypeOffset])
		case isTeam:
			teamNotInBand++
			t.Errorf("row %d is a Spirit Summon but sits outside the clone namespace", row.ID)
		}
	}
	if matched != 115 {
		t.Errorf("%d rows are both, want the 115 research measured", matched)
	}
}
