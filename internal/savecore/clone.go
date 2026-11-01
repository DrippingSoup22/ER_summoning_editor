package savecore

// The summon-side actor recipe.
//
// internal/formation answers which actor rows a summon needs: world actor N is
// summoned through a clone at N + cloneRowOffset. That answer is an address,
// and an address is not a summon.
//
// Not one of the 94 shipped clones with a world twin is a byte copy of it.
// Every clone is on team 47 and no world row anywhere in the table is; most
// world rows carry an item lot and no clone does. A bare copy would put an
// actor on an enemy team, holding the enemy's rewards, into a formation the
// player summons - so the copy is only where a clone starts. What makes it
// summon-side is written on top of it here.
//
// regulation.InsertRows proves the new row is a byte copy of a shipped one and
// that nothing else in the archive moved; the recipe is then written over that
// copy and proved to stay inside it, through the same verifyBoundedMutation
// every other write in this product goes through. The recipe is research's,
// seen on the console: clones of world actors summon correctly while their
// world twins stay hostile and keep dropping loot.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

const (
	// NpcParam fields the recipe writes. The offsets are research's, and the
	// table's geometry is asserted by npcLayout rather than assumed.
	npcHPOffset              = 36
	npcHitHeightOffset       = 20
	npcHitRadiusOffset       = 24
	npcGetSoulOffset         = 44
	npcItemLotEnemyOffset    = 48
	npcItemLotMapOffset      = 52
	npcGameClearEffectOffset = 108
	npcTeamTypeOffset        = 307
	npcSoulByBossFlagsOffset = 349

	npcThinkIsBuddyAIOffset = 153

	// spiritSummonTeam is the friend/foe team every shipped summon is on, and
	// the one field that separates a companion from an enemy.
	spiritSummonTeam = 47
	// npcSoulByBossFlagMask is the bit that makes an actor pay out a boss soul.
	// It is set on no shipped clone.
	npcSoulByBossFlagMask = byte(1)

	// The collision ceilings. These are bounds, not target values: the largest
	// hitHeight on any shipped clone is 2.4 and the largest hitRadius is
	// exactly 1, so capping holds an oversized world actor inside the range the
	// game's own summons occupy while leaving one that already fits alone.
	summonHitHeightCeiling = 5
	summonHitRadiusCeiling = 1

	// The world-area scaling band. These effects scale an actor to the region
	// it was placed in, which means nothing for something the player carries
	// around, and no shipped clone carries one.
	areaScalingFirst = 7000
	areaScalingPast  = 7700
	// The Shadow of the Erdtree band, 20007000-20007130: fourteen tiers from
	// Gravesite Plain (x7.05 health) to x15.34, attack up to x3.84. Every
	// summon has the same scaling, so it is stripped too; none of the 115
	// shipped clone rows carries one.
	dlcAreaScalingFirst = 20007000
	dlcAreaScalingPast  = 20007700
)

// spiritCompanionEffects are the three resident effects that make a summon a
// summon: 295000 presence, 296000 the Spirit balance package, 297000 companion
// movement.
//
// All three are on all 115 shipped clones, except that seven spell presence as
// 295200 rather than 295000 - a partition and not a gap: 108 carry 295000,
// seven carry 295200, none carries both and none carries neither. The recipe
// writes the form the other 108 use.
//
// 296000 is here because every shipped summon has it, not as a balance
// judgement. It is a trade rather than a nerf - incoming damage halved,
// outgoing quartered, poise damage at a twentieth, no HP term - and dropping it
// is what makes a Fighter.
var spiritCompanionEffects = []int32{spiritPresenceEffect, spiritBalanceEffect, spiritFollowEffect}

// The three by name, because two of them are always written and the third is
// the one a Fighter goes without.
//
// 296000 is named in the regulation "[Spirit Summon] -75% Damage, +50%
// Absorption, -95% Poise Damage", and those three numbers are the whole of what
// separates a helper from the creature it copies.
const (
	spiritPresenceEffect = int32(295000)
	spiritBalanceEffect  = int32(296000)
	spiritFollowEffect   = int32(297000)
	// worldStayEffect is 5011, a bare permanent marker no NpcParam row carries:
	// the first branch of the world's non-battle goal, "stay where you are".
	// Without it a summoned actor on the world branch walks back to an engine
	// home it was never at, blind on the way.
	worldStayEffect = int32(5011)
)

// worldBranchThinkBand is the home band FromSoftware writes on all 98 shipped
// Spirit Ash think rows and on 272 arena rows: above the engine's move-point
// range, so an actor with a target fights at its battle start instead of
// only inside the logic script's 2 m literal. A world-branch think clone
// gets it whatever its perception level says.
var worldBranchThinkBand = map[string]float64{"maxBackhomeDist": 9999, "backhomeDist": 9979}

// npcResidentEffectOffsets are the 32 resident special-effect slots the recipe
// reads and rewrites. The table spells spEffectID0..31 as three separate runs
// rather than one array, and all three are processed: FromSoftware puts every
// one of its 230 companion effects in the third run, so a recipe that looked
// only at the first would conclude no shipped clone has any.
var npcResidentEffectOffsets = func() []int {
	var offsets []int
	for _, run := range []struct{ start, count int }{{76, 8}, {376, 8}, {488, 16}} {
		for slot := 0; slot < run.count; slot++ {
			offsets = append(offsets, run.start+slot*4)
		}
	}
	return offsets
}()

// insertNpcClones adds the summon-side NpcParam clones of the given world
// actors. weaponOut maps a source to the world think row whose sheathed state
// is taken off its clone (sheathedStateOf).
func insertNpcClones(bnd4 []byte, sources []int32, strength map[int32]npcCloneStrength, teams map[int32]uint8, worldBranch, unheld map[int32]bool, weaponOut map[int32]int32) ([]byte, error) {
	recipe, err := npcCloneRecipe(bnd4, strength, teams, worldBranch, unheld, weaponOut)
	if err != nil {
		return nil, err
	}
	return insertActorClonesFrom(bnd4, npcLayout, sources, recipe)
}

// npcCloneRecipe is what insertNpcClones writes over each NpcParam clone, as a
// function of the clone key and the world row's copy. MatchClones renders with
// the same function, so recognising a clone and writing one cannot disagree.
func npcCloneRecipe(bnd4 []byte, strength map[int32]npcCloneStrength, teams map[int32]uint8, worldBranch, unheld map[int32]bool, weaponOut map[int32]int32) (func(int32, []byte) error, error) {
	// A slot naming an effect the regulation does not hold would be a silent
	// no-op, and that the write is not one is the whole point of it.
	_, effects, err := regulation.LoadRowsByID(bnd4, "SpEffectParam.param", "", 0)
	if err != nil {
		return nil, err
	}
	for _, effect := range spiritCompanionEffects {
		if _, exists := effects[effect]; !exists {
			return nil, fmt.Errorf("Spirit Summon effect %d is absent from SpEffectParam: %w", effect, ErrRowNotFound)
		}
	}
	sheathed, err := sheathedStateOf(bnd4, weaponOut)
	if err != nil {
		return nil, err
	}
	return func(key int32, row []byte) error {
		if err := makeSummonSide(row, strength[key], teams[key], worldBranch[key], unheld[key]); err != nil {
			return err
		}
		if effect, drop := sheathed[key]; drop {
			for _, offset := range npcResidentEffectOffsets {
				if readS32(row, offset) == effect {
					writeS32(row, offset, -1)
				}
			}
		}
		return nil
	}, nil
}

