// Package plan builds the manifest that the product-owned save core applies.
//
// This project deliberately implements no save cryptography, DCX, BND4 or PARAM
// handling. The focused savecore package owns that tested boundary. Here we
// decide *what* to change; it decides how to write it.
package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// Manifest is the JSON shape the focused save core consumes. Every list is
// optional and an absent field writes nothing, so an older manifest replays
// byte for byte. A conversion omits Operation, and the save core reads a
// missing operation as a conversion.
type Manifest struct {
	SourceSHA256 string `json:"source_sha256"`
	// Operation is empty for a conversion and "reset" for a reset.
	Operation string `json:"operation,omitempty"`
	// Parts is set only on a session manifest, and then it is the whole of it:
	// one commit carrying several operations, each still a strict
	// single-operation manifest. A manifest with parts has no patches and no
	// structural work of its own. See Compose.
	Parts        []*Manifest `json:"parts,omitempty"`
	BuddyPatches []RowPatch  `json:"buddy_patches,omitempty"`
	// NpcPatches writes values into NpcParam rows that already exist. It is
	// how a role reaches FromSoftware's own summon rows - a Fighter's values,
	// or the recorded shipped ones back - and it is the only thing in this
	// product that writes an actor row rather than inserting or removing one.
	NpcPatches        []RowPatch   `json:"npc_patches,omitempty"`
	BuddyStonePatches []TablePatch `json:"buddy_stone_patches,omitempty"`
	// VfxPatches is the colour switch and nothing else: one field of
	// SpEffectVfxParam row 57000. See tint.go.
	VfxPatches []RowPatch `json:"vfx_patches,omitempty"`
	// EffectPatches are shared effect writes: fields of shipped SpEffectParam
	// rows an entry needs while an Ash holds it. See sharedeffects.go.
	EffectPatches []RowPatch `json:"effect_patches,omitempty"`
	// PersonPatches are person writes: fields of shipped CharaInitParam rows
	// an armed person needs while an Ash holds it. See sharedeffects.go.
	PersonPatches []RowPatch `json:"person_patches,omitempty"`
	// BuddyStoneRowPatches carries shipped values per row: every field for a
	// reset, the fields of whichever half of the monument switch is off for
	// the switch. The recipe writes the table uniformly, so undoing it needs a
	// per-row record it cannot itself produce.
	BuddyStoneRowPatches []RowPatch `json:"buddy_stone_row_patches,omitempty"`
	GoodsPatches         []RowPatch `json:"goods_patches,omitempty"`

	// The structural half: which rows exist, rather than what is inside one.
	// Authoring may only add and a reset may only remove, which the save core
	// re-checks rather than trusting.
	//
	// An actor clone is named by its world SOURCE row. The clone's own ID is
	// never written here because it is source + 100000000, derived where the
	// row is written; a destination that cannot be named cannot be named
	// wrongly.
	NpcCloneSources      []int32         `json:"npc_clone_sources,omitempty"`
	NpcThinkCloneSources []int32         `json:"npc_think_clone_sources,omitempty"`
	NpcCloneStrength     []CloneStrength `json:"npc_clone_strength,omitempty"`
	// NpcCloneWorldBranch names the clones written without companion
	// movement (297000), so the actor's logic script takes the world branch:
	// no following, no warping, no leash, no target fed by the player. What
	// an enemy is, and what a Loner is. Every source must be in
	// NpcCloneSources.
	NpcCloneWorldBranch []int32 `json:"npc_clone_world_branch,omitempty"`
	// NpcCloneWorldBranchUnheld names the world-branch clones written as the
	// engine leaves them - without the 5011 resident the branch adds - and
	// whose think clones get no home band. Research only.
	NpcCloneWorldBranchUnheld []int32 `json:"npc_clone_world_branch_unheld,omitempty"`
	// NpcCloneWeaponOut names the clones written with the weapon out: the
	// sheathed-state effect the named world think row carries
	// (weaponOffSpecialEffectId) is taken off the clone's residents, so
	// nothing is left to draw (formation.WeaponAlwaysOut - the Statue). Every
	// source must be in NpcCloneSources.
	NpcCloneWeaponOut []CloneWeaponOut `json:"npc_clone_weapon_out,omitempty"`
	// NpcSummonCopies are shipped summon-side rows (the person shells) copied
	// into the copy band for a slot that puts a shell on a team; the copy's
	// ID is formation.SummonCopyRow of the source, derived where it is
	// written like a clone's. NpcCloneTeams puts a clone or a copy on a team
	// other than Spirit Summon. A clone neither list names stays on Spirit
	// Summon.
	NpcSummonCopies []int32     `json:"npc_summon_copies,omitempty"`
	NpcCloneTeams   []CloneTeam `json:"npc_clone_teams,omitempty"`
	// NpcThinkPerception names the perception profile written over each think
	// clone (formation.Perception). A think clone this list does not name
	// keeps its world row's perception and gets the buddy flag alone.
	NpcThinkPerception []ThinkPerception   `json:"npc_think_perception,omitempty"`
	BuddyRowInsertions []BuddyRowInsertion `json:"buddy_row_insertions,omitempty"`

	// Removals name rows that exist, so these are clone IDs rather than
	// sources.
	BuddyRowRemovals      []int32 `json:"buddy_row_removals,omitempty"`
	NpcCloneRemovals      []int32 `json:"npc_clone_removals,omitempty"`
	NpcThinkCloneRemovals []int32 `json:"npc_think_clone_removals,omitempty"`

	// CharacterRows and CharacterRowRemovals are the player's own characters,
	// summoned as people: rows of the character band written whole, and the
	// ones nothing names any more. See characters.go.
	CharacterRows        []CharacterRow    `json:"character_rows,omitempty"`
	CharacterBodies      []CharacterBody   `json:"character_bodies,omitempty"`
	CharacterRowRemovals []CharacterRowKey `json:"character_row_removals,omitempty"`

	// OwnEffectInsertions and OwnEffectRemovals are the effect rows of our own
	// a clone's presence or marker names (owneffects.go), by id.
	OwnEffectInsertions []int32 `json:"own_effect_insertions,omitempty"`
	OwnEffectRemovals   []int32 `json:"own_effect_removals,omitempty"`
	// StoneInsertions and StoneRemovals are the sleeping monuments, by
	// entity: the wide position of the monument switch writes them afresh and
	// the other position, like a reset, takes them out. The save core holds
	// what each copies and writes.
	StoneInsertions []int32 `json:"stone_insertions,omitempty"`
	StoneRemovals   []int32 `json:"stone_removals,omitempty"`
}

