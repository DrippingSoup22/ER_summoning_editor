package savecore

// A session manifest: one commit carrying several operations.
//
// The four operations are still four separate rule sets, and this file does not
// add a fifth that is permissive enough to describe all of them. It adds a
// container. Each part is validated by exactly the validator it would face on
// its own, and what is checked here is only what is true of the combination and
// of no part of it: that two parts do not describe the same thing, that no row
// is added by one and removed by another (a creature row may be replaced - see
// validateSessionComposition), and that the container itself asks for nothing.
//
// "The same thing" is a row everywhere but EquipParamGoods, where it is a
// field. A conversion re-points a goods row and a cost edit prices it; the two
// write different offsets, so both descriptions survive in either order and
// staging them against one Ash is a legitimate commit.
//
// After validation the parts are flattened into the parent, and the write and
// its proofs run unchanged. That is the point of the arrangement rather than a
// convenience: a commit is one structural stage, one patch stage, one bounded
// mutation proof and one atomic publication, so nothing about the save write
// has to learn that sessions exist. verifyStructuralStage in particular was
// built in G1 to prove a composition of operations against the whole save; a
// session is that composition, larger.

import (
	"fmt"
	"slices"
)

// operationSession is the operation value a composed manifest carries. A
// session manifest holds parts and nothing else - no patches, no structural
// work, no stone recipe of its own.
const operationSession = "session"

// validateSession checks a composed manifest and flattens it.
//
// The order matters. Every part is validated in full before anything is merged,
// so a part that would be rejected on its own is rejected here too rather than
// disappearing into a union where its patches look like some other operation's.
func validateSession(plan *manifest) error {
	if err := sessionCarriesNoWorkOfItsOwn(plan); err != nil {
		return err
	}
	// Two, because a commit carrying one operation is emitted as that
	// operation. A file claiming to be a session while holding a single part is
	// not what this product writes, and accepting it would mean two spellings
	// of one manifest.
	if len(plan.Parts) < 2 {
		return fmt.Errorf("a session manifest holds at least 2 operations, got %d", len(plan.Parts))
	}
	for index, part := range plan.Parts {
		if part == nil {
			return fmt.Errorf("session operation %d is missing", index+1)
		}
		if len(part.Parts) != 0 {
			return fmt.Errorf("session operation %d is itself a session; a commit is one level deep", index+1)
		}
		// A part carries the whole commit's source hash. Empty is refused
		// rather than filled in: a part that does not say which save it was
		// planned against is a part that could have been moved here from
		// another plan.
		if part.SourceSHA256 != plan.SourceSHA256 {
			return fmt.Errorf("session operation %d was planned against %q, not %q",
				index+1, part.SourceSHA256, plan.SourceSHA256)
		}
		switch part.Operation {
		case operationConvert, operationReset, operationAuthor, operationCost, operationStrength, operationArea, operationTint, operationPrune, operationSharedEffects, operationCharacters, operationOwnEffects:
		case "":
			// Only outside a session does an empty operation mean a
			// conversion. Inside one there is no single operation for the file
			// to imply, so every part has to say what it is.
			return fmt.Errorf("session operation %d does not say which operation it is", index+1)
		default:
			return fmt.Errorf("session operation %d has unknown operation %q", index+1, part.Operation)
		}
		if err := validateSingleOperation(part); err != nil {
			return fmt.Errorf("session operation %d (%s): %w", index+1, part.Operation, err)
		}
	}
	if err := validateSessionComposition(plan.Parts); err != nil {
		return err
	}
	flattenSession(plan)
	return nil
}

// sessionCarriesNoWorkOfItsOwn refuses a container that also describes an edit.
//
// Without it a session would have two places to put a patch - its own list and
// a part's - and only one of them would be validated against an operation's
// rules. The container's own lists are the unvalidated one, so they must be
// empty rather than merely ignored.
func sessionCarriesNoWorkOfItsOwn(plan *manifest) error {
	own := len(plan.GoodsPatches) + len(plan.BuddyPatches) +
		len(plan.BuddyStonePatches) + len(plan.BuddyStoneRowPatches) +
		len(plan.NpcCloneSources) + len(plan.NpcThinkCloneSources) + len(plan.NpcSummonCopies) +
		len(plan.BuddyRowInsertions) + len(plan.BuddyRowRemovals) +
		len(plan.NpcCloneRemovals) + len(plan.NpcThinkCloneRemovals) +
		len(plan.VfxPatches) + len(plan.EffectPatches) + len(plan.PersonPatches) + len(plan.NpcPatches) +
		len(plan.CharacterRows) + len(plan.CharacterBodies) + len(plan.CharacterRowRemovals) +
		len(plan.OwnEffectInsertions) + len(plan.OwnEffectRemovals) +
		len(plan.StoneInsertions) + len(plan.StoneRemovals)
	if own != 0 {
		return fmt.Errorf("a session manifest describes its operations only through parts, and this one also carries %d edits of its own", own)
	}
	return nil
}