// sheathedStateOf reads, for each clone written with its weapon out, the
// sheathed-state effect its world think row names: weaponOffSpecialEffectId,
// the effect the logic script tests before it plays a draw, which the actor
// row carries resident when the creature starts sheathed (the Banished Knight
// soldier c4312: 13065 in both). A think row naming none leaves nothing to
// take off - a creature that is never sheathed has nothing to draw - so its
// source is left out.
func sheathedStateOf(bnd4 []byte, weaponOut map[int32]int32) (map[int32]int32, error) {
	sheathed := make(map[int32]int32, len(weaponOut))
	if len(weaponOut) == 0 {
		return sheathed, nil
	}
	table, err := regulation.OpenTable(bnd4, npcThinkLayout)
	if err != nil {
		return nil, err
	}
	offset := thinkFields["weaponOffSpecialEffectId"].offset
	for source, think := range weaponOut {
		row, found := table.ByID[think]
		if !found {
			return nil, fmt.Errorf("%s: think row %d, which names %d's sheathed state, was not found: %w",
				npcThinkLayout.Entry, think, source, ErrRowNotFound)
		}
		if effect := readS32(table.Data[row.DataOffset:row.DataOffset+npcThinkLayout.RowSize], offset); effect > 0 {
			sheathed[source] = effect
		}
	}
	return sheathed, nil
}

// summonTeams is the closed vocabulary of teams a summon may be put on beside
// the Spirit Summon team it ships on: the four Spirit Battler writes to make
// a summon hostile. The model layer carries the same list (formation.Teams).
var summonTeams = map[uint8]string{6: "enemy", 24: "beast", 27: "hostile NPC", 33: "arch-enemy"}

// summonTeam is the teamType written on a clone: the Spirit Summon team unless
// the manifest put it elsewhere.
func summonTeam(team uint8) (byte, error) {
	if team == 0 {
		return spiritSummonTeam, nil
	}
	if _, known := summonTeams[team]; !known {
		return 0, fmt.Errorf("team %d is not one a summon may stand on (%v)", team, summonTeams)
	}
	return team, nil
}

// summonCopyRow is where a shipped summon-side row is copied to: the copy
// band's two arithmetic rules (formation.SummonCopyRow, written twice and
// checked once). Only the shells' windows are accepted.
func summonCopyRow(source int32) (int32, error) {
	switch {
	case source >= cloneRowOffset && source < cloneRowOffset+summonCopyWindow:
		return summonCopyFirst + (source - cloneRowOffset), nil
	case source >= dlcShellFirst && source < dlcShellFirst+summonCopyWindow:
		return summonCopySplit + (source - dlcShellFirst), nil
	default:
		return 0, fmt.Errorf("%d is not a shipped shell (%d..%d or %d..%d), so the copy band has no row for it: %w",
			source, cloneRowOffset, cloneRowOffset+summonCopyWindow-1, dlcShellFirst, dlcShellFirst+summonCopyWindow-1, ErrRowNotFound)
	}
}

// insertSummonCopies copies the given shipped summon-side rows into the copy
// band and writes each copy's team. The source is a summon already - the
// companion effects, the proportions, no payout - so nothing but the team
// moves, and a copy this manifest gives no team to would be a pointless
// twin: every copy must be named in teams.
func insertSummonCopies(bnd4 []byte, sources []int32, teams map[int32]uint8) ([]byte, error) {
	table, err := regulation.OpenTable(bnd4, npcLayout)
	if err != nil {
		return nil, err
	}
	ordered := append([]int32(nil), sources...)
	slices.Sort(ordered)
	present := make([]int32, len(table.Rows))
	for index, row := range table.Rows {
		present[index] = row.ID
	}
	insertions := make([]regulation.RowInsertion, 0, len(ordered))
	var previous int32
	for index, source := range ordered {
		if index > 0 && source == ordered[index-1] {
			return nil, fmt.Errorf("%s: summon row %d is copied twice", npcLayout.Entry, source)
		}
		destination, err := summonCopyRow(source)
		if err != nil {
			return nil, err
		}
		if _, exists := table.ByID[source]; !exists {
			return nil, fmt.Errorf("%s: summon row %d was not found: %w", npcLayout.Entry, source, ErrRowNotFound)
		}
		if _, exists := table.ByID[destination]; exists {
			return nil, fmt.Errorf("%s: copy %d of summon row %d already exists", npcLayout.Entry, destination, source)
		}
		if _, named := teams[source]; !named {
			return nil, fmt.Errorf("%s: copy %d of summon row %d is given no team, and a copy on the same team is the source", npcLayout.Entry, destination, source)
		}
		anchor := present[sort.Search(len(present), func(index int) bool { return present[index] >= destination })-1]
		if anchor < previous {
			anchor = previous
		}
		insertions = append(insertions, regulation.RowInsertion{RowID: destination, CopyFrom: source, InsertAfter: &anchor})
		previous = destination
	}
	grown, err := regulation.InsertRows(bnd4, npcLayout, insertions)
	if err != nil {
		return nil, err
	}
	copied := bytes.Clone(grown)
	table, err = regulation.OpenTable(grown, npcLayout)
	if err != nil {
		return nil, err
	}
	ranges := make([]byteRange, 0, len(insertions))
	for _, insertion := range insertions {
		row, present := table.ByID[insertion.RowID]
		if !present {
			return nil, fmt.Errorf("%s: inserted copy %d is absent after reopen", npcLayout.Entry, insertion.RowID)
		}
		team, err := summonTeam(teams[insertion.CopyFrom])
		if err != nil {
			return nil, fmt.Errorf("%s row %d: %w", npcLayout.Entry, insertion.RowID, err)
		}
		table.Data[row.DataOffset+npcTeamTypeOffset] = team
		ranges = append(ranges, rowRange(table, row))
	}
	if err := verifyBoundedMutation(copied, grown, ranges); err != nil {
		return nil, fmt.Errorf("%s: writing the copies' teams: %w", npcLayout.Entry, err)
	}
	return grown, nil
}

// insertNpcThinkClones adds the summon-side NpcThinkParam clones of the given
// world actors. An actor's AI row is the other half of its identity, and a
// summon whose think row is not marked as one stands still.
//
// perception names, per source, the profile written over the clone (see
// thinkProfiles). A source it does not name keeps the world row's perception
// and gets the buddy flag alone.
func insertNpcThinkClones(bnd4 []byte, sources []int32, perception map[int32]thinkChoice) ([]byte, error) {
	recipe, err := thinkCloneRecipe(perception)
	if err != nil {
		return nil, err
	}
	return insertActorClonesFrom(bnd4, npcThinkLayout, sources, recipe)
}

// thinkCloneRecipe is what insertNpcThinkClones writes over each think clone,
// as a function of the clone key and the world row's copy - shared with
// MatchClones for the reason npcCloneRecipe is.
func thinkCloneRecipe(perception map[int32]thinkChoice) (func(int32, []byte) error, error) {
	for source, choice := range perception {
		if _, known := thinkProfiles[choice.Level]; !known {
			return nil, fmt.Errorf("think clone %d asks for perception %q, which is not a profile this recipe knows", source, choice.Level)
		}
		if choice.OwnTarget && !ownTargetLevels[choice.Level] {
			return nil, fmt.Errorf("think clone %d asks for its own target on perception %q, which takes no share box", source, choice.Level)
		}
	}
	return func(source int32, row []byte) error {
		choice, named := perception[source]
		row[npcThinkIsBuddyAIOffset] = 1
		if !named {
			return nil
		}
		// The row is still the world row's byte copy here, so a level can read
		// the creature's own sight and eye origin before writing over them.
		profile := thinkProfiles[choice.Level]
		if _, onTheLadder := ladderBeyondBody[choice.Level]; onTheLadder {
			profile = ladderProfile(choice.Level, choice.Body, choice.Front, row, choice.NoTurn)
		} else if choice.Level == "loner" {
			profile = lonerProfile(choice.Body, row)
		} else if choice.Level == "eyes" && choice.Body > 0 {
			profile = eyesProfile(choice.Body)
		}
		if choice.OwnTarget {
			// ladderProfile built this map for this clone alone, so writing
			// into it cannot reach the shared thinkProfiles table.
			profile["targetSys_DmgEffectRate"] = ownTargetDamageRate
		}
		if err := applyThinkProfile(row, profile); err != nil {
			return err
		}
		if choice.WorldBranch {
			return applyThinkProfile(row, worldBranchThinkBand)
		}
		return nil
	}, nil
}

