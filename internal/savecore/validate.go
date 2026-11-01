package savecore

// What a whole manifest has to be before any of it is written: one operation,
// a write surface that operation is allowed, and structural work only where
// structural work is permitted. The rules for a single row patch are in
// rowrules.go.

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

func validateManifest(plan *manifest) error {
	hash, err := hex.DecodeString(strings.TrimSpace(plan.SourceSHA256))
	if err != nil || len(hash) != 32 {
		return fmt.Errorf("source_sha256 must be a 64-character SHA-256 hex digest")
	}
	plan.SourceSHA256 = strings.ToLower(strings.TrimSpace(plan.SourceSHA256))
	switch plan.Operation {
	case "":
		plan.Operation = operationConvert
	case operationConvert, operationReset, operationAuthor, operationCost, operationStrength, operationArea, operationTint, operationPrune, operationSharedEffects, operationCharacters, operationOwnEffects:
	case operationSession:
		return validateSession(plan)
	default:
		return fmt.Errorf("unknown operation %q", plan.Operation)
	}
	// Only a session has parts. Refused rather than ignored: a part smuggled
	// into a single-operation manifest would reach the write through the
	// flattening below if this file ever grew one, and an unread field in an
	// audit artifact is a field that can say something untrue.
	if len(plan.Parts) != 0 {
		return fmt.Errorf("only a session manifest holds parts, and this one is a %s", plan.Operation)
	}
	return validateSingleOperation(plan)
}

// validateSingleOperation is the whole rule set for one operation. It is what a
// standalone manifest faces, and what each part of a session faces unchanged -
// which is the property that keeps a composition from relaxing anything.
//
// A session is never judged by it. validateManifest hands one to validateSession
// instead, and the flattened parent is deliberately not re-checked here: the
// flattened lists are the union of what the parts asked for, and no single
// operation describes a union. A conversion part and a cost part write different
// fields of one goods row, which is legitimate and which every rule below would
// have to be loosened to permit.
func validateSingleOperation(plan *manifest) error {
	if err := validateWriteSurface(plan); err != nil {
		return err
	}
	if err := validateStructuralWork(plan); err != nil {
		return err
	}
	if err := validateGoodsPatches(plan); err != nil {
		return err
	}
	if err := validateVfxPatches(plan); err != nil {
		return err
	}
	if err := validateEffectPatches(plan); err != nil {
		return err
	}
	if err := validatePersonPatches(plan); err != nil {
		return err
	}
	if err := validateCharacterRows(plan); err != nil {
		return err
	}
	if err := validateOwnEffects(plan); err != nil {
		return err
	}
	if err := validateMonumentRows(plan); err != nil {
		return err
	}
	if err := validateBuddyPatches(plan); err != nil {
		return err
	}
	if err := validateNpcPatches(plan); err != nil {
		return err
	}
	return validateStonePatches(plan)
}