// validateSessionComposition rejects what is wrong about the combination.
//
// Every rule has one shape: two parts describe the same row, and there is no
// order in which both descriptions survive. No value is looked at - each part's
// own validator did that - so what is compared is only which rows each part
// claims.
//
// One pair of claims is not a conflict: a creature row one
// part removes and another inserts is REPLACED - the row the save held goes,
// and a clone of the same creature on another recipe takes its number. It is
// how a creature row a player no longer uses frees its place for the recipe
// that replaces it in the same save (the application's clone placement), and
// the structural stage runs the removal first and proves the result
// (applyStructuralChanges, verifyStructuralStage). Only the two actor tables
// may be replaced; a BuddyParam row added and removed is still refused.
func validateSessionComposition(parts []*manifest) error {
	// Goods claims are per field, and the other tables' are per row.
	//
	// The rule everywhere here is that two parts must not describe the same
	// thing, because there is no order in which both descriptions survive. In
	// EquipParamGoods that thing is a field: a conversion re-points a row and a
	// cost edit prices it, and the two write different offsets, so both
	// descriptions do survive and staging them against one Ash is a legitimate
	// commit. In BuddyParam it stays the row, because authoring and a reset
	// both write the whole row and one would overwrite what the other restored.
	goods := map[string]*rowClaims{}
	buddy := newRowClaims("BuddyParam row")
	vfx := newRowClaims("SpEffectVfxParam row")
	effects := newRowClaims("SpEffectParam row")
	persons := newRowClaims("CharaInitParam row")
	stone := newRowClaims("BuddyStoneParam row")
	added := newRowClaims("row")
	removed := newRowClaims("row")
	// The actor-table rows among them, apart, because a buddy row and a
	// creature row can share a number and only the second may be replaced.
	actorAdded, actorRemoved := map[int32]bool{}, map[int32]int{}
	npcPatched := map[int32]int{}
	// A character row is claimed whole by the one part that writes or removes
	// it; no other operation reaches the character tables. An effect row of our
	// own likewise.
	characters := map[CharacterRowKey]int{}
	ownEffects := newRowClaims("effect row of our own")
	monuments := newRowClaims("sleeping monument")

	// A part's all-row stone patch and its own shipped rows are the two
	// halves of one switch; another part's rows beside it are not.
	tableRecipe, rowRecipes := -1, map[int]bool{}
	for index, part := range parts {
		for _, patch := range part.GoodsPatches {
			for name := range patch.Set {
				claims, known := goods[name]
				if !known {
					claims = newRowClaims(name + " of goods row")
					goods[name] = claims
				}
				if err := claims.claim(patch.RowID, index); err != nil {
					return err
				}
			}
		}
		for _, patch := range part.BuddyPatches {
			if err := buddy.claim(patch.RowID, index); err != nil {
				return err
			}
		}
		// Two parts moving the colour switch would leave the file saying
		// whichever ran last, which is the one answer a switch may not give -
		// the same reason the stone recipe claims its whole table.
		for _, patch := range part.VfxPatches {
			if err := vfx.claim(patch.RowID, index); err != nil {
				return err
			}
		}
		for _, patch := range part.EffectPatches {
			if err := effects.claim(patch.RowID, index); err != nil {
				return err
			}
		}
		for _, patch := range part.PersonPatches {
			if err := persons.claim(patch.RowID, index); err != nil {
				return err
			}
		}
		for _, patch := range part.BuddyStoneRowPatches {
			if err := stone.claim(patch.RowID, index); err != nil {
				return err
			}
			rowRecipes[index] = true
		}
		// The all-row recipe claims every row of the stone table at once, so
		// there can be one of it and it cannot sit beside a per-row
		// restoration of the same table.
		if len(part.BuddyStonePatches) > 0 {
			if tableRecipe >= 0 {
				return fmt.Errorf("session operations %d and %d both rewrite the whole BuddyStoneParam table",
					tableRecipe+1, index+1)
			}
			tableRecipe = index
		}
		for _, insertion := range part.BuddyRowInsertions {
			if err := added.claim(insertion.RowID, index); err != nil {
				return err
			}
		}
		for _, source := range part.NpcCloneSources {
			if err := added.claim(source+cloneRowOffset, index); err != nil {
				return err
			}
			actorAdded[source+cloneRowOffset] = true
		}
		for _, source := range part.NpcThinkCloneSources {
			if err := added.claim(thinkTableKey(source+cloneRowOffset), index); err != nil {
				return err
			}
			actorAdded[thinkTableKey(source+cloneRowOffset)] = true
		}
		for _, source := range part.NpcSummonCopies {
			destination, err := summonCopyRow(source)
			if err != nil {
				return err
			}
			if err := added.claim(destination, index); err != nil {
				return err
			}
			actorAdded[destination] = true
		}
		for _, rowID := range part.BuddyRowRemovals {
			if err := removed.claim(rowID, index); err != nil {
				return err
			}
		}
		for _, rowID := range part.NpcCloneRemovals {
			if err := removed.claim(rowID, index); err != nil {
				return err
			}
			actorRemoved[rowID] = index
		}
		for _, rowID := range part.NpcThinkCloneRemovals {
			if err := removed.claim(thinkTableKey(rowID), index); err != nil {
				return err
			}
			actorRemoved[thinkTableKey(rowID)] = index
		}
		for _, patch := range part.NpcPatches {
			npcPatched[patch.RowID] = index
		}
		keys := make([]CharacterRowKey, 0, len(part.CharacterRows)+len(part.CharacterRowRemovals))
		for _, row := range part.CharacterRows {
			keys = append(keys, CharacterRowKey{row.Table, row.RowID})
		}
		for _, body := range part.CharacterBodies {
			keys = append(keys, CharacterRowKey{characterBodyTable, body.RowID})
		}
		keys = append(keys, part.CharacterRowRemovals...)
		for _, key := range keys {
			if previous, taken := characters[key]; taken {
				return fmt.Errorf("session operations %d and %d both describe %s row %d",
					previous+1, index+1, key.Table, key.RowID)
			}
			characters[key] = index
		}
		for _, rowID := range append(slices.Clone(part.OwnEffectInsertions), part.OwnEffectRemovals...) {
			if err := ownEffects.claim(rowID, index); err != nil {
				return err
			}
		}
		// One part wakes a monument afresh or puts it back, never two.
		for _, rowID := range slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(part.StoneInsertions), part.StoneRemovals...)))) {
			if err := monuments.claim(rowID, index); err != nil {
				return err
			}
		}
	}
	for rowRecipe := range rowRecipes {
		if tableRecipe >= 0 && rowRecipe != tableRecipe {
			return fmt.Errorf("session operation %d rewrites the whole BuddyStoneParam table while %d restores its rows one by one",
				tableRecipe+1, rowRecipe+1)
		}
	}

	// A row cannot both exist and not exist when the write ends, and a row on
	// its way out is not worth writing a value into. The save core checks the
	// result against the save itself either way; checked here, the failure can
	// name the two staged changes that disagree.
	for rowID, part := range removed.by {
		if other, alsoAdded := added.by[rowID]; alsoAdded {
			if _, actor := actorRemoved[rowID]; actor && actorAdded[rowID] {
				// Replaced: see the note on this function.
				continue
			}
			return fmt.Errorf("session operation %d adds %s and %d removes it",
				other+1, removed.describe(rowID), part+1)
		}
	}
	// A creature row on its way out - or being replaced by a clone written
	// whole by another part - is not a row to write a value into.
	for rowID, part := range npcPatched {
		if other, gone := actorRemoved[rowID]; gone {
			return fmt.Errorf("session operation %d removes NpcParam row %d and %d writes a value into it",
				other+1, rowID, part+1)
		}
	}
	for rowID, part := range buddy.by {
		if other, alsoRemoved := removed.by[rowID]; alsoRemoved {
			return fmt.Errorf("session operation %d removes BuddyParam row %d and %d writes a value into it",
				other+1, rowID, part+1)
		}
	}
	return nil
}