// thinkChoice is one think clone's perception: the level by name and, for the
// eyes level, the body radius its reach is measured from.
type thinkChoice struct {
	Level string
	Body  float64
	// Front is how far the actor reaches in front of its centre; zero falls
	// back to Body.
	Front float64
	// WorldBranch marks the think clone of an actor written without 297000:
	// it gets worldBranchThinkBand after its profile.
	WorldBranch bool
	// NoTurn marks an actor whose battle script has no Turn action; the three
	// middle levels give it a nose (ladderNoseForTheBack).
	NoTurn bool
	// OwnTarget: the share box is off, so the clone fights whoever hits it
	// (ownTargetLevels).
	OwnTarget bool
}

// ownTargetLevels are the levels whose target share the player chooses
// (formation.LevelShare): a clone of one of them that does not take the
// player's target is written targetSys_DmgEffectRate 100 in place of the
// level's 0 - the share box's think half, FromSoftware's pairing of share off
// with damage re-targeting (8 of its 11 share-off rows). The Statue and the
// Loner fix their own rate and take no box.
var ownTargetLevels = map[string]bool{"follower": true, "guard": true, "sentinel": true, "hunter": true}

// ownTargetDamageRate is what a clone on its own senses carries.
const ownTargetDamageRate = 100

// eyesBeyondTheBody is how far the eyes level sees past the actor's own
// collision radius, in metres. None of the creatures the game lets a player
// summon is big, so FromSoftware never had
// to say what a summoned dragon should see; a fixed distance blinds a body
// larger than it, and a world value on a boss row (Malenia 200 m) hunts
// half a map. Twenty metres beyond the body is what a man-sized creature
// sees in the world (modal 20/16/10 m over 1,856 rows).
const eyesBeyondTheBody = 20

// leashCeiling is the far edge of a leashed summon's KEEP: the brake fires for
// an enemy more than 23 + 5 m from the HOST, so holding a fight beyond it buys
// hovering rather than fighting. It does not cap the
// REACH: a level notices its whole distance past the front of every body, and
// past 23 + 5 m from the host a leashed summon notices and guards but does not
// charge, which is FromSoftware's own leash code.
const leashCeiling = 28

// ladderReach is how far a level notices, counted from the actor's centre: a
// constant PAST THE FRONT of the body.
//
// Past the front rather than a flat number because a flat one blinds anything
// large - a Follower on FromSoftware's literal 2 m would see two metres from
// the centre of a seventeen-metre dragon, which is inside its own model. The
// front is where the head ends (formation.Actor.Front), which on a round
// creature is its collision radius and on a long one is not: the Flying
// Dragon's head ends 14 m ahead of its 12 m radius. Rounded UP: the field is
// whole metres, and rounding to nearest would shorten the promise on most
// bodies (a Follower on a 0.4 m soldier would see 1.6 m past the body).
func ladderReach(beyond, front float64) float64 {
	return math.Ceil(beyond + front)
}

// A level NOTICES at its reach and KEEPS a fight at the creature's own sight
// (docs/DESIGN.md, the ladder).
//
// ladderKeepFloor is the least a fighting level keeps a fight at, beyond the
// body: FromSoftware's own keep on 14 of the 20 world rows that notice short
// and keep long (three more keep at 16), and the world's median eye. The keep
// is the battle sight - isUpdateBattleSight and battleEye_updateDist, "the
// COMBAT field of view" - so it changes what a creature holds in a fight and
// not what it does on noticing.
const ladderKeepFloor = 20

// ladderNoseForTheBack is the nose, beyond the body, given to a creature of
// the three middle levels whose battle script has no Turn action: hitting
// range all round. An eye cone is a width capped at 180 degrees, so such a
// creature loses whatever gets behind it however far it sees; a nose is a
// sphere with no cone and no line of sight. Kept to hitting range because a
// nose also notices outside a fight.
const ladderNoseForTheBack = 2

// ladderKeeps names the levels that fight on their own eyes and so keep a
// fight at the creature's own sight. The Hunter's fifty metres already keep
// anything a melee fight produces; the Statue has nothing to keep.
var ladderKeeps = map[string]bool{"follower": true, "guard": true, "sentinel": true}

// The think fields the recipe READS off the world row's copy before writing:
// where the creature's eye cone starts, and what it sees and keeps.
const (
	thinkEyeDistOffset         = 52
	thinkEyeAngXOffset         = 85
	thinkEyeAngYOffset         = 86
	thinkBattleSightOffset     = 121
	thinkBattleSightDistOffset = 122
	thinkEyeBackOffsetOffset   = 142
)

// eyeOrigin is how far BEHIND the actor's centre its world eye cone starts:
// eye_BackOffsetDist, "the vision cone starts this distance behind the
// character's centre" (FromSoftware's own description). The ladder writes it
// 0 - counted from 35 m behind, the Flying Dragon saw only under its head - so
// a level's cone starts at the centre as FromSoftware's own 98 Ash think rows
// all do, and this is read
// only to turn the creature's world sight into a distance from the centre.
func eyeOrigin(row []byte) float64 {
	return float64(binary.LittleEndian.Uint16(row[thinkEyeBackOffsetOffset:]))
}

// ownSight is how far the creature sees in the world, counted from its own eye
// origin: its eye, or its battle sight where the row carries a longer one.
func ownSight(row []byte) float64 {
	sight := float64(binary.LittleEndian.Uint16(row[thinkEyeDistOffset:]))
	if row[thinkBattleSightOffset] != 0 {
		sight = math.Max(sight, float64(binary.LittleEndian.Uint16(row[thinkBattleSightDistOffset:])))
	}
	return sight
}

// ladderProfile is one level of the ladder for a body of the given radius and
// front, written over row - the world row's copy, read before it is written.
//
// Everything counts from the CENTRE: the eye origin and its cut distance are
// written 0, the reach is the level's distance past the front, uncapped, and
// the three middle levels carry a nose the size of the body, so what touches
// the creature anywhere is noticed however long it is. A creature whose script
// cannot turn keeps the larger nose it had
// (ladderNoseForTheBack past the front). A level that keeps (ladderKeeps)
// writes the battle sight at the creature's own sight in front of its centre,
// floored at ladderKeepFloor past the front and capped at the leash, and never
// below its reach; every other level writes the battle sight OFF, so a world
// row that carries one cannot shrink the Hunter's fifty metres once it is
// fighting.
func ladderProfile(level string, body, front float64, row []byte, noTurn bool) map[string]float64 {
	profile := map[string]float64{}
	for field, value := range thinkProfiles[level] {
		profile[field] = value
	}
	beyond, ok := ladderBeyondBody[level]
	if !ok {
		return profile
	}
	if front < body {
		front = body
	}
	profile["eye_BackOffsetDist"] = 0
	profile["eye_BeginDist"] = 0
	reach := ladderReach(beyond, front)
	profile["eye_dist"] = reach
	profile["BattleStartDist"] = reach
	if nose, hasNose := ladderNoseBeyondBody[level]; hasNose {
		profile["nose_dist"] = math.Ceil(nose + front)
	}
	profile["isUpdateBattleSight"] = 0
	if ladderKeeps[level] {
		profile["nose_dist"] = math.Ceil(front)
		if noTurn {
			profile["nose_dist"] = math.Max(profile["nose_dist"], math.Ceil(ladderNoseForTheBack+front))
		}
		ahead := math.Max(ownSight(row)-eyeOrigin(row), 0)
		keep := math.Max(ahead, math.Ceil(ladderKeepFloor+front))
		if ladderLeashed[level] {
			keep = math.Min(keep, leashCeiling)
		}
		keep = math.Max(keep, reach)
		profile["isUpdateBattleSight"] = 1
		profile["battleEye_updateDist"] = keep
		profile["battleEye_updateAngX"] = profile["eye_angX"]
		profile["battleEye_updateAngY"] = profile["eye_angY"]
	}
	return profile
}

