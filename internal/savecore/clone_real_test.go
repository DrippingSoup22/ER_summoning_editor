// The summon-side recipe, held against the summons the game ships.
//
// The obvious test would be to apply the recipe to a world actor and check the
// result equals FromSoftware's own clone of it. That test would be false, and
// measuring it is what these tests are instead. Of the 94 clones with a world
// twin, none is reproducible from the twin: the shipped clones carry curated
// resident-effect vectors and reduced hitboxes that no rule derives, because
// each was authored rather than generated.
//
// So the claim is not that the recipe reproduces a shipped clone. It is that
// every property the recipe guarantees is one that all 115 shipped clones
// already have - the game's own summons pass the rules an inserted clone is
// written to. That is checkable, it is what makes the rules the game's rather
// than this product's, and the two places the shipped data does not agree with
// research's recipe are named below rather than smoothed over.
//
// Like every other real-thing test this needs the private research checkout and
// skips without it.
package savecore

import (
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// shippedClones is every NpcParam row in the clone namespace, by ID, with its
// body. It reads through rawTable rather than OpenTable for the reason given
// there: a measurement of the shipped regulation should not be gated on the
// layout being measured.
func shippedClones(t *testing.T, bnd4 []byte) ([]int32, map[int32][]byte) {
	t.Helper()
	rows, data := rawTable(t, bnd4, npcLayout.Entry)
	stride, err := regulation.UniformRowSize(data, rows)
	if err != nil {
		t.Fatal(err)
	}
	if stride != npcLayout.RowSize {
		t.Fatalf("NpcParam rows are %d bytes, npcLayout declares %d", stride, npcLayout.RowSize)
	}
	ids := make([]int32, 0, 128)
	bodies := make(map[int32][]byte, 128)
	for _, row := range rows {
		if row.ID < cloneRowOffset || row.ID >= cloneRowCeiling {
			continue
		}
		ids = append(ids, row.ID)
		bodies[row.ID] = data[row.DataOffset : row.DataOffset+stride]
	}
	slices.Sort(ids)
	return ids, bodies
}

// Every scalar the recipe writes, it writes to a value all 115 shipped summons
// already hold. These are not this product's opinions about what a summon
// should be; they are what the game's own summons are.
func TestEveryShippedSummonAlreadySatisfiesTheScalarRecipe(t *testing.T) {
	source := realSave(t)
	ids, bodies := shippedClones(t, source.BND4)
	if len(ids) != 115 {
		t.Fatalf("found %d clone rows, want the recorded 115", len(ids))
	}
	for _, id := range ids {
		row := bodies[id]
		if row[npcTeamTypeOffset] != spiritSummonTeam {
			t.Errorf("clone %d is on team %d, not Spirit Summon", id, row[npcTeamTypeOffset])
		}
		if got := readS32(row, npcGetSoulOffset); got != 0 {
			t.Errorf("clone %d pays out %d runes", id, got)
		}
		if got := readS32(row, npcItemLotEnemyOffset); got != -1 {
			t.Errorf("clone %d carries enemy item lot %d", id, got)
		}
		if got := readS32(row, npcItemLotMapOffset); got != -1 {
			t.Errorf("clone %d carries map item lot %d", id, got)
		}
		if row[npcSoulByBossFlagsOffset]&npcSoulByBossFlagMask != 0 {
			t.Errorf("clone %d pays out a boss soul", id)
		}
		if got := f32At(row, npcHitHeightOffset); got > summonHitHeightCeiling {
			t.Errorf("clone %d is %v tall, past the ceiling %v", id, got, float32(summonHitHeightCeiling))
		}
		if got := f32At(row, npcHitRadiusOffset); got > summonHitRadiusCeiling {
			t.Errorf("clone %d is %v wide, past the ceiling %v", id, got, float32(summonHitRadiusCeiling))
		}
	}
}
