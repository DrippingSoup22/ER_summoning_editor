// Package formation is the v2 authoring model: what a player's chosen
// formation is, and which rows it becomes.
//
// A formation is not one row with a count field. It is a block of BuddyParam
// rows sharing one triggerSpEffectId, one row per summoned actor, each naming
// its own actor independently. Lone Wolf Ashes is three rows all naming the
// same wolf; Godrick Soldier Ashes is two rows naming two different NPCs. So a
// repeated formation and a mixed one are the same write, and there is no
// "single entity type" tier to build first.
//
// The package is pure. It reads no save, opens no file and knows no byte
// offsets: it is handed what the regulation says and returns the decision.
// internal/savecore owns how that decision is written, exactly as it does for
// internal/plan.
package formation

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// MaxSlots is the largest formation the game delivers intact: it runs ten rows
// of a longer block and silently drops the rest, so a larger one is refused.
const MaxSlots = 10

// DopingLevels is the size of the dopingSpEffect_lv0..lv10 table every
// formation row carries. The game indexes it with the player's own item level,
// so it is per row and not per formation.
const DopingLevels = 11

// The clone namespace. FromSoftware separates summon actors from world actors
// by ID: a summon-side clone of world actor N is N + CloneOffset. Every world
// NpcParam row in the shipped regulation is below CloneOffset — the highest is
// 95090000 — and every ID at or above it is already a summon-side row.
const (
	CloneOffset  = 100000000
	cloneCeiling = 200000000
)