// lonerProfile is the Loner written over row: the creature's own senses, and a
// fight kept at no less than ladderKeepFloor beyond the body. A creature that
// already sees that far keeps its own battle sight untouched; a shorter one
// gets the battle sight at the floor, on its own eye cone.
func lonerProfile(body float64, row []byte) map[string]float64 {
	profile := map[string]float64{}
	for field, value := range thinkProfiles["loner"] {
		profile[field] = value
	}
	floor := math.Round(ladderKeepFloor+body) + eyeOrigin(row)
	if ownSight(row) >= floor {
		return profile
	}
	profile["isUpdateBattleSight"] = 1
	profile["battleEye_updateDist"] = floor
	profile["battleEye_updateAngX"] = float64(row[thinkEyeAngXOffset])
	profile["battleEye_updateAngY"] = float64(row[thinkEyeAngYOffset])
	return profile
}

// eyesProfile is the eyes research level: world eyes, no nose, 20 m beyond the body.
func eyesProfile(body float64) map[string]float64 {
	profile := map[string]float64{}
	for field, value := range thinkProfiles["eyes"] {
		profile[field] = value
	}
	reach := math.Round(eyesBeyondTheBody + body)
	profile["eye_dist"] = reach
	profile["BattleStartDist"] = reach
	return profile
}

// A think field the perception profiles write: its offset in the 228-byte
// NPC_THINK_PARAM_ST row and its width. The offsets are research's
// (internal/editor/thinkpatch.go), and the row size is asserted by
// npcThinkLayout.
//
// THERE ARE TWO WRITABLE SURFACES AND THIS IS THE SMALLER ONE. It is stage
// one - what a LEVEL writes onto every clone that takes it - and a field
// missing here is a field no level can write. Stage two is research's
// `writableThinkFields`, which a seat reaches through the console test tool's `think_set`
// column; it is larger and holds fields this one does not
// (`TeamAttackEffectivity`, `rangedAttackId`, `callHelp_CallValidRange`...).
// So "can a batch write this field?" is not answered by this table.
type thinkField struct {
	offset int
	kind   string // "u8", "u16", "s32", "f32"
}

var thinkFields = map[string]thinkField{
	// The search stage, all five of it. THE THRESHOLD IS THE SWITCH: -1
	// disables search outright, which is what 90 of FromSoftware's 98 Ash
	// think rows carry and what 1,810 of the 1,856 world rows do NOT - so
	// inheriting this field means search is on for almost every creature a
	// player can pick.
	"searchThreshold_Lv0toLv1":  {104, "s32"},
	"searchEye_dist":            {12, "u16"},
	"searchEye_angX":            {120, "u8"},
	"searchEye_angY":            {14, "u8"},
	"searchTargetLv1ForgetTime": {20, "f32"},
	"searchTargetLv2ForgetTime": {24, "f32"},
	"eye_dist":                  {52, "u16"},
	"nose_dist":                 {58, "u16"},
	// Where the eye cone starts and how much of it is cut: written 0 by every
	// ladder level, as FromSoftware's 98 Ash rows all carry them.
	"eye_BackOffsetDist":      {142, "u16"},
	"eye_BeginDist":           {144, "u16"},
	"maxBackhomeDist":         {60, "u16"},
	"backhomeDist":            {62, "u16"},
	"backhomeBattleDist":      {64, "u16"},
	"nonBattleActLife":        {66, "u16"},
	"BackHome_LookTargetTime": {68, "u16"},
	"BackHome_LookTargetDist": {70, "u16"},
	// The three forget times. SightTargetForgetTime is the one that decides
	// whether a summon lets go of a target it can no longer see; see
	// summonSightForget.
	"SightTargetForgetTime": {32, "f32"},
	"SoundTargetForgetTime": {72, "f32"},
	"BattleStartDist":       {76, "u16"},
	"eye_angX":              {85, "u8"},
	"eye_angY":              {86, "u8"},
	"actTypeOnFailedPath":   {146, "u8"},
	// disableDark exempts a row from the night sight penalty; see summonDaylight.
	"disableDark": {87, "u8"},
	// battleGoalID names the battle script. Only the Statue writes it: 11000 is
	// FromSoftware's Nanimosinai, "does nothing". Writing it on any other level
	// would change the creature's moveset, which is the one thing a summon must
	// keep.
	"battleGoalID": {8, "s32"},
	// logicId names the LOGIC script, the layer that runs first every think
	// cycle and decides everything outside battle - sounds, facing, following,
	// the weapon. Only the Statue writes
	// it: 11000 is the null twin of the battle goal above, and a creature's own
	// logic script would turn the Statue to face a sound. Offset from the schema
	// decode
	// (Paramdex NpcThinkParam: s32 logicId, before battleGoalID), and it agrees
	// with research's writer (internal/editor/thinkpatch.go).
	"logicId": {4, "s32"},
	// targetSys_DmgEffectRate, a PERCENTAGE weight on damage in the target
	// system's evaluation. 100 is Paramdex's default - the untouched value -
	// and 0 is the considered one: FromSoftware writes 0 on 67 of its 98 Ash
	// rows. With 0 an ally that is not being hit takes the player's shared
	// target and comes to help; with 100 it never does.
	"targetSys_DmgEffectRate": {82, "u16"},
	"MemoryTargetForgetTime":  {160, "f32"},
	"backToHomeStuckAct":      {171, "u8"},
	// The goal actions the logic script dispatches on a sound, a lost target
	// and the two search levels (measured on the console); the offsets are the research writer's, verified against the schema
	// decode.
	"goalAction_ToCaution":          {94, "u8"},
	"goalAction_ToCautionImportant": {147, "u8"},
	"goalAction_ToSearchLv1":        {154, "u8"},
	"goalAction_ToSearchLv2":        {155, "u8"},
	"goalAction_ToDisappear":        {158, "u8"},
	// The ear: the AiSoundParam behaviour ids this actor listens for, -1 for
	// an empty slot. See summonEar.
	"soundBehaviorId01": {176, "s32"},
	"soundBehaviorId02": {180, "s32"},
	"soundBehaviorId03": {184, "s32"},
	"soundBehaviorId04": {188, "s32"},
	"soundBehaviorId05": {192, "s32"},
	"soundBehaviorId06": {196, "s32"},
	"soundBehaviorId07": {200, "s32"},
	"soundBehaviorId08": {204, "s32"},
	// The keep - the battle sight, "the COMBAT field of view". Offsets from the
	// schema decode, and they agree with research's writer.
	"isUpdateBattleSight":  {121, "u8"},
	"battleEye_updateDist": {122, "u16"},
	"battleEye_updateAngX": {124, "u8"},
	"battleEye_updateAngY": {125, "u8"},
	// thinkAttr_doAdmirer, "plays a role of surrounding": 1 turns on the
	// spectate and circle roles a creature's battle script branches on. 0 on
	// the three middle levels, as FromSoftware writes it on 91 of its 97
	// near-blind Ash rows.
	"thinkAttr_doAdmirer": {99, "u8"},
	// The weapon draw and sheathe. The logic script plays them before any
	// battle goal whenever the target state leaves idle, so battleGoalID 11000
	// cannot stop them; the Statue clears all four (FromSoftware's -1 on 86 of
	// its 98 Ash rows). Not enableWeaponOnOff: that is bit 1 of a shared byte.
	"weaponOffSpecialEffectId": {208, "s32"},
	"weaponOnSpecialEffectId":  {212, "s32"},
	"weaponOffAnimId":          {216, "s32"},
	"weaponOnAnimId":           {220, "s32"},
}