// ThinkPerception is the perception profile one think clone is written with,
// by level name (formation.Perceptions).
type ThinkPerception struct {
	Source int32  `json:"source"`
	Level  string `json:"level"`
	// Body is the actor's collision radius in metres, recorded so a reach
	// measured beyond the body replays from the manifest alone.
	Body float64 `json:"body,omitempty"`
	// Front is how far the actor reaches in front of its centre - the larger
	// of Body and where its head ends (formation.Actor.Front). The ladder
	// counts its reach past it; absent, it counts from Body.
	Front float64 `json:"front,omitempty"`
	// WorldBranch is set for the think clone of an actor written without
	// 297000: the write puts the shipped Ash home band on it, or the actor
	// walks home.
	WorldBranch bool `json:"world_branch,omitempty"`
	// NoTurn: the actor's battle script has no Turn action, so the three
	// middle levels give it a nose (formation.Actor.NoTurn).
	NoTurn bool `json:"no_turn,omitempty"`
	// OwnTarget is the share box's think half (formation.Share): a Follower,
	// Guard, Sentinel or Hunter that does not take the player's target is
	// written targetSys_DmgEffectRate 100, so it fights whoever hits it -
	// FromSoftware's share-off independents carry 100 on 8 of 11 rows.
	OwnTarget bool `json:"own_target,omitempty"`
}

// CloneTeam puts one clone, or one copy of a shell, on a team other than the
// Spirit Summon team: hostile to the player as well as to the world. The
// values are formation.Teams.
type CloneTeam struct {
	Source int32 `json:"source"`
	Team   uint8 `json:"team"`
}

// CloneWeaponOut is one clone written with its weapon out: Source is the
// NpcParam source, Think the world think row whose sheathed-state effect is
// taken off the clone's residents.
type CloneWeaponOut struct {
	Source int32 `json:"source"`
	Think  int32 `json:"think"`
}

