package savecore

// The manifest: what one edit asks for, and the five disjoint operations it
// can ask it under. This is the wire format - decoding it is here, judging it
// is in validate.go and rowrules.go, and performing it is in write.go.

import (
	"encoding/json"
	"fmt"
	"io"
)

// operationConvert and operationReset are the two things a manifest can ask
// for. They are separate rule sets rather than one relaxed set because each is
// the other's inverse: a conversion clears appearOnAroundSekihi and writes one
// uniform stone recipe, a reset restores the flag and 304 distinct rows. A
// validator permissive enough to express both would stop rejecting either one's
// mistakes.
const (
	operationConvert = "convert"
	operationReset   = "reset"
	// operationAuthor fills a host Ash's own BuddyParam block with actors the
	// player chose. It is a third rule set rather than a relaxed convert for
	// the same reason reset is: the three write disjoint surfaces. A conversion
	// writes goods and clears one BuddyParam flag; authoring writes the three
	// actor fields of a block and no goods row at all; a reset restores both
	// and is the only one that may remove a row.
	operationAuthor = "author"
	// operationCost writes a host Ash's resource price across its eleven goods
	// rows. It is a fourth rule set for the same reason the third was: it owns
	// two fields of one table and touches nothing else, and letting a
	// conversion carry it would mean one validator describing two intents.
	operationCost = "cost"
	// operationStrength writes what the summon strength switch decides: the
	// upgrade ladder on every formation row, and the values of the summon rows
	// themselves. It is a fifth rule set because its surface is a fifth: it is
	// the only operation that writes an NpcParam row that already exists, and
	// the only one that writes BuddyParam without claiming to compose a
	// formation. It adds and removes nothing.
	operationStrength = "strength"
	// operationArea writes the monument switch and nothing else: every
	// BuddyStoneParam row, appearOnAroundSekihi on the twenty BuddyParam rows
	// that ship it set, and the sleeping monuments (monuments.go).
	//
	// It is a sixth rule set because it is the only operation that writes the
	// stone table without composing anything. The recipe used to ride on a
	// conversion, which meant an authored formation came out monument-locked
	// however the player had set the switch - authoring writes the host's own
	// block and has nothing to say to the monument table.
	operationArea = "area"
	// operationTint writes the colour switch and nothing else: one field of one
	// SpEffectVfxParam row.
	//
	// A seventh rule set because its surface is a sixth table, and because it is
	// the only operation whose whole effect is a pointer. It moves
	// phantomParamOverwriteId on the row spEffect 295000 delivers between
	// PhantomParam 200, the white spirit look, and 0, which is the game's own
	// spelling for no overwrite. It adds and removes nothing, and it names no
	// actor, no formation and no Ash.
	operationTint = "tint"
	// operationPrune removes the creature rows this editor inserted that no
	// formation row names any more, and does nothing else: it carries clone
	// removals and no patch, no insertion and no BuddyParam removal.
	//
	// An eighth rule set (the file is written from the whole configuration at
	// Save, and a creature row nothing summons is not written). Until then only a reset removed anything, so a
	// player who reconfigured a creature left its old rows behind for good.
	// It stays apart from a reset because the two mean different things: a
	// reset puts the save back as the game shipped it, a prune tidies the one
	// the player is keeping. Which rows are unused is the application's
	// finding, made against every formation row in the save; this layer holds
	// a prune to the clone bands (validateStructuralWork), as it holds a
	// reset.
	operationPrune = "prune"
	// operationCharacters is the ninth, in characters.go: the player's own
	// characters' rows, in a band of three tables nothing else writes.
	// operationOwnEffects is the tenth, in owneffects.go: the effect rows of
	// our own a clone may carry in place of the presence or the marker.
)

// structuralOperations are the operations permitted to change which rows exist.
// Everything else in this product edits bytes inside rows that already exist.
// A session is here because it may carry an authoring, a reset or a prune
// part; what it is actually permitted to do to the row set is decided part by
// part, before the parts are flattened into one.
var structuralOperations = map[string]bool{
	operationAuthor:     true,
	operationReset:      true,
	operationPrune:      true,
	operationSession:    true,
	operationCharacters: true,
	operationOwnEffects: true,
}