// summonEar is what a summon listens for: FromSoftware's own answer, and only
// it.
//
// 901000 is the behaviour id of AiSoundParam 6600 - radius 300 m, life 5 s,
// priority 2000, aiSoundLevel 1 - and aicommon/common_sound_behavior.dec.lua
// answers it with GOAL_COMMON_ApproachTarget(8, TARGET_SOUND, 1.5-3 m) unless
// the actor carries 297103 PLAN_SP_EFFECT_BUDDY_NO_FOLLOW, walking instead of
// running when it carries 297100 BUDDY_WALK_FOLLOW. It is the host's regroup
// call, part of the follow system, and it is not a threat channel: **57 of the
// game's 98 shipped Spirit Ash think rows listen for exactly this and nothing
// else, and NOT ONE of the 1,856 world rows listens for it at all**.
//
// The world rows hear something else entirely. A Radahn or Leyndell Foot
// Soldier carries 431000-431002: a walk, a run, a weapon swing, radius 50 and
// priority 50. A summon built from such a row and left with its own ears hears
// THE PLAYER'S OWN WEAPON and walks to it. Deafening it would also throw away
// the regroup call, which is how a near-blind Ash stays with its master, so the
// ear is FromSoftware's convention, not silence.
var summonEar = map[string]float64{
	// In FromSoftware's own slot. 57 of the 98 shipped Ash think rows carry
	// exactly [-1 -1 -1 -1 -1 -1 -1 901000], and the rows that also keep a
	// world sound put THAT in the low slots with 901000 still in the eighth -
	// so the engine reads all eight and the eighth is where the summon call
	// goes.
	"soundBehaviorId01": -1, "soundBehaviorId02": -1, "soundBehaviorId03": -1,
	"soundBehaviorId04": -1, "soundBehaviorId05": -1, "soundBehaviorId06": -1,
	"soundBehaviorId07": -1, "soundBehaviorId08": 901000,
}

// summonPackage is everything EVERY level shares: what turns a copied world
// creature into a companion, as opposed to how far that companion can see.
// Each value is FromSoftware's own on the 98 think rows its Spirit Ash
// formations run.
//
// Without it a clone keeps the placed creature's idea of home and of what to
// do with a lost target. A world row's maxBackhomeDist is typically 85, the
// literal in _COMMON_SetBattleActLogic: a summoned actor is beyond its
// move-point range, so with the world's band it runs GOAL_COMMON_BackToHome
// toward an engine home it was never at, BLIND ON THE WAY.
// And a world row's goalAction_ToDisappear 2 walks to the spot a target was
// lost at, usually where the player stands, and swings there.
var summonPackage = map[string]float64{
	"SightTargetForgetTime": summonSightForget,
	// 0.5 is unanimous on all 98 shipped rows; 0.1 is on 93 of them.
	"SoundTargetForgetTime": 0.5, "MemoryTargetForgetTime": 0.1,
	// The home band: above the engine's move-point range, so a summon fights
	// where it stands instead of walking back to a home it never had.
	"maxBackhomeDist": 9999, "backhomeDist": 9979, "backhomeBattleDist": 10,
	"nonBattleActLife": 1, "BackHome_LookTargetTime": 7, "BackHome_LookTargetDist": 12,
	"actTypeOnFailedPath": 1, "backToHomeStuckAct": 0,
	// Face a sound, never walk to it; do NOTHING at the spot a target was lost
	// at. The first two are modal in both halves of the 98; the third is
	// FromSoftware's on 90 of them.
	"goalAction_ToCaution": 1, "goalAction_ToCautionImportant": 1,
	"goalAction_ToDisappear": 0, "goalAction_ToSearchLv1": 3, "goalAction_ToSearchLv2": 3,
	"disableDark": 1,
	// NO SEARCH: a summon that loses its target does NOTHING at the spot
	// (goalAction_ToDisappear 0, above) and the buddy branch takes it back to
	// the player. It never sweeps.
	//
	// The threshold is the switch and the rest are the stage's instruments.
	// Measured over FromSoftware's 98 Ash think rows, split the way a gated
	// field has to be - on the 90 that do NOT search:
	//
	//	searchThreshold_Lv0toLv1   -1    90 of 90, UNANIMOUS
	//	searchTargetLv1ForgetTime  1.0   90 of 90, UNANIMOUS
	//	searchTargetLv2ForgetTime  1.0   90 of 90, UNANIMOUS
	//	searchEye_dist / angX / angY  0  87 of 90
	//
	// Against the world, where 1,810 of 1,856 rows search: inheriting the
	// threshold would make almost every creature a player can pick search.
	//
	// goalAction_ToSearchLv1/2 stay 3 above: they are dead letters once the
	// threshold is -1, and 3 is what FromSoftware's own non-searching rows
	// carry (89 of 90).
	"searchThreshold_Lv0toLv1":  -1,
	"searchEye_dist":            0,
	"searchEye_angX":            0,
	"searchEye_angY":            0,
	"searchTargetLv1ForgetTime": 1,
	"searchTargetLv2ForgetTime": 1,
}

// thinkProfiles are the perception levels, by the names plan.ThinkPerception
// carries. Each is a set of field values written over the think clone after
// the buddy flag.
//
// The shared fields copy the game's own Spirit Ash convention: the modal value
// of each field across the 98 NpcThinkParam rows its Spirit Ash formations run
// - see all round, never go home, let the player's shared target do the
// long-range work. Every ladder level but the Loner nullifies the nose: a world
// nose is the creature's, 50 to 9999 m on a boss, which hunts the moment it
// arrives. ear_dist is 0.1 on every row in the game and is left alone.
//
// The research levels: "world" writes nothing but the buddy flag; "eyes" writes
// the nose to 0 and, where the manifest carries the actor's body radius, sets
// sight and the start of battle 20 m beyond it (eyesProfile); "blind" sees
// nothing on its own.
//
// CHANGING A LEVEL IS A DECISION, AND IT OWES TWO THINGS:
//
//  1. Run the ladder check (the author's ladder_diff) against a save built from
//     the changed ladder. It sorts every NpcThinkParam field by whether this
//     table writes it differently from FromSoftware's 98 Ash think rows, writes
//     it to match, or inherits it from the creature.
//  2. Write the reason into docs/DESIGN.md, the ladder (why each part),
//     which justifies every remaining difference field by field.

// statueGoal is FromSoftware's own "does nothing" battle goal, and statueLogic
// its "does nothing" logic script: one number, two scripts of the same name
// (Nanimosinai), and 12 of the game's think rows pair them.
const (
	statueGoal  = 11000
	statueLogic = 11000
)

// ladderBeyondBody is each level's reach, measured beyond the actor's body, and
// ladderLeashed says whether the leash ceiling applies to it. A level absent
// from these keeps the creature's own eyes (the Loner) or has none (the Statue).
//
// The Follower keeps FromSoftware's 2. A following summon trails the player by
// three or four metres, so an enemy ahead of the player is that much further
// from it, and the Guard's 12 makes that up. The Sentinel's 30, two and a half
// times the Guard, spots an enemy before the player does; past 23 + 5 m from
// the HOST a leashed summon notices and guards rather than engages, so its last metres buy warning, not fights.
var ladderBeyondBody = map[string]float64{"follower": 2, "guard": 12, "sentinel": 30, "hunter": 50}

