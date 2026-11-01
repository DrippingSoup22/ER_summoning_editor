// What an inserted clone has to be, checked on the synthetic fixture.
//
// The fixture's world actors are seeded as enemies that pay out, scale to their
// region and are too large to stand beside the player, so every field the
// recipe writes starts somewhere it must not stay. clone_real_test.go then
// holds the same recipe against the 115 summons the game ships.
package savecore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// worldActor is a fixture world row with no clone beside it, which is the only
// thing clone insertion is for.
var worldActor = savefixture.CloneableWorldActors[0]

// cloneFixture is the fixture's decrypted regulation, fresh each time, because
// insertion rebuilds the archive around the slice it is handed.
func cloneFixture(t *testing.T) []byte {
	t.Helper()
	return loadFixture(t, fixtureSave(t)).BND4
}

// clonedRow returns the inserted clone's body, read back from the archive
// through an independent open rather than from the writer's own slice.
func clonedRow(t *testing.T, bnd4 []byte, layout regulation.TableLayout, source int32) []byte {
	t.Helper()
	table, err := regulation.OpenTable(bnd4, layout)
	if err != nil {
		t.Fatal(err)
	}
	row, present := table.ByID[source+cloneRowOffset]
	if !present {
		t.Fatalf("%s holds no clone of actor %d", layout.Entry, source)
	}
	return table.Data[row.DataOffset : row.DataOffset+layout.RowSize]
}

func f32At(row []byte, offset int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(row[offset:]))
}

func residentEffects(row []byte) []int32 {
	effects := make([]int32, 0, len(npcResidentEffectOffsets))
	for _, offset := range npcResidentEffectOffsets {
		effects = append(effects, readS32(row, offset))
	}
	return effects
}