// CloneStrength is what one clone carries for its role: the health it is
// written with, the residents a grant adds, and - for a Fighter, a clone named
// here without KeepBalance - the Spirit Summon balance package left off.
//
// See savecore, which applies it, and docs/DESIGN.md (Summon strength).
type CloneStrength struct {
	Source int32 `json:"source"`
	HP     int32 `json:"hp,omitempty"`
	// Residents are added to the clone's own resident-effect vector: a
	// level's own and the ones the map grants the creature.
	Residents []int32 `json:"residents,omitempty"`
	// Removes are resident effects the source row carries that the clone must
	// not: a composite that takes one off (the God-Devouring Serpent's respawn
	// state 11937).
	Removes []int32 `json:"removes,omitempty"`
	// Presence is the effect the clone carries in place of 295000, the
	// Spirit Summon presence - a copy of it with another look (the
	// per-creature spirit look). Zero is 295000. Marker is the same for
	// 297000, the companion marker.
	Presence int32 `json:"presence,omitempty"`
	Marker   int32 `json:"marker,omitempty"`
	// KeepBalance keeps the Spirit Summon balance package on the clone: the
	// Helper. Absent, the entry is a Fighter.
	KeepBalance bool `json:"keep_balance,omitempty"`
}

// BuddyRowInsertion adds one row to a formation block.
//
// InsertAfter is required. A block is a contiguous run of row headers that the
// runtime walks, so where the new header lands is part of the operation and not
// an implementation detail; each insertion anchors on the row before it, which
// is how a block grows from its own last row.
type BuddyRowInsertion struct {
	RowID       int32 `json:"row_id"`
	CopyFrom    int32 `json:"copy_from"`
	InsertAfter int32 `json:"insert_after"`
}

// OperationReset is the operation value a reset manifest carries. A
// conversion leaves the field empty rather than writing "convert", so its
// bytes stay those of the first seen conversion.
const OperationReset = "reset"

// Vanilla is the value of every field a reset touches, used both for the
// shipped target and for what a save says now.
//
// Plain data rather than a reader of the embedded snapshot, so this package
// stays free of embedding concerns and a test can hand it three rows instead
// of the game's 924, 20 and 304.
type Vanilla struct {
	// Goods maps a goods row ID to its shipped value for each of the three
	// writable fields: which formation it delivers, and what it costs.
	Goods map[int32]map[string]int64
	// Buddy maps a BuddyParam row ID to its shipped value for each of the four
	// restorable fields.
	//
	// It was once the list of rows shipping appearOnAroundSekihi = 1, which was
	// enough while a conversion was the only thing that could edit the table.
	// An authored formation writes three more fields on rows a flag list does
	// not mention, so the record has to be per row and per field or a reset
	// would report success having restored a flag and left a formation.
	Buddy map[int32]map[string]float64
	// Ladders maps a BuddyParam row ID to the eleven upgrade-level effects it
	// ships with - for a row this editor inserted, the ones of the row it was
	// copied from. An authored row is written with its own, so a ladder some
	// earlier build changed goes back to the Ash's.
	Ladders map[int32][formation.DopingLevels]int32
	// CharacterLadder is the ladder a row summoning one of the player's
	// characters carries in place of its Ash's: a share of the character as
	// played (savecore.CharacterLadder). Zero where the caller gives none, and
	// the row then carries its Ash's.
	CharacterLadder [formation.DopingLevels]int32
	// Stone maps a BuddyStoneParam row ID to its shipped value for each of the
	// six expanded-area fields.
	Stone map[int32]map[string]int64
	// NpcCloneRows and NpcThinkCloneRows are the actor rows in the 100000000+
	// clone namespace. As the shipped record they are what the game came with;
	// read from a save they are what it holds now, and the difference is what
	// this editor added.
	//
	// Only the clone namespace, because a row below the offset is a world actor
	// that nothing may insert and therefore nothing may remove. Recording the
	// thousands of IDs that can never be in question would make the record
	// larger and no more able to answer anything.
	NpcCloneRows      []int32
	NpcThinkCloneRows []int32
}

// RestorableGoodsFields are the EquipParamGoods fields a reset writes. It must
// match savecore.GoodsFieldNames, which owns the offsets; a test asserts that
// rather than this comment asking for it.
var RestorableGoodsFields = []string{"consumeHP", "consumeMP", "refId_default"}