var ladderLeashed = map[string]bool{"follower": true, "guard": true, "sentinel": true}

// ladderNoseBeyondBody is the nose, for the one level that has one.
//
// Every other level nullifies it, because a world nose is the creature's and on
// a boss it is 50 to 9999 m, which makes a boss hunt the moment it arrives -
// why nose_dist 0 is in ladderLevel. The Hunter is the level that WANTS that
// behaviour, bounded: 25 m beyond the body.
//
// It carries the level's whole rear coverage, and it does so because the eye
// cannot. eye_angY is 視覚_角度（幅）, a WIDTH capped at 180, so no cone sees
// behind the actor, and the idle roam does not sweep either - the buddy branch
// passes lookAroundAnimId -1 to GOAL_COMMON_WalkAround_Anywhere, and with -1
// the look-around phase is a plain Wait of 4 to 8 seconds with no animation
// (world actors get 3020; a buddy does not). The nose is a sphere: no cone, no
// line of sight. Anything inside 25 m is noticed whichever way he faces, and
// that is what stands in for the search stage, which is off.
var ladderNoseBeyondBody = map[string]float64{"hunter": 25}

// ladderLevel is one level's own fields: its cone, its sight-forget time and
// its damage rate. The reach is written per actor by ladderProfile, because it
// is measured from the body.
func ladderLevel(cone, sightForget, dmgRate float64) map[string]float64 {
	return map[string]float64{
		"eye_angX": cone, "eye_angY": cone, "nose_dist": 0,
		"SightTargetForgetTime":   sightForget,
		"targetSys_DmgEffectRate": dmgRate,
	}
}

// withoutRoles switches off the spectate and circle roles on a level.
func withoutRoles(profile map[string]float64) map[string]float64 {
	profile["thinkAttr_doAdmirer"] = 0
	return profile
}

var thinkProfiles = map[string]map[string]float64{
	"world": {},
	// "eyes" keeps the world row's eyes and nullifies the nose; it takes the
	// summon ear for the same reason the nose was nullified - a world row's
	// ears are the creature's own, and on a foot soldier they lead it to the
	// player's weapon (summonEar).
	// THE LADDER - docs/DESIGN.md. Each level owns its reach
	// and its cone and shares everything else. The forget time is 8 on every
	// level, the Follower included: FromSoftware's Ash keeps 15 because it
	// fights only the player's target, and ours also keeps fights of its own.
	// 8 is the world's value and the one all 20 of FromSoftware's battle-sight
	// rows carry.
	//
	// The three middle levels write thinkAttr_doAdmirer 0, as FromSoftware does
	// on 91 of its 97 near-blind Ash rows: a summon should not take the
	// spectator's or the circler's role around its own target.
	//
	// The Statue is not blindness: a blind actor still fights back when hit.
	// It runs battleGoalID 11000 - Nanimosinai, "does nothing", whose Interupt
	// is an unconditional false - and logicId 11000, whose whole body is a
	// five-second Wait: no sound behaviour, no caution action, no buddy branch,
	// no weapon toggle. A creature's own logic script would turn it to face a
	// sound, and no battle goal can stop that.
	//
	// It is handed no target: it hears nothing (its eighth sound slot is -1
	// like the other seven) and share is off in its stance
	// (formation.StanceStill); a hit still reaches it. All four weapon fields
	// are cleared and its weapon is out from the start
	// (formation.WeaponAlwaysOut), since a hit still draws it through the
	// animation layer. What is left is the engine's: its head follows the
	// target it holds. The senses stay zeroed as a net.
	"statue": withEar(map[string]float64{
		"logicId":      statueLogic,
		"battleGoalID": statueGoal,
		"eye_dist":     0, "BattleStartDist": 0, "nose_dist": 0, "searchEye_dist": 0,
		"targetSys_DmgEffectRate": 0, "isUpdateBattleSight": 0,
		"weaponOffSpecialEffectId": -1, "weaponOnSpecialEffectId": -1,
		"weaponOffAnimId": -1, "weaponOnAnimId": -1,
		"soundBehaviorId08": -1,
	}),
	"follower": withEar(withoutRoles(ladderLevel(180, 8, 0))),
	"guard":    withEar(withoutRoles(ladderLevel(90, 8, 0))),
	"sentinel": withEar(withoutRoles(ladderLevel(45, 8, 0))),
	// The Hunter is the unleashed level, and the leash it is free of is not in
	// this table: it is BuddyParam.pcFollowType, which the stance writes, and
	// formation.RequiredStance couples the two so a Hunter cannot be seated
	// leashed. See formation.PerceptionHunter for the predicate and why no
	// resident effect is used.
	//
	// Its three numbers, against FromSoftware's own 98 Ash think rows:
	//
	//	reach 50 + body   its independent Ashes carry 30 (9 of 11); a DEPARTURE,
	//	                  and it is what the level promises. Uncapped - the leash
	//	                  ceiling is the 23 + 5 m brake measured from the HOST
	//	                  and this level has no brake.
	//	nose 25 + body    all 11 independent rows carry 0; a DEPARTURE, and it
	//	                  is the rear coverage (ladderNoseBeyondBody).
	//	cone 180          matches: 180 on all 11, and on 85 of 85 turret rows.
	//
	// sight-forget 8 rather than the Follower's 15: hunting argues for letting
	// a lost target go and turning to the next thing, and 8 is the world's own
	// (1,257 rows) and four of the eleven independent Ashes'.
	//
	// damage rate 0 rather than the independent Ash's 100 BECAUSE SHARE IS ON.
	// The field is ダメージ影響率, the weight damage taken carries in this
	// actor's own target scoring, and it has nothing to do with the player. At
	// 100 a stray hit re-scores his target and pulls him off the host's, which
	// is the assist this level exists to provide - which is exactly why
	// FromSoftware's share-on companions carry 0 (62 of 86) and its share-off
	// independents carry 100 (8 of 11). Only 0 and 100 appear on any Ash row.
	//
	// Share is off by default on the Hunter, and the 100 that goes with share
	// off is written by the share box, not here (ownTargetLevels): this 0 is
	// the Hunter a player turns share on for.
	"hunter": withEar(ladderLevel(180, 8, 0)),
	// The Loner keeps the creature's own senses, and is the ONE level that does
	// not take the summon package.
	//
	// It is the configuration an ENEMY gets, and the point of it is fidelity:
	// the creature behaves as the player has met it in the world. So its eyes,
	// cone, nose, back-offset pair, forget times, lost-target actions and ears
	// are all the creature's.
	//
	// It takes exactly two things. The HOME BAND, because a world-branch clone
	// without it walks off toward an engine home it was never at, blind on the
	// way, and that is mechanical, not a
	// matter of taste. And targetSys_DmgEffectRate 100, because an enemy takes
	// aggro from whoever struck it last with no favourites, and 22 world rows
	// carry 0. And disableDark 1: every summoned creature keeps its full sight
	// after dark, the Loner included.
	"loner": {
		"maxBackhomeDist": 9999, "backhomeDist": 9979, "backhomeBattleDist": 10,
		"targetSys_DmgEffectRate": 100, "disableDark": 1,
	},

	"eyes":  withEar(map[string]float64{"nose_dist": 0, "SightTargetForgetTime": summonSightForget}),
	"blind": thinkProfile(0, 0, 0, 0, 0.5, 0.1),
}