// validateWriteSurface refuses a manifest that reaches into a table its
// operation does not own, before any value in it is looked at.
//
// The four operations are disjoint by design, and this is where that is said
// once instead of being implied by a scatter of later checks. A conversion
// writes goods rows and the shipping recipe. Authoring writes a formation block
// and the actor rows it needs, and no goods row at all, which is what leaves it
// structurally unable to reach enable_multi. A cost edit writes two named
// fields of a goods row and nothing else. A reset restores what any of them
// wrote and is alone in being allowed to remove anything.
//
// A conversion and a cost edit are the one pair that share a table, and they
// share it by field: validateGoodsPatches holds each to its own set, so neither
// can carry the other's work while claiming to be itself.
func validateWriteSurface(plan *manifest) error {
	additions := len(plan.NpcCloneSources) + len(plan.NpcThinkCloneSources) + len(plan.NpcSummonCopies) + len(plan.BuddyRowInsertions)
	removals := len(plan.BuddyRowRemovals) + len(plan.NpcCloneRemovals) + len(plan.NpcThinkCloneRemovals)
	switch plan.Operation {
	case operationConvert:
		// A conversion always writes both halves of the shipping recipe. Part
		// one without part two makes a formation spawn at a distant monument
		// and appear to vanish, so neither list may be empty.
		if len(plan.GoodsPatches) == 0 {
			return fmt.Errorf("a conversion needs at least one goods patch")
		}
		if len(plan.BuddyPatches) == 0 {
			return fmt.Errorf("a conversion needs the monument-origin BuddyParam fix")
		}
		// A conversion always says where both halves of the monument switch
		// stand: a half that is on rides on the one all-row patch, a half that
		// is off is put back row by row from the shipped snapshot
		// (validateStonePatches). Saying nothing would leave the table saying
		// whatever the last conversion said, which is the one answer a switch
		// may not give.
		if recipe, restore := len(plan.BuddyStonePatches), len(plan.BuddyStoneRowPatches); recipe > 1 || recipe+restore == 0 {
			return fmt.Errorf(
				"a conversion carries at most one all-row BuddyStone patch and shipped rows to restore, not %d and %d",
				recipe, restore)
		}
		if additions+removals != 0 {
			return fmt.Errorf("a conversion re-points shipped rows and may not add or remove any")
		}
	case operationArea:
		// The switch owns two tables and touches nothing else. In particular no
		// goods row: what an Ash costs and what it delivers are not the
		// switch's business, and a switch able to write them would be a switch
		// able to change which creature is summoned.
		if len(plan.GoodsPatches) != 0 {
			return fmt.Errorf("the monument switch writes no goods row")
		}
		// No floor on the BuddyParam rows. Every one of the twenty is also a
		// formation block, so a session whose formations are authored may have
		// claimed all of them - and authoring writes the same value the switch
		// does. What decides where a summon may be called is the stone table,
		// and that is checked below.

		if recipe, restore := len(plan.BuddyStonePatches), len(plan.BuddyStoneRowPatches); recipe > 1 || recipe+restore == 0 {
			return fmt.Errorf(
				"the monument switch carries at most one all-row BuddyStone patch and shipped rows to restore, not %d and %d",
				recipe, restore)
		}
		// The sleeping monuments are its only rows (validateMonumentRows).
		if additions+removals != 0 {
			return fmt.Errorf("the monument switch adds and removes no row but a sleeping monument")
		}
	case operationAuthor:
		// Authoring edits the host's own block, so there is always at least one
		// row patch: even a formation built entirely on inserted rows patches
		// the block's first row, which the game shipped.
		if len(plan.BuddyPatches) == 0 {
			return fmt.Errorf("an authored formation needs at least one BuddyParam row")
		}
		if len(plan.GoodsPatches) != 0 {
			return fmt.Errorf("authoring reaches every upgrade level through the shipped trigger and writes no goods row")
		}
		if len(plan.BuddyStonePatches)+len(plan.BuddyStoneRowPatches) != 0 {
			return fmt.Errorf("authoring writes BuddyParam and the actor tables, never BuddyStoneParam")
		}
		if removals != 0 {
			return fmt.Errorf("authoring may add rows and may not remove any; only a reset removes")
		}
	case operationCost:
		// The price is the whole edit. Every other list empty is what makes a
		// cost unable to re-point a formation while claiming to change a
		// number, which is the same disjointness the other three rely on.
		if len(plan.GoodsPatches) == 0 {
			return fmt.Errorf("a cost edit needs at least one goods patch")
		}
		if len(plan.BuddyPatches)+len(plan.BuddyStonePatches)+len(plan.BuddyStoneRowPatches) != 0 {
			return fmt.Errorf("a cost edit writes the goods price and nothing else")
		}
		if additions+removals != 0 {
			return fmt.Errorf("a cost edit changes values in shipped rows and may not add or remove any")
		}
	case operationTint:
		// One row, one field, and nothing else in the save. The colour switch
		// says how a summon renders; it names no Ash, no formation and no
		// actor, so every other list must be empty. validateVfxPatches has
		// already bounded the row and the value.
		if len(plan.VfxPatches) == 0 {
			return fmt.Errorf("the colour switch needs the row it writes")
		}
		if len(plan.GoodsPatches)+len(plan.BuddyPatches)+len(plan.NpcPatches) != 0 {
			return fmt.Errorf("the colour switch writes SpEffectVfxParam and nothing else")
		}
		if len(plan.BuddyStonePatches)+len(plan.BuddyStoneRowPatches) != 0 {
			return fmt.Errorf("the colour switch writes SpEffectVfxParam and nothing else")
		}
		if additions+removals != 0 {
			return fmt.Errorf("the colour switch writes one value into a row that exists and may not add or remove any")
		}
	case operationSharedEffects:
		// Shared effect and person fields and nothing else: it names no Ash,
		// no formation and no actor. validateEffectPatches and
		// validatePersonPatches have bounded rows and values.
		if len(plan.EffectPatches)+len(plan.PersonPatches) == 0 {
			return fmt.Errorf("a shared effect write needs the rows it writes")
		}
		if len(plan.GoodsPatches)+len(plan.BuddyPatches)+len(plan.NpcPatches)+len(plan.VfxPatches) != 0 {
			return fmt.Errorf("a shared effect write writes SpEffectParam and nothing else")
		}
		if len(plan.BuddyStonePatches)+len(plan.BuddyStoneRowPatches) != 0 {
			return fmt.Errorf("a shared effect write writes SpEffectParam and nothing else")
		}
		if additions+removals != 0 {
			return fmt.Errorf("a shared effect write writes values into rows that exist and may not add or remove any")
		}
	case operationStrength:
		// The switch writes ladders, summon rows, or both. All three lists
		// empty means it decided nothing and the manifest should not exist.
		if len(plan.BuddyPatches)+len(plan.NpcPatches) == 0 {
			return fmt.Errorf("a strength change needs at least one row to write")
		}
		if len(plan.GoodsPatches) != 0 {
			return fmt.Errorf("the strength switch writes no goods row")
		}
		if len(plan.BuddyStonePatches)+len(plan.BuddyStoneRowPatches) != 0 {
			return fmt.Errorf("the strength switch writes BuddyParam and NpcParam, never BuddyStoneParam")
		}
		if additions+removals != 0 {
			return fmt.Errorf("the strength switch writes values into rows that exist and may not add or remove any")
		}
	case operationReset:
		// A reset patches only rows that differ and removes only rows the
		// editor added, so which lists are populated depends on what the save
		// says. All of them empty means there was nothing to undo and the
		// manifest should never have been built.
		if len(plan.GoodsPatches)+len(plan.BuddyPatches)+len(plan.BuddyStoneRowPatches)+removals+len(plan.StoneRemovals) == 0 {
			return fmt.Errorf("a reset needs at least one row to restore or remove")
		}
		if len(plan.BuddyStonePatches) != 0 {
			return fmt.Errorf("a reset restores BuddyStone rows individually, never as one all-row recipe")
		}
		if additions != 0 {
			return fmt.Errorf("a reset undoes additions and may not make any")
		}
		// No requirement that a reset touch the stone table at all. That rule
		// held while the only thing a reset could undo was a conversion, which
		// always writes the expanded-area recipe. An authored formation writes
		// BuddyParam and nothing else, so its reset restores BuddyParam and
		// nothing else; the "at least one row" check above is what keeps an
		// empty manifest out.
	case operationPrune:
		// Clone removals and nothing else. No patch of any table: a prune
		// that could write a value would be a second way to change what a
		// formation summons. No BuddyParam removal: a formation row is the
		// authoring part's business, and a prune removes only what nothing
		// names. And no addition - which is what a reinserted row is, since a
		// replaced creature row arrives through the authoring part.
		if len(plan.NpcCloneRemovals)+len(plan.NpcThinkCloneRemovals) == 0 {
			return fmt.Errorf("a prune needs at least one creature row to remove")
		}
		if len(plan.GoodsPatches)+len(plan.BuddyPatches)+len(plan.NpcPatches)+len(plan.VfxPatches) != 0 {
			return fmt.Errorf("a prune removes rows and writes no value")
		}
		if len(plan.BuddyStonePatches)+len(plan.BuddyStoneRowPatches) != 0 {
			return fmt.Errorf("a prune removes rows and writes no value")
		}
		if len(plan.BuddyRowRemovals) != 0 {
			return fmt.Errorf("a prune removes creature rows and never a formation row")
		}
		if additions != 0 {
			return fmt.Errorf("a prune removes rows and may not add any")
		}
	}
	return nil
}