// RestorableBuddyFields are the BuddyParam fields a reset writes. It must match
// savecore.BuddyFieldNames, which owns the offsets; a test asserts that rather
// than this comment asking for it.
// The eleven reinforcement fields are here because authoring can now write
// them - a formation scaled to the creature it delivers carries its own ladder
// - and a reset has to put back whatever authoring wrote.
//
// The stance pair, npcPlayerInitParamId and generateAnimId joined them for the
// same reason and later: authoring writes all four now, so a reset that could
// not put them back would leave a Puppet permanently unable to deliver its own
// person. See archive/history/editor/IDENTIFICATION-RUN.md, N1.
var RestorableBuddyFields = []string{
	"appearOnAroundSekihi",
	// Placement, for the reason the stance pair is here: authoring writes it
	// now, and a reset that could not put it back would leave every host it
	// touched carrying a layout the game never shipped.
	"disablePCTargetShare",
	"dopingSpEffect_lv0",
	"dopingSpEffect_lv1",
	"dopingSpEffect_lv10",
	"dopingSpEffect_lv2",
	"dopingSpEffect_lv3",
	"dopingSpEffect_lv4",
	"dopingSpEffect_lv5",
	"dopingSpEffect_lv6",
	"dopingSpEffect_lv7",
	"dopingSpEffect_lv8",
	"dopingSpEffect_lv9",
	"generateAnimId",
	"npcParamId",
	"npcPlayerInitParamId",
	"npcThinkParamId",
	"pcFollowType",
	"triggerSpEffectId",
	"x_offset",
	"y_angle",
	"z_offset",
}

// ActorStrength is what one creature's clones carry beyond their recipe: the
// health of each role, and the effects the creature is granted or loses. The
// role itself is the clone's (formation.Actor.Fighter), and the formation
// row's ladder is the Ash's own and is not the actor's to change.
//
// See internal/strength and docs/DESIGN.md (Summon strength).
type ActorStrength struct {
	// HelperHP and FighterHP are the health a Helper's and a Fighter's clone
	// is written with, or 0 to leave the copied row's own alone.
	HelperHP, FighterHP int32
	// Residents are the effects the clone carries on top of its own: a slot's
	// or the map's grants.
	Residents []int32
	// Removes are the creature's resident effects its clone leaves off - a
	// composite's removal (catalog Variant.Removed).
	Removes []int32
}

// Strength is what each creature's clones carry, keyed by WORLD actor row -
// the number the player dropped, before the clone arithmetic.
//
// A creature absent from it keeps the health and residents its copied row has.
type Strength map[int32]ActorStrength

// RowPatch sets named fields on one identified row.
type RowPatch struct {
	RowID int32          `json:"row_id"`
	Name  string         `json:"name"`
	Set   map[string]any `json:"set"`
}

// TablePatch sets named fields on every row of a table.
type TablePatch struct {
	Name string         `json:"name"`
	Set  map[string]any `json:"set"`
}

// Conversion re-points one Spirit Ash at another formation.
//
// It writes no NpcParam and inserts no rows, so the model-namespace hazard
// cannot arise: nothing is copied between actors, only a reference is
// changed.
type Conversion struct {
	// HostGoodsFamily is the base goods row of the Ash the player owns.
	HostGoodsFamily int32
	// HostName is used only to label the patch for a human reader.
	HostName string
	// UpgradeLevels is how many rows the family has, normally eleven.
	UpgradeLevels int
	// LevelTriggers is the destination trigger for each upgrade level, in level
	// order, and must hold exactly UpgradeLevels entries.
	//
	// A list rather than one value because a goods row's refId_default selects
	// the formation, not merely the level of it, and one shipped family uses
	// that: Giant Rat Ashes points +0..+4 at one trigger, +5..+9 at a second and
	// +10 at a third, for three, four and five rats. Copying a single trigger to
	// all eleven rows would silently give every level the +0 formation.
	LevelTriggers []int32
	// DestinationName labels the patch. The player will still see the host
	// Ash's name in game; see docs/DESIGN.md.
	DestinationName string
}

// Recipe carries the parts of the shipping fix that are properties of a
// regulation rather than of a conversion.
//
// MonumentOriginRows are the BuddyParam rows that ship
// `appearOnAroundSekihi = 1`. Leaving them set makes a formation resolve its
// spawn point to an arbitrary distant monument and appear to vanish, so part one
// of the recipe without part two is a bug, not a partial fix.
type Recipe struct {
	MonumentOriginRows []int32
	// Area says whether this conversion lifts the monument restriction or puts
	// it back. There is no default: see Area.
	Area Area
	// Rest says the same of the once-per-rest limit: see Rest.
	Rest Rest
	// Shipped is read when either half is put back, and carries what to
	// restore - the six expanded-area fields per BuddyStoneParam row, and
	// appearOnAroundSekihi per monument-origin BuddyParam row.
	Shipped Vanilla
	// Sleeping are the monuments Anywhere wakes (savecore.SleepingMonuments)
	// and Woken those of them the save holds now. Anywhere writes every
	// sleeping one afresh, so Woken go first; Monuments takes Woken out.
	// Empty Sleeping wakes none, as every version before them.
	Sleeping, Woken []int32
}