// thinkProfile is the Spirit Ash convention with its reach and memory set: the
// home band, the non-battle life, the stuck action and the eye angles are what
// make a summon follow the player rather than return to a home it does not
// have.
// summonSightForget is how long a summon keeps a target it can no longer see,
// as the BATTLE target at the position it last saw it. It must be written: the
// Sound and Memory forget times cannot end a battle target, and about 200 of
// the creatures a player can pick carry 999 or 9999 here - a summon that keeps
// swinging at the spot its enemy left.
//
// 15 is FromSoftware's own answer for a summon: the modal value across the 98
// NpcThinkParam rows its Spirit Ash formations run, on 93 of them. Every
// ladder level that can act writes 8 over it (ladderLevel); 15 remains on the
// research levels and under the Statue, which never acts on a target.
const summonSightForget = 15

func thinkProfile(searchEye, eye, battleStart, nose, soundForget, memoryForget float64) map[string]float64 {
	return withEar(map[string]float64{
		"SightTargetForgetTime": summonSightForget,
		"searchEye_dist":        searchEye, "eye_dist": eye, "BattleStartDist": battleStart, "nose_dist": nose,
		"eye_angX": 180, "eye_angY": 180,
		"maxBackhomeDist": 9999, "backhomeDist": 9979, "backhomeBattleDist": 10,
		"nonBattleActLife": 1, "BackHome_LookTargetTime": 7, "BackHome_LookTargetDist": 12,
		"SoundTargetForgetTime": soundForget, "MemoryTargetForgetTime": memoryForget,
		"actTypeOnFailedPath": 1, "backToHomeStuckAct": 0,
		// The shipped Ash convention, on 89 of the 98 rows: face a sound, never
		// walk to it; do nothing at the spot a target was lost at; RUN to the
		// search target. Without these a profile keeps the world row's acts,
		// and a blind summon on a foot-soldier row walks to a swing and attacks
		// the air.
		"goalAction_ToCaution": 1, "goalAction_ToCautionImportant": 1,
		"goalAction_ToDisappear": 0, "goalAction_ToSearchLv1": 3, "goalAction_ToSearchLv2": 3,
	})
}

// (The daylight exemption is in summonPackage: every level keeps its full
// reach after dark, a deliberate departure - FromSoftware leaves the flag at 0
// on 69 of its own 98 rows.)

// withEar adds the summon ear and the shared summon package to a profile,
// leaving anything the level already says for itself alone - so a level still
// owns its REACH and its cone, and shares everything that makes it a companion. "world" does not get it: it
// exists to be the world row as the game has it.
func withEar(profile map[string]float64) map[string]float64 {
	for _, shared := range []map[string]float64{summonEar, summonPackage} {
		for field, value := range shared {
			if _, said := profile[field]; !said {
				profile[field] = value
			}
		}
	}
	return profile
}

// applyThinkProfile writes one profile's fields over a think row.
func applyThinkProfile(row []byte, profile map[string]float64) error {
	for name, value := range profile {
		field, known := thinkFields[name]
		if !known {
			return fmt.Errorf("think profile names field %q, which the recipe has no offset for", name)
		}
		switch field.kind {
		case "u8":
			if value < 0 || value > 255 {
				return fmt.Errorf("%s = %g is outside a byte", name, value)
			}
			row[field.offset] = byte(value)
		case "u16":
			if value < 0 || value > 65535 {
				return fmt.Errorf("%s = %g is outside two bytes", name, value)
			}
			binary.LittleEndian.PutUint16(row[field.offset:], uint16(value))
		case "s32":
			if value < -2147483648 || value > 2147483647 || value != math.Trunc(value) {
				return fmt.Errorf("%s = %g is not an int32", name, value)
			}
			binary.LittleEndian.PutUint32(row[field.offset:], uint32(int32(value)))
		case "f32":
			binary.LittleEndian.PutUint32(row[field.offset:], math.Float32bits(float32(value)))
		default:
			return fmt.Errorf("think field %s has kind %q", name, field.kind)
		}
	}
	return nil
}

// insertActorClonesFrom inserts one clone per source and writes the recipe over
// it, with the source row's ID handed to the recipe.
//
// The two proofs are the ones each half already carries. InsertRows shows that
// every shipped row survived byte-identically, that the only new rows are the
// declared ones, and that each is a copy of its source; verifyBoundedMutation
// then shows the recipe changed nothing outside the rows just inserted.
func insertActorClonesFrom(bnd4 []byte, layout regulation.TableLayout, sources []int32, recipe func(int32, []byte) error) ([]byte, error) {
	insertions, err := planActorClones(bnd4, layout, sources)
	if err != nil {
		return nil, err
	}
	grown, err := regulation.InsertRows(bnd4, layout, insertions)
	if err != nil {
		return nil, err
	}
	copied := bytes.Clone(grown)
	table, err := regulation.OpenTable(grown, layout)
	if err != nil {
		return nil, err
	}
	ranges := make([]byteRange, 0, len(insertions))
	for _, insertion := range insertions {
		row, present := table.ByID[insertion.RowID]
		if !present {
			return nil, fmt.Errorf("%s: inserted clone %d is absent after reopen", layout.Entry, insertion.RowID)
		}
		// The recipe is looked up by the clone KEY, which is the row less the
		// offset in every band: the manifest describes a clone by its key.
		if err := recipe(insertion.RowID-cloneRowOffset, table.Data[row.DataOffset:row.DataOffset+layout.RowSize]); err != nil {
			return nil, fmt.Errorf("%s row %d: %w", layout.Entry, insertion.RowID, err)
		}
		ranges = append(ranges, rowRange(table, row))
	}
	if err := verifyBoundedMutation(copied, grown, ranges); err != nil {
		return nil, fmt.Errorf("%s: writing the summon recipe: %w", layout.Entry, err)
	}
	return grown, nil
}

// planActorClones turns clone keys into the insertions that clone them.
//
// The destination is arithmetic and never an argument, which is what keeps
// docs/SAFETY.md rule 1 true by construction: the row is key + cloneRowOffset,
// its last eight digits are the source exactly in every clone band, so a clone
// can only land in its own cXXXX model namespace, and only a world actor - a
// row below the offset - can be copied at all.
//
// Placement is arithmetic too. Every shipped table this product opens carries
// its row headers in strictly ascending ID order, and research's seen
// actor insertions each anchored on the row below the new one, so the anchor is
// the largest ID below the destination rather than a caller's choice. The
// source is itself such an ID, so one always exists.
func planActorClones(bnd4 []byte, layout regulation.TableLayout, sources []int32) ([]regulation.RowInsertion, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("%s: no actors to clone", layout.Entry)
	}
	table, err := regulation.OpenTable(bnd4, layout)
	if err != nil {
		return nil, err
	}
	ordered := append([]int32(nil), sources...)
	slices.Sort(ordered)
	present := make([]int32, len(table.Rows))
	for index, row := range table.Rows {
		present[index] = row.ID
	}
	insertions := make([]regulation.RowInsertion, 0, len(ordered))
	var previous int32
	for index, key := range ordered {
		if index > 0 && key == ordered[index-1] {
			return nil, fmt.Errorf("%s: actor %d is cloned twice", layout.Entry, key)
		}
		source, ok := cloneKeySource(key)
		if !ok {
			return nil, fmt.Errorf("%s: %d is not a clone key, so it has no clone to insert: %w",
				layout.Entry, key, ErrRowNotFound)
		}
		if _, exists := table.ByID[source]; !exists {
			return nil, fmt.Errorf("%s: actor %d was not found: %w", layout.Entry, source, ErrRowNotFound)
		}
		destination := key + cloneRowOffset
		if _, exists := table.ByID[destination]; exists {
			return nil, fmt.Errorf("%s: clone %d of actor %d already exists", layout.Entry, destination, source)
		}
		anchor := present[sort.Search(len(present), func(index int) bool { return present[index] >= destination })-1]
		if anchor < previous {
			anchor = previous
		}
		insertions = append(insertions, regulation.RowInsertion{RowID: destination, CopyFrom: source, InsertAfter: &anchor})
		previous = destination
	}
	return insertions, nil
}

