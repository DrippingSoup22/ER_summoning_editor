package savecore

// The player's own characters, summoned as people: rows of our own in four
// tables.
//
// A character from a save slot is a person like any other - a person shell
// wearing a CharaInitParam row - but no row is one the game ships. It is built
// from the slot when the save is written: a CharaInitParam row, a FaceParam row
// for the face and an EquipParamCustomWeapon row for each weapon carrying an
// Ash of War of its own (internal/character rows.go), all written whole, and a
// body - a copy of the person shell carrying the character's own health and
// active effects, which a person's NpcParam row supplies. These are new rows
// that nothing in the world references, which docs/SAFETY.md says needs no
// exception; what keeps them that is the bands below. A character row lives at
// 2990000-2990199 in its three tables and a body at 199900000-199900199 in
// NpcParam - the first hundred of each a slot's, the second a built
// character's (internal/character build.go); ids every supported regulation
// leaves free, the bodies cut out of the clone bands (inCloneBand) - and this
// operation writes nothing outside them and nothing else at all.
//
// The operation inserts a row the save lacks, rewrites one it holds (a
// character changes what they wear between two saves) and removes one no
// formation will name. An insertion is a copy - of the row it is anchored
// after, or a body's of its shell - written over in the patch stage, so each
// half keeps its own proof as a clone's does.

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// operationCharacters writes the character rows and nothing else.
const operationCharacters = "characters"

// The two character bands.
const (
	characterRowFloor    = 2990000
	characterRowCeiling  = 2990200
	characterBodyFloor   = 199900000
	characterBodyCeiling = 199900200
	characterBodyTable   = "NpcParam"
)

var (
	faceLayout         = regulation.MustLayout("FaceParam")
	customWeaponLayout = regulation.MustLayout("EquipParamCustomWeapon")
)

// characterTables are the tables a whole character row may be in, by the name
// a manifest uses. A body is in NpcParam and is written apart.
var characterTables = map[string]regulation.TableLayout{
	"CharaInitParam":         charaInitLayout,
	"FaceParam":              faceLayout,
	"EquipParamCustomWeapon": customWeaponLayout,
}

// characterRow is one character row, whole. Insert says the save does not hold
// it yet: the manifest declares every row it adds, as it does a clone.
type characterRow struct {
	Table  string `json:"table"`
	RowID  int32  `json:"row_id"`
	Insert bool   `json:"insert,omitempty"`
	// Data is the row's bytes, hex-encoded, exactly the table's row size.
	Data string `json:"data"`
}

// characterBody is one character's body: a copy of a person shell with the
// character's health and active effects on it. Its bytes are not in the
// manifest: the shell is copied as the save holds it when the patch stage runs,
// after a role chosen for the shell has written it, so a body always matches
// its shell.
type characterBody struct {
	RowID   int32   `json:"row_id"`
	Shell   int32   `json:"shell"`
	Insert  bool    `json:"insert,omitempty"`
	HP      int32   `json:"hp"`
	Effects []int32 `json:"effects,omitempty"`
	// OwnColours draws the body in the character's own colours instead of
	// its Puppet shell's coat, and Fighter writes it without the balance
	// package (CharacterBody).
	OwnColours bool `json:"own_colours,omitempty"`
	Fighter    bool `json:"fighter,omitempty"`
}

// CharacterRowKey names one character row: a body's table is NpcParam.
type CharacterRowKey struct {
	Table string `json:"table"`
	RowID int32  `json:"row_id"`
}

// layoutOf is a character table's layout and band.
func layoutOf(table string) (regulation.TableLayout, int32, int32, bool) {
	if table == characterBodyTable {
		return npcLayout, characterBodyFloor, characterBodyCeiling, true
	}
	layout, known := characterTables[table]
	return layout, characterRowFloor, characterRowCeiling, known
}