// Area is whether a conversion lifts the monument restriction on summoning.
//
// The zero value is refused rather than defaulting. A switch whose off position
// is also its unset position is not a switch: every caller has to say which it
// means, and Build fails loudly rather than quietly applying the recipe to a
// save whose owner asked for the opposite.
type Area int

const (
	_ Area = iota // the zero value, never valid
	// Anywhere applies the expanded-area recipe to every BuddyStoneParam row
	// and clears appearOnAroundSekihi on the monument-origin rows, which
	// together make every monument in the map satisfy activation. This is what
	// every version before the switch did unconditionally.
	Anywhere
	// Monuments puts both halves back: the shipped six fields on every stone
	// row, and shipped appearOnAroundSekihi on the monument-origin rows.
	//
	// Both halves, because part one without part two is the bug the recipe's
	// own comment describes. Off has to undo exactly what on did, or the switch
	// leaves the save in a state neither position describes.
	Monuments
)

// Rest is whether the once-per-rest limit is lifted: summonedEventFlagId, the
// flag a monument sets when an Ash is used near it and resting clears.
//
// The zero value follows Area, which is how the two always moved before they
// were two switches: lifted with Anywhere, kept with Monuments.
type Rest int

const (
	RestFollowsArea Rest = iota
	// RestLifted writes 0 on every stone row: an Ash may be used again
	// without resting.
	RestLifted
	// RestKept puts every row's shipped flag back.
	RestKept
)

// lifted resolves the zero value against the area half.
func (rest Rest) lifted(area Area) bool {
	if rest == RestFollowsArea {
		return area == Anywhere
	}
	return rest == RestLifted
}

// restField is the once-per-rest half of expandedAreaRecipe; the other five
// fields are the area half.
const restField = "summonedEventFlagId"

// ErrAreaUnset is returned when a caller did not say which way the switch is.
var ErrAreaUnset = fmt.Errorf("a conversion must say whether it lifts the monument restriction")

// MonumentOriginRows1_17 is the set measured on regulation 11701000 (and
// identical on 11711000 and 11721000, the embedded snapshot's). It is data about
// one regulation version, so a caller that knows better should supply its own.
var MonumentOriginRows1_17 = []int32{
	21500000, 21500001, 21500002, 21500003,
	23300000, 23300001, 23300002,
	23310000, 23310001, 23310002, 23310003,
	23320000, 23320001, 23320002, 23320003, 23320004,
	24400000, 24400001, 24400002,
	30400000,
}

// expandedAreaRecipe is the all-row BuddyStoneParam edit research seen.
// Every field is required: it makes every monument in the map eligible and
// clears the once-per-rest gate.
var expandedAreaRecipe = map[string]any{
	"activateRange":                   65535,
	"overwriteReturnRange":            -1,
	"overwriteActivateRegionEntityId": 0,
	"warnRegionEntityId":              0,
	"summonedEventFlagId":             0,
	"eliminateTargetEntityId":         0,
}

// ErrNoConversions is returned rather than emitting a manifest that would
// rewrite the stone table and change nothing else.
var ErrNoConversions = fmt.Errorf("a manifest needs at least one conversion")

// ErrIncompleteVanilla is returned rather than emitting a reset that would
// restore part of the save and leave the rest converted. A half reset is a
// state no version of this product produces and none has been tested in game.
var ErrIncompleteVanilla = fmt.Errorf("a reset needs shipped values for all three tables")

// ErrAlreadyVanilla means nothing in the save differs from shipped values, so
// there is no reset to perform.
var ErrAlreadyVanilla = fmt.Errorf("every field this product writes already holds its shipped value")