// makeSummonSide turns a copied world-actor row into a summon-side one.
//
// The resident-effect vector is decided before anything is written, because it
// is the only part that can fail, and a half-converted row is a worse thing to
// hand back than one not converted at all.
func makeSummonSide(row []byte, strength npcCloneStrength, team uint8, worldBranch, unheld bool) error {
	// full is the switch, and naming a clone in the strength list is what
	// turns it on: the Spirit Summon balance package is left out, so the
	// summon deals the creature's own damage, takes the creature's own damage
	// and staggers what it hits like the creature does. See docs/DESIGN.md (Summon strength).
	full := strength.Source != 0 && !strength.KeepBalance
	effects, err := summonResidentEffects(row, full, worldBranch, unheld, strength.Residents, strength.Removes, strength.Presence, strength.Marker)
	if err != nil {
		return err
	}
	// The team is the other switch: Spirit Summon unless the manifest put the
	// clone on one of the hostile teams, decided before anything is written
	// for the same reason the effects are.
	teamType, err := summonTeam(team)
	if err != nil {
		return err
	}
	if strength.HP > 0 {
		// The base the ladder multiplies. Written before the ladder can see
		// it, and only where the caller said what it should be.
		writeS32(row, npcHPOffset, strength.HP)
	}
	capF32(row, npcHitHeightOffset, summonHitHeightCeiling)
	capF32(row, npcHitRadiusOffset, summonHitRadiusCeiling)
	// A summon is carried, not found: it pays out neither runes nor items, and
	// it does not stand in for a boss the player has yet to beat.
	writeS32(row, npcGetSoulOffset, 0)
	writeS32(row, npcItemLotEnemyOffset, -1)
	writeS32(row, npcItemLotMapOffset, -1)
	writeS32(row, npcGameClearEffectOffset, -1)
	row[npcSoulByBossFlagsOffset] &^= npcSoulByBossFlagMask
	row[npcTeamTypeOffset] = teamType
	for index, offset := range npcResidentEffectOffsets {
		writeS32(row, offset, effects[index])
	}
	return nil
}

// summonResidentEffects is the row's resident-effect vector as a summon needs
// it: both area scaling bands cleared, the three companion effects present.
//
// worldBranch leaves companion movement (297000) out and strips it where the
// source carried it. 297000 is PLAN_SP_EFFECT_BUDDY_DECLARE, the one effect
// the summon's logic script forks on: with it the actor runs the buddy branch
// - follows and warps to the player, is leashed to 23 m of the host, is fed
// the host's target - and without it the WORLD branch, as any enemy the game
// places: alive,
// fighting, neither following nor warping. It is what an enemy needs
// (docs/DESIGN.md, the ladder).
//
// presence, when non-zero, is written in place of 295000 wherever the recipe
// would write it: a copy of the presence at another id, carrying another look
// (the per-creature spirit look). Zero is 295000. marker is the same for
// 297000, the companion marker (QuietMarker).
func summonResidentEffects(row []byte, full, worldBranch, unheld bool, extra, removes []int32, presence, marker int32) ([]int32, error) {
	effects := make([]int32, len(npcResidentEffectOffsets))
	for index, offset := range npcResidentEffectOffsets {
		effect := readS32(row, offset)
		if (effect >= areaScalingFirst && effect < areaScalingPast) ||
			(effect >= dlcAreaScalingFirst && effect < dlcAreaScalingPast) {
			effect = -1
		}
		effects[index] = effect
	}
	// A composite's removal, before anything is added, so its slot is free for
	// the recipe. Every copy goes; one the source does not carry is refused,
	// because a removal that removes nothing is a manifest describing another
	// creature than the one it copies.
	for _, effect := range removes {
		if slotOf(effects, effect) < 0 {
			return nil, fmt.Errorf("the source row carries no resident effect %d to remove: %w", effect, ErrManifestInvalid)
		}
		for slot := slotOf(effects, effect); slot >= 0; slot = slotOf(effects, effect) {
			effects[slot] = -1
		}
	}
	required := spiritCompanionEffects
	if full {
		// Presence and companion movement stay; only the balance package goes.
		// A summon without 295000 is not a summon and one without 297000 does
		// not follow, and neither has anything to do with how hard it hits.
		required = []int32{spiritPresenceEffect, spiritFollowEffect}
		if slot := slotOf(effects, spiritBalanceEffect); slot >= 0 {
			effects[slot] = -1
		}
	}
	if worldBranch {
		// Presence stays (the tint, and the one companion effect never yet
		// removed from a clone); companion movement goes, so the logic script
		// takes the world branch. A world row that happens to carry 297000
		// (none does, but the strip is what the switch promises) loses it.
		required = []int32{spiritPresenceEffect, worldStayEffect}
		if unheld {
			// The engine's enemy, on purpose: no 5011, so the actor walks home.
			required = []int32{spiritPresenceEffect}
		}
		if !full {
			required = append(required, spiritBalanceEffect)
		}
		for slot := slotOf(effects, spiritFollowEffect); slot >= 0; slot = slotOf(effects, spiritFollowEffect) {
			effects[slot] = -1
		}
	}
	if presence > 0 || marker > 0 {
		swapped := make([]int32, len(required))
		for index, effect := range required {
			switch {
			case effect == spiritPresenceEffect && presence > 0:
				effect = presence
			case effect == spiritFollowEffect && marker > 0:
				effect = marker
			}
			swapped[index] = effect
		}
		required = swapped
	}
	for _, required := range required {
		if slotOf(effects, required) >= 0 {
			continue
		}
		empty := slotOfEmpty(effects)
		if empty < 0 {
			return nil, fmt.Errorf("no empty resident slot for Spirit Summon effect %d", required)
		}
		effects[empty] = required
	}
	// The extra residents - the creature's grants, a level's own - after the
	// required three, because they are worth less than the summon working at
	// all. Added rather than deduplicated: the same effect twice is a square,
	// and collapsing a repeat would quietly halve what was asked for.
	for _, effect := range extra {
		if effect <= 0 {
			return nil, fmt.Errorf("resident effect %d is not a row ID", effect)
		}
		if effect == worldStayEffect && slotOf(effects, effect) >= 0 {
			// A seat that spells 5011 out beside the recipe does not get it
			// twice.
			continue
		}
		empty := slotOfEmpty(effects)
		if empty < 0 {
			return nil, fmt.Errorf("no empty resident slot for New Game + effect %d", effect)
		}
		effects[empty] = effect
	}
	return effects, nil
}

func slotOf(effects []int32, want int32) int {
	for index, effect := range effects {
		if effect == want {
			return index
		}
	}
	return -1
}

// slotOfEmpty treats any non-positive value as empty, which is what the shipped
// data means by one: the vector spells "nothing here" as both -1 and 0, and
// neither is a row ID the runtime could resolve.
func slotOfEmpty(effects []int32) int {
	for index, effect := range effects {
		if effect <= 0 {
			return index
		}
	}
	return -1
}

func readS32(row []byte, offset int) int32 {
	return int32(binary.LittleEndian.Uint32(row[offset:]))
}

func writeS32(row []byte, offset int, value int32) {
	binary.LittleEndian.PutUint32(row[offset:], uint32(value))
}

// capF32 lowers a field to the ceiling and leaves anything already below it
// alone, so a clone keeps the source's own proportions unless they would not
// fit where a summon has to stand.
func capF32(row []byte, offset int, ceiling float32) {
	if math.Float32frombits(binary.LittleEndian.Uint32(row[offset:])) <= ceiling {
		return
	}
	binary.LittleEndian.PutUint32(row[offset:], math.Float32bits(ceiling))
}