// validateStructuralWork checks every declaration that changes which rows
// exist, without opening the save.
//
// Two rules carry the weight here. An actor clone is named by its SOURCE, so
// the destination is arithmetic and a manifest has no way to ask for a row
// outside its source's model namespace. And a removal is refused below the
// clone offset, which is rule 1's "a world actor is not removable under any
// circumstances" written as a bound rather than as a sentence.
func validateStructuralWork(plan *manifest) error {
	if !structuralOperations[plan.Operation] {
		return nil
	}
	authorised := make(map[int32]bool, len(plan.NpcCloneSources))
	for _, source := range plan.NpcCloneSources {
		authorised[source] = true
	}
	seenStrength := make(map[int32]bool, len(plan.NpcCloneStrength))
	for _, entry := range plan.NpcCloneStrength {
		// The strength list modifies insertions; it cannot conjure one. A
		// source it names that is not being inserted would be a write to a row
		// this manifest never claimed, which is the shape of mistake rule 1
		// exists to make impossible.
		if !authorised[entry.Source] {
			return fmt.Errorf("npc_clone_strength names %d, which npc_clone_sources does not insert", entry.Source)
		}
		if seenStrength[entry.Source] {
			return fmt.Errorf("npc_clone_strength names %d twice", entry.Source)
		}
		if entry.HP < 0 {
			return fmt.Errorf("npc_clone_strength gives %d a base health of %d", entry.Source, entry.HP)
		}
		removed := make(map[int32]bool, len(entry.Removes))
		for _, effect := range entry.Removes {
			switch {
			case effect <= 0:
				return fmt.Errorf("npc_clone_strength removes %d from %d, which is not a row ID", effect, entry.Source)
			case removed[effect]:
				return fmt.Errorf("npc_clone_strength removes %d from %d twice", effect, entry.Source)
			case slices.Contains(entry.Residents, effect):
				return fmt.Errorf("npc_clone_strength both adds and removes %d on %d", effect, entry.Source)
			case slices.Contains(spiritCompanionEffects, effect) || effect == worldStayEffect:
				// The recipe puts these on; a removal that fought it would be
				// undone by the recipe, or worse, not undone.
				return fmt.Errorf("npc_clone_strength removes %d from %d, which the summon recipe requires", effect, entry.Source)
			}
			removed[effect] = true
		}
		switch {
		case entry.Presence < 0:
			return fmt.Errorf("npc_clone_strength gives %d the presence %d, which is not a row ID", entry.Source, entry.Presence)
		case entry.Presence > 0 && (entry.Presence == spiritBalanceEffect || entry.Presence == spiritFollowEffect || entry.Presence == worldStayEffect):
			return fmt.Errorf("npc_clone_strength gives %d the presence %d, which the summon recipe writes as itself", entry.Source, entry.Presence)
		case entry.Presence > 0 && (slices.Contains(entry.Residents, entry.Presence) || removed[entry.Presence]):
			return fmt.Errorf("npc_clone_strength gives %d the presence %d and also adds or removes it", entry.Source, entry.Presence)
		}
		switch {
		case entry.Marker < 0:
			return fmt.Errorf("npc_clone_strength gives %d the marker %d, which is not a row ID", entry.Source, entry.Marker)
		case entry.Marker > 0 && slices.Contains([]int32{spiritPresenceEffect, spiritBalanceEffect, spiritFollowEffect, worldStayEffect, entry.Presence}, entry.Marker):
			return fmt.Errorf("npc_clone_strength gives %d the marker %d, which the summon recipe writes as itself", entry.Source, entry.Marker)
		case entry.Marker > 0 && (slices.Contains(entry.Residents, entry.Marker) || removed[entry.Marker]):
			return fmt.Errorf("npc_clone_strength gives %d the marker %d and also adds or removes it", entry.Source, entry.Marker)
		}
		seenStrength[entry.Source] = true
	}
	seenBranch := make(map[int32]bool, len(plan.NpcCloneWorldBranch))
	for _, source := range plan.NpcCloneWorldBranch {
		// Like the strength list, it modifies an insertion and cannot conjure
		// one; and a shell copy is a summon already, so it is not offered.
		if !authorised[source] {
			return fmt.Errorf("npc_clone_world_branch names %d, which npc_clone_sources does not insert", source)
		}
		if seenBranch[source] {
			return fmt.Errorf("npc_clone_world_branch names %d twice", source)
		}
		seenBranch[source] = true
	}
	for _, entry := range plan.NpcCloneStrength {
		// The world branch takes 297000 off, so there is no marker to replace.
		if entry.Marker > 0 && seenBranch[entry.Source] {
			return fmt.Errorf("npc_clone_strength gives %d a marker, and the world branch carries none", entry.Source)
		}
	}
	seenWeaponOut := make(map[int32]bool, len(plan.NpcCloneWeaponOut))
	for _, entry := range plan.NpcCloneWeaponOut {
		// A modifier again: it names an insertion, once, and a think row the
		// writer reads the sheathed state from.
		if !authorised[entry.Source] {
			return fmt.Errorf("npc_clone_weapon_out names %d, which npc_clone_sources does not insert", entry.Source)
		}
		if seenWeaponOut[entry.Source] {
			return fmt.Errorf("npc_clone_weapon_out names %d twice", entry.Source)
		}
		if entry.Think <= 0 {
			return fmt.Errorf("npc_clone_weapon_out gives %d think row %d", entry.Source, entry.Think)
		}
		seenWeaponOut[entry.Source] = true
	}
	copied := make(map[int32]bool, len(plan.NpcSummonCopies))
	for _, source := range plan.NpcSummonCopies {
		// A copy is of a shipped shell and lands in the copy band; the
		// arithmetic refuses everything else, as the clone rule does for a
		// world row.
		if _, err := summonCopyRow(source); err != nil {
			return fmt.Errorf("npc_summon_copies: %w", err)
		}
		if copied[source] {
			return fmt.Errorf("npc_summon_copies names %d twice", source)
		}
		copied[source] = true
	}
	seenTeam := make(map[int32]bool, len(plan.NpcCloneTeams))
	for _, entry := range plan.NpcCloneTeams {
		// Like the strength list, the team list modifies an insertion and
		// cannot conjure one.
		if !authorised[entry.Source] && !copied[entry.Source] {
			return fmt.Errorf("npc_clone_teams names %d, which neither npc_clone_sources nor npc_summon_copies inserts", entry.Source)
		}
		if seenTeam[entry.Source] {
			return fmt.Errorf("npc_clone_teams names %d twice", entry.Source)
		}
		if _, err := summonTeam(entry.Team); err != nil || entry.Team == 0 {
			return fmt.Errorf("npc_clone_teams puts %d on team %d, which is not one of %v", entry.Source, entry.Team, summonTeams)
		}
		seenTeam[entry.Source] = true
	}
	for _, source := range plan.NpcSummonCopies {
		if !seenTeam[source] {
			return fmt.Errorf("npc_summon_copies copies %d but npc_clone_teams gives the copy no team, and a copy on the same team is the source", source)
		}
	}
	for _, sourceList := range []struct {
		label   string
		sources []int32
	}{
		{"npc_clone_sources", plan.NpcCloneSources},
		{"npc_think_clone_sources", plan.NpcThinkCloneSources},
	} {
		seen := make(map[int32]bool, len(sourceList.sources))
		for _, source := range sourceList.sources {
			if _, ok := cloneKeySource(source); !ok {
				return fmt.Errorf("%s names %d, which is not a clone key (a world actor row, in a clone band)", sourceList.label, source)
			}
			if seen[source] {
				return fmt.Errorf("%s names %d twice", sourceList.label, source)
			}
			seen[source] = true
		}
	}
	insertedRows := make(map[int32]bool, len(plan.BuddyRowInsertions))
	for index, insertion := range plan.BuddyRowInsertions {
		switch {
		case insertion.RowID <= 0:
			return fmt.Errorf("buddy insertion %d has invalid row ID %d", index+1, insertion.RowID)
		case insertion.CopyFrom <= 0:
			return fmt.Errorf("buddy insertion %d must copy an existing row, not %d", index+1, insertion.CopyFrom)
		case insertion.InsertAfter <= 0:
			// Placement is part of the operation. A block is a contiguous run
			// of headers, so an insertion that did not say where it goes could
			// break the formation it is extending without changing one value.
			return fmt.Errorf("buddy insertion %d must name the row it follows", index+1)
		case insertion.RowID == insertion.CopyFrom:
			return fmt.Errorf("buddy insertion %d would copy row %d onto itself", index+1, insertion.RowID)
		case insertedRows[insertion.RowID]:
			return fmt.Errorf("buddy insertion %d adds row %d twice", index+1, insertion.RowID)
		}
		insertedRows[insertion.RowID] = true
	}
	for _, removalList := range []struct {
		label    string
		removals []int32
	}{
		{"npc_clone_removals", plan.NpcCloneRemovals},
		{"npc_think_clone_removals", plan.NpcThinkCloneRemovals},
	} {
		seen := make(map[int32]bool, len(removalList.removals))
		for _, rowID := range removalList.removals {
			if !inCloneBand(rowID) {
				return fmt.Errorf("%s names %d, which is outside the clone bands and is not removable",
					removalList.label, rowID)
			}
			if seen[rowID] {
				return fmt.Errorf("%s names %d twice", removalList.label, rowID)
			}
			seen[rowID] = true
		}
	}
	removedBuddy := make(map[int32]bool, len(plan.BuddyRowRemovals))
	for _, rowID := range plan.BuddyRowRemovals {
		if rowID <= 0 {
			return fmt.Errorf("buddy_row_removals names invalid row %d", rowID)
		}
		if removedBuddy[rowID] {
			return fmt.Errorf("buddy_row_removals names %d twice", rowID)
		}
		removedBuddy[rowID] = true
	}
	// Restoring a row and deleting it are contradictory instructions about one
	// row, and the manifest is the audit record of what was done. It has to be
	// readable as a single intent.
	for _, patch := range plan.BuddyPatches {
		if removedBuddy[patch.RowID] {
			return fmt.Errorf("BuddyParam row %d is both restored and removed", patch.RowID)
		}
	}
	return nil
}