// Build assembles the manifest for a set of conversions.
//
// The expanded-area recipe is emitted with its monument-origin fix, never
// without. Goods patches contain only refId_default: multiplayer fields are
// outside this product's write surface.
func Build(sourceSHA256 string, conversions []Conversion, recipe Recipe) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(conversions) == 0 {
		return nil, ErrNoConversions
	}
	seen := map[int32]string{}
	manifest := &Manifest{SourceSHA256: sourceSHA256}

	for _, conversion := range conversions {
		if conversion.UpgradeLevels <= 0 {
			return nil, fmt.Errorf("%s: upgrade_levels must be positive", conversion.HostName)
		}
		if len(conversion.LevelTriggers) != conversion.UpgradeLevels {
			return nil, fmt.Errorf("%s: got %d level triggers for %d upgrade levels",
				conversion.HostName, len(conversion.LevelTriggers), conversion.UpgradeLevels)
		}
		for level, trigger := range conversion.LevelTriggers {
			if trigger <= 0 {
				return nil, fmt.Errorf("%s +%d: trigger must be positive, got %d", conversion.HostName, level, trigger)
			}
		}
		if previous, clash := seen[conversion.HostGoodsFamily]; clash {
			return nil, fmt.Errorf("goods family %d is converted twice, by %s and %s",
				conversion.HostGoodsFamily, previous, conversion.HostName)
		}
		seen[conversion.HostGoodsFamily] = conversion.HostName
		for level := 0; level < conversion.UpgradeLevels; level++ {
			manifest.GoodsPatches = append(manifest.GoodsPatches, RowPatch{
				RowID: conversion.HostGoodsFamily + int32(level),
				Name: fmt.Sprintf("%s +%d delivers the %s formation",
					conversion.HostName, level, conversion.DestinationName),
				Set: map[string]any{
					"refId_default": conversion.LevelTriggers[level],
				},
			})
		}
	}

	if err := writeArea(manifest, recipe); err != nil {
		return nil, err
	}
	return manifest, nil
}

// writeArea puts the monument switch into a manifest, in whichever positions
// the recipe names. It is shared by a conversion and by BuildArea so the two
// cannot drift: whatever a conversion writes here is exactly what the switch
// alone writes, and exactly what the other position undoes.
//
// Each half that is on rides on one all-row patch at the recipe's values; each
// half that is off is put back on every shipped row from the snapshot.
func writeArea(manifest *Manifest, recipe Recipe) error {
	if recipe.Area != Anywhere && recipe.Area != Monuments {
		return ErrAreaUnset
	}
	wide, noRest := recipe.Area == Anywhere, recipe.Rest.lifted(recipe.Area)
	if (!wide || !noRest) && (len(recipe.Shipped.Stone) == 0 || !wide && len(recipe.Shipped.Buddy) == 0) {
		return fmt.Errorf("restoring the monument restriction needs shipped values: stone=%d buddy=%d: %w",
			len(recipe.Shipped.Stone), len(recipe.Shipped.Buddy), ErrIncompleteVanilla)
	}
	rows := append([]int32(nil), recipe.MonumentOriginRows...)
	slices.Sort(rows)
	for _, rowID := range rows {
		if wide {
			manifest.BuddyPatches = append(manifest.BuddyPatches, RowPatch{
				RowID: rowID,
				Name:  "Shipping recipe part 2: player origin on a monument-origin row",
				Set:   map[string]any{"appearOnAroundSekihi": 0},
			})
			continue
		}
		values, known := recipe.Shipped.Buddy[rowID]
		if !known {
			return fmt.Errorf("shipped snapshot is missing buddy row %d", rowID)
		}
		shipped, known := values["appearOnAroundSekihi"]
		if !known {
			return fmt.Errorf("shipped snapshot has no appearOnAroundSekihi for buddy row %d", rowID)
		}
		manifest.BuddyPatches = append(manifest.BuddyPatches, RowPatch{
			RowID: rowID,
			Name:  "Monument restriction restored: shipped origin on a monument-origin row",
			Set:   map[string]any{"appearOnAroundSekihi": shipped},
		})
	}

	on := func(name string) bool {
		if name == restField {
			return noRest
		}
		return wide
	}
	recipeSet := map[string]any{}
	for name, value := range expandedAreaRecipe {
		if on(name) {
			recipeSet[name] = value
		}
	}
	if len(recipeSet) > 0 {
		manifest.BuddyStonePatches = []TablePatch{{
			Name: "Shipping recipe part 1: expanded-area and re-summoning recipe",
			Set:  recipeSet,
		}}
	}
	if !wide || !noRest {
		for _, rowID := range sortedKeys(recipe.Shipped.Stone) {
			set := map[string]any{}
			for name, value := range recipe.Shipped.Stone[rowID] {
				if !on(name) {
					set[name] = value
				}
			}
			manifest.BuddyStoneRowPatches = append(manifest.BuddyStoneRowPatches, RowPatch{
				RowID: rowID,
				Name:  "Monument restriction restored: shipped expanded-area fields",
				Set:   set,
			})
		}
	}

	manifest.StoneRemovals = slices.Sorted(slices.Values(recipe.Woken))
	if wide {
		manifest.StoneInsertions = slices.Sorted(slices.Values(recipe.Sleeping))
	}
	return nil
}

