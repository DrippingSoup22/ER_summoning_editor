package reference

import (
	"encoding/json"
	"fmt"
	"slices"
)

// VanillaSnapshotFile is the name of the embedded shipped-value snapshot. The
// regulation it was read from is inside it; every supported version is proven
// to ship the same values.
const VanillaSnapshotFile = "vanilla.json"

// VanillaSnapshot is the shipped value of every field a reset restores.
//
// It is extracted from research's baseline save by the author's reference sync tool, never
// hand-written: these are 924 goods triggers and 304 rows
// of stone data, and a typo in any of them would be a reset that quietly writes
// a value the game never shipped.
type VanillaSnapshot struct {
	RegulationVersion uint32 `json:"regulation_version"`
	// SourceSHA256 is the save the values were read out of, so a snapshot can
	// always be traced back to the file that produced it.
	SourceSHA256 string `json:"source_sha256"`
	// GoodsFields names the columns of every Goods entry, in order, for the
	// same reason StoneFields does.
	GoodsFields []string `json:"goods_fields"`
	// Goods maps a goods row ID to one value per GoodsFields entry: the
	// formation it delivers and the price of using it.
	Goods map[int32][]int64 `json:"goods"`
	// BuddyFields names the columns of every Buddy entry, in order, for the
	// same reason StoneFields does.
	BuddyFields []string `json:"buddy_fields"`
	// Buddy maps a BuddyParam row ID to one value per BuddyFields entry. Its
	// key set is also the set of row IDs the regulation ships, which is how a
	// reset can tell a row the editor added from one that was always there.
	//
	// It replaced a list of the rows shipping appearOnAroundSekihi = 1. That
	// list was enough while a conversion was the only thing that could edit the
	// table; an authored formation writes three more fields, on rows a flag
	// list does not mention.
	// Values are float64 because three BuddyParam fields - x_offset, z_offset
	// and y_angle - are f32. Every other field is an integer of at most 32
	// bits, which float64 holds exactly and Go serialises identically, so the
	// three new columns are the only change to the file.
	Buddy map[int32][]float64 `json:"buddy"`
	// NpcCloneRows and NpcThinkCloneRows are the shipped actor rows in the
	// 100000000+ clone namespace. Rows below it are world actors, which nothing
	// may insert and therefore nothing may remove, so they are not recorded.
	NpcCloneRows      []int32 `json:"npc_clone_rows"`
	NpcThinkCloneRows []int32 `json:"npc_think_clone_rows"`
	// NpcFields names the columns of every Npc entry, in order; Npc is the
	// shipped value of every field the summon strength switch can write, for
	// each row in NpcCloneRows - the base health and the 32 resident effect
	// slots.
	//
	// They exist so the switch can tell whether the save it opened was written
	// at full strength, and put it back when it is turned off. They are not
	// part of a reset: Reset never writes an actor row's values. The clones
	// this editor inserts are absent on purpose - those are rebuildable from
	// the world actor the save still holds, and only FromSoftware's own rows
	// have shipped values that overwriting destroys.
	NpcFields []string          `json:"npc_fields"`
	Npc       map[int32][]int64 `json:"npc"`
	// StoneFields names the columns of every Stone entry, in order. Stored
	// rather than assumed so the file stays readable on its own and a field
	// order change cannot silently transpose 304 rows of values.
	StoneFields []string `json:"stone_fields"`
	// Stone maps a BuddyStoneParam row ID to one value per StoneFields entry.
	Stone map[int32][]int64 `json:"stone"`
}

// Vanilla parses the embedded shipped-value snapshot.
func Vanilla() (*VanillaSnapshot, error) {
	var snapshot VanillaSnapshot
	if err := json.Unmarshal(mustRead(VanillaSnapshotFile), &snapshot); err != nil {
		return nil, fmt.Errorf("parse %s: %w", VanillaSnapshotFile, err)
	}
	if err := snapshot.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", VanillaSnapshotFile, err)
	}
	return &snapshot, nil
}