// CloneBands hold a creature's clones, one per recipe. A clone is a copy of
// the creature's world row with a recipe written over it - a level, a share, a
// side - and one row holds one recipe, so a creature on two recipes needs two
// rows. The first is FromSoftware's own address, source + CloneOffset (band
// 1); the n-th is source + n x CloneOffset. The game's NpcParam and
// NpcThinkParam hold rows in bands 0 (the world), 1 (the summons) and 5 only,
// and an s32 id ends inside band 21, so band 5 is skipped and a clone in any
// band keeps the creature's whole id, model and all, in its last eight digits.
//
// A clone is named by its KEY, never by its row: key = source + (band - 1) x
// CloneOffset, so the row is always key + CloneOffset (docs/SAFETY.md rule 1).
//
// Untested: whether the game summons a row above band 1.
var CloneBands = []int32{1, 2, 3, 4, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

// CloneKey is the key of source's clone in band.
func CloneKey(source, band int32) (int32, error) {
	if source <= 0 || source >= CloneOffset {
		return 0, fmt.Errorf("%d is not a world row: %w", source, ErrNotSummonable)
	}
	if !slices.Contains(CloneBands, band) {
		return 0, fmt.Errorf("band %d is not a clone band %v: %w", band, CloneBands, ErrNotSummonable)
	}
	return source + (band-1)*CloneOffset, nil
}

// SplitCloneKey is a key's world source and band; ok is false for anything
// that is not a key.
func SplitCloneKey(key int32) (source, band int32, ok bool) {
	source, band = key%CloneOffset, key/CloneOffset+1
	if key <= 0 || source == 0 || !slices.Contains(CloneBands, band) {
		return 0, 0, false
	}
	return source, band, true
}

// CloneSource is the world row a summon-side clone in any band was copied
// from, and whether row is such a clone at all. Band 1 is also where
// FromSoftware's own summons live, so "a clone" here means "a row in a clone
// band", not "a row this product wrote".
func CloneSource(row int32) (int32, bool) {
	if row < CloneOffset {
		return 0, false
	}
	source, _, ok := SplitCloneKey(row - CloneOffset)
	return source, ok
}

// InCloneBand reports whether row is in a clone band: band 1 whole - the
// summon namespace, where FromSoftware ships rows that are no clone of anything
// (the person shell at 100000000) - and the clones of world rows above it.
// The character bodies are cut out of it (IsCharacterBody).
func InCloneBand(row int32) bool {
	if IsCharacterBody(row) {
		return false
	}
	if row >= CloneOffset && row < cloneCeiling {
		return true
	}
	_, ok := CloneSource(row)
	return ok
}

// The summon-side copy band. A row the game ships summon-side - a person
// shell, 100000010..60 and 150000000..5 - is never written, so a slot that
// needs one changed (a person on a team) names the shell and the compiler
// copies it. The copy lands above every clone a world row can have: world
// rows end at 95090000, their clones at 195090000, and 199000000..199999999
// holds nothing the game shipped. Two arithmetic rules, each invertible like
// the clone rule: a row within summonCopyWindow of CloneOffset copies to
// summonCopyFirst + its distance (100000020 -> 199000020), one within the
// window of dlcShellFirst to summonCopySplit + its distance (150000003 ->
// 199500003). The window is narrow on purpose: the shells sit at
// 100000000..60 and 150000000..12, and the first shipped creature clone in
// either range (150400000) is outside it. A summon-side row outside both
// windows has no copy the band can hold, and is refused - a shipped creature
// row is summoned hostile through its world twin's clone, which carries the
// team itself.
const (
	summonCopyFirst  = 199000000
	summonCopySplit  = 199500000
	summonCopyWindow = 100000
	dlcShellFirst    = 150000000
)

var (
	// ErrEmptyFormation means no slots were requested. A formation with
	// nothing in it is not a smaller formation; it is a formation that summons
	// nothing, which the player cannot have meant.
	ErrEmptyFormation = errors.New("a formation needs at least one slot")
	// ErrTooManySlots means the game would discard the surplus silently.
	ErrTooManySlots = errors.New("a formation may hold at most ten slots")
	// ErrNotSummonable means an actor ID is outside the range the clone rule
	// can express, so no safe destination exists for it.
	ErrNotSummonable = errors.New("actor ID is outside the summonable range")
	// ErrIncompleteReinforcement means the host block has a gap in its level
	// table at a level that reaches it (Block.Levels), where a summon would
	// arrive with no reinforcement.
	ErrIncompleteReinforcement = errors.New("host formation is missing reinforcement levels")
	// ErrUnusableBlock means the host block does not describe a formation this
	// package can author into.
	ErrUnusableBlock = errors.New("host formation block is unusable")
	// ErrNoFreeRow means the block cannot be extended without colliding with a
	// row that already exists.
	ErrNoFreeRow = errors.New("no free BuddyParam row follows the host block")
	// ErrUnknownTeam means a slot asks for a team no summon has been seen on.
	ErrUnknownTeam = errors.New("team is not one a summon may stand on")
	// ErrStanceMismatch means a level whose behaviour lives partly in
	// BuddyParam was seated on a stance that contradicts it. The Hunter is the
	// only such level: its leash is lifted by pcFollowType, not by anything in
	// the think row, so a Hunter on any other stance is a leashed creature
	// still calling itself a Hunter. Refused rather than corrected, because a
	// silent correction is how a seat gets built asking one question and
	// answering another.
	ErrStanceMismatch = errors.New("perception level requires a stance the slot does not carry")
	// ErrTwoTeams means two slots put one shipped shell on two different
	// teams, and a shell's one copy carries one team. (A world creature on two
	// teams is two clones.)
	ErrTwoTeams = errors.New("one shell cannot stand on two teams in one formation")
	// ErrShareFixed means a slot chose the player's target on or off for a
	// level whose share the level itself fixes: the Statue and the Loner never
	// share.
	ErrShareFixed = errors.New("this level's target share is fixed and cannot be chosen")
	// ErrEnemyLevel means an enemy was given a level that serves the player.
	// An enemy may be a Hunter, a Statue or a Loner, the Loner by default; the
	// Follower, the Guard and the Sentinel are not helping anyone on the other
	// team.
	ErrEnemyLevel = errors.New("an enemy can only be a Hunter, a Statue or a Loner")
	// ErrEnemyShares means an enemy was asked to share the player's target. An
	// enemy is on its own, the Hunter included.
	ErrEnemyShares = errors.New("an enemy never shares the player's target")
)

// Share is whether a summon takes the player's target. On, the summon acts on
// what the player does and what happens to him, and stays in a fight for as
// long as the world does; off, it thinks with its own senses and lets go of
// what it cannot see after its forget time.
//
// It is a PACKAGE, not a field: share on is BuddyParam.disablePCTargetShare 0
// with the think clone's targetSys_DmgEffectRate 0, share off is 1 with 100 -
// FromSoftware's own pairing (its share-on companions carry 0 on 62 of 86 think
// rows, its share-off independents 100 on 8 of 11). Nothing else rides on it.
//
// The zero value is the level's default (LevelShare). It lives on the Actor, not
// the Slot, because the damage rate is on the think clone and two slots naming
// one think row share one clone.
type Share uint8

const (
	// ShareDefault is whatever the level shares by default.
	ShareDefault Share = 0
	// ShareOn hands the summon the player's target.
	ShareOn Share = 1
	// ShareOff leaves the summon on its own senses.
	ShareOff Share = 2
)

// LevelShare is a ladder level's target share: on by default or not, and
// whether the player may choose. known is false for the research levels and
// the as-held marker, which take their stance as the slot gives it.
//
//	Follower  on,  chosen   FromSoftware's Ash: its 2 m find nothing without it
//	Guard     off, chosen   a fighter on its own eyes (12 m)
//	Sentinel  off, chosen   the same, further (30 m)
//	Hunter    off, chosen   it hunts on its own; a shared target reaches it only
//	                        about as far as the leash
//	Statue    off, fixed    it is handed nothing
//	Loner     off, fixed    FromSoftware's independent Ash
func LevelShare(p Perception) (defaultOn, choosable, known bool) {
	switch p {
	case PerceptionFollower:
		return true, true, true
	case PerceptionGuard, PerceptionSentinel, PerceptionHunter:
		return false, true, true
	case PerceptionStatue, PerceptionLoner:
		return false, false, true
	}
	return false, false, false
}

// ResolveShare is whether the actor takes the player's target: its choice, or
// its level's default. It refuses a choice the level does not offer and a
// share an enemy would carry.
func ResolveShare(actor Actor) (bool, error) {
	defaultOn, choosable, known := LevelShare(actor.Perception)
	if actor.Share > ShareOff {
		return false, fmt.Errorf("share %d is not default, on or off: %w", actor.Share, ErrShareFixed)
	}
	if actor.Share != ShareDefault && !choosable {
		if !known {
			return false, fmt.Errorf("perception %q takes its share from the stance: %w", actor.Perception.Name(), ErrShareFixed)
		}
		return false, fmt.Errorf("perception %q never shares: %w", actor.Perception.Name(), ErrShareFixed)
	}
	on := defaultOn
	if actor.Share != ShareDefault {
		on = actor.Share == ShareOn
	}
	if on && actor.Enemy() {
		return false, fmt.Errorf("perception %q: %w", actor.Perception.Name(), ErrEnemyShares)
	}
	return on, nil
}

// Enemy reports whether the actor is summoned as an ENEMY: on a team other than
// the Spirit Summon team and on the world branch, so it neither follows the
// player nor is fed his target.
func (actor Actor) Enemy() bool {
	return actor.WorldBranch && actor.Team != TeamSummon
}

// EnemyLevels are the ladder levels an enemy may take, its default first: the
// three that do not serve the player.
var EnemyLevels = []Perception{PerceptionLoner, PerceptionHunter, PerceptionStatue}

// DefaultLevel is the level a creature gets when nobody chose one: the Follower
// for a summon, the Loner for an enemy.
func DefaultLevel(enemy bool) Perception {
	if enemy {
		return PerceptionLoner
	}
	return PerceptionFollower
}

// LevelStance is the stance a slot carries for its actor's level and share
// choice: the level's follow type where it fixes one (RequiredStance) and the
// product's follow otherwise, with the share the actor resolves to. known is
// false off the ladder, where the stance is whatever the slot gives. A
// frontend offering a level fills the slot's stance from this, so the slot is
// never refused for a stance the player did not choose.
func LevelStance(actor Actor) (Stance, bool) {
	if _, _, known := LevelShare(actor.Perception); !known {
		return Stance{}, false
	}
	stance := StanceFollow
	if required, coupled := RequiredStance(actor.Perception); coupled {
		stance.PcFollowType = required.PcFollowType
	}
	on, err := ResolveShare(actor)
	if err != nil {
		defaultOn, _, _ := LevelShare(actor.Perception)
		on = defaultOn && !actor.Enemy()
	}
	if on {
		stance.DisablePCTargetShare = 0
	} else {
		stance.DisablePCTargetShare = 1
	}
	return stance, true
}

// Actor is one summonable entity, named by the pair of rows that define it:
// what it is and how it thinks. Both are needed, because a BuddyParam row
// carries both and a row that keeps a stale npcThinkParamId spawns something
// that stands still.
type Actor struct {
	Npc   int32
	Think int32
	// Perception is how far the actor's AI sees on its own once summoned: the
	// profile written over its think clone. The zero value is the Follower.
	// Two slots naming one think row share one clone, and the first slot's
	// perception is the one written.
	Perception Perception
	// Body is the actor's collision radius in metres, the one most of its
	// model's placed rows carry (carried in
	// reference/bodies.csv). Reach is measured beyond it, so nothing is blind
	// to what stands inside its own body. Zero means unknown.
	Body float64
	// Front is how far the actor reaches IN FRONT of its centre: where its
	// look from above ends ahead, never short of Body (the same table). The ladder counts a level's reach past it and sizes the nose to
	// it: counted from the radius, a dragon whose head ends well ahead of it
	// sees only under its head. Zero means unknown, and the writer falls back
	// to Body.
	Front float64
	// NoTurn says the actor's battle script has no Turn action (carried in reference/think-turns.csv).
	// An eye cone never sees behind, so such a creature loses a target that
	// gets behind it whatever its reach; the Follower, the Guard and the
	// Sentinel give it a nose of hitting range to cover its back (docs/DESIGN.md, the ladder).
	NoTurn bool
	// Person is the CharaInitParam row this slot summons INSTEAD of a creature:
	// a human being with that record's stats, equipment and face, built on
	// the shell Npc names - one of the game's own person shells (100000010,
	// 100000020, ...: human proportions, teamType 47, no identity of their
	// own). Zero is a creature. A person and a creature may share a formation;
	// the shells are summon-side already, so a person slot inserts no clone.
	Person int32
	// Team is the NpcParam teamType the summon stands on. Zero is the Spirit
	// Summon team every Ash the game ships carries (47, which the save core
	// writes on every clone); any other value is written on the clone in its
	// place and makes the summon hostile, the route Spirit Battler takes on PC.
	// A team on a summon-side row (a person's shell) puts the slot on a copy
	// of that row, since the shipped row is never written.
	Team Team
	// WorldBranch summons the actor WITHOUT companion movement (resident
	// effect 297000, PLAN_SP_EFFECT_BUDDY_DECLARE): the one effect the
	// summon's logic script forks on. With it the actor runs the buddy branch
	// - follows and warps to the player, breaks off a fight at 23 m from the
	// host, is fed the host's target; without it the world branch, as every
	// enemy the game places: alive and fighting, neither following nor
	// warping. It is what an enemy is.
	// Only a creature clone can carry it: a person's shell is a shipped
	// summon row and is never rewritten.
	WorldBranch bool
	// WorldBranchUnheld leaves a world-branch clone as the engine leaves it:
	// WITHOUT the 5011 resident and WITHOUT the shipped Ash home band that
	// the write otherwise adds, so the actor walks back to an engine home,
	// blind, and once held fights only inside 2 m (measured on the console). A research switch for
	// the author's console test tool; it means nothing without WorldBranch and no level the
	// product offers sets it.
	WorldBranchUnheld bool
	// Share is the player's choice of whether this creature takes his target -
	// the box beside the level, live on the Follower, the Guard, the Sentinel
	// and the Hunter (see Share). The zero value is the level's default.
	Share Share
	// Presence is the effect the clone carries in place of 295000, the Spirit
	// Summon presence: a copy of it with another look (the per-creature spirit
	// look). Marker is the same for 297000, the companion marker - a copy no
	// battle script tests, for a creature whose script disarms it under the
	// shipped one. Zero keeps the shipped effect. Both are part of the
	// recipe: two slots of one creature that differ in either are two clones.
	Presence int32
	Marker   int32
	// Fighter summons the actor as the creature rather than as a Helper: no
	// Spirit Summon balance package on its clone, and its own health instead
	// of a Helper's (docs/DESIGN.md, Summon strength). Part of the recipe like
	// the look: a Helper and a Fighter of one creature are two clones. A row
	// the save already holds is never cloned, so it means nothing there.
	Fighter bool
	// Arrival is the animation the actor arrives on where its creature's own
	// suits it better than its kind's (St. Trina is
	// drawn only on her 30000). Zero is its kind's. A row's, not the recipe's.
	Arrival int32
}

// Team is an NpcParam teamType. The vocabulary is closed to the values a
// Spirit Ash has been seen standing on: the game's own (47) and the four
// Spirit Battler writes to make a summon hostile.
type Team uint8

const (
	// TeamSummon is the zero value: the Spirit Summon team, written as 47.
	TeamSummon Team = 0
	// TeamEnemy is the ordinary enemy team (6): hostile to the player and
	// to every summon.
	TeamEnemy Team = 6
	// TeamBeast is the wildlife team (24).
	TeamBeast Team = 24
	// TeamHostileNPC is the team an invading or angered character stands on (27).
	TeamHostileNPC Team = 27
	// TeamArchEnemy is the boss team (33).
	TeamArchEnemy Team = 33
)

// Teams is the closed vocabulary, the default first.
var Teams = []Team{TeamSummon, TeamEnemy, TeamBeast, TeamHostileNPC, TeamArchEnemy}

// Valid reports whether the team is one a summon may stand on.
func (team Team) Valid() bool {
	for _, known := range Teams {
		if team == known {
			return true
		}
	}
	return false
}

// Perception is the profile written over a summon's think clone: how far and
// how wide it looks for a target on its own, and how quickly it forgets one.
//
// A summon reaches a target by two paths - its own perception, and the
// player's shared target (BuddyParam.disablePCTargetShare 0). FromSoftware's
// own Spirit Ash think rows are near-blind by design - eye_dist and
// BattleStartDist 2 on 87 of the 98 its formations run - so the shared target
// does the long-range work and the player chooses the fight. A clone that
// keeps its WORLD think numbers (a 60-degree cone at 13-20 m, and on a boss
// row far more) is more alert than any shipped Ash and crosses the map on its
// own.
type Perception string

const (
	// THE LADDER (docs/DESIGN.md, the ladder). Six levels the
	// product offers, each a ROLE. A level owns its REACH and its
	// cone; everything that makes a clone a companion is shared (savecore's
	// summonPackage). Reach is a constant BEYOND THE BODY, capped at the leash
	// ceiling, and BattleStartDist equals it - so what a summon sees it fights,
	// closing in its own battle script's way rather than by a rule we invented.
	//
	// PerceptionFollower is the zero value and the default, because it is what
	// FromSoftware ships: near-blind, fighting what the player fights.
	PerceptionFollower Perception = ""
	// PerceptionStatue reacts to nothing at all. Not blindness - a blind actor
	// still fights back when it is hit - but battleGoalID and logicId 11000,
	// FromSoftware's own Nanimosinai, "does nothing".
	//
	// It needs the hold stance: on follow its idle branch walks after the
	// player and warps. It is handed no target - StanceStill, share off, and a
	// think clone that hears nothing - because its head follows whatever
	// target it holds. It draws no weapon (the draw is the logic script's), and
	// stands with its weapon out (WeaponAlwaysOut), because a hit still draws
	// it through the animation layer.
	PerceptionStatue Perception = "statue"
	// PerceptionGuard scans nearby: twelve metres beyond the body, because a
	// following summon trails the player, so an enemy ahead of the player is
	// further from it than from him.
	PerceptionGuard Perception = "guard"
	// PerceptionSentinel watches far and ahead: thirty metres past the front of
	// the body, on a narrower cone, so it can spot an enemy before the player
	// does. Past 23 + 5 m from the host a leashed summon notices and guards
	// rather than charges, so its last metres buy warning.
	PerceptionSentinel Perception = "sentinel"
	// PerceptionHunter is the independent one: fifty metres beyond the body on
	// a full 180-degree cone, a twenty-five metre nose, and NO LEASH. It hunts
	// on its own. It needs pcFollowType 1 (RequiredStance), because the leash
	// is not a think-row field at all.
	//
	// The engine decides both the leash and the idle behaviour with one
	// predicate, written twice in the game's own AI code
	// (aicommon/common_logic_func.dec.lua's _COMMON_SetBuddyBattleActLogic and
	// non_battle_act.dec.lua's NonBattleAct_Activate):
	//
	//	pcFollowType == FOLLOW and not 3381 and not 10535
	//
	// True: a summon with an enemy is recalled to the player at a literal 23 m,
	// and follows him when idle. False: it fights what it sees wherever that
	// is, and roams when idle. There is no third position: 67 of the game's 70
	// logic scripts route through it, and the other three, the white-phantom
	// summon's, leash harder.
	//
	// 3381 and 10535 (PLAN_SP_EFFECT_BUDDY_AGGRESSIVE and its demihuman twin,
	// the Cursed-Blood Pot's effect) are not used: pcFollowType 1 sets the
	// predicate false on its own, and no NpcParam row carries either as a
	// resident. pcFollowType 1 rather than 0 because the field is
	// PC追従＆ワープタイプ, "PC follow AND WARP type": on 0 the engine warps the
	// summon to the player at 33 m with the line of sight broken.
	//
	// Share is off by default: a shared target reaches the Hunter only about as
	// far as the leash, so it hunts on its own, on FromSoftware's own 1/1 with
	// the damage rate 100 that goes with it. A player may turn share on
	// (Share); the damage rate is then 0, so a stray hit does not drag it off
	// the host's target.
	PerceptionHunter Perception = "hunter"
	// PerceptionLoner keeps the creature's own senses - eyes, cone, nose and
	// the back-offset pair - and is the configuration an ENEMY gets. It still
	// takes the home band, because a world-branch clone without it walks off
	// toward a home it never had.
	//
	// It needs StanceIndependent, FromSoftware's own independent-Ash pair:
	// share off and wander, so no leash, and it does not come to the player's
	// fight. It keeps a fight at no less than 20 m beyond the body, so a
	// short-sighted creature does not lose what it is fighting (docs/DESIGN.md, the ladder).
	PerceptionLoner Perception = "loner"

	// The research levels. the author's console test tool seats them to ask what the ladder
	// does not; the product never offers them.
	//
	// PerceptionEyes keeps the world row's cone, nullifies the nose and sees
	// 20 m beyond the body. It is the console test tool's default for a seat that names no
	// level.
	PerceptionEyes Perception = "eyes"
	// PerceptionBlind sees nothing on its own: a seat that must stand still to
	// be looked at.
	PerceptionBlind Perception = "blind"
	// PerceptionWorld leaves the world row's perception as it is: an enemy seat.
	PerceptionWorld Perception = "world"
)

// PerceptionAsHeld marks a slot re-staged as the save already holds it. It is
// not a level: it never reaches a clone (plan refuses it there), and the slot's
// own stance stands.
const PerceptionAsHeld Perception = "as-held"

// Ladder is the levels the product offers, the default first.
var Ladder = []Perception{
	PerceptionFollower, PerceptionStatue, PerceptionGuard, PerceptionSentinel,
	PerceptionHunter, PerceptionLoner,
}

// Perceptions is every level a clone can be written with: the ladder and the
// research levels.
var Perceptions = append(slices.Clone(Ladder), PerceptionEyes, PerceptionBlind, PerceptionWorld)

// MeasuredFromTheBody reports whether a level's reach is a constant BEYOND the
// actor's collision radius, so the manifest has to carry that radius. The
// Loner keeps its own reach, but the distance it KEEPS a fight at is floored at
// 20 m beyond the body.
func MeasuredFromTheBody(p Perception) bool {
	switch p {
	case PerceptionFollower, PerceptionGuard, PerceptionSentinel, PerceptionHunter, PerceptionLoner, PerceptionEyes:
		return true
	}
	return false
}

// RequiredStance is the stance a level cannot be itself without, with its
// default share, and whether it has one. Three levels do.
//
// Perception is a think-row profile and stance is a pair of BuddyParam fields,
// and for the three middle levels the FOLLOW TYPE is independent - a Follower
// holds or wanders and is still a Follower. Three are not, because what makes
// them themselves is a stance field:
//
//	Statue  still, share off   its idle branch waits; on follow it would walk
//	        (1/2)              after the player and warp; and
//	                           share would hand it the player's target, which
//	                           its head follows
//	Hunter  wander             the leash is pcFollowType; share is the player's
//	        (1/1 by default)   choice, off by default
//	Loner   share off, wander  FromSoftware's own independent-Ash pair, on all
//	        (1/1)              28 of its rows: no leash, no share
//
// So each is coupled here and Formation.Validate enforces the follow type; the
// SHARE half of every ladder level's stance is the actor's Share (LevelShare,
// LevelStance), enforced beside it.
func RequiredStance(p Perception) (Stance, bool) {
	switch p {
	case PerceptionStatue:
		return StanceStill, true
	case PerceptionHunter:
		return StanceIndependent, true
	case PerceptionLoner:
		return StanceIndependent, true
	}
	return Stance{}, false
}

// FollowerTargetPriority is "[Spirit Summon] -50% Target Priority": enemies
// prefer the player over a summon carrying it. FromSoftware puts it on 77 of
// its 115 summon actors, and it is what protects its near-blind Ash - the Ash
// is rarely left in a fight the player is not in, the one case its two-metre
// eyes cannot keep.
const FollowerTargetPriority = int32(296010)

// LevelResidents are the resident effects a level adds to its actor's clone.
// Only the Follower has one: it is an assistant, and the Guard and the
// Sentinel are fighters that must draw fights, not shed them.
func LevelResidents(p Perception) []int32 {
	if p == PerceptionFollower {
		return []int32{FollowerTargetPriority}
	}
	return nil
}

// WeaponAlwaysOut reports whether a level's actor is written with its weapon
// out: the sheathed-state effect its world think row names
// (weaponOffSpecialEffectId) is taken off the clone's residents. Only the
// Statue: with the logic script's toggle cleared a hit still draws its weapon,
// and the draw is the animation layer's, which no think field reaches. A
// creature that is never sheathed has nothing to draw.
func WeaponAlwaysOut(p Perception) bool {
	return p == PerceptionStatue
}

// Valid reports whether the perception is one this product knows.
func (p Perception) Valid() bool {
	for _, known := range Perceptions {
		if p == known {
			return true
		}
	}
	return false
}

// Name is the perception as a manifest spells it: "follower" for the zero value.
func (p Perception) Name() string {
	if p == PerceptionFollower {
		return "follower"
	}
	return string(p)
}

// Stance is whether a summoned actor follows the player or holds the ground it
// was called on, as the pair of BuddyParam fields the game spells it with.
//
// It is a per-slot value because the fields are per row: a formation is a run
// of BuddyParam rows, one per actor, and each carries its own. The game never
// uses that - all 170 shipped rows give a block one stance - but the save
// format allows it, and a test that has to hold the actor constant while the
// stance varies needs exactly that freedom.
//
// # What the values are
//
// Seen on the console, and matching the community Spirit Battler tool's
// own comment:
//
//	0  follows the player, running to catch up when you get far ahead
//	1  wanders: walks off in a random direction, stops, waits, turns, walks
//	   again. It does NOT follow, and it is not a slow follow
//	2  holds the ground it was called on, and does NOT warp to you
//
// All three fight an enemy that comes near, so none of them is inert.
//
// The field's own name says as much: FromSoftware calls it PC追従＆ワープタイプ,
// "PC follow and warp type". The pinned Paramdex declares Maximum 1, which is
// wrong - Latenna and Mimic Tear ship 2.
//
// disablePCTargetShare is 「PCとのターゲット共有をスキップするか」, "whether to
// skip target sharing with the PC". Set it and the summon stops inheriting the
// player's target and falls back on its own perception, which on a summon-side
// think row is deliberately tiny: FromSoftware ships eye_dist 2 and
// BattleStartDist 2 on them, against 13 to 20 on the world rows they came from.
// The result is a summon that will not engage until an enemy is a step away.
// It reads like a detection setting and it is not one: detection lives in
// NpcThinkParam.
type Stance struct {
	DisablePCTargetShare int
	PcFollowType         int
}

var (
	// StanceFollow is the summon that comes with you: what 140 of the 170
	// shipped rows carry, and what the product writes.
	StanceFollow = Stance{DisablePCTargetShare: 0, PcFollowType: 0}
	// StanceWander is the summon that goes where it likes: it does not follow,
	// and it is not a slow follow. The pair is not a shipped value - no
	// shipped row pairs share ON with wander (of all 170: 140 at 0/0, 28 at
	// 1/1, two at 0/2) - and only the author's console test tool seats it.
	StanceWander = Stance{DisablePCTargetShare: 0, PcFollowType: 1}
	// StanceHold is the summon that stays where it was called. It is Latenna's
	// shipped value, and her formation is the one seen standing still.
	StanceHold = Stance{DisablePCTargetShare: 0, PcFollowType: 2}
	// StanceIndependent is FromSoftware's independent Ash: its own senses, the
	// player's target NOT handed to it, and wandering rather than following -
	// the pair all 28 of its share-off buddy rows carry (Putrid Corpse,
	// Marionette Soldier, the Giant Rats, Floh, Ogha, Meera...). It is the
	// Hunter's and the Loner's required stance.
	StanceIndependent = Stance{DisablePCTargetShare: 1, PcFollowType: 1}
	// StanceStill is hold without the player's target: the Statue's required
	// stance, because share hands a summon the player's target and a Statue is
	// handed nothing. No shipped row carries the pair: the two that hold,
	// Latenna and Mimic Tear, both share.
	StanceStill = Stance{DisablePCTargetShare: 1, PcFollowType: 2}
)

// Valid bounds a stance to the values 1.17 actually holds. Anything else is a
// save this product did not write and nobody has measured.
func (stance Stance) Valid() bool {
	return stance.DisablePCTargetShare >= 0 && stance.DisablePCTargetShare <= 1 &&
		stance.PcFollowType >= 0 && stance.PcFollowType <= 2
}

// Slot is one member of a formation. It is a struct rather than a bare Actor
// because per-slot placement — x_offset, z_offset, y_angle — is the next thing
// that lands here, and callers should not have to change shape for it.
type Slot struct {
	Actor Actor
	// Stance is how this actor moves. The zero value follows, which is both the
	// shipped majority and what a caller that has not thought about it means.
	Stance Stance
	// Placement is where this actor stands relative to the player. Nil means
	// the caller has no opinion, and the compiler lays the formation out in a
	// ring - which is the point of the field existing at all, because the
	// alternative it replaces was every actor on one spot.
	Placement *Placement
}

// Placement is where one actor stands and which way it faces, from the player:
// metres to the right, metres ahead, degrees turned right (seen on the console).
type Placement struct {
	X, Z, Angle float64
}

// Ahead is how much further along the player's facing a summon stands than its
// row's z_offset says: about 3.2 m, whatever its body, facing or arrival
// (seen on the console). x and
// the facing are as the row holds them.
const Ahead = 3.2

// RowZ is the z_offset a row holds for its actor to stand at p.
func (p Placement) RowZ() float64 { return round2(p.Z - Ahead) }

// PlacementOfRow is where a row's actor stands, from its x_offset, z_offset
// and y_angle.
func PlacementOfRow(x, z, angle float32) Placement {
	return Placement{X: round2(float64(x)), Z: round2(float64(z) + Ahead), Angle: round2(float64(angle))}
}

// RingRadius is where the shipped four-actor formations sit around the point
// they stand about, and so where a formation with no stated layout is put.
const RingRadius = 1.3

// RingPlacement lays slot index out of count out evenly in the shipped ring,
// Ahead of the player.
//
// A default is required: placement is always written, and without one every
// inserted row would stand on its template's spot.
func RingPlacement(index, count int) Placement {
	if count < 1 {
		count = 1
	}
	turn := 2 * math.Pi * float64(index) / float64(count)
	return Placement{
		X: round2(RingRadius * math.Sin(turn)),
		Z: round2(Ahead + RingRadius*math.Cos(turn)),
	}
}

// round2 keeps the written values in the shape the shipped rows use - two
// decimal places - so a layout reads like the game's own rather than like a
// float.
func round2(value float64) float64 {
	rounded := math.Round(value*100) / 100
	if rounded == 0 {
		// math.Round keeps the sign, and a negative zero reads as -0 in every
		// report this value reaches.
		return 0
	}
	return rounded
}

// Formation is what the player authored: an ordered list of 1..MaxSlots slots.
// The order is the row order of the block it becomes.
type Formation struct {
	Name  string
	Slots []Slot
}

// BlockRow is one shipped BuddyParam row of the host's formation, in the
// physical order the table holds it.
type BlockRow struct {
	RowID int32
	// Doping is the row's dopingSpEffect_lv0..lv10 table as the save holds it.
	// -1 is a gap.
	Doping [DopingLevels]int32
}

// Block is the host Ash's shipped formation, read from the save.
//
// Authoring edits this block in place rather than building a new one somewhere
// else, because the goods rows already resolve it: every upgrade level of an
// Ash family resolves the formation owned by its +0 trigger, so one BuddyParam
// edit reaches all eleven levels while a goods edit would have to be repeated
// on all eleven rows. Reusing the shipped trigger also means no SpEffectParam
// row has to be invented for the manager to find.
//
// Giant Rat Ashes is the one family whose levels reach three blocks: 233000
// owns +0..+4, 233100 +5..+9 and 233200 +10, and each block's rows carry
// reinforcement only at its own levels. A formation for it is compiled into
// each of the three, with Levels saying which levels reach which.
type Block struct {
	// Trigger is the triggerSpEffectId every row in the block carries.
	Trigger int32
	// Levels are the upgrade levels whose goods rows reach this block; nil is
	// all of them, as for every family but the Giant Rat.
	Levels []int
	// Rows are the block's rows in physical order. Row order is slot order.
	Rows []BlockRow
	// Inert are the rows directly after Rows that the block itself retired
	// (savecore.FormationBlock.Inert says how they are recognised), in order.
	// A formation larger than Rows re-fills them before it inserts anything:
	// they are the block's own rows, and they sit exactly where an insertion
	// would have to go, so without them a formation shrunk and saved could
	// never grow again.
	Inert []BlockRow
}

// Save is what the compiler needs to know about the regulation it will edit.
// Everything here is read from the save; nothing is assumed about it.
type Save struct {
	BuddyRows    map[int32]bool
	NpcRows      map[int32]bool
	NpcThinkRows map[int32]bool
	// Allocator decides which clone key a creature's recipe is written under
	// (see Allocator). Nil is the compiler's own: the first recipe of a
	// creature at its band-1 clone, reused if the save holds one, every other
	// recipe in the first free band.
	Allocator Allocator
}

// Table names one of the two actor tables a clone is inserted into.
type Table uint8

const (
	NpcTable   Table = 1
	ThinkTable Table = 2
)

// Allocator places a creature's recipe: the clone key it is written under, and
// whether this formation must insert that clone (false when the save already
// holds a clone written that way). A session that stages several formations
// supplies one allocator to all of them, so one recipe gets one key however
// many formations name it.
type Allocator interface {
	Place(table Table, source int32, recipe string) (key int32, insert bool, err error)
}

// ErrNoCloneBand means a creature carries more recipes than there are clone
// bands (CloneBands) to hold them.
var ErrNoCloneBand = errors.New("every clone band of this creature is taken")

// NpcRecipe is everything that decides how an actor's NpcParam clone is
// written and can differ between two slots of one creature: its team and
// branch, the residents its level adds, the weapon-out the Statue asks for
// (read off the think row it names), its presence and marker, and whether it
// is a Fighter. Two slots with one recipe share a clone; two with different
// recipes get one each.
func NpcRecipe(actor Actor) string {
	weapon := int32(0)
	if WeaponAlwaysOut(actor.Perception) {
		weapon = actor.Think
	}
	return fmt.Sprintf("team=%d world=%t unheld=%t residents=%v weapon-out=%d presence=%d marker=%d fighter=%t",
		actor.Team, actor.WorldBranch, actor.WorldBranchUnheld, LevelResidents(actor.Perception), weapon,
		actor.Presence, actor.Marker, actor.Fighter)
}

// ThinkRecipe is everything that decides how an actor's think clone is written:
// the level, the share box's damage rate, the world branch's home band, and
// the body the reach is measured from. It spells out exactly what
// plan.BuildAuthor writes into the clone's perception entry.
func ThinkRecipe(actor Actor) string {
	ownTarget := false
	if _, choosable, _ := LevelShare(actor.Perception); choosable {
		on, _ := ResolveShare(actor)
		ownTarget = !on
	}
	body, front := 0.0, 0.0
	if MeasuredFromTheBody(actor.Perception) {
		body, front = actor.Body, actor.Front
	}
	return fmt.Sprintf("level=%s own-target=%t world=%t no-turn=%t body=%g front=%g",
		actor.Perception.Name(), ownTarget, actor.WorldBranch && !actor.WorldBranchUnheld, actor.NoTurn, body, front)
}

// bandAllocator is the compiler's own Allocator, for a caller that brings none
// (the product's own caller brings application.cloneAllocator). Every recipe
// takes the lowest band whose row the save does not hold, and nothing held is
// ever reused: this package has no bytes to tell this editor's own clone from
// FromSoftware's row (for Commander Gaius, band 1 is the Florissax Ash's AI).
// A held row is reused only where it can be proved to be the recipe, which is
// cloneAllocator's MatchClones and never this.
type bandAllocator struct {
	save   Save
	placed map[placement]placed
	taken  map[Table]map[int32]bool
}

type placement struct {
	table  Table
	source int32
	recipe string
}

type placed struct {
	key    int32
	insert bool
}

func newBandAllocator(save Save) *bandAllocator {
	return &bandAllocator{save: save, placed: map[placement]placed{},
		taken: map[Table]map[int32]bool{NpcTable: {}, ThinkTable: {}}}
}

func (allocator *bandAllocator) Place(table Table, source int32, recipe string) (int32, bool, error) {
	at := placement{table, source, recipe}
	if done, seen := allocator.placed[at]; seen {
		return done.key, done.insert, nil
	}
	rows := allocator.save.NpcRows
	if table == ThinkTable {
		rows = allocator.save.NpcThinkRows
	}
	for _, band := range CloneBands {
		key, err := CloneKey(source, band)
		if err != nil {
			return 0, false, err
		}
		if rows[key+CloneOffset] || allocator.taken[table][key] {
			continue
		}
		allocator.taken[table][key] = true
		done := placed{key: key, insert: true}
		allocator.placed[at] = done
		return done.key, done.insert, nil
	}
	return 0, false, fmt.Errorf("row %d: %w", source, ErrNoCloneBand)
}

// RowEdit re-points a shipped block row at a different actor. The row keeps its
// own reinforcement table, its placement and its ID.
type RowEdit struct {
	Slot   int
	RowID  int32
	Actor  Actor
	Stance Stance
	Place  Placement
}

// RowInsert adds a row to the block for a slot the shipped block has no row
// for. It is a byte copy of CopyFrom — which carries the reinforcement table
// the new row inherits — placed directly after InsertAfter and then re-pointed.
type RowInsert struct {
	Slot        int
	RowID       int32
	CopyFrom    int32
	InsertAfter int32
	Actor       Actor
	Stance      Stance
	Place       Placement
}

// CloneRow is one row to insert into NpcParam or NpcThinkParam.
//
// CopyFrom is the clone KEY (see CloneBands): the world row the clone copies
// is CopyFrom % CloneOffset, and the band is CopyFrom / CloneOffset + 1. RowID
// is always CopyFrom + CloneOffset. It is arithmetic rather than an argument so
// that no caller can name a destination, which is what keeps docs/SAFETY.md
// rule 1's model-namespace requirement true by construction: the last eight
// digits of the row are the source exactly, in every band, so a clone can
// never land in a different cXXXX model's range.
//
// It says where the row goes and what it starts as, and what it starts as is
// not yet a summon. Of the 94 shipped clones with a world twin, not one is a
// byte copy of it: every clone is on the Spirit Summon team and no world row
// anywhere in the table is, and the resident effects differ on 93. A copy alone
// would name an actor on an enemy team that drops loot.
//
// What is written on top of the copy is savecore's summon-side recipe. This
// package cannot express it - it is field-level work on a table this package
// knows no offsets for - which is the same division of labour Compile already
// has with every other row it decides.
type CloneRow struct {
	RowID    int32
	CopyFrom int32
}

// Clone is the NpcParam and NpcThinkParam insertions one world actor needs
// before it can be summoned. Either half may be absent, because a save can
// already hold one clone and not the other.
type Clone struct {
	Source Actor
	Npc    *CloneRow
	Think  *CloneRow
}

// CopyRow is one summon-side NpcParam row copied into the copy band, for a
// slot that puts a shipped shell on a team. RowID is SummonCopyRow(CopyFrom),
// arithmetic for the reason CloneRow's is; the copy is the deliberate
// exception to "a clone is a world row plus CloneOffset", and the only one.
// Source is the slot's actor, whose Team the copy carries.
type CopyRow struct {
	RowID    int32
	CopyFrom int32
	Source   Actor
}

// Compiled is the row work one authored formation becomes.
type Compiled struct {
	Trigger int32
	// Edits re-point block rows the formation reuses.
	Edits []RowEdit
	// Inserts extend the block for slots it has no row for.
	Inserts []RowInsert
	// Retired are block rows the formation does not need. They are removed
	// from the formation by writing triggerSpEffectId = -1, which is how the
	// game itself spells an inert row; the row is not deleted, so a reset
	// restores it with an ordinary patch. The application writes a retired row
	// back naming the actor the game ships for it, so it keeps no unused
	// creature row alive (application's clonealloc.go).
	Retired []int32
	// Clones are the actor rows that must exist before Edits and Inserts can
	// name them, ordered by source NpcParam ID.
	Clones []Clone
	// Copies are the summon-side rows copied into the copy band for the
	// slots that put a shipped shell on a team, ordered by source row.
	Copies []CopyRow
}

// Validate reports whether a formation is one the game can deliver, without
// reference to any particular host.
func (f Formation) Validate() error {
	if len(f.Slots) == 0 {
		return ErrEmptyFormation
	}
	if len(f.Slots) > MaxSlots {
		return fmt.Errorf("%d slots: %w", len(f.Slots), ErrTooManySlots)
	}
	for index, slot := range f.Slots {
		if _, err := SummonRow(slot.Actor.Npc); err != nil {
			return fmt.Errorf("slot %d npcParamId: %w", index+1, err)
		}
		if _, err := SummonRow(slot.Actor.Think); err != nil {
			return fmt.Errorf("slot %d npcThinkParamId: %w", index+1, err)
		}
		if !slot.Actor.Team.Valid() {
			return fmt.Errorf("slot %d team %d is not one of %v: %w", index+1, slot.Actor.Team, Teams, ErrUnknownTeam)
		}
		if slot.Actor.Team != TeamSummon && slot.Actor.Npc >= CloneOffset {
			if _, err := SummonCopyRow(slot.Actor.Npc); err != nil {
				return fmt.Errorf("slot %d team %d: %w", index+1, slot.Actor.Team, err)
			}
		}
		if want, coupled := RequiredStance(slot.Actor.Perception); coupled && slot.Stance.PcFollowType != want.PcFollowType {
			return fmt.Errorf("slot %d: perception %q needs pcFollowType %d and the slot carries %d: %w",
				index+1, slot.Actor.Perception.Name(), want.PcFollowType, slot.Stance.PcFollowType, ErrStanceMismatch)
		}
		if slot.Actor.Enemy() && !slices.Contains(EnemyLevels, slot.Actor.Perception) {
			if _, _, ladder := LevelShare(slot.Actor.Perception); ladder {
				return fmt.Errorf("slot %d: perception %q: %w", index+1, slot.Actor.Perception.Name(), ErrEnemyLevel)
			}
		}
		// The share half of the stance is the actor's choice on every ladder
		// level (LevelShare): the stance must carry what the choice resolves
		// to, or the BuddyParam row and the think clone's damage rate would
		// say two different things.
		if _, _, ladder := LevelShare(slot.Actor.Perception); ladder {
			on, err := ResolveShare(slot.Actor)
			if err != nil {
				return fmt.Errorf("slot %d: %w", index+1, err)
			}
			if want := !on; (slot.Stance.DisablePCTargetShare == 1) != want {
				return fmt.Errorf("slot %d: perception %q shares the player's target %t and the slot carries disablePCTargetShare %d: %w",
					index+1, slot.Actor.Perception.Name(), on, slot.Stance.DisablePCTargetShare, ErrStanceMismatch)
			}
		}
		if slot.Actor.Person > 0 && (slot.Actor.Presence != 0 || slot.Actor.Marker != 0) {
			return fmt.Errorf("slot %d: a person is summoned on its shell, which carries no presence or marker to replace: %w",
				index+1, ErrNotSummonable)
		}
		if slot.Actor.Marker != 0 && slot.Actor.WorldBranch {
			return fmt.Errorf("slot %d: the world branch carries no companion marker to replace: %w", index+1, ErrNotSummonable)
		}
		if slot.Actor.WorldBranch && slot.Actor.Npc >= CloneOffset {
			return fmt.Errorf("slot %d: the world branch needs a clone to leave 297000 off, and %d is summon-side already: %w",
				index+1, slot.Actor.Npc, ErrNotSummonable)
		}
		if slot.Actor.WorldBranchUnheld && !slot.Actor.WorldBranch {
			return fmt.Errorf("slot %d: unheld is a world-branch switch and the actor is not on the world branch: %w",
				index+1, ErrNotSummonable)
		}
	}
	return nil
}

// SummonCopyRow is the row a summon-side row is copied to when a slot needs
// it changed: the copy band's two arithmetic rules. A world row has a clone
// instead and is refused here.
func SummonCopyRow(id int32) (int32, error) {
	switch {
	case id >= CloneOffset && id < CloneOffset+summonCopyWindow:
		return summonCopyFirst + (id - CloneOffset), nil
	case id >= dlcShellFirst && id < dlcShellFirst+summonCopyWindow:
		return summonCopySplit + (id - dlcShellFirst), nil
	default:
		return 0, fmt.Errorf("%d is not a shipped shell (%d..%d or %d..%d), so the copy band has no row for it: %w",
			id, CloneOffset, CloneOffset+summonCopyWindow-1, dlcShellFirst, dlcShellFirst+summonCopyWindow-1, ErrNotSummonable)
	}
}

// The character bodies: a person shell's copy per character slot or built
// character, carrying the character's own health and active effects
// (internal/character BodyOf, BuiltBodyOf). They
// sit in band 1 but are no clone: the character write owns them, and the
// prune, the reset, a role chosen for a row and clone matching never see them.
const (
	characterBodyFloor   = 199900000
	characterBodyCeiling = 199900200
)

// IsCharacterBody reports whether row is a character's body.
func IsCharacterBody(row int32) bool {
	return row >= characterBodyFloor && row < characterBodyCeiling
}

// The CharaInitParam band of the player's own characters, a save slot's and a
// built one's (internal/character rows.go and build.go).
const (
	characterRowFloor   = 2990000
	characterRowCeiling = 2990200
)

// IsCharacter reports whether a person row is one of the player's characters.
func IsCharacter(person int32) bool {
	return person >= characterRowFloor && person < characterRowCeiling
}

// SummonRow is the row a formation must name to summon the given actor row.
//
// An ID already in the clone namespace is summon-side already and is returned
// unchanged: a vanilla Ash actor is one of these, which is why re-pointing one
// cannot disturb a world enemy. A world row is answered with its clone, which
// the caller must insert if the save does not already hold it.
func SummonRow(id int32) (int32, error) {
	switch {
	case id >= CloneOffset && id < cloneCeiling, InCloneBand(id):
		// A clone in a higher band is summon-side too: a save this product
		// wrote names one wherever a creature carries a second recipe.
		return id, nil
	case id > 0 && id < CloneOffset:
		return id + CloneOffset, nil
	default:
		return 0, fmt.Errorf("%d is not 1..%d or %d..%d: %w",
			id, CloneOffset-1, CloneOffset, cloneCeiling-1, ErrNotSummonable)
	}
}

// Compile turns an authored formation into the rows that deliver it from the
// given host block.
//
// It is deterministic: the same formation, block and save always produce the
// same lists in the same order, so two runs can be diffed against each other.
func Compile(f Formation, block Block, save Save) (*Compiled, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if err := validateBlock(block, save); err != nil {
		return nil, err
	}
	if err := oneTeamPerShell(f); err != nil {
		return nil, err
	}

	compiled := &Compiled{Trigger: block.Trigger}
	allocator := save.Allocator
	if allocator == nil {
		allocator = newBandAllocator(save)
	}
	clones := cloneSet{}

	// The template is the block's last row: inserts are byte copies of it, so
	// a new slot inherits a reinforcement table that validateBlock has already
	// proved complete. They anchor after the last row the formation fills,
	// which is the template or the last inert row it re-filled.
	template := block.Rows[len(block.Rows)-1]
	anchor := template.RowID

	for index, slot := range f.Slots {
		actor, err := clones.place(slot.Actor, allocator)
		if err != nil {
			return nil, fmt.Errorf("slot %d: %w", index+1, err)
		}
		place := RingPlacement(index, len(f.Slots))
		if slot.Placement != nil {
			place = *slot.Placement
		}
		if index < len(block.Rows) {
			compiled.Edits = append(compiled.Edits, RowEdit{
				Slot: index + 1, RowID: block.Rows[index].RowID, Actor: actor,
				Stance: slot.Stance, Place: place,
			})
			continue
		}
		// A row the block retired is re-filled where it stands: an edit, so
		// the row is written whole with the block's trigger and keeps its own
		// reinforcement table, which validateBlock has proved complete.
		if reserve := index - len(block.Rows); reserve < len(block.Inert) {
			row := block.Inert[reserve].RowID
			compiled.Edits = append(compiled.Edits, RowEdit{
				Slot: index + 1, RowID: row, Actor: actor, Stance: slot.Stance, Place: place,
			})
			anchor = row
			continue
		}
		nextRowID := anchor + 1
		if save.BuddyRows[nextRowID] {
			return nil, fmt.Errorf("extending formation %d past row %d would collide with row %d: %w",
				block.Trigger, anchor, nextRowID, ErrNoFreeRow)
		}
		compiled.Inserts = append(compiled.Inserts, RowInsert{
			Slot: index + 1, RowID: nextRowID, CopyFrom: template.RowID,
			InsertAfter: anchor, Actor: actor, Stance: slot.Stance, Place: place,
		})
		// Each insertion anchors on the one before it, so the block stays one
		// contiguous run of headers: the convention every native multi-summon
		// uses. Whether the engine strictly requires it is not proved.
		anchor = nextRowID
	}

	for _, row := range block.Rows[min(len(f.Slots), len(block.Rows)):] {
		compiled.Retired = append(compiled.Retired, row.RowID)
	}
	compiled.Clones = clones.list()
	compiled.Copies = copiesFor(f, save)
	return compiled, nil
}

// cloneSet gathers the clones a formation's slots are placed on and names each
// one to insert exactly once.
type cloneSet struct {
	clones []Clone
	seen   map[Table]map[int32]bool
}

// place is the slot's actor as the formation row names it - its two rows moved
// to the summon side, each onto the clone key its recipe was placed under - and
// records the clones that must be inserted for it. A creature has a clone per
// recipe (CloneBands), so every slot gets what it asked for.
func (set *cloneSet) place(actor Actor, allocator Allocator) (Actor, error) {
	summon := actor
	var clone Clone
	clone.Source = actor
	if actor.Npc < CloneOffset {
		key, insert, err := allocator.Place(NpcTable, actor.Npc, NpcRecipe(actor))
		if err != nil {
			return Actor{}, err
		}
		summon.Npc = key + CloneOffset
		if insert && set.claim(NpcTable, key) {
			clone.Npc = &CloneRow{RowID: key + CloneOffset, CopyFrom: key}
		}
	} else {
		npc, err := SummonRow(actor.Npc)
		if err != nil {
			return Actor{}, err
		}
		summon.Npc = npc
		if actor.Team != TeamSummon {
			// A team on a shipped summon-side row goes on a copy of it.
			if summon.Npc, err = SummonCopyRow(actor.Npc); err != nil {
				return Actor{}, err
			}
		}
	}
	if actor.Think < CloneOffset {
		key, insert, err := allocator.Place(ThinkTable, actor.Think, ThinkRecipe(actor))
		if err != nil {
			return Actor{}, err
		}
		summon.Think = key + CloneOffset
		if insert && set.claim(ThinkTable, key) {
			clone.Think = &CloneRow{RowID: key + CloneOffset, CopyFrom: key}
		}
	} else {
		think, err := SummonRow(actor.Think)
		if err != nil {
			return Actor{}, err
		}
		summon.Think = think
	}
	if clone.Npc != nil || clone.Think != nil {
		set.clones = append(set.clones, clone)
	}
	return summon, nil
}

func (set *cloneSet) claim(table Table, key int32) bool {
	if set.seen == nil {
		set.seen = map[Table]map[int32]bool{NpcTable: {}, ThinkTable: {}}
	}
	if set.seen[table][key] {
		return false
	}
	set.seen[table][key] = true
	return true
}

// list is the clones to insert, ordered by the key of their NpcParam half (or
// their think half where they have none), so the result is stable.
func (set *cloneSet) list() []Clone {
	order := func(clone Clone) int32 {
		if clone.Npc != nil {
			return clone.Npc.CopyFrom
		}
		return clone.Think.CopyFrom
	}
	clones := slices.Clone(set.clones)
	slices.SortStableFunc(clones, func(a, b Clone) int { return int(order(a)) - int(order(b)) })
	return clones
}

// copiesFor lists the summon-side rows the formation's teams need copied:
// one per shell, for the slots that put a shipped shell on a team. Ordered
// by source row.
func copiesFor(f Formation, save Save) []CopyRow {
	byNpc := map[int32]Actor{}
	for _, slot := range f.Slots {
		if slot.Actor.Team == TeamSummon || slot.Actor.Npc < CloneOffset {
			continue
		}
		if _, seen := byNpc[slot.Actor.Npc]; !seen {
			byNpc[slot.Actor.Npc] = slot.Actor
		}
	}
	sources := make([]int32, 0, len(byNpc))
	for npc := range byNpc {
		sources = append(sources, npc)
	}
	slices.Sort(sources)
	var copies []CopyRow
	for _, npc := range sources {
		actor := byNpc[npc]
		// Validate has already proved the row has a copy.
		destination, _ := SummonCopyRow(npc)
		if save.NpcRows[destination] {
			continue
		}
		copies = append(copies, CopyRow{RowID: destination, CopyFrom: npc, Source: actor})
	}
	return copies
}

// oneTeamPerShell refuses a formation whose slots put one shipped shell on two
// teams: a shell's copy is one row in the copy band, and it carries one team. A
// world creature on two teams is two clones (CloneBands) and is not refused.
func oneTeamPerShell(f Formation) error {
	teams := map[int32]Team{}
	for index, slot := range f.Slots {
		if slot.Actor.Npc < CloneOffset {
			continue
		}
		if team, seen := teams[slot.Actor.Npc]; seen && team != slot.Actor.Team {
			return fmt.Errorf("slot %d puts shell %d on team %d, an earlier slot on team %d: %w",
				index+1, slot.Actor.Npc, slot.Actor.Team, team, ErrTwoTeams)
		}
		teams[slot.Actor.Npc] = slot.Actor.Team
	}
	return nil
}

// reached is the levels that reach the block: Levels, or all of them.
func (block Block) reached() []int {
	if block.Levels != nil {
		return block.Levels
	}
	all := make([]int, DopingLevels)
	for level := range all {
		all[level] = level
	}
	return all
}

func validateBlock(block Block, save Save) error {
	if block.Trigger <= 0 {
		return fmt.Errorf("trigger %d is not a positive SpEffect: %w", block.Trigger, ErrUnusableBlock)
	}
	if len(block.Rows) == 0 {
		return fmt.Errorf("trigger %d owns no rows: %w", block.Trigger, ErrUnusableBlock)
	}
	seen := make(map[int32]bool, len(block.Rows)+len(block.Inert))
	previous := block.Rows[len(block.Rows)-1].RowID
	for _, row := range block.Inert {
		// The inert rows are a run: each directly after the one before it,
		// starting directly after the block. Anything else is not a row this
		// block retired, and re-filling it could write another block's row.
		if row.RowID != previous+1 {
			return fmt.Errorf("formation %d's inert row %d does not follow row %d: %w",
				block.Trigger, row.RowID, previous, ErrUnusableBlock)
		}
		previous = row.RowID
	}
	for _, row := range slices.Concat(block.Rows, block.Inert) {
		if seen[row.RowID] {
			return fmt.Errorf("row %d appears twice in formation %d: %w", row.RowID, block.Trigger, ErrUnusableBlock)
		}
		seen[row.RowID] = true
		if !save.BuddyRows[row.RowID] {
			return fmt.Errorf("formation %d names row %d, which the save does not hold: %w",
				block.Trigger, row.RowID, ErrUnusableBlock)
		}
		// Every row needs every level that reaches the block, not the
		// formation as a whole. The table is indexed by the player's own item
		// level and an inserted row inherits whatever its source had, so a gap
		// at a level that reaches the block is a summon with no reinforcement.
		// A Giant Rat block's gaps are the levels another block owns.
		for _, level := range block.reached() {
			if row.Doping[level] <= 0 {
				return fmt.Errorf("formation %d row %d has no reinforcement at +%d: %w",
					block.Trigger, row.RowID, level, ErrIncompleteReinforcement)
			}
		}
	}
	return nil
}
