package savecore

// The structural stage: the part of applying a manifest that changes which
// rows exist, rather than what is inside one.
//
// It runs before every patch, and it is separated from them because the two
// cannot share a proof. A patch is proved by comparing byte windows before and
// after, which requires the two buffers to be the same length and every address
// to mean the same thing in both. An insertion or a removal breaks exactly that
// assumption: the bytes that move are unchanged in content and different in
// address, so a window comparison would report the whole tail of the archive as
// damage.
//
// So the stage carries its own proof, stated over row IDs instead of offsets,
// and it is strictly the stronger claim: every row that was there before is
// still there and byte-identical, the only rows that appeared are the ones the
// manifest declared, and the only rows that vanished are the ones it declared.
// That is docs/SAFETY.md rule 1's acceptance test, applied to every table this
// product can open rather than only to the ones the manifest happened to name.
//
// regulation.InsertRows and RemoveRows each verify their own table already. This verifies
// the composition of them, which is the thing a manifest can get wrong: a
// removal in one table paired with an insertion in another leaves each
// operation's own proof satisfied and can still describe a save nobody intended.

import (
	"crypto/sha256"
	"fmt"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// structuralTables are every table whose row set this product proves. It is not
// the set of tables a manifest may change - that is narrower, and enforced in
// validateStructuralWork - but the set a manifest must not change by accident.
var structuralTables = []regulation.TableLayout{
	goodsLayout, buddyLayout, stoneLayout, npcLayout, npcThinkLayout,
	charaInitLayout, faceLayout, customWeaponLayout, effectLayout,
}

// hasStructuralWork reports whether this manifest changes any table's row set.
func (plan *manifest) hasStructuralWork() bool {
	return plan.hasActorOrFormationRowWork() || plan.hasCharacterStructure() || plan.hasOwnEffectWork() ||
		plan.hasMonumentWork()
}

// declaredRowSetChange is what a manifest says a table's row set will do.
type declaredRowSetChange struct {
	added   map[int32]bool
	removed map[int32]bool
}

// declaredRowSetChanges resolves the manifest into one entry per table.
//
// Actor clones are resolved from source to destination here, which is the only
// place in the write path that arithmetic happens, and it happens once. The
// verifier and the writer therefore cannot disagree about which row an
// insertion produced: planActorClones derives the same destination from the
// same source, and if either changed independently the verifier would report an
// undeclared row rather than passing.
func declaredRowSetChanges(plan *manifest) map[string]declaredRowSetChange {
	changes := map[string]declaredRowSetChange{}
	forTable := func(layout regulation.TableLayout) declaredRowSetChange {
		change, present := changes[layout.Entry]
		if !present {
			change = declaredRowSetChange{added: map[int32]bool{}, removed: map[int32]bool{}}
			changes[layout.Entry] = change
		}
		return change
	}
	for _, source := range plan.NpcCloneSources {
		forTable(npcLayout).added[source+cloneRowOffset] = true
	}
	for _, source := range plan.NpcThinkCloneSources {
		forTable(npcThinkLayout).added[source+cloneRowOffset] = true
	}
	for _, source := range plan.NpcSummonCopies {
		// validateStructuralWork has already refused a source outside the
		// shells' windows, so the copy row always exists here.
		destination, _ := summonCopyRow(source)
		forTable(npcLayout).added[destination] = true
	}
	for _, insertion := range plan.BuddyRowInsertions {
		forTable(buddyLayout).added[insertion.RowID] = true
	}
	for _, rowID := range plan.BuddyRowRemovals {
		forTable(buddyLayout).removed[rowID] = true
	}
	for _, rowID := range plan.NpcCloneRemovals {
		forTable(npcLayout).removed[rowID] = true
	}
	for _, rowID := range plan.NpcThinkCloneRemovals {
		forTable(npcThinkLayout).removed[rowID] = true
	}
	for name, rows := range plan.characterInsertions() {
		layout, _, _, _ := layoutOf(name)
		for _, row := range rows {
			forTable(layout).added[row.rowID] = true
		}
	}
	for _, key := range plan.CharacterRowRemovals {
		layout, _, _, _ := layoutOf(key.Table)
		forTable(layout).removed[key.RowID] = true
	}
	for _, rowID := range plan.OwnEffectInsertions {
		forTable(effectLayout).added[rowID] = true
	}
	for _, rowID := range plan.OwnEffectRemovals {
		forTable(effectLayout).removed[rowID] = true
	}
	for _, rowID := range plan.StoneInsertions {
		forTable(stoneLayout).added[rowID] = true
	}
	for _, rowID := range plan.StoneRemovals {
		forTable(stoneLayout).removed[rowID] = true
	}
	return changes
}

// applyStructuralChanges returns a new regulation with the manifest's row-set
// work done. The input slice is never modified.
//
// REMOVALS RUN FIRST. A single operation still never both
// adds and removes (validateWriteSurface), but a session may carry a prune or
// a reset beside an authoring part, and a creature row one removes and the
// other inserts is REPLACED (validateSessionComposition): the old row has to be
// gone before its number can be inserted again, which planActorClones refuses
// otherwise. Removing first changes nothing for any other composition - the
// rows removed and the rows inserted are then different rows.
//
// Among the removals the order is formation rows first: a BuddyParam row
// naming a clone outlives that clone for no instant if the row goes first.
// Nothing is published mid-stage, so this is tidiness rather than safety, but
// a stage that is tidy is one whose intermediate states can be inspected when
// something does go wrong.
func applyStructuralChanges(bnd4 []byte, plan *manifest) ([]byte, error) {
	current := bnd4
	var err error

	if len(plan.BuddyRowRemovals) > 0 {
		if current, err = regulation.RemoveRows(current, buddyLayout, plan.BuddyRowRemovals); err != nil {
			return nil, err
		}
	}
	if len(plan.NpcCloneRemovals) > 0 {
		if current, err = regulation.RemoveRows(current, npcLayout, plan.NpcCloneRemovals); err != nil {
			return nil, err
		}
	}
	if len(plan.NpcThinkCloneRemovals) > 0 {
		if current, err = regulation.RemoveRows(current, npcThinkLayout, plan.NpcThinkCloneRemovals); err != nil {
			return nil, err
		}
	}

	// The character rows are in tables no other work touches, so where they
	// run among the rest changes nothing; their removals and insertions both
	// happen inside.
	if plan.hasCharacterStructure() {
		if current, err = applyCharacterStructure(current, plan); err != nil {
			return nil, err
		}
	}

	// The effect rows of our own before any clone, so a clone that names one
	// is written into a save that holds it.
	if plan.hasOwnEffectWork() {
		if current, err = applyOwnEffects(current, plan); err != nil {
			return nil, err
		}
	}
	// The monuments are a table nothing else here touches.
	if plan.hasMonumentWork() {
		if current, err = applyMonumentRows(current, plan); err != nil {
			return nil, err
		}
	}

	recipes := cloneRecipesOf(plan)
	if len(plan.NpcCloneSources) > 0 {
		if current, err = insertNpcClones(current, plan.NpcCloneSources, recipes.strength, recipes.teams, recipes.worldBranch, recipes.unheld, recipes.weaponOut); err != nil {
			return nil, err
		}
	}
	if len(plan.NpcSummonCopies) > 0 {
		if current, err = insertSummonCopies(current, plan.NpcSummonCopies, recipes.teams); err != nil {
			return nil, err
		}
	}
	if len(plan.NpcThinkCloneSources) > 0 {
		if current, err = insertNpcThinkClones(current, plan.NpcThinkCloneSources, recipes.perception); err != nil {
			return nil, err
		}
	}
	if len(plan.BuddyRowInsertions) > 0 {
		insertions := make([]regulation.RowInsertion, 0, len(plan.BuddyRowInsertions))
		for _, insertion := range plan.BuddyRowInsertions {
			anchor := insertion.InsertAfter
			insertions = append(insertions, regulation.RowInsertion{
				RowID: insertion.RowID, CopyFrom: insertion.CopyFrom, InsertAfter: &anchor,
			})
		}
		if current, err = regulation.InsertRows(current, buddyLayout, insertions); err != nil {
			return nil, err
		}
	}
	return current, nil
}

// verifyStructuralStage proves the composition of the stage against every table
// this product can open.
//
// An inserted row's content is deliberately not checked here. InsertRows
// already proved it is a byte copy of its source, and insertActorClones already
// proved the summon recipe written over it stayed inside it; re-stating either
// would be a second opinion from the same evidence. What is checked is the
// claim neither of those can make: that nothing else in the archive gained or
// lost a row.
func verifyStructuralStage(before, after []byte, plan *manifest) error {
	declared := declaredRowSetChanges(plan)
	for _, layout := range structuralTables {
		change := declared[layout.Entry]
		beforeRows, err := tableRowDigests(before, layout)
		if err != nil {
			return err
		}
		afterRows, err := tableRowDigests(after, layout)
		if err != nil {
			return err
		}
		for rowID, digest := range beforeRows {
			current, present := afterRows[rowID]
			switch {
			case change.removed[rowID] && change.added[rowID]:
				// Replaced (validateSessionComposition): the old row went and a
				// new one took its number. Its presence is checked with the
				// other insertions below; its content is InsertRows' proof and
				// the recipe's, as for any inserted clone.
			case change.removed[rowID]:
				if present {
					return fmt.Errorf("%s row %d was declared removed and is still there", layout.Entry, rowID)
				}
			case !present:
				return fmt.Errorf("%s row %d vanished and no removal declared it", layout.Entry, rowID)
			case current != digest:
				return fmt.Errorf("%s row %d changed during the structural stage", layout.Entry, rowID)
			}
		}
		for rowID := range afterRows {
			if _, existed := beforeRows[rowID]; existed {
				continue
			}
			if !change.added[rowID] {
				return fmt.Errorf("%s row %d appeared and no insertion declared it", layout.Entry, rowID)
			}
		}
		for rowID := range change.added {
			if _, present := afterRows[rowID]; !present {
				return fmt.Errorf("%s row %d was declared inserted and is not there", layout.Entry, rowID)
			}
		}
		for rowID := range change.removed {
			if _, existed := beforeRows[rowID]; !existed {
				return fmt.Errorf("%s row %d was declared removed and was never there: %w",
					layout.Entry, rowID, ErrRowNotFound)
			}
		}
	}
	return nil
}

// tableRowDigests reads one table as a row ID to content digest map, which is
// the representation every claim in this file is stated over. Comparing by ID
// rather than by offset is the whole point: an insertion moves rows it does not
// change, and a digest map is blind to exactly that.
func tableRowDigests(bnd4 []byte, layout regulation.TableLayout) (map[int32][32]byte, error) {
	table, err := regulation.OpenTable(bnd4, layout)
	if err != nil {
		return nil, err
	}
	digests := make(map[int32][32]byte, len(table.Rows))
	for _, row := range table.Rows {
		digests[row.ID] = sha256.Sum256(table.Data[row.DataOffset : row.DataOffset+layout.RowSize])
	}
	return digests, nil
}

// cloneRecipes is everything a manifest says about how its clones are written,
// by clone key: the one place the manifest's lists become the writer's maps, so
// inserting a clone and recognising one the save already holds (MatchClones)
// read the manifest the same way.
type cloneRecipes struct {
	teams       map[int32]uint8
	strength    map[int32]npcCloneStrength
	worldBranch map[int32]bool
	unheld      map[int32]bool
	weaponOut   map[int32]int32
	perception  map[int32]thinkChoice
}

func cloneRecipesOf(plan *manifest) cloneRecipes {
	recipes := cloneRecipes{
		teams:       make(map[int32]uint8, len(plan.NpcCloneTeams)),
		strength:    make(map[int32]npcCloneStrength, len(plan.NpcCloneStrength)),
		worldBranch: make(map[int32]bool, len(plan.NpcCloneWorldBranch)),
		unheld:      make(map[int32]bool, len(plan.NpcCloneWorldBranchUnheld)),
		weaponOut:   make(map[int32]int32, len(plan.NpcCloneWeaponOut)),
		perception:  make(map[int32]thinkChoice, len(plan.NpcThinkPerception)),
	}
	for _, entry := range plan.NpcCloneTeams {
		recipes.teams[entry.Source] = entry.Team
	}
	for _, entry := range plan.NpcCloneStrength {
		recipes.strength[entry.Source] = entry
	}
	for _, source := range plan.NpcCloneWorldBranch {
		recipes.worldBranch[source] = true
	}
	for _, source := range plan.NpcCloneWorldBranchUnheld {
		recipes.unheld[source] = true
	}
	for _, entry := range plan.NpcCloneWeaponOut {
		recipes.weaponOut[entry.Source] = entry.Think
	}
	for _, entry := range plan.NpcThinkPerception {
		recipes.perception[entry.Source] = thinkChoice{Level: entry.Level, Body: entry.Body, Front: entry.Front,
			WorldBranch: entry.WorldBranch, NoTurn: entry.NoTurn, OwnTarget: entry.OwnTarget}
	}
	return recipes
}