// flattenSession merges the validated parts into the parent, which is what the
// write then runs against.
//
// The parts are left in place rather than discarded. They are the record of
// what the player actually asked for, and the manifest is an audit artifact
// before it is an instruction: a flat union of forty patches says far less than
// three operations that produced them.
func flattenSession(plan *manifest) {
	for _, part := range plan.Parts {
		plan.GoodsPatches = append(plan.GoodsPatches, part.GoodsPatches...)
		plan.BuddyPatches = append(plan.BuddyPatches, part.BuddyPatches...)
		plan.BuddyStonePatches = append(plan.BuddyStonePatches, part.BuddyStonePatches...)
		plan.BuddyStoneRowPatches = append(plan.BuddyStoneRowPatches, part.BuddyStoneRowPatches...)
		plan.NpcCloneSources = append(plan.NpcCloneSources, part.NpcCloneSources...)
		// The strength list travels with the sources it modifies. Left out, a
		// player who staged two formations with the switch on would get one
		// part's worth of creatures and one part's worth of helpers - and the
		// difference would be invisible in the save until it was played.
		plan.NpcCloneStrength = append(plan.NpcCloneStrength, part.NpcCloneStrength...)
		plan.NpcCloneWorldBranch = append(plan.NpcCloneWorldBranch, part.NpcCloneWorldBranch...)
		plan.NpcCloneWorldBranchUnheld = append(plan.NpcCloneWorldBranchUnheld, part.NpcCloneWorldBranchUnheld...)
		plan.NpcCloneWeaponOut = append(plan.NpcCloneWeaponOut, part.NpcCloneWeaponOut...)
		plan.NpcSummonCopies = append(plan.NpcSummonCopies, part.NpcSummonCopies...)
		plan.NpcCloneTeams = append(plan.NpcCloneTeams, part.NpcCloneTeams...)
		plan.NpcPatches = append(plan.NpcPatches, part.NpcPatches...)
		plan.VfxPatches = append(plan.VfxPatches, part.VfxPatches...)
		plan.EffectPatches = append(plan.EffectPatches, part.EffectPatches...)
		plan.PersonPatches = append(plan.PersonPatches, part.PersonPatches...)
		plan.NpcThinkCloneSources = append(plan.NpcThinkCloneSources, part.NpcThinkCloneSources...)
		plan.NpcThinkPerception = append(plan.NpcThinkPerception, part.NpcThinkPerception...)
		plan.BuddyRowInsertions = append(plan.BuddyRowInsertions, part.BuddyRowInsertions...)
		plan.BuddyRowRemovals = append(plan.BuddyRowRemovals, part.BuddyRowRemovals...)
		plan.NpcCloneRemovals = append(plan.NpcCloneRemovals, part.NpcCloneRemovals...)
		plan.NpcThinkCloneRemovals = append(plan.NpcThinkCloneRemovals, part.NpcThinkCloneRemovals...)
		plan.CharacterRows = append(plan.CharacterRows, part.CharacterRows...)
		plan.CharacterBodies = append(plan.CharacterBodies, part.CharacterBodies...)
		plan.CharacterRowRemovals = append(plan.CharacterRowRemovals, part.CharacterRowRemovals...)
		plan.OwnEffectInsertions = append(plan.OwnEffectInsertions, part.OwnEffectInsertions...)
		plan.OwnEffectRemovals = append(plan.OwnEffectRemovals, part.OwnEffectRemovals...)
		plan.StoneInsertions = append(plan.StoneInsertions, part.StoneInsertions...)
		plan.StoneRemovals = append(plan.StoneRemovals, part.StoneRemovals...)
	}
}

// thinkTableKey moves an NpcThinkParam row out of NpcParam's numbering so one
// claim set can answer for both tables. The mapping never leaves this file; it
// exists only so two rows that share a number in different tables do not read
// as one row.
func thinkTableKey(rowID int32) int32 { return -rowID }

// rowClaims records which part claimed which row, so a second claim can name
// the first rather than only reporting that a duplicate exists.
type rowClaims struct {
	kind string
	by   map[int32]int
}

func newRowClaims(kind string) *rowClaims {
	return &rowClaims{kind: kind, by: make(map[int32]int)}
}

func (c *rowClaims) claim(rowID int32, part int) error {
	if previous, taken := c.by[rowID]; taken {
		return fmt.Errorf("session operations %d and %d both describe %s",
			previous+1, part+1, c.describe(rowID))
	}
	c.by[rowID] = part
	return nil
}

func (c *rowClaims) describe(rowID int32) string {
	if rowID < 0 {
		return fmt.Sprintf("NpcThinkParam %s %d", c.kind, -rowID)
	}
	return fmt.Sprintf("%s %d", c.kind, rowID)
}