// validateCharacterRows holds the character lists to their operation and
// their bands, before the save is opened.
func validateCharacterRows(plan *manifest) error {
	written := len(plan.CharacterRows) + len(plan.CharacterBodies) + len(plan.CharacterRowRemovals)
	if plan.Operation != operationCharacters {
		if written != 0 {
			return fmt.Errorf("operation %q may not write a character row: %w", plan.Operation, ErrManifestInvalid)
		}
		return nil
	}
	if written == 0 {
		return fmt.Errorf("a character write needs the rows it writes or removes")
	}
	if len(plan.GoodsPatches)+len(plan.BuddyPatches)+len(plan.NpcPatches)+len(plan.VfxPatches)+
		len(plan.EffectPatches)+len(plan.PersonPatches)+len(plan.BuddyStonePatches)+len(plan.BuddyStoneRowPatches) != 0 {
		return fmt.Errorf("a character write writes character rows and nothing else")
	}
	if plan.hasActorOrFormationRowWork() {
		return fmt.Errorf("a character write adds and removes character rows and no other row")
	}
	seen := map[CharacterRowKey]bool{}
	check := func(key CharacterRowKey) error {
		_, floor, ceiling, known := layoutOf(key.Table)
		switch {
		case !known:
			return fmt.Errorf("%q is not a table a character row may be in", key.Table)
		case key.RowID < floor || key.RowID >= ceiling:
			return fmt.Errorf("%s row %d is outside the character band %d-%d", key.Table, key.RowID, floor, ceiling-1)
		case seen[key]:
			return fmt.Errorf("%s row %d is named twice", key.Table, key.RowID)
		}
		seen[key] = true
		return nil
	}
	for _, row := range plan.CharacterRows {
		if row.Table == characterBodyTable {
			return fmt.Errorf("a body is written as a body, not as a row")
		}
		if err := check(CharacterRowKey{row.Table, row.RowID}); err != nil {
			return err
		}
		data, err := hex.DecodeString(row.Data)
		if err != nil || len(data) != characterTables[row.Table].RowSize {
			return fmt.Errorf("%s row %d carries %d bytes, not the table's %d",
				row.Table, row.RowID, len(row.Data)/2, characterTables[row.Table].RowSize)
		}
	}
	for _, body := range plan.CharacterBodies {
		if err := check(CharacterRowKey{characterBodyTable, body.RowID}); err != nil {
			return err
		}
		// A body copies a person shell, which the copy-band arithmetic
		// recognises and refuses everything else.
		if _, err := summonCopyRow(body.Shell); err != nil {
			return fmt.Errorf("body %d: %w", body.RowID, err)
		}
		if body.HP <= 0 {
			return fmt.Errorf("body %d is given %d health", body.RowID, body.HP)
		}
		for _, effect := range body.Effects {
			if effect <= 0 {
				return fmt.Errorf("body %d is given effect %d, which is not a row ID", body.RowID, effect)
			}
		}
	}
	for _, key := range plan.CharacterRowRemovals {
		if err := check(key); err != nil {
			return err
		}
	}
	return nil
}

// hasActorOrFormationRowWork reports whether a manifest adds or removes any
// row outside the character bands.
func (plan *manifest) hasActorOrFormationRowWork() bool {
	return len(plan.NpcCloneSources)+len(plan.NpcThinkCloneSources)+len(plan.NpcSummonCopies)+
		len(plan.BuddyRowInsertions)+len(plan.BuddyRowRemovals)+len(plan.NpcCloneRemovals)+
		len(plan.NpcThinkCloneRemovals) > 0
}

// characterInsertion is one character row to add and the row it copies; zero
// copies the row it is anchored after.
type characterInsertion struct {
	rowID, copyFrom int32
}

// characterInsertions and characterRemovals are the character lists by table,
// in id order.
func (plan *manifest) characterInsertions() map[string][]characterInsertion {
	out := map[string][]characterInsertion{}
	for _, row := range plan.CharacterRows {
		if row.Insert {
			out[row.Table] = append(out[row.Table], characterInsertion{rowID: row.RowID})
		}
	}
	for _, body := range plan.CharacterBodies {
		if body.Insert {
			out[characterBodyTable] = append(out[characterBodyTable], characterInsertion{rowID: body.RowID, copyFrom: body.Shell})
		}
	}
	for _, rows := range out {
		slices.SortFunc(rows, func(a, b characterInsertion) int { return int(a.rowID) - int(b.rowID) })
	}
	return out
}

func (plan *manifest) characterRemovals() map[string][]int32 {
	out := map[string][]int32{}
	for _, key := range plan.CharacterRowRemovals {
		out[key.Table] = append(out[key.Table], key.RowID)
	}
	for _, ids := range out {
		slices.Sort(ids)
	}
	return out
}

