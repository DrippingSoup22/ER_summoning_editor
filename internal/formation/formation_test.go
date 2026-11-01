// These tests hold the authoring model to what the shipped data says a
// formation is. The facts they encode are measured on regulation 11701000 and
// seen in game by research, and each test names the one it depends on
// rather than asserting a shape.
package formation_test

import (
	"errors"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// Lone Wolf Ashes: three rows, all naming wolf 140700000, reinforced at every
// level. It is the ordinary host and the one most tests start from.
func loneWolfBlock() formation.Block {
	return formation.Block{
		Trigger: 232000,
		Rows: []formation.BlockRow{
			{RowID: 23200000, Doping: canonicalDoping()},
			{RowID: 23200001, Doping: canonicalDoping()},
			{RowID: 23200002, Doping: canonicalDoping()},
		},
	}
}

// canonicalDoping is the 290000..290010 series 83 of the 84 shipped families
// carry on every member row.
func canonicalDoping() [formation.DopingLevels]int32 {
	var table [formation.DopingLevels]int32
	for level := range table {
		table[level] = 290000 + int32(level)
	}
	return table
}

// saveHolding is a regulation that holds the block's rows and nothing else of
// interest: no clones exist yet, so every world actor needs one.
func saveHolding(block formation.Block, extraBuddyRows ...int32) formation.Save {
	save := formation.Save{
		BuddyRows:    map[int32]bool{},
		NpcRows:      map[int32]bool{},
		NpcThinkRows: map[int32]bool{},
	}
	for _, row := range block.Rows {
		save.BuddyRows[row.RowID] = true
	}
	for _, rowID := range extraBuddyRows {
		save.BuddyRows[rowID] = true
	}
	return save
}

func slots(actors ...formation.Actor) []formation.Slot {
	list := make([]formation.Slot, 0, len(actors))
	for _, actor := range actors {
		list = append(list, formation.Slot{Actor: actor})
	}
	return list
}

// wolf is a world actor: NpcParam 40700000 with its own think row. Its
// summon-side clone is 140700000, which is what vanilla Lone Wolf names.
var (
	wolf    = formation.Actor{Npc: 40700000, Think: 40700000}
	soldier = formation.Actor{Npc: 43111300, Think: 43111300}
	// tiche is already summon-side: a vanilla Ash actor is itself a clone.
	tiche = formation.Actor{Npc: 121000000, Think: 121000000}
)

func mustCompile(t *testing.T, f formation.Formation, block formation.Block, save formation.Save) *formation.Compiled {
	t.Helper()
	compiled, err := formation.Compile(f, block, save)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

// A world actor is never named directly. The formation names its clone, and
// the clone is an insertion at source + 100000000.
func TestAWorldActorIsSummonedThroughAnInsertedClone(t *testing.T) {
	block, save := loneWolfBlock(), saveHolding(loneWolfBlock())

	compiled := mustCompile(t, formation.Formation{Slots: slots(wolf)}, block, save)

	if len(compiled.Clones) != 1 {
		t.Fatalf("got %d clones, want 1", len(compiled.Clones))
	}
	clone := compiled.Clones[0]
	if clone.Npc == nil || clone.Npc.CopyFrom != wolf.Npc || clone.Npc.RowID != wolf.Npc+formation.CloneOffset {
		t.Errorf("NpcParam clone = %+v, want %d copied from %d", clone.Npc, wolf.Npc+formation.CloneOffset, wolf.Npc)
	}
	if clone.Think == nil || clone.Think.RowID != wolf.Think+formation.CloneOffset {
		t.Errorf("NpcThinkParam clone = %+v, want %d", clone.Think, wolf.Think+formation.CloneOffset)
	}
	if compiled.Edits[0].Actor.Npc != wolf.Npc+formation.CloneOffset {
		t.Errorf("the row summons %d, which is the world actor itself", compiled.Edits[0].Actor.Npc)
	}
}

// Ten is the runtime ceiling, measured by authoring twelve and getting ten.
// Eleven is refused here rather than discovered in game.
func TestTheTenthSlotIsAllowedAndTheEleventhIsNot(t *testing.T) {
	block := loneWolfBlock()
	save := saveHolding(block)

	ten := make([]formation.Slot, 0, 11)
	for index := 0; index < formation.MaxSlots; index++ {
		ten = append(ten, formation.Slot{Actor: wolf})
	}
	if _, err := formation.Compile(formation.Formation{Slots: ten}, block, save); err != nil {
		t.Fatalf("ten slots refused: %v", err)
	}
	eleven := append(ten, formation.Slot{Actor: wolf})
	if _, err := formation.Compile(formation.Formation{Slots: eleven}, block, save); !errors.Is(err, formation.ErrTooManySlots) {
		t.Fatalf("eleven slots err = %v, want ErrTooManySlots", err)
	}
}

// A block the save shrank carries the rows it retired right after it (Inert).
// A formation larger than the block re-fills them, in order, as edits - they
// are the block's own rows - and inserts only past the last of them, anchored
// on it; were the first of them a collision, a formation shrunk and saved
// could never grow again.
func TestAFormationGrowsBackIntoTheRowsItsBlockRetired(t *testing.T) {
	block := loneWolfBlock()
	block.Rows = block.Rows[:1]
	block.Inert = []formation.BlockRow{
		{RowID: 23200001, Doping: canonicalDoping()},
		{RowID: 23200002, Doping: canonicalDoping()},
	}
	save := saveHolding(block, 23200001, 23200002)

	compiled := mustCompile(t, formation.Formation{Slots: slots(wolf, wolf, wolf, wolf)}, block, save)
	var edited []int32
	for _, edit := range compiled.Edits {
		edited = append(edited, edit.RowID)
	}
	if len(edited) != 3 || edited[0] != 23200000 || edited[1] != 23200001 || edited[2] != 23200002 {
		t.Fatalf("edits = %v, want the live row then both retired rows re-filled", edited)
	}
	if len(compiled.Inserts) != 1 {
		t.Fatalf("inserts = %+v, want one past the retired rows", compiled.Inserts)
	}
	if insert := compiled.Inserts[0]; insert.RowID != 23200003 || insert.InsertAfter != 23200002 || insert.CopyFrom != 23200000 {
		t.Errorf("insert = %+v, want row 23200003 after 23200002, copied from the live template 23200000", insert)
	}
	if len(compiled.Retired) != 0 {
		t.Errorf("retired = %v, want none", compiled.Retired)
	}

	// A formation that needs fewer leaves the rest of them as they are.
	compiled = mustCompile(t, formation.Formation{Slots: slots(wolf, wolf)}, block, save)
	if len(compiled.Edits) != 2 || compiled.Edits[1].RowID != 23200001 || len(compiled.Inserts) != 0 || len(compiled.Retired) != 0 {
		t.Errorf("two slots: edits %+v inserts %+v retired %v, want 23200000 and 23200001 edited and nothing else",
			compiled.Edits, compiled.Inserts, compiled.Retired)
	}
}

// The compiler's own allocator - the fallback for a caller that brings none -
// never reuses a row the save holds: it cannot tell this editor's clone from
// FromSoftware's row, so a held address is skipped and the recipe inserted in
// the lowest free band, each table answered on its own. The product's
// allocator reuses a held clone only when MatchClones proves it the recipe
// (application: TestReopeningASaveReusesWhatIsTheSameAndWritesWhatChanged).
// Reusing it here would put Commander Gaius on the Florissax Ash's AI.
func TestTheFallbackAllocatorNeverReusesAHeldRow(t *testing.T) {
	block := loneWolfBlock()
	save := saveHolding(block)
	save.NpcRows[wolf.Npc+formation.CloneOffset] = true

	compiled := mustCompile(t, formation.Formation{Slots: slots(wolf)}, block, save)

	if len(compiled.Clones) != 1 || compiled.Clones[0].Npc == nil || compiled.Clones[0].Think == nil {
		t.Fatalf("got clones %+v, want both halves inserted", compiled.Clones)
	}
	if got, want := compiled.Clones[0].Npc.RowID, wolf.Npc+2*formation.CloneOffset; got != want {
		t.Errorf("NpcParam clone at %d, want band 2 (%d) past the held band-1 row", got, want)
	}
	if got, want := compiled.Clones[0].Think.RowID, wolf.Think+formation.CloneOffset; got != want {
		t.Errorf("think clone at %d, want band 1 (%d), which the save does not hold", got, want)
	}

	// Commander Gaius: both think rows' band-1 addresses hold another
	// creature's (the Florissax Ash's) rows.
	gaius := formation.Actor{Npc: 50000092, Think: 50000010}
	save = saveHolding(block)
	save.NpcThinkRows[150000010] = true
	compiled = mustCompile(t, formation.Formation{Slots: slots(gaius)}, block, save)
	if got := compiled.Clones[0].Think.RowID; got != 250000010 {
		t.Errorf("Gaius thinks with %d, want 250000010, never the held 150000010", got)
	}
}

// Two compiles of one formation must be identical, or a diff between them
// means nothing. Map iteration order is the usual way this breaks.
func TestCompilingIsDeterministic(t *testing.T) {
	block, save := loneWolfBlock(), saveHolding(loneWolfBlock())
	authored := formation.Formation{Slots: slots(soldier, tiche, wolf, soldier, wolf)}

	first := mustCompile(t, authored, block, save)
	for attempt := 0; attempt < 20; attempt++ {
		again := mustCompile(t, authored, block, save)
		if len(again.Clones) != len(first.Clones) {
			t.Fatalf("clone count varies: %d then %d", len(first.Clones), len(again.Clones))
		}
		for index := range first.Clones {
			if again.Clones[index].Source != first.Clones[index].Source {
				t.Fatalf("clone order varies at %d: %+v then %+v",
					index, first.Clones[index].Source, again.Clones[index].Source)
			}
		}
	}
	// Ordered by source NpcParam ID, so the list reads the same as it sorts.
	for index := 1; index < len(first.Clones); index++ {
		if first.Clones[index-1].Source.Npc >= first.Clones[index].Source.Npc {
			t.Errorf("clones are not in ascending source order: %d then %d",
				first.Clones[index-1].Source.Npc, first.Clones[index].Source.Npc)
		}
	}
}

// Compile widens nothing: the tables it names are BuddyParam and the two clone
// tables, and every clone row is an insertion at source + 100000000. Nothing it
// produces can modify a row that already exists in NpcParam or NpcThinkParam,
// which is docs/SAFETY.md rule 1 restated as a property of the output.
func TestNothingCompiledEverModifiesAnExistingActorRow(t *testing.T) {
	block := loneWolfBlock()
	save := saveHolding(block)
	save.NpcRows[40700000] = true // the world row itself is in the save
	save.NpcThinkRows[40700000] = true

	compiled := mustCompile(t, formation.Formation{Slots: slots(wolf, soldier)}, block, save)

	for _, clone := range compiled.Clones {
		for _, row := range []*formation.CloneRow{clone.Npc, clone.Think} {
			if row == nil {
				continue
			}
			if row.RowID != row.CopyFrom+formation.CloneOffset {
				t.Errorf("clone %d is not %d + %d", row.RowID, row.CopyFrom, formation.CloneOffset)
			}
			if save.NpcRows[row.RowID] || save.NpcThinkRows[row.RowID] {
				t.Errorf("clone would land on existing row %d", row.RowID)
			}
		}
	}
}

// The share box: on
// the Follower by default, off on the Guard, the Sentinel and the Hunter, and
// the player's on those four; fixed off on the Statue and the Loner. The slot's
// stance must carry what the box resolves to, since the box also writes the
// think clone's damage rate.
func TestTheShareBoxIsTheLevelsDefaultOrThePlayersChoice(t *testing.T) {
	for _, want := range []struct {
		level             formation.Perception
		defaultOn, chosen bool
	}{
		{formation.PerceptionFollower, true, true},
		{formation.PerceptionGuard, false, true},
		{formation.PerceptionSentinel, false, true},
		{formation.PerceptionHunter, false, true},
		{formation.PerceptionStatue, false, false},
		{formation.PerceptionLoner, false, false},
	} {
		on, chosen, known := formation.LevelShare(want.level)
		if !known || on != want.defaultOn || chosen != want.chosen {
			t.Errorf("%s shares %t by default, chosen %t (known %t); want %t, %t", want.level.Name(), on, chosen, known, want.defaultOn, want.chosen)
		}
	}
	if _, _, known := formation.LevelShare(formation.PerceptionWorld); known {
		t.Error("a research level has a share box; it takes its stance from the seat")
	}

	guard := wolf
	guard.Perception = formation.PerceptionGuard
	onDefault, _ := formation.LevelStance(guard)
	if onDefault != (formation.Stance{DisablePCTargetShare: 1, PcFollowType: 0}) {
		t.Errorf("a guard at its default is seated %+v, want follow with share off", onDefault)
	}
	if err := (formation.Formation{Slots: []formation.Slot{{Actor: guard, Stance: formation.StanceFollow}}}).Validate(); !errors.Is(err, formation.ErrStanceMismatch) {
		t.Errorf("a guard at its default (share off) on a sharing stance gives %v, want ErrStanceMismatch", err)
	}
	guard.Share = formation.ShareOn
	if err := (formation.Formation{Slots: []formation.Slot{{Actor: guard, Stance: formation.StanceFollow}}}).Validate(); err != nil {
		t.Errorf("a guard with share chosen on, on follow, is refused: %v", err)
	}
	// The follow type stays free on the middle levels: a sharing guard may hold.
	if err := (formation.Formation{Slots: []formation.Slot{{Actor: guard, Stance: formation.StanceHold}}}).Validate(); err != nil {
		t.Errorf("a sharing guard on hold is refused: %v", err)
	}

	for _, level := range []formation.Perception{formation.PerceptionStatue, formation.PerceptionLoner} {
		fixed := wolf
		fixed.Perception = level
		fixed.Share = formation.ShareOff
		if _, err := formation.ResolveShare(fixed); !errors.Is(err, formation.ErrShareFixed) {
			t.Errorf("choosing share on a %s gives %v, want ErrShareFixed - the box is greyed out", level.Name(), err)
		}
	}

	// One creature, two share choices: that is two think
	// clones, not a refusal - the second in band 2 (CloneBands). The NpcParam
	// half is one clone: the share box writes nothing on it.
	block := loneWolfBlock()
	sharing, alone := wolf, wolf
	sharing.Perception, alone.Perception = formation.PerceptionGuard, formation.PerceptionGuard
	sharing.Share = formation.ShareOn
	mixed := formation.Formation{Slots: []formation.Slot{
		{Actor: sharing, Stance: formation.StanceFollow},
		{Actor: alone, Stance: formation.Stance{DisablePCTargetShare: 1}},
	}}
	compiled, err := formation.Compile(mixed, block, saveHolding(block))
	if err != nil {
		t.Fatalf("two slots of one creature choosing share differently are refused: %v", err)
	}
	var npcKeys, thinkKeys []int32
	for _, clone := range compiled.Clones {
		if clone.Npc != nil {
			npcKeys = append(npcKeys, clone.Npc.CopyFrom)
		}
		if clone.Think != nil {
			thinkKeys = append(thinkKeys, clone.Think.CopyFrom)
		}
	}
	second, _ := formation.CloneKey(wolf.Think, 2)
	if len(npcKeys) != 1 || len(thinkKeys) != 2 || thinkKeys[0] != wolf.Think || thinkKeys[1] != second {
		t.Errorf("clones inserted: npc %v think %v, want one npc clone and think clones %d and %d", npcKeys, thinkKeys, wolf.Think, second)
	}
	if compiled.Edits[0].Actor.Think == compiled.Edits[1].Actor.Think {
		t.Errorf("both slots think with %d; each share choice needs its own clone", compiled.Edits[0].Actor.Think)
	}
}

// The enemy box: an enemy is a Hunter, a Statue or a
// Loner - the Loner by default - and never shares the player's target.
func TestAnEnemyTakesOnlyTheLevelsThatDoNotServeThePlayer(t *testing.T) {
	if got := formation.DefaultLevel(true); got != formation.PerceptionLoner {
		t.Errorf("an enemy defaults to %q, want the Loner", got.Name())
	}
	if got := formation.DefaultLevel(false); got != formation.PerceptionFollower {
		t.Errorf("a summon defaults to %q, want the Follower", got.Name())
	}
	enemy := wolf
	enemy.Team, enemy.WorldBranch = formation.TeamEnemy, true
	for _, level := range formation.EnemyLevels {
		actor := enemy
		actor.Perception = level
		stance, _ := formation.LevelStance(actor)
		if stance.DisablePCTargetShare != 1 {
			t.Errorf("an enemy %s is seated sharing (%+v)", level.Name(), stance)
		}
		if err := (formation.Formation{Slots: []formation.Slot{{Actor: actor, Stance: stance}}}).Validate(); err != nil {
			t.Errorf("an enemy %s is refused: %v", level.Name(), err)
		}
	}
	for _, level := range []formation.Perception{formation.PerceptionFollower, formation.PerceptionGuard, formation.PerceptionSentinel} {
		actor := enemy
		actor.Perception = level
		stance, _ := formation.LevelStance(actor)
		if err := (formation.Formation{Slots: []formation.Slot{{Actor: actor, Stance: stance}}}).Validate(); !errors.Is(err, formation.ErrEnemyLevel) {
			t.Errorf("an enemy %s gives %v, want ErrEnemyLevel", level.Name(), err)
		}
	}
	hunter := enemy
	hunter.Perception, hunter.Share = formation.PerceptionHunter, formation.ShareOn
	if _, err := formation.ResolveShare(hunter); !errors.Is(err, formation.ErrEnemyShares) {
		t.Errorf("an enemy hunter asked to share gives %v, want ErrEnemyShares", err)
	}
	// A research level on an enemy is a research seat's, and stays open to it.
	world := enemy
	world.Perception = formation.PerceptionWorld
	if err := (formation.Formation{Slots: []formation.Slot{{Actor: world, Stance: formation.StanceHold}}}).Validate(); err != nil {
		t.Errorf("an enemy on the world level is refused: %v", err)
	}
}

// The Statue never moves and the Loner never follows or shares: each is
// refused on any stance but its own, exactly as the Hunter is.
func TestTheStatueAndTheLonerAreCoupledToTheirStances(t *testing.T) {
	for _, want := range []struct {
		level  formation.Perception
		stance formation.Stance
	}{
		{formation.PerceptionStatue, formation.StanceStill},
		{formation.PerceptionLoner, formation.StanceIndependent},
	} {
		actor := wolf
		actor.Perception = want.level
		ok := formation.Formation{Slots: []formation.Slot{{Actor: actor, Stance: want.stance}}}
		if err := ok.Validate(); err != nil {
			t.Errorf("a %s on its own stance is refused: %v", want.level.Name(), err)
		}
		for _, wrong := range []formation.Stance{formation.StanceFollow, formation.StanceWander, formation.StanceHold, formation.StanceIndependent, formation.StanceStill} {
			if wrong == want.stance {
				continue
			}
			bad := formation.Formation{Slots: []formation.Slot{{Actor: actor, Stance: wrong}}}
			if err := bad.Validate(); !errors.Is(err, formation.ErrStanceMismatch) {
				t.Errorf("a %s on %+v gives %v, want ErrStanceMismatch", want.level.Name(), wrong, err)
			}
		}
	}
	if got := formation.LevelResidents(formation.PerceptionFollower); len(got) != 1 || got[0] != formation.FollowerTargetPriority {
		t.Errorf("the follower carries %v, want only %d", got, formation.FollowerTargetPriority)
	}
	for _, level := range []formation.Perception{formation.PerceptionGuard, formation.PerceptionSentinel, formation.PerceptionHunter, formation.PerceptionLoner} {
		if got := formation.LevelResidents(level); len(got) != 0 {
			t.Errorf("%s carries %v; 296010 is the Follower's alone - the others must draw fights", level.Name(), got)
		}
	}
}

// The clone bands: a creature's n-th recipe lives at source +
// n x CloneOffset, its last eight digits the creature - model and all - in
// every band. Band 5 is the game's own scheme and band 21 is past an s32; band
// 1 is the whole summon namespace, the shipped shell at 100000000 included.
func TestTheCloneBandsKeepTheCreatureInItsLastEightDigits(t *testing.T) {
	for _, band := range formation.CloneBands {
		key, err := formation.CloneKey(40700000, band)
		if err != nil {
			t.Fatalf("band %d: %v", band, err)
		}
		row := key + formation.CloneOffset
		if source, ok := formation.CloneSource(row); !ok || source != 40700000 || !formation.InCloneBand(row) {
			t.Errorf("band %d row %d reads back as %d (%v)", band, row, source, ok)
		}
		if row%formation.CloneOffset/10000 != 4070 {
			t.Errorf("band %d row %d is not model c4070", band, row)
		}
	}
	for _, band := range []int32{0, 5, 21} {
		if _, err := formation.CloneKey(40700000, band); !errors.Is(err, formation.ErrNotSummonable) {
			t.Errorf("band %d accepted", band)
		}
	}
	for _, row := range []int32{500000000 + 40700000, 99999999, 200000000} {
		if formation.InCloneBand(row) {
			t.Errorf("%d reads as a clone row", row)
		}
	}
	if !formation.InCloneBand(100000000) {
		t.Error("the shipped shell at 100000000 fell out of band 1")
	}
	// The compiler's own allocator: the first recipe at band 1, the others in
	// turn, and band 5 skipped.
	block := loneWolfBlock()
	save := saveHolding(block)
	for _, band := range []int32{2, 3, 4} {
		key, _ := formation.CloneKey(wolf.Npc, band)
		save.NpcRows[key+formation.CloneOffset] = true
	}
	hostile := wolf
	hostile.Team = formation.TeamEnemy
	compiled := mustCompile(t, formation.Formation{Slots: slots(wolf, hostile)}, block, save)
	sixth, _ := formation.CloneKey(wolf.Npc, 6)
	if got := compiled.Edits[1].Actor.Npc; got != sixth+formation.CloneOffset {
		t.Errorf("the second recipe with bands 2-4 held lands at %d, want band 6's %d", got, sixth+formation.CloneOffset)
	}
}