// validate rejects a snapshot that could not describe a whole reset. A partial
// snapshot is worse than none: it would restore some rows and leave others
// carrying the recipe, which is a state no version of this product produces.
func (s *VanillaSnapshot) validate() error {
	if s.RegulationVersion == 0 {
		return fmt.Errorf("no regulation version")
	}
	if len(s.SourceSHA256) != 64 {
		return fmt.Errorf("source_sha256 must be a 64-character digest, got %d characters", len(s.SourceSHA256))
	}
	if len(s.GoodsFields) == 0 {
		return fmt.Errorf("no goods field names")
	}
	if len(s.Goods) == 0 {
		return fmt.Errorf("no goods rows")
	}
	for rowID, values := range s.Goods {
		if len(values) != len(s.GoodsFields) {
			return fmt.Errorf("goods row %d has %d values for %d fields", rowID, len(values), len(s.GoodsFields))
		}
	}
	if len(s.BuddyFields) == 0 {
		return fmt.Errorf("no buddy field names")
	}
	if len(s.Buddy) == 0 {
		return fmt.Errorf("no buddy rows")
	}
	for rowID, values := range s.Buddy {
		if len(values) != len(s.BuddyFields) {
			return fmt.Errorf("buddy row %d has %d values for %d fields", rowID, len(values), len(s.BuddyFields))
		}
	}
	if len(s.NpcCloneRows) == 0 || len(s.NpcThinkCloneRows) == 0 {
		return fmt.Errorf("no shipped clone rows: npc=%d think=%d", len(s.NpcCloneRows), len(s.NpcThinkCloneRows))
	}
	if len(s.NpcFields) == 0 {
		return fmt.Errorf("no npc field names")
	}
	// One record per shipped summon row, no more and no less. A row listed and
	// not recorded is a row the switch could unlock and never put back, which
	// is the one failure this table exists to prevent.
	for _, rowID := range s.NpcCloneRows {
		values, present := s.Npc[rowID]
		if !present {
			return fmt.Errorf("shipped summon row %d has no recorded values", rowID)
		}
		if len(values) != len(s.NpcFields) {
			return fmt.Errorf("npc row %d has %d values for %d fields", rowID, len(values), len(s.NpcFields))
		}
	}
	if len(s.Npc) != len(s.NpcCloneRows) {
		return fmt.Errorf("%d npc records for %d shipped summon rows", len(s.Npc), len(s.NpcCloneRows))
	}
	if len(s.StoneFields) == 0 {
		return fmt.Errorf("no stone field names")
	}
	if len(s.Stone) == 0 {
		return fmt.Errorf("no stone rows")
	}
	for rowID, values := range s.Stone {
		if len(values) != len(s.StoneFields) {
			return fmt.Errorf("stone row %d has %d values for %d fields", rowID, len(values), len(s.StoneFields))
		}
	}
	return nil
}

// NpcRow returns one shipped summon row's values keyed by field name.
func (s *VanillaSnapshot) NpcRow(rowID int32) (map[string]int64, bool) {
	return namedRow(s.NpcFields, s.Npc, rowID)
}

// GoodsRow returns one row's shipped values keyed by field name.
func (s *VanillaSnapshot) GoodsRow(rowID int32) (map[string]int64, bool) {
	return namedRow(s.GoodsFields, s.Goods, rowID)
}

// BuddyRow returns one row's shipped values keyed by field name. Floats,
// because three of the fields are.
func (s *VanillaSnapshot) BuddyRow(rowID int32) (map[string]float64, bool) {
	return namedRow(s.BuddyFields, s.Buddy, rowID)
}

// MonumentOriginRows are the BuddyParam rows shipping appearOnAroundSekihi = 1,
// in ascending order. It is derived from Buddy rather than stored beside it:
// two records of one fact can disagree, and this one is a projection.
func (s *VanillaSnapshot) MonumentOriginRows() []int32 {
	column := -1
	for index, name := range s.BuddyFields {
		if name == "appearOnAroundSekihi" {
			column = index
		}
	}
	if column < 0 {
		return nil
	}
	var rows []int32
	for rowID, values := range s.Buddy {
		if values[column] != 0 {
			rows = append(rows, rowID)
		}
	}
	slices.Sort(rows)
	return rows
}

// StoneRow returns one row's shipped values keyed by field name.
func (s *VanillaSnapshot) StoneRow(rowID int32) (map[string]int64, bool) {
	return namedRow(s.StoneFields, s.Stone, rowID)
}

func namedRow[V int64 | float64](fields []string, table map[int32][]V, rowID int32) (map[string]V, bool) {
	values, ok := table[rowID]
	if !ok {
		return nil, false
	}
	row := make(map[string]V, len(fields))
	for index, name := range fields {
		row[name] = values[index]
	}
	return row, true
}