// The whole point of the phase: a clone is not the row it was copied from.
//
// Each of these is a way the copy would have been wrong, and the fixture seeds
// every one of them to a value the recipe has to move.
func TestAnInsertedCloneIsSummonSideAndNotACopy(t *testing.T) {
	seed := savefixture.NpcWorldActor
	grown, err := insertNpcClones(cloneFixture(t), []int32{worldActor}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clone := clonedRow(t, grown, npcLayout, worldActor)

	if clone[npcTeamTypeOffset] != spiritSummonTeam {
		t.Errorf("clone is on team %d, not the Spirit Summon %d it must join",
			clone[npcTeamTypeOffset], spiritSummonTeam)
	}
	for _, field := range []struct {
		name   string
		offset int
		want   int32
	}{
		{"getSoul", npcGetSoulOffset, 0},
		{"itemLot_enemy", npcItemLotEnemyOffset, -1},
		{"itemLot_map", npcItemLotMapOffset, -1},
		{"GameClearSpEffectID", npcGameClearEffectOffset, -1},
	} {
		if got := readS32(clone, field.offset); got != field.want {
			t.Errorf("clone %s is %d, want %d: a summon is carried, not found",
				field.name, got, field.want)
		}
	}
	if clone[npcSoulByBossFlagsOffset]&npcSoulByBossFlagMask != 0 {
		t.Error("clone still pays out a boss soul")
	}
	// The rest of the flags byte is not the recipe's business, and clearing it
	// wholesale would be a wider write than the one that was proved.
	if want := seed.SoulByBossFlags &^ npcSoulByBossFlagMask; clone[npcSoulByBossFlagsOffset] != want {
		t.Errorf("clone soulByBossFlags is %#b, want %#b: only the boss-soul bit may move",
			clone[npcSoulByBossFlagsOffset], want)
	}
	if got := f32At(clone, npcHitHeightOffset); got != summonHitHeightCeiling {
		t.Errorf("clone hitHeight is %v, want the ceiling %v", got, float32(summonHitHeightCeiling))
	}
	if got := f32At(clone, npcHitRadiusOffset); got != summonHitRadiusCeiling {
		t.Errorf("clone hitRadius is %v, want the ceiling %v", got, float32(summonHitRadiusCeiling))
	}

	effects := residentEffects(clone)
	for _, effect := range effects {
		if effect >= areaScalingFirst && effect < areaScalingPast {
			t.Errorf("clone still carries world-area scaling effect %d", effect)
		}
		if effect >= dlcAreaScalingFirst && effect < dlcAreaScalingPast {
			t.Errorf("clone still carries Shadow of the Erdtree scaling effect %d (on the console: felt as too much damage on a +0 Ash)", effect)
		}
	}
	if slotOf(effects, seed.DlcScalingEffect) >= 0 {
		t.Errorf("clone kept the source's DLC tier %d", seed.DlcScalingEffect)
	}
	for _, required := range spiritCompanionEffects {
		if slotOf(effects, required) < 0 {
			t.Errorf("clone does not carry Spirit Summon effect %d", required)
		}
	}
	if slotOf(effects, seed.KeptEffect) < 0 {
		t.Errorf("clone lost the source's own effect %d, which the recipe has no reason to touch",
			seed.KeptEffect)
	}
}

// Insertion's own proof still has to hold underneath the recipe: the archive
// grew by exactly the declared rows and nothing that was already there moved.
func TestWritingTheRecipeDisturbsNothingOutsideTheNewRows(t *testing.T) {
	before := cloneFixture(t)
	original, err := regulation.OpenTable(before, npcLayout)
	if err != nil {
		t.Fatal(err)
	}
	shipped := map[int32][]byte{}
	for _, row := range original.Rows {
		shipped[row.ID] = append([]byte(nil), original.Data[row.DataOffset:row.DataOffset+npcLayout.RowSize]...)
	}
	grown, err := insertNpcClones(before, []int32{worldActor}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := regulation.OpenTable(grown, npcLayout)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Rows) != len(original.Rows)+1 {
		t.Fatalf("table holds %d rows, want %d", len(after.Rows), len(original.Rows)+1)
	}
	for id, body := range shipped {
		row, present := after.ByID[id]
		if !present {
			t.Errorf("row %d disappeared", id)
			continue
		}
		if !bytes.Equal(after.Data[row.DataOffset:row.DataOffset+npcLayout.RowSize], body) {
			t.Errorf("row %d changed while a clone was being inserted beside it", id)
		}
	}
}

// The clone destination is arithmetic, so the rules docs/SAFETY.md rule 1
// states are refusals rather than conventions a caller is asked to follow.
func TestCloningRefusesAnythingRule1Forbids(t *testing.T) {
	for _, refusal := range []struct {
		name    string
		sources []int32
		want    string
	}{
		// A key at or above the offset names a clone in a
		// higher band (136610000 is 36610000's band-2 clone), so what rule 1
		// refuses is a key outside the bands: band 5 is the game's own
		// scheme, band 21 is past an s32, and a key must name a row.
		{"a key in band 5, the game's own scheme", []int32{400000000 + 36610000}, "not a clone key"},
		{"a key past the last band", []int32{2000000000 + 36610000}, "not a clone key"},
		{"a key naming no row", []int32{cloneRowOffset}, "not a clone key"},
		{"a negative ID", []int32{-1}, "not a clone key"},
		{"a higher band of an actor the save does not hold", []int32{cloneRowOffset + 40000000}, "was not found"},
		{"an actor the save does not hold", []int32{40000000}, "was not found"},
		{"the same actor twice", []int32{worldActor, worldActor}, "cloned twice"},
		{"nothing at all", nil, "no actors to clone"},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			_, err := insertNpcClones(cloneFixture(t), refusal.sources, nil, nil, nil, nil, nil)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), refusal.want) {
				t.Errorf("refused with %q, want something about %q", err, refusal.want)
			}
		})
	}
}

