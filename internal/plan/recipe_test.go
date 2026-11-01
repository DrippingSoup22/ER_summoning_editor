package plan

import (
	"slices"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// The recipe reaches the manifest in two places. A Follower's
// clone carries 296010 as a resident - on a keep-balance entry of its own, the
// route a slot's residents take, so the summon stays a helper - and no other
// level carries it. And an actor whose battle script cannot turn is marked
// no_turn on its think clone's perception entry, which is where the clone
// recipe reads it to give the three middle levels a nose.
func TestTheRecipeReachesTheManifest(t *testing.T) {
	block := formation.Block{Trigger: 259000, Rows: []formation.BlockRow{
		{RowID: 25900000, Doping: shippedDoping()}, {RowID: 25900001, Doping: shippedDoping()}}}
	save := formation.Save{BuddyRows: map[int32]bool{25900000: true, 25900001: true}}
	follower := formation.Actor{Npc: 40700000, Think: 40700000, Perception: formation.PerceptionFollower, NoTurn: true}
	guard := formation.Actor{Npc: 42600000, Think: 42600000, Perception: formation.PerceptionGuard}

	guardStance, _ := formation.LevelStance(guard)
	compiled, err := formation.Compile(formation.Formation{Slots: []formation.Slot{{Actor: follower}, {Actor: guard, Stance: guardStance}}}, block, save)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildAuthor(strings.Repeat("0", 64), *compiled, Vanilla{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	residents := map[int32]CloneStrength{}
	for _, entry := range manifest.NpcCloneStrength {
		residents[entry.Source] = entry
	}
	got, carried := residents[40700000]
	if !carried || !got.KeepBalance || got.HP != 0 || !slices.Equal(got.Residents, []int32{formation.FollowerTargetPriority}) {
		t.Errorf("the follower's clone carries %+v (present %v), want 296010 alone on a keep-balance entry", got, carried)
	}
	if other, carried := residents[42600000]; carried {
		t.Errorf("the guard's clone carries %+v; 296010 is the Follower's alone", other)
	}
	noTurn := map[int32]bool{}
	for _, entry := range manifest.NpcThinkPerception {
		noTurn[entry.Source] = entry.NoTurn
	}
	if !noTurn[40700000] || noTurn[42600000] {
		t.Errorf("no_turn = %v, want it on the follower's think clone alone", noTurn)
	}

	// And beside the creature's own strength the Follower's resident joins
	// its entry rather than standing beside it: one clone, one entry.
	full, err := BuildAuthor(strings.Repeat("0", 64), *compiled, Vanilla{},
		Strength{40700000: {HelperHP: 500, Residents: []int32{7100}}})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range full.NpcCloneStrength {
		if entry.Source != 40700000 {
			continue
		}
		count++
		if entry.HP != 500 || !slices.Equal(entry.Residents, []int32{7100, formation.FollowerTargetPriority}) {
			t.Errorf("the follower's own strength is %+v, want hp 500 and residents 7100 then 296010", entry)
		}
	}
	if count != 1 {
		t.Errorf("the follower has %d strength entries, want one", count)
	}
}

// The share box reaches the manifest on the think clone's perception entry
// a Guard, Sentinel or Hunter left at its default -
// share off - and a Follower the player turned off are marked own_target, so
// the write gives them the damage rate 100 that goes with share off. A
// Follower at its default, a Guard turned on, and the Statue (whose share the
// level fixes) are not.
func TestTheShareBoxReachesTheManifest(t *testing.T) {
	for _, want := range []struct {
		name  string
		actor formation.Actor
		own   bool
	}{
		{"a follower at its default", formation.Actor{Perception: formation.PerceptionFollower}, false},
		{"a follower turned off", formation.Actor{Perception: formation.PerceptionFollower, Share: formation.ShareOff}, true},
		{"a guard at its default", formation.Actor{Perception: formation.PerceptionGuard}, true},
		{"a guard turned on", formation.Actor{Perception: formation.PerceptionGuard, Share: formation.ShareOn}, false},
		{"a sentinel at its default", formation.Actor{Perception: formation.PerceptionSentinel}, true},
		{"a hunter at its default", formation.Actor{Perception: formation.PerceptionHunter}, true},
		{"the statue", formation.Actor{Perception: formation.PerceptionStatue}, false},
	} {
		block := formation.Block{Trigger: 259000, Rows: []formation.BlockRow{{RowID: 25900000, Doping: shippedDoping()}}}
		save := formation.Save{BuddyRows: map[int32]bool{25900000: true}}
		actor := want.actor
		actor.Npc, actor.Think = 42600000, 42600000
		stance, _ := formation.LevelStance(actor)
		compiled, err := formation.Compile(formation.Formation{Slots: []formation.Slot{{Actor: actor, Stance: stance}}}, block, save)
		if err != nil {
			t.Fatalf("%s: %v", want.name, err)
		}
		manifest, err := BuildAuthor(strings.Repeat("0", 64), *compiled, Vanilla{}, nil)
		if err != nil {
			t.Fatalf("%s: %v", want.name, err)
		}
		if len(manifest.NpcThinkPerception) != 1 || manifest.NpcThinkPerception[0].OwnTarget != want.own {
			t.Errorf("%s: think perception %+v, want own_target %t", want.name, manifest.NpcThinkPerception, want.own)
		}
	}
}