type manifest struct {
	SourceSHA256 string `json:"source_sha256"`
	// Operation is empty on a conversion: empty means convert.
	Operation string `json:"operation,omitempty"`
	// Parts is the whole of a session manifest: one commit carrying several
	// operations, each still a strict single-operation manifest with its own
	// rule set. Anything else is empty on a session until the parts are
	// flattened into it; see session.go.
	Parts        []*manifest `json:"parts,omitempty"`
	BuddyPatches []rowPatch  `json:"buddy_patches,omitempty"`
	VfxPatches   []rowPatch  `json:"vfx_patches,omitempty"`
	// EffectPatches are shared effect writes: fields of SHIPPED SpEffectParam
	// rows an entry needs, the one exception to "nothing else in the world",
	// bounded to the list in sharedeffects.go.
	EffectPatches []rowPatch `json:"effect_patches,omitempty"`
	// PersonPatches are person writes: fields of SHIPPED CharaInitParam rows
	// an armed person needs, the second exception, bounded to the list in
	// personwrites.go and carried by the shared write's operation.
	PersonPatches []rowPatch `json:"person_patches,omitempty"`
	// NpcPatches writes named field values into NpcParam rows that already
	// exist, which is how a role reaches FromSoftware's own 115 summon rows - a
	// Fighter's values, or the recorded shipped ones back. See npcwrite.go; the
	// values are the caller's, because what a creature is worth is a catalog
	// fact and this package holds no catalog.
	NpcPatches        []rowPatch   `json:"npc_patches,omitempty"`
	BuddyStonePatches []tablePatch `json:"buddy_stone_patches,omitempty"`
	// BuddyStoneRowPatches restores shipped values row by row: a reset's
	// every field, and the monument switch's the fields of whichever half is
	// off, the other half riding on the one all-row patch.
	BuddyStoneRowPatches []rowPatch `json:"buddy_stone_row_patches,omitempty"`
	GoodsPatches         []rowPatch `json:"goods_patches,omitempty"`

	// The structural half. These fields change which rows exist rather than
	// what is inside one, so they are proved differently: a byte-window
	// comparison cannot describe an operation that moves every following
	// address, and each of these carries the by-row-ID proof instead.
	//
	// They come in inverse pairs, and authoring may only use the left half
	// while a reset may only use the right. That is the schema stating what
	// docs/SAFETY.md rule 1 requires: this editor adds rows and undoes its own
	// additions, and there is no third thing it can do to a row's existence.

	// NpcCloneSources and NpcThinkCloneSources are WORLD actor row IDs. The
	// clone's own ID is never in the manifest: it is source + 100000000,
	// computed where the row is written. A destination that cannot be named
	// cannot be named wrongly, which is how rule 1's model-namespace
	// requirement stays true by construction rather than by a check.
	NpcCloneSources      []int32 `json:"npc_clone_sources,omitempty"`
	NpcThinkCloneSources []int32 `json:"npc_think_clone_sources,omitempty"`
	// NpcSummonCopies are SHIPPED SUMMON-SIDE rows - the game's person shells
	// - copied into the copy band, for a slot that puts a shell on a team: a
	// shipped row is never written, so the change goes on a copy. The copy's
	// ID is arithmetic like a clone's (summonCopyRow), and the copy is a byte
	// copy of its source with only the team written over it - the source is
	// a summon already and needs no recipe.
	NpcSummonCopies []int32 `json:"npc_summon_copies,omitempty"`
	// NpcCloneTeams puts a clone or a copy on a team other than Spirit Summon:
	// a summon that is also hostile to the player, off by default, and for a
	// person Spirit Battler's route on PC (NpcParam.teamType on a copied
	// row). Every source
	// it names must be in NpcCloneSources or NpcSummonCopies, and the team
	// one of summonTeams. A clone this list does not name is on 47, as every
	// archived manifest wrote it.
	NpcCloneTeams []npcCloneTeam `json:"npc_clone_teams,omitempty"`
	// NpcCloneStrength changes the recipe for the clones it names, and naming
	// one is the whole of the switch: a clone with an entry here is written
	// WITHOUT the Spirit Summon balance package, and with the base health the
	// entry carries.
	//
	// Every source it names must also be in NpcCloneSources - it modifies an
	// insertion rather than authorising one - and the values are the caller's,
	// because what a creature is worth in the world is a catalog fact and this
	// package holds no catalog. See internal/strength and docs/DESIGN.md (Summon strength).
	NpcCloneStrength []npcCloneStrength `json:"npc_clone_strength,omitempty"`
	// NpcCloneWorldBranch names the clones written WITHOUT companion movement
	// (297000): the actor's logic script then takes the world branch - no
	// following, no warping, no leash to the host, no target fed by the
	// player - which is what a hostile summon built as a real enemy is, and
	// what a Loner is. Every source it names must be in NpcCloneSources; it
	// modifies an insertion. A clone this list does not name carries 297000,
	// as every archived manifest wrote it. See summonResidentEffects.
	NpcCloneWorldBranch []int32 `json:"npc_clone_world_branch,omitempty"`
	// NpcCloneWorldBranchUnheld: world-branch clones written without 5011
	// (plan.Manifest.NpcCloneWorldBranchUnheld). Their think clones carry no
	// band because the plan does not mark them world_branch.
	NpcCloneWorldBranchUnheld []int32 `json:"npc_clone_world_branch_unheld,omitempty"`
	// NpcCloneWeaponOut names the clones written with the weapon out
	// (plan.Manifest.NpcCloneWeaponOut): the sheathed-state effect the named
	// world think row carries is taken off the clone's residents. Every source
	// must be in NpcCloneSources; it modifies an insertion. A clone this list
	// does not name keeps its residents, as every archived manifest wrote it.
	NpcCloneWeaponOut []npcCloneWeaponOut `json:"npc_clone_weapon_out,omitempty"`
	// NpcThinkPerception is the perception profile per think clone. A think
	// source this list does not name keeps its world perception and gets the
	// buddy flag alone.
	NpcThinkPerception []npcThinkPerception `json:"npc_think_perception,omitempty"`
	// BuddyRowInsertions extend a formation block past the rows the game
	// shipped for it.
	BuddyRowInsertions []buddyRowInsertion `json:"buddy_row_insertions,omitempty"`

	// The removal half, reset only. Actor removals are clone IDs rather than
	// sources, because removal names a row that exists rather than deriving one
	// that does not.
	BuddyRowRemovals      []int32 `json:"buddy_row_removals,omitempty"`
	NpcCloneRemovals      []int32 `json:"npc_clone_removals,omitempty"`
	NpcThinkCloneRemovals []int32 `json:"npc_think_clone_removals,omitempty"`

	// CharacterRows and CharacterRowRemovals are the player's own characters,
	// summoned as people (characters.go): rows written whole, inserted where
	// the save lacks them, and removed once nothing names them. Only the
	// characters operation carries them.
	CharacterRows        []characterRow    `json:"character_rows,omitempty"`
	CharacterBodies      []characterBody   `json:"character_bodies,omitempty"`
	CharacterRowRemovals []CharacterRowKey `json:"character_row_removals,omitempty"`

	// OwnEffectInsertions and OwnEffectRemovals are effect rows of our own
	// (owneffects.go), by id: the save core holds what each copies and writes.
	// Only the own-effects operation carries them.
	OwnEffectInsertions []int32 `json:"own_effect_insertions,omitempty"`
	OwnEffectRemovals   []int32 `json:"own_effect_removals,omitempty"`

	// StoneInsertions and StoneRemovals are the sleeping monuments
	// (monuments.go), by entity: the monument switch wakes them and puts them
	// back, and a reset puts them back.
	StoneInsertions []int32 `json:"stone_insertions,omitempty"`
	StoneRemovals   []int32 `json:"stone_removals,omitempty"`
}

