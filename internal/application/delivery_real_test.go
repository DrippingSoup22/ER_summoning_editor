package application_test

import (
	"context"
	"encoding/binary"
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// How a creature is delivered reaches the save, on the real catalog: St. Trina
// arrives on her own 30000; the mimic tear is a pair - the tear holding its
// spot on 1831 and the Silver Tear person on the same spot, on the Mimic Tear's
// shell, on 60500 - a spot 3 m ahead written 3.2 m short, -0.2, since a summon
// stands that much further ahead than its row says (seen on the console); the torch Commoner carries 14709, a copy of 14707, in its
// place; and the Promised Consort the rows of our own that hold his phase.
func TestACreaturesDeliveryReachesTheSave(t *testing.T) {
	t.Parallel()
	const tear, torchCommoner, consort = int32(33201065), int32(36603020), int32(52200089)
	ctx := context.Background()
	session := loadedSession(t, realSavePath(t))
	spot := application.Placement{X: 2, Z: 3}
	slots := []application.AuthoredSlot{
		{Npc: 53300087, Think: 53300000},
		{Npc: tear, Think: 33200000, Placement: &spot},
		{Npc: torchCommoner, Think: 36603000},
		{Npc: consort, Think: 52200000},
	}
	for index := range slots {
		slots[index].Stance, _ = application.LevelStance(slots[index])
	}
	if _, err := session.StageFormation(authoringHost, slots); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "delivered.dat")
	if _, err := session.Commit(ctx, mustPrepare(t, session), path, ""); err != nil {
		t.Fatal(err)
	}

	formations, err := savecore.ReadFormations(ctx, path, []int32{authoringTrigger})
	if err != nil {
		t.Fatal(err)
	}
	rows := formations.Blocks[authoringTrigger].Rows
	if len(rows) != len(slots)+1 {
		t.Fatalf("four slots with one pair wrote %d rows, want %d", len(rows), len(slots)+1)
	}
	trina, blob, person := rows[0], rows[1], rows[2]
	if trina.GenerateAnimId != 30000 || blob.GenerateAnimId != 1831 || person.GenerateAnimId != 60500 {
		t.Errorf("St. Trina, the tear and its person arrive on %d, %d and %d, want 30000, 1831 and 60500",
			trina.GenerateAnimId, blob.GenerateAnimId, person.GenerateAnimId)
	}
	if person.Npc != 100000010 || person.NpcPlayerInitParamId != 23682 || person.XOffset != blob.XOffset ||
		person.ZOffset != blob.ZOffset || blob.XOffset != float32(spot.X) || blob.ZOffset != -0.2 {
		t.Errorf("the tear stands at %v/%v and its partner is %d as %d at %v/%v", blob.XOffset, blob.ZOffset,
			person.Npc, person.NpcPlayerInitParamId, person.XOffset, person.ZOffset)
	}
	if statue, _ := application.LevelStance(application.AuthoredSlot{Perception: application.PerceptionStatue}); blob.PcFollowType != int32(statue.PcFollowType) {
		t.Errorf("the tear follows as %d, want the Statue's hold %d", blob.PcFollowType, statue.PcFollowType)
	}

	state, err := savecore.ReadState(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for clone, want := range map[int32][]int32{
		rows[3].Npc: {14709},
		rows[4].Npc: {20011605, 20011606},
	} {
		for _, effect := range want {
			if !state.OwnEffects[effect] || !slices.Contains(state.OwnEffectNames[clone], effect) {
				t.Errorf("clone %d names %v, want our own %d, which the save holds: %t", clone,
					state.OwnEffectNames[clone], effect, state.OwnEffects[effect])
			}
		}
	}
	// SpEffectParam: motionInterval 12, cycleOccurrenceSpEffectId 296.
	loaded, err := regulation.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	effects, effectRows, err := regulation.LoadRowsByID(loaded.BND4, "SpEffectParam.param", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	hold := effects[effectRows[20011605].DataOffset:]
	if interval, cycled := math.Float32frombits(binary.LittleEndian.Uint32(hold[12:])), int32(binary.LittleEndian.Uint32(hold[296:])); interval != 0.01 || cycled != 20011598 {
		t.Errorf("20011605 re-applies %d every %v s, want 20011598 every 0.01", cycled, interval)
	}
}