// A session's parts each keep their own clone strength when they are merged.
//
// flattenSession is a field-by-field union, so a field left out of it is a
// field that works alone and disappears the moment the player stages a second
// operation. The strength list has to travel with the clone sources it
// modifies: without it, one formation would summon creatures and the next
// would summon helpers, and nothing in the save would say why.
func TestFlatteningASessionKeepsEveryPartsCloneStrength(t *testing.T) {
	plan := &manifest{
		Operation: operationSession,
		Parts: []*manifest{
			{
				Operation:        operationAuthor,
				NpcCloneSources:  []int32{11000000},
				NpcCloneStrength: []npcCloneStrength{{Source: 11000000, HP: 1234}},
			},
			{
				Operation:       operationAuthor,
				NpcCloneSources: []int32{22000000},
				NpcCloneStrength: []npcCloneStrength{
					{Source: 22000000, HP: 5678, Residents: []int32{7540}},
				},
			},
		},
	}
	flattenSession(plan)
	if len(plan.NpcCloneSources) != 2 {
		t.Fatalf("merged %d clone sources, want 2", len(plan.NpcCloneSources))
	}
	if len(plan.NpcCloneStrength) != 2 {
		t.Fatalf("merged %d strength entries for 2 sources, want 2", len(plan.NpcCloneStrength))
	}
	bySource := map[int32]npcCloneStrength{}
	for _, entry := range plan.NpcCloneStrength {
		bySource[entry.Source] = entry
	}
	if got := bySource[11000000]; got.HP != 1234 {
		t.Errorf("the first part's strength merged as %+v", got)
	}
	if got := bySource[22000000]; got.HP != 5678 || len(got.Residents) != 1 || got.Residents[0] != 7540 {
		t.Errorf("the second part's strength merged as %+v", got)
	}
}