// hasCharacterStructure reports whether the character lists add or remove a
// row.
func (plan *manifest) hasCharacterStructure() bool {
	return len(plan.characterInsertions())+len(plan.CharacterRowRemovals) > 0
}

// applyCharacterStructure removes and inserts the character rows. Each
// insertion is anchored after the largest id below it, or the character row
// inserted just before it, which keeps the table in id order; the patch stage
// then writes the row.
func applyCharacterStructure(bnd4 []byte, plan *manifest) ([]byte, error) {
	current := bnd4
	var err error
	removals := plan.characterRemovals()
	for _, name := range sortedKeys(removals) {
		layout, _, _, _ := layoutOf(name)
		if current, err = regulation.RemoveRows(current, layout, removals[name]); err != nil {
			return nil, err
		}
	}
	insertions := plan.characterInsertions()
	for _, name := range sortedKeys(insertions) {
		layout, _, _, _ := layoutOf(name)
		table, err := regulation.OpenTable(current, layout)
		if err != nil {
			return nil, err
		}
		present := make([]int32, len(table.Rows))
		for index, row := range table.Rows {
			present[index] = row.ID
		}
		slices.Sort(present)
		anchored := make([]regulation.RowInsertion, 0, len(insertions[name]))
		var previous int32
		for _, one := range insertions[name] {
			below := sort.Search(len(present), func(index int) bool { return present[index] >= one.rowID })
			if below == 0 {
				return nil, fmt.Errorf("%s has no row below %d to anchor it after", layout.Entry, one.rowID)
			}
			anchor := present[below-1]
			copyFrom := one.copyFrom
			if copyFrom == 0 {
				copyFrom = anchor
			}
			if anchor < previous {
				anchor = previous
			}
			anchored = append(anchored, regulation.RowInsertion{RowID: one.rowID, CopyFrom: copyFrom, InsertAfter: &anchor})
			previous = one.rowID
		}
		if current, err = regulation.InsertRows(current, layout, anchored); err != nil {
			return nil, err
		}
	}
	return current, nil
}

// applyCharacterRows writes every character row whole, inserted or held.
func applyCharacterRows(bnd4 []byte, rows []characterRow) ([]byteRange, error) {
	ranges := make([]byteRange, 0, len(rows))
	for _, one := range rows {
		table, err := regulation.OpenTable(bnd4, characterTables[one.Table])
		if err != nil {
			return nil, err
		}
		row, ok := table.ByID[one.RowID]
		if !ok {
			return nil, fmt.Errorf("%s row %d was not found: %w", one.Table, one.RowID, ErrRowNotFound)
		}
		data, _ := hex.DecodeString(one.Data) // validateCharacterRows decoded it
		copy(table.Data[row.DataOffset:row.DataOffset+table.Layout.RowSize], data)
		ranges = append(ranges, rowRange(table, row))
	}
	return ranges, nil
}

// applyCharacterBodies writes every body from its shell as the save holds it
// now - after the NpcParam patches, so a body follows a role chosen for its
// shell in the same commit.
func applyCharacterBodies(bnd4 []byte, bodies []characterBody) ([]byteRange, error) {
	if len(bodies) == 0 {
		return nil, nil
	}
	table, err := regulation.OpenTable(bnd4, npcLayout)
	if err != nil {
		return nil, err
	}
	ranges := make([]byteRange, 0, len(bodies))
	for _, body := range bodies {
		shell, ok := table.ByID[body.Shell]
		if !ok {
			return nil, fmt.Errorf("person shell %d was not found: %w", body.Shell, ErrRowNotFound)
		}
		row, ok := table.ByID[body.RowID]
		if !ok {
			return nil, fmt.Errorf("body %d was not found: %w", body.RowID, ErrRowNotFound)
		}
		written, err := CharacterBody(table.Data[shell.DataOffset:shell.DataOffset+npcLayout.RowSize], body.HP, body.Effects,
			body.OwnColours, body.Fighter)
		if err != nil {
			return nil, fmt.Errorf("body %d: %w", body.RowID, err)
		}
		copy(table.Data[row.DataOffset:row.DataOffset+npcLayout.RowSize], written)
		ranges = append(ranges, rowRange(table, row))
	}
	return ranges, nil
}