// buddyRowInsertion adds one row to a formation block.
//
// InsertAfter is required and is not an optimisation. BuddyParam's runtime
// consumer walks a contiguous run of row headers, so a free numeric ID does not
// by itself authorise a placement: where the header lands is part of the
// operation. Research seen a block grown from its own last row in batch
// 34, and each insertion anchoring on the one before it is what keeps the run
// contiguous.
type buddyRowInsertion struct {
	RowID       int32 `json:"row_id"`
	CopyFrom    int32 `json:"copy_from"`
	InsertAfter int32 `json:"insert_after"`
}

// npcThinkPerception is one think clone's perception profile, by name. The
// names and their values are thinkProfiles in clone.go.
type npcThinkPerception struct {
	Source int32   `json:"source"`
	Level  string  `json:"level"`
	Body   float64 `json:"body,omitempty"`
	// Front: how far the actor reaches in front of its centre, which the
	// ladder counts its reach past (clone.go ladderProfile). Zero falls back
	// to Body.
	Front float64 `json:"front,omitempty"`
	// WorldBranch: the actor is written without 297000, and its think clone
	// carries the shipped Ash home band after its profile (clone.go).
	WorldBranch bool `json:"world_branch,omitempty"`
	// NoTurn: the actor's battle script has no Turn action, and the three
	// middle levels give it a nose (clone.go ladderProfile).
	NoTurn bool `json:"no_turn,omitempty"`
	// OwnTarget: the share box is off on a level that offers it, and the
	// think clone fights whoever hits it (clone.go ownTargetLevels).
	OwnTarget bool `json:"own_target,omitempty"`
}