// The namespace bound, the field set, and the operation that owns them.
func TestAStrengthPatchIsRefusedOutsideItsSurface(t *testing.T) {
	for name, plan := range map[string]*manifest{
		"a world actor": {Operation: operationStrength, NpcPatches: []rowPatch{
			{RowID: 40700000, Set: map[string]float64{"hp": 1}}}},
		"above the namespace": {Operation: operationStrength, NpcPatches: []rowPatch{
			{RowID: 200000000, Set: map[string]float64{"hp": 1}}}},
		"an unknown field": {Operation: operationStrength, NpcPatches: []rowPatch{
			{RowID: 140700000, Set: map[string]float64{"teamType": 47}}}},
		"nothing set": {Operation: operationStrength, NpcPatches: []rowPatch{
			{RowID: 140700000, Set: map[string]float64{}}}},
		"the same row twice": {Operation: operationStrength, NpcPatches: []rowPatch{
			{RowID: 140700000, Set: map[string]float64{"hp": 1}},
			{RowID: 140700000, Set: map[string]float64{"hp": 2}}}},
		"another operation": {Operation: operationAuthor, NpcPatches: []rowPatch{
			{RowID: 140700000, Set: map[string]float64{"hp": 1}}}},
	} {
		if err := validateNpcPatches(plan); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A ladder patch writes all eleven reinforcement levels and nothing else. A
// gap would be a formation that is the creature at some upgrade levels and a
// helper at others; an identity field would be a strength switch able to change
// which creature is summoned.
func TestAStrengthPatchWritesTheWholeLadderAndOnlyTheLadder(t *testing.T) {
	whole := func() map[string]float64 {
		set := map[string]float64{}
		for level := 0; level < dopingLevels; level++ {
			set[fmt.Sprintf("dopingSpEffect_lv%d", level)] = float64(290000 + level)
		}
		return set
	}
	if err := validateBuddyLadderPatch(0, rowPatch{RowID: 1, Set: whole()}); err != nil {
		t.Fatalf("a whole ladder was refused: %v", err)
	}
	gap := whole()
	delete(gap, "dopingSpEffect_lv7")
	if err := validateBuddyLadderPatch(0, rowPatch{RowID: 1, Set: gap}); err == nil {
		t.Error("a ladder with a gap was accepted")
	}
	extra := whole()
	extra["npcParamId"] = 140700000
	if err := validateBuddyLadderPatch(0, rowPatch{RowID: 1, Set: extra}); err == nil {
		t.Error("a strength patch naming an actor was accepted")
	}
}

// A think clone carries its level's profile - "blind": no reach, all round,
// never home - and nothing else on the row moves; "world" is the buddy flag
// alone; a level the recipe does not know is refused before anything is
// written.
func TestAThinkCloneCarriesTheNamedPerceptionProfileAndNothingElseMoves(t *testing.T) {
	grown, err := insertNpcThinkClones(cloneFixture(t), []int32{worldActor}, map[int32]thinkChoice{worldActor: {Level: "blind"}})
	if err != nil {
		t.Fatal(err)
	}
	clone := clonedRow(t, grown, npcThinkLayout, worldActor)
	want := map[string]float64{"eye_dist": 0, "BattleStartDist": 0, "searchEye_dist": 0, "nose_dist": 0, "eye_angX": 180, "eye_angY": 180,
		"backhomeDist": 9979, "maxBackhomeDist": 9999, "SoundTargetForgetTime": 0.5, "MemoryTargetForgetTime": 0.1}
	for name, value := range want {
		field := thinkFields[name]
		var got float64
		switch field.kind {
		case "u8":
			got = float64(clone[field.offset])
		case "u16":
			got = float64(binary.LittleEndian.Uint16(clone[field.offset:]))
		case "f32":
			got = float64(math.Float32frombits(binary.LittleEndian.Uint32(clone[field.offset:])))
		}
		if float32(got) != float32(value) {
			t.Errorf("%s = %g on the blind clone, want %g", name, got, value)
		}
	}
	if clone[npcThinkIsBuddyAIOffset] != 1 {
		t.Error("the blind clone lost the buddy flag")
	}
	// Everything outside the profile's fields and the flag is the source.
	source, err := regulation.OpenTable(grown, npcThinkLayout)
	if err != nil {
		t.Fatal(err)
	}
	world := source.ByID[worldActor]
	body := append([]byte(nil), source.Data[world.DataOffset:world.DataOffset+npcThinkLayout.RowSize]...)
	touched := map[int]bool{npcThinkIsBuddyAIOffset: true}
	for name := range thinkProfiles["blind"] {
		field := thinkFields[name]
		width := map[string]int{"u8": 1, "u16": 2, "f32": 4, "s32": 4}[field.kind]
		if width == 0 {
			t.Fatalf("profile field %q is of kind %q, which this test measures as zero bytes wide - "+
				"it would report every byte it writes as unexplained", name, field.kind)
		}
		for i := 0; i < width; i++ {
			touched[field.offset+i] = true
		}
	}
	for i := range body {
		if !touched[i] && body[i] != clone[i] {
			t.Errorf("byte %d moved on the think clone and no profile field covers it", i)
		}
	}

	plain, err := insertNpcThinkClones(cloneFixture(t), []int32{worldActor}, map[int32]thinkChoice{worldActor: {Level: "world"}})
	if err != nil {
		t.Fatal(err)
	}
	body[npcThinkIsBuddyAIOffset] = 1
	if !bytes.Equal(clonedRow(t, plain, npcThinkLayout, worldActor), body) {
		t.Error("the world profile wrote more than the buddy flag")
	}

	if _, err := insertNpcThinkClones(cloneFixture(t), []int32{worldActor}, map[int32]thinkChoice{worldActor: {Level: "hawk"}}); err == nil {
		t.Error("an unknown perception level was written")
	}
}

// The world branch is one effect: 297000, PLAN_SP_EFFECT_BUDDY_DECLARE, the
// one the logic script forks on. A clone on the world branch is the friendly
// clone with that effect gone and nothing else moved - a real enemy delivered
// by an Ash, or a Loner. It composes
// with the strength switch: full strength drops the balance package as well.
func TestTheWorldBranchDropsCompanionMovementAndNothingElse(t *testing.T) {
	buddy, err := insertNpcClones(cloneFixture(t), []int32{worldActor}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	world, err := insertNpcClones(cloneFixture(t), []int32{worldActor}, nil, nil, map[int32]bool{worldActor: true}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, b := clonedRow(t, buddy, npcLayout, worldActor), clonedRow(t, world, npcLayout, worldActor)
	effects := func(row []byte) []int32 {
		var out []int32
		for _, offset := range npcResidentEffectOffsets {
			out = append(out, readS32(row, offset))
		}
		return out
	}
	if slotOf(effects(a), spiritFollowEffect) < 0 {
		t.Fatal("the buddy clone does not carry 297000")
	}
	if slotOf(effects(b), spiritFollowEffect) >= 0 {
		t.Error("the world-branch clone still carries 297000")
	}
	for _, kept := range []int32{spiritPresenceEffect, spiritBalanceEffect} {
		if slotOf(effects(b), kept) < 0 {
			t.Errorf("the world-branch clone lost %d, which the branch does not touch", kept)
		}
	}
	// The branch also carries 5011, "stay where you are": a world-branch
	// actor without it walks off blind to an engine home.
	if slotOf(effects(b), worldStayEffect) < 0 {
		t.Error("the world-branch clone does not carry 5011")
	}
	if slotOf(effects(a), worldStayEffect) >= 0 {
		t.Error("the buddy clone carries 5011, which is the world branch's alone")
	}
	// The slot 297000 sat in and the slot 5011 took are the only byte runs
	// that differ.
	runs := [][2]int{}
	for _, slot := range []int{slotOf(effects(a), spiritFollowEffect), slotOf(effects(b), worldStayEffect)} {
		offset := npcResidentEffectOffsets[slot]
		runs = append(runs, [2]int{offset, offset + 4})
	}
	for i := range a {
		inRun := false
		for _, run := range runs {
			if i >= run[0] && i < run[1] {
				inRun = true
			}
		}
		if !inRun && a[i] != b[i] {
			t.Errorf("byte %d differs between the buddy and the world-branch clone", i)
		}
	}
	// Full strength on the world branch: neither the package nor the movement.
	row := make([]byte, npcLayout.RowSize)
	if err := makeSummonSide(row, npcCloneStrength{Source: 1}, 6, true, false); err != nil {
		t.Fatal(err)
	}
	got := effects(row)
	if slotOf(got, spiritBalanceEffect) >= 0 || slotOf(got, spiritFollowEffect) >= 0 {
		t.Errorf("a full-strength world-branch clone carries %v; want presence and 5011 alone", got)
	}
	if slotOf(got, spiritPresenceEffect) < 0 || slotOf(got, worldStayEffect) < 0 {
		t.Error("a full-strength world-branch clone lost presence or 5011")
	}
	// A seat that spells 5011 out as well gets it once.
	row = make([]byte, npcLayout.RowSize)
	if err := makeSummonSide(row, npcCloneStrength{Source: 1, Residents: []int32{worldStayEffect}}, 6, true, false); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, effect := range effects(row) {
		if effect == worldStayEffect {
			count++
		}
	}
	if count != 1 {
		t.Errorf("5011 appears %d times on a clone that asked for it and is on the world branch; want once", count)
	}
}

// Every level shares the package that makes a clone a COMPANION, and differs
// only in reach.
//
// The default level did not. It wrote its reach, its nose, the ear, the
// sight-forget and the daylight flag, and inherited the rest from the creature
// - so on the level most players get, a summon carried the placed creature's
// maxBackhomeDist of 85 against FromSoftware's 9999 (unanimous on all 98 shipped
// rows). 85 is a literal in the game's own AI code: the summon
// runs BackToHome toward an engine home it was never at, blind on the way. It
// also walked to the spot a target was lost at, which is usually where the
// player is standing.
func TestEveryLevelSharesTheSummonPackageAndDiffersOnlyInReach(t *testing.T) {
	reach := map[string]bool{
		"eye_dist": true, "BattleStartDist": true, "nose_dist": true,
		"searchEye_dist": true, "eye_angX": true, "eye_angY": true,
		// The forget times a level may set for itself.
		"SoundTargetForgetTime": true, "MemoryTargetForgetTime": true,
	}
	for _, level := range []string{"eyes", "blind"} {
		profile := thinkProfiles[level]
		for field, want := range summonPackage {
			if reach[field] {
				continue
			}
			if profile[field] != want {
				t.Errorf("%s writes %s = %g, but every level shares %g - a level owns its reach, "+
					"not what makes it a companion", level, field, profile[field], want)
			}
		}
	}
	if summonPackage["maxBackhomeDist"] != 9999 || summonPackage["backhomeDist"] != 9979 {
		t.Fatalf("the home band is %g / %g; below the engine's move-point range a summon runs "+
			"BackToHome to a home it never had, blind on the way",
			summonPackage["maxBackhomeDist"], summonPackage["backhomeDist"])
	}
	if summonPackage["goalAction_ToDisappear"] != 0 {
		t.Errorf("goalAction_ToDisappear is %g; 2 walks to the spot a target was lost at, which is "+
			"usually the spot the player is standing in", summonPackage["goalAction_ToDisappear"])
	}
}

// NO SEARCH: a summon that loses its enemy
// returns to the player instead of sweeping for it.
//
// This is pinned rather than left to the profile table because the ladder's
// DESIGN said search was off for a day while the build inherited the creature's
// threshold and searched anyway - the same shape of defect as the home band,
// and invisible for exactly the same reason. 1,810 of the 1,856 world rows
// search, so inheriting it is not a neutral default.
func TestNoLevelSearches(t *testing.T) {
	if summonPackage["searchThreshold_Lv0toLv1"] != -1 {
		t.Fatalf("searchThreshold_Lv0toLv1 is %g; -1 is the switch that disables the search stage, "+
			"and 90 of FromSoftware's 98 Ash think rows carry it",
			summonPackage["searchThreshold_Lv0toLv1"])
	}
	for _, field := range []string{"searchEye_dist", "searchEye_angX", "searchEye_angY"} {
		if summonPackage[field] != 0 {
			t.Errorf("%s is %g; a summon that never searches has no search eye (0 on 87 of the 90 "+
				"shipped rows that do not search)", field, summonPackage[field])
		}
	}
	// Every level that takes the package must actually get it - the Loner is
	// the one level that does not, because it is what an ENEMY gets and an
	// enemy searches as the player has met it.
	for _, level := range []string{"statue", "follower", "guard", "sentinel"} {
		if got := thinkProfiles[level]["searchThreshold_Lv0toLv1"]; got != -1 {
			t.Errorf("level %s carries searchThreshold_Lv0toLv1 %g, want -1", level, got)
		}
	}
	if _, takesIt := thinkProfiles["loner"]["searchThreshold_Lv0toLv1"]; takesIt {
		t.Error("the Loner writes the search threshold; it is the one level that keeps the " +
			"creature's own senses, and an enemy searches")
	}
	// The five fields have to be writable at all - none of them was until this
	// ruling, which is why the threshold was inherited in the first place.
	for _, field := range []string{
		"searchThreshold_Lv0toLv1", "searchEye_dist", "searchEye_angX",
		"searchEye_angY", "searchTargetLv1ForgetTime", "searchTargetLv2ForgetTime",
	} {
		if _, known := thinkFields[field]; !known {
			t.Errorf("the recipe has no offset for %s, so the profile cannot write it", field)
		}
	}
}

// The ladder: six levels, each owning its reach and its cone, sharing the rest.
// docs/DESIGN.md, the ladder.
func TestTheLadderIsBuiltAsTheRecordDescribesIt(t *testing.T) {
	for _, want := range []struct {
		level   string
		cone    float64
		forget  float64
		dmgRate float64
	}{
		// 8 on every level that acts, the Follower included: FromSoftware's
		// Ash keeps 15 because it fights only the
		// player's target, and ours also keeps fights of its own.
		{"follower", 180, 8, 0},
		{"guard", 90, 8, 0},
		{"sentinel", 45, 8, 0},
		// The Hunter: the widest cone the field allows, a short forget time
		// because hunting argues for letting a ghost go, and damage rate 0
		// because share is ON and a stray hit must not drag it off the host's
		// target - which is the assist the level exists for.
		{"hunter", 180, 8, 0},
	} {
		profile := thinkProfiles[want.level]
		if profile["eye_angX"] != want.cone || profile["eye_angY"] != want.cone {
			t.Errorf("%s cone = %g/%g, want %g", want.level, profile["eye_angX"], profile["eye_angY"], want.cone)
		}
		if profile["SightTargetForgetTime"] != want.forget {
			t.Errorf("%s sight-forget = %g, want %g", want.level, profile["SightTargetForgetTime"], want.forget)
		}
		if profile["targetSys_DmgEffectRate"] != want.dmgRate {
			t.Errorf("%s damage rate = %g, want %g", want.level, profile["targetSys_DmgEffectRate"], want.dmgRate)
		}
	}
	// The Statue is a goal, not blindness: a blind actor still fights back when
	// hit, and 11000's Interupt is an unconditional false.
	if thinkProfiles["statue"]["battleGoalID"] != statueGoal {
		t.Errorf("the statue writes battleGoalID %g, want %d", thinkProfiles["statue"]["battleGoalID"], statueGoal)
	}
	// And no logic script: the logic layer is what would turn it,
	// and it runs before any battle goal. No other level may write logicId - a
	// creature's logic script carries what its own moveset needs.
	if thinkProfiles["statue"]["logicId"] != statueLogic {
		t.Errorf("the statue writes logicId %g, want %d", thinkProfiles["statue"]["logicId"], statueLogic)
	}
	for level, profile := range thinkProfiles {
		if _, writes := profile["logicId"]; writes && level != "statue" {
			t.Errorf("level %q writes logicId; only the Statue may", level)
		}
	}
	// The Loner is the one level that does NOT take the summon package: it is
	// what an enemy gets, and the point of it is that the creature behaves as
	// the player has met it. It takes the home band, because a world-branch
	// clone without it walks off blind, and the damage rate, because an enemy
	// takes aggro from whoever struck it last.
	loner := thinkProfiles["loner"]
	if loner["maxBackhomeDist"] != 9999 || loner["targetSys_DmgEffectRate"] != 100 {
		t.Errorf("the loner writes %v, want the home band and rate 100", loner)
	}
	for _, kept := range []string{"SightTargetForgetTime", "eye_dist", "eye_angY", "nose_dist",
		"soundBehaviorId08", "goalAction_ToDisappear"} {
		if _, written := loner[kept]; written {
			t.Errorf("the loner writes %s; it must keep the creature's own", kept)
		}
	}
	// Except the dark: every summoned creature sees fully after dark, the
	// Loner included.
	if loner["disableDark"] != 1 {
		t.Errorf("the loner writes disableDark %g, want 1 - every summon keeps its sight at night", loner["disableDark"])
	}
}

// The Hunter is the only level with a nose and the only one off the leash, and
// both of those are what make it a hunter rather than a long-sighted Sentinel.
//
// The nose carries its rear coverage because nothing else can: eye_angY is a
// WIDTH capped at 180, so no cone sees behind, and the buddy roam passes
// lookAroundAnimId -1, which makes the look-around phase a plain Wait with no
// animation. A sphere with no cone and no line of sight is what stands in for
// the search stage, which is off.
func TestOnlyTheHunterSmellsAndOnlyTheHunterIsUnleashed(t *testing.T) {
	for level := range ladderBeyondBody {
		nose, hasNose := ladderNoseBeyondBody[level]
		if level == "hunter" {
			if !hasNose || nose != 25 {
				t.Errorf("the hunter's nose is %g (present %v), want 25", nose, hasNose)
			}
			if ladderLeashed[level] {
				t.Error("the hunter is leashed; the level is defined by not being")
			}
			continue
		}
		if hasNose {
			t.Errorf("%s writes a nose of %g past its front; the hunter's is the only smelling nose, the middle levels' is the body's own (TestTheMiddleLevelsSmellTheirOwnBody)", level, nose)
		}
		if !ladderLeashed[level] {
			t.Errorf("%s is unleashed; only the hunter is", level)
		}
	}
	// The nose is measured from the body for the same reason the reach is: a
	// flat 25 on a seventeen-metre dragon smells inside its own model. (The
	// other levels' nose for a creature that never turns is a different rule,
	// TestANoseOnlyWhereTheScriptCannotTurn.)
	profile := ladderProfile("hunter", 17, 0, thinkRow(0, 0, 0, 0), false)
	if profile["nose_dist"] != 42 {
		t.Errorf("a seventeen-metre hunter smells %g, want 42", profile["nose_dist"])
	}
	// And the reach is NOT capped: the leash ceiling is the 23 + 5 m brake
	// measured from the host, and this level has no brake to measure against.
	if profile["eye_dist"] != 67 || profile["BattleStartDist"] != 67 {
		t.Errorf("a seventeen-metre hunter sees %g/%g, want 67 uncapped",
			profile["eye_dist"], profile["BattleStartDist"])
	}
}

// thinkRow is a world think row carrying only what the recipe reads before it
// writes: the eye, where the eye cone starts, the cone, and a battle sight
// where battle is above zero.
func thinkRow(eye, origin, battle float64, cone byte) []byte {
	row := make([]byte, npcThinkLayout.RowSize)
	binary.LittleEndian.PutUint16(row[thinkEyeDistOffset:], uint16(eye))
	binary.LittleEndian.PutUint16(row[thinkEyeBackOffsetOffset:], uint16(origin))
	row[thinkEyeAngXOffset], row[thinkEyeAngYOffset] = cone, cone
	if battle > 0 {
		row[thinkBattleSightOffset] = 1
		binary.LittleEndian.PutUint16(row[thinkBattleSightDistOffset:], uint16(battle))
	}
	return row
}

// The Statue draws no weapon, and the three middle levels
// take no spectator or circler role (FromSoftware's 0 on 91 of its 97
// near-blind Ash rows).
func TestTheStatueDrawsNoWeaponAndTheMiddleLevelsPlayNoRole(t *testing.T) {
	for _, field := range []string{"weaponOffSpecialEffectId", "weaponOnSpecialEffectId", "weaponOffAnimId", "weaponOnAnimId"} {
		if got, written := thinkProfiles["statue"][field]; !written || got != -1 {
			t.Errorf("the statue writes %s = %g (written %v), want -1 - the draw is the logic script's and 11000 cannot stop it", field, got, written)
		}
		if _, written := thinkProfiles["follower"][field]; written {
			t.Errorf("the follower writes %s; every level but the statue keeps the creature's own draw", field)
		}
	}
	for _, level := range []string{"follower", "guard", "sentinel"} {
		if got, written := thinkProfiles[level]["thinkAttr_doAdmirer"]; !written || got != 0 {
			t.Errorf("%s writes thinkAttr_doAdmirer %g (written %v), want 0", level, got, written)
		}
	}
}

// seedSheathed makes the fixture's world actor start sheathed the way the
// Banished Knight soldier does: its think row names effect as the sheathed
// state and its actor row carries the same effect resident in the last slot.
func seedSheathed(t *testing.T, bnd4 []byte, effect int32) {
	t.Helper()
	think, err := regulation.OpenTable(bnd4, npcThinkLayout)
	if err != nil {
		t.Fatal(err)
	}
	row := think.ByID[worldActor]
	writeS32(think.Data[row.DataOffset:row.DataOffset+npcThinkLayout.RowSize], thinkFields["weaponOffSpecialEffectId"].offset, effect)
	npc, err := regulation.OpenTable(bnd4, npcLayout)
	if err != nil {
		t.Fatal(err)
	}
	row = npc.ByID[worldActor]
	writeS32(npc.Data[row.DataOffset:row.DataOffset+npcLayout.RowSize], npcResidentEffectOffsets[len(npcResidentEffectOffsets)-1], effect)
}

// A clone written with its weapon out loses the sheathed state its world think
// row names, and nothing else: the Statue's, since a hit draws the weapon
// through the animation layer.
func TestWeaponOutTakesTheSheathedStateOffAndNothingElse(t *testing.T) {
	const sheathed = 13065
	plainSave, outSave := cloneFixture(t), cloneFixture(t)
	seedSheathed(t, plainSave, sheathed)
	seedSheathed(t, outSave, sheathed)
	plain, err := insertNpcClones(plainSave, []int32{worldActor}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := insertNpcClones(outSave, []int32{worldActor}, nil, nil, nil, nil, map[int32]int32{worldActor: worldActor})
	if err != nil {
		t.Fatal(err)
	}
	a, b := clonedRow(t, plain, npcLayout, worldActor), clonedRow(t, out, npcLayout, worldActor)
	if slotOf(residentEffects(a), sheathed) < 0 {
		t.Fatal("the plain clone does not carry the seeded sheathed state; the seed did not reach the clone")
	}
	if slotOf(residentEffects(b), sheathed) >= 0 {
		t.Error("the weapon-out clone still carries its sheathed state")
	}
	slot := slotOf(residentEffects(a), sheathed)
	offset := npcResidentEffectOffsets[slot]
	for i := range a {
		if (i < offset || i >= offset+4) && a[i] != b[i] {
			t.Fatalf("byte %d differs between the two clones; only the sheathed slot at %d may", i, offset)
		}
	}
	if got := readS32(b, offset); got != -1 {
		t.Errorf("the emptied slot holds %d, want -1", got)
	}
}