// The Puppet shells' coat: 295200 is the spirit
// presence 295000 drawn with the Puppets' own profile, and 295201 two particle
// effects of theirs, with no state of their own.
const (
	puppetPresence  = int32(295200)
	puppetParticles = int32(295201)
)

// CharacterBody is a character's body as written: the shell's row with the
// character's health, and each active effect resident in an empty slot. An
// effect the shell already carries is not added twice. In its own colours the
// body carries OwnColoursPresence in place of the shell's coat and none of
// its particles: the same presence, with no overwrite (a character is a puppet or in its own colours, nothing between).
// The role is the character's own, whatever the shell's: the Fighter goes
// without the balance package, as a clone does, and the Helper carries it even
// where a role chosen for the shell took it off.
func CharacterBody(shell []byte, hp int32, effects []int32, ownColours, fighter bool) ([]byte, error) {
	if len(shell) != npcLayout.RowSize {
		return nil, fmt.Errorf("a shell row is %d bytes, not %d", len(shell), npcLayout.RowSize)
	}
	body := bytes.Clone(shell)
	writeS32(body, npcHPOffset, hp)
	held := make([]int32, len(npcResidentEffectOffsets))
	for index, offset := range npcResidentEffectOffsets {
		held[index] = readS32(body, offset)
	}
	switch balance := slotOf(held, spiritBalanceEffect); {
	case fighter && balance >= 0:
		held[balance] = -1
		writeS32(body, npcResidentEffectOffsets[balance], -1)
	case !fighter && balance < 0:
		effects = append([]int32{spiritBalanceEffect}, effects...)
	}
	if ownColours {
		coat := slotOf(held, puppetPresence)
		if coat < 0 {
			return nil, fmt.Errorf("the shell carries no puppet coat %d to draw over", puppetPresence)
		}
		held[coat] = OwnColoursPresence
		writeS32(body, npcResidentEffectOffsets[coat], OwnColoursPresence)
		if particles := slotOf(held, puppetParticles); particles >= 0 {
			held[particles] = -1
			writeS32(body, npcResidentEffectOffsets[particles], -1)
		}
	}
	for _, effect := range effects {
		if slotOf(held, effect) >= 0 {
			continue
		}
		empty := slotOfEmpty(held)
		if empty < 0 {
			return nil, fmt.Errorf("no empty resident slot for effect %d", effect)
		}
		held[empty] = effect
		writeS32(body, npcResidentEffectOffsets[empty], effect)
	}
	return body, nil
}

// readCharacterRows is every row the save holds in the character bands.
func readCharacterRows(bnd4 []byte) (map[CharacterRowKey][]byte, error) {
	out := map[CharacterRowKey][]byte{}
	for _, name := range append(sortedKeys(map[string][]int32{"CharaInitParam": nil, "FaceParam": nil,
		"EquipParamCustomWeapon": nil}), characterBodyTable) {
		layout, floor, ceiling, _ := layoutOf(name)
		table, err := regulation.OpenTable(bnd4, layout)
		if err != nil {
			return nil, err
		}
		for _, row := range table.Rows {
			if row.ID >= floor && row.ID < ceiling {
				out[CharacterRowKey{name, row.ID}] = bytes.Clone(table.Data[row.DataOffset : row.DataOffset+layout.RowSize])
			}
		}
	}
	return out, nil
}

// readPersonShells is every person shell row the save holds - the rows a body
// may copy - so the application can say what a body would be.
func readPersonShells(bnd4 []byte) (map[int32][]byte, error) {
	table, err := regulation.OpenTable(bnd4, npcLayout)
	if err != nil {
		return nil, err
	}
	out := map[int32][]byte{}
	for _, row := range table.Rows {
		if _, err := summonCopyRow(row.ID); err == nil {
			out[row.ID] = bytes.Clone(table.Data[row.DataOffset : row.DataOffset+npcLayout.RowSize])
		}
	}
	return out, nil
}

func countIDs[T any](byTable map[string][]T) int {
	count := 0
	for _, ids := range byTable {
		count += len(ids)
	}
	return count
}

func sortedKeys[T any](byTable map[string][]T) []string {
	names := make([]string, 0, len(byTable))
	for name := range byTable {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