// TouchedTables reports every table the manifest writes.
//
// v1 must never appear in this list with NpcParam or NpcThinkParam. A caller can
// assert that cheaply, and the tests here do.
func (m *Manifest) TouchedTables() []string {
	tables := []string{}
	if len(m.GoodsPatches) > 0 {
		tables = append(tables, "EquipParamGoods")
	}
	if len(m.BuddyPatches) > 0 {
		tables = append(tables, "BuddyParam")
	}
	if len(m.BuddyStonePatches)+len(m.BuddyStoneRowPatches)+len(m.StoneInsertions)+len(m.StoneRemovals) > 0 {
		tables = append(tables, "BuddyStoneParam")
	}
	sort.Strings(tables)
	return tables
}

// Encode emits the manifest as the save core expects it. It is deliberately
// not WriteTo: that name belongs to io.WriterTo and means something else.
func (m *Manifest) Encode(writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(m)
}

// BuildReset assembles the manifest that returns a save to shipped behavior.
//
// It is the inverse of Build and writes the same three tables, but every value
// comes from the recorded snapshot rather than from a rule.
//
// History does not matter here, which is the point. The target state is
// absolute, so a save edited three times by three versions of this tool resets
// in one step; there is no manifest to find and replay, and no partially
// replayed state to land in.
//
// It emits a patch ONLY for a row whose current value differs from the shipped
// one. Writing the whole table would produce the same save — the extra writes
// are all no-ops — but the manifest is also the declared mutation range that
// savecore fences every changed byte against, so restoring 11 goods rows must
// not license writes into the other 913. It keeps the audit manifest a record
// of what was actually restored rather than a list of every row considered.
//
// enable_multi is deliberately absent, exactly as it is from a conversion. The
// product rule is that no edit and no reset writes a multiplayer field, so a
// save made by the earlier clearing behavior needs its untouched original or a
// separately reviewed one-time migration - never a hidden exception here.
func BuildReset(sourceSHA256 string, shipped, current Vanilla) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(shipped.Goods) == 0 || len(shipped.Buddy) == 0 || len(shipped.Stone) == 0 {
		return nil, fmt.Errorf("goods=%d buddy=%d stone=%d: %w",
			len(shipped.Goods), len(shipped.Buddy), len(shipped.Stone), ErrIncompleteVanilla)
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationReset}

	// All three fields go back together, for the reason the Buddy loop below
	// gives: one changed field is reason enough to put the row back, and a row
	// restored halfway holds a combination the game never shipped.
	for _, rowID := range sortedKeys(shipped.Goods) {
		values := shipped.Goods[rowID]
		if len(values) != len(RestorableGoodsFields) {
			return nil, fmt.Errorf("goods row %d has %d shipped fields, want all %d",
				rowID, len(values), len(RestorableGoodsFields))
		}
		if trigger := values["refId_default"]; trigger <= 0 {
			return nil, fmt.Errorf("goods row %d has a non-positive shipped trigger %d", rowID, trigger)
		}
		set := make(map[string]any, len(values))
		matches := true
		now, known := current.Goods[rowID]
		for _, name := range RestorableGoodsFields {
			value, present := values[name]
			if !present {
				return nil, fmt.Errorf("goods row %d has no shipped %s", rowID, name)
			}
			set[name] = value
			if !known || now[name] != value {
				matches = false
			}
		}
		if matches {
			continue
		}
		manifest.GoodsPatches = append(manifest.GoodsPatches, RowPatch{
			RowID: rowID,
			Name:  "Reset: shipped formation reference and price",
			Set:   set,
		})
	}

	// Every field is restored in both directions. A monument-origin row the
	// game ships at 1 that the save has cleared is the ordinary case, produced
	// by this product's own conversion recipe. A row the game ships at 0 that
	// the save has set is not something a conversion can produce, but Reset's
	// requirement is to restore shipped behavior whatever edited the save.
	for _, rowID := range sortedKeys(shipped.Buddy) {
		values := shipped.Buddy[rowID]
		if len(values) != len(RestorableBuddyFields) {
			return nil, fmt.Errorf("Buddy row %d has %d shipped fields, want all %d",
				rowID, len(values), len(RestorableBuddyFields))
		}
		set := make(map[string]any, len(values))
		matches := true
		now, known := current.Buddy[rowID]
		for _, name := range RestorableBuddyFields {
			value, present := values[name]
			if !present {
				return nil, fmt.Errorf("Buddy row %d has no shipped %s", rowID, name)
			}
			set[name] = value
			if !known || now[name] != value {
				matches = false
			}
		}
		if matches {
			continue
		}
		manifest.BuddyPatches = append(manifest.BuddyPatches, RowPatch{
			RowID: rowID,
			Name:  "Reset: shipped formation row",
			Set:   set,
		})
	}

	for _, rowID := range sortedKeys(shipped.Stone) {
		values := shipped.Stone[rowID]
		if len(values) != len(expandedAreaRecipe) {
			return nil, fmt.Errorf("stone row %d has %d shipped fields, want all %d",
				rowID, len(values), len(expandedAreaRecipe))
		}
		set := make(map[string]any, len(values))
		matches := true
		now, known := current.Stone[rowID]
		for name, value := range values {
			if _, isRecipeField := expandedAreaRecipe[name]; !isRecipeField {
				return nil, fmt.Errorf("stone row %d names %s, which the recipe never writes", rowID, name)
			}
			set[name] = value
			if !known || now[name] != value {
				matches = false
			}
		}
		if matches {
			continue
		}
		manifest.BuddyStoneRowPatches = append(manifest.BuddyStoneRowPatches, RowPatch{
			RowID: rowID,
			Name:  "Reset: shipped expanded-area values",
			Set:   set,
		})
	}

	// The removal half. A reset restores values in rows the game shipped and
	// deletes the rows it did not, and the second half is what makes a reset of
	// an AUTHORED formation complete: restoring the shipped rows of a block
	// that was grown would put four rows back and leave the fifth summoning.
	//
	// The rule is one subtraction, and it is the same one three times: a row
	// present now that the shipped record does not name is this editor's, and
	// only this editor's rows may go. Nothing here decides whether a row looks
	// like the editor's - the shipped record answers that, which is why it
	// carries key sets and not only values.
	//
	// The subtraction is only meaningful when the shipped side actually has a
	// record. An empty shipped list beside a populated current one does not
	// mean the game shipped no clones - it means this record cannot answer the
	// question - and treating it as an answer would delete every clone in the
	// save, including the 115 the game came with.
	if len(shipped.NpcCloneRows) == 0 && len(current.NpcCloneRows) > 0 {
		return nil, fmt.Errorf("shipped record names no NpcParam clone rows: %w", ErrIncompleteVanilla)
	}
	if len(shipped.NpcThinkCloneRows) == 0 && len(current.NpcThinkCloneRows) > 0 {
		return nil, fmt.Errorf("shipped record names no NpcThinkParam clone rows: %w", ErrIncompleteVanilla)
	}
	manifest.BuddyRowRemovals = extraRows(sortedKeys(current.Buddy), sortedKeys(shipped.Buddy))
	manifest.NpcCloneRemovals = extraRows(current.NpcCloneRows, shipped.NpcCloneRows)
	manifest.NpcThinkCloneRemovals = extraRows(current.NpcThinkCloneRows, shipped.NpcThinkCloneRows)
	// The woken monuments: the only stone rows this editor adds, so the
	// caller reports no other (application.currentVanilla).
	manifest.StoneRemovals = extraRows(sortedKeys(current.Stone), sortedKeys(shipped.Stone))

	if len(manifest.GoodsPatches) == 0 && len(manifest.BuddyPatches) == 0 &&
		len(manifest.BuddyStoneRowPatches) == 0 && len(manifest.BuddyRowRemovals) == 0 &&
		len(manifest.NpcCloneRemovals) == 0 && len(manifest.NpcThinkCloneRemovals) == 0 &&
		len(manifest.StoneRemovals) == 0 {
		return nil, ErrAlreadyVanilla
	}
	return manifest, nil
}

// extraRows returns the ascending rows present now that the shipped record does
// not name.
//
// A row the shipped record names and the save has lost is deliberately not
// reported. Reset cannot put a missing row back - it restores values, and a
// value needs a row to sit in - so naming it here would only produce a removal
// list the save core would reject for a row that is not there. A save missing a
// shipped row is damaged by something outside this product, and reporting that
// honestly belongs to the reader rather than to a repair this tool cannot make.
func extraRows(present, shipped []int32) []int32 {
	known := make(map[int32]bool, len(shipped))
	for _, rowID := range shipped {
		known[rowID] = true
	}
	var extra []int32
	for _, rowID := range present {
		if !known[rowID] {
			extra = append(extra, rowID)
		}
	}
	return extra
}

// sortedKeys keeps a manifest's row order deterministic, so two resets of one
// save produce byte-identical manifests and a diff between them means something.
func sortedKeys[V any](values map[int32]V) []int32 {
	return slices.Sorted(maps.Keys(values))
}