// npcCloneTeam is one clone (or copy) on a team other than Spirit Summon.
type npcCloneTeam struct {
	Source int32 `json:"source"`
	Team   uint8 `json:"team"`
}

// npcCloneWeaponOut is one clone written with its weapon out, and the world
// think row that names its sheathed state.
type npcCloneWeaponOut struct {
	Source int32 `json:"source"`
	Think  int32 `json:"think"`
}

// npcCloneStrength is one clone written at the creature's own strength.
type npcCloneStrength struct {
	// Source is the world actor row, the same identity NpcCloneSources uses.
	Source int32 `json:"source"`
	// HP is the base health to write onto the clone, which is the strongest
	// placement the creature has in the world. The clone copies whichever row
	// the player dropped, and that is often a weaker placement than the one
	// they fought; the level ladder multiplies whatever base it is given, so
	// the base has to be the right one before the multiplier is right.
	//
	// Zero leaves the copied row's health alone, which is what a caller that
	// only wants the balance package dropped passes.
	HP int32 `json:"hp,omitempty"`
	// Residents are added to the clone's resident-effect vector on top of the
	// three the summon recipe requires: the creature's grants and its level's
	// own. They are the caller's numbers for the same reason HP is.
	Residents []int32 `json:"residents,omitempty"`
	// Removes are resident effects the SOURCE row carries that the clone must
	// not - a composite that takes one off (the God-Devouring Serpent's
	// 11937, 'Character Respawn', which keeps a summoned Serpent from
	// dying). Each must be on the source row; none may be an effect the summon
	// recipe itself requires, and none may be in Residents too.
	Removes []int32 `json:"removes,omitempty"`
	// Presence is written in place of 295000, the Spirit Summon presence:
	// a copy of it at another id, with another look. It must be a
	// row ID and not one of the recipe's other effects. Zero is 295000.
	Presence int32 `json:"presence,omitempty"`
	// Marker is written in place of 297000, the companion marker, the same
	// way: a copy at another id that no battle script tests (QuietMarker).
	// A clone on the world branch carries no marker to replace. Zero is
	// 297000.
	Marker int32 `json:"marker,omitempty"`
	// KeepBalance leaves the balance package in place: the clone takes its
	// Residents and stays a helper. Off on every archived manifest.
	KeepBalance bool `json:"keep_balance,omitempty"`
}

type rowPatch struct {
	RowID int32              `json:"row_id"`
	Name  string             `json:"name,omitempty"`
	Set   map[string]float64 `json:"set"`
}

type tablePatch struct {
	Name string             `json:"name,omitempty"`
	Set  map[string]float64 `json:"set"`
}

func decodeManifest(reader io.Reader) (*manifest, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var decoded manifest
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parse manifest: unexpected data after manifest")
		}
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := validateManifest(&decoded); err != nil {
		return nil, err
	}
	return &decoded, nil
}
