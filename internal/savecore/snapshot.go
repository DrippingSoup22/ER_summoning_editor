package savecore

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// GoodsRow is one EquipParamGoods row's shipped value for each of the three
// fields this product writes, keyed by the same names the manifest uses.
//
// It replaced a bare refId_default. That was enough while re-pointing a
// formation was the only thing that could change a goods row; a cost edit
// changes two more, and a reset that could only restore the trigger would leave
// a price the player set while claiming to have undone it.
type GoodsRow map[string]int64

// BuddyRow is one BuddyParam row's shipped value for each of the four fields a
// reset restores, keyed by the same names the manifest uses.
//
// The three formation fields are here because v2 authors formations by editing
// a host's own row block, and a reset that could only restore
// appearOnAroundSekihi would leave an authored formation in place while
// claiming to have undone it.
// Values are float64 rather than int64 because three of the fields a reset
// restores - x_offset, z_offset and y_angle - are f32. Every other field is an
// integer of at most 32 bits, which float64 holds exactly, so widening costs no
// precision and the serialised snapshot is unchanged for them.
type BuddyRow map[string]float64

// StoneRow is one BuddyStoneParam row's shipped value for each of the six
// expanded-area fields, keyed by the same names the manifest uses.
//
// Values are int64 because the six fields span u32 and s16: every shipped
// value fits, and no field here is fractional.
type StoneRow map[string]int64

// VanillaSnapshot is the shipped value of every field a reset restores, and
// nothing else.
//
// It is the counterpart of State. State answers "what does this save say now";
// this answers "what did the game ship", and reset is the difference. The two
// are deliberately bounded to the same fields as the writer, so a snapshot can
// never describe a change the writer could not make, nor miss one it could.
//
// The stone table is per row, unlike the conversion recipe that writes one
// uniform value set to all 304 rows. That asymmetry is the whole reason a
// snapshot has to exist: applying the recipe destroys 304 distinct rows of
// shipped data, and only a recorded copy can put them back.
type VanillaSnapshot struct {
	SourceSHA256      string
	RegulationVersion uint32
	// Goods is the shipped value of every writable field of the goods rows the
	// caller asked about. Only those rows: which rows are Spirit Ashes is
	// catalog knowledge and does not belong in the save layer.
	Goods map[int32]GoodsRow
	// Buddy is every BuddyParam row, keyed by row ID. Its key set is also the
	// set of row IDs the regulation ships, which is what tells a reset that a
	// row it finds in a save is one the editor added.
	Buddy map[int32]BuddyRow
	// Stone is every BuddyStoneParam row, keyed by row ID.
	Stone map[int32]StoneRow
	// NpcCloneRows and NpcThinkCloneRows are the shipped rows that already sit
	// in the 100000000+ clone namespace, in ascending order.
	//
	// Only the clone namespace is recorded, not the whole tables. A row below
	// the offset is a world actor, which nothing may insert and therefore
	// nothing may remove; recording thousands of IDs that can never be in
	// question would make the snapshot larger and no more able to answer
	// anything. What a reset needs to know is which clone-namespace rows the
	// game shipped, so that the rest are the editor's.
	NpcCloneRows      []int32
	NpcThinkCloneRows []int32
	// Npc is the shipped value of every field the strength switch can write,
	// for each row in NpcCloneRows. See npcsnapshot.go for why the inserted
	// clones need no equivalent.
	Npc map[int32]NpcRow
}

// MonumentOriginRows are the BuddyParam rows shipping appearOnAroundSekihi = 1,
// in ascending order. It is derived from Buddy rather than stored beside it: two
// records of one fact can disagree, and this one is a projection of the other.
func (s *VanillaSnapshot) MonumentOriginRows() []int32 {
	var rows []int32
	for rowID, values := range s.Buddy {
		if values["appearOnAroundSekihi"] != 0 {
			rows = append(rows, rowID)
		}
	}
	slices.Sort(rows)
	return rows
}

// ReadVanillaSnapshot extracts shipped values from a save.
//
// It asserts nothing about whether the save is actually unedited: that is the
// caller's responsibility and the generator tool's provenance record. Reading
// an already-converted save would produce a snapshot that resets to the
// conversion, which is why the tool records the source hash and regulation
// version alongside the data rather than leaving them implicit.
func ReadVanillaSnapshot(ctx context.Context, path string, goodsRows []int32) (*VanillaSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := loadSave(path)
	if err != nil {
		return nil, err
	}
	snapshot := &VanillaSnapshot{
		SourceSHA256:      regulation.Sha256Hex(loaded.FileBytes),
		RegulationVersion: loaded.Version,
		Goods:             make(map[int32]GoodsRow, len(goodsRows)),
	}

	if len(goodsRows) > 0 {
		goods, err := regulation.OpenTable(loaded.BND4, goodsLayout)
		if err != nil {
			return nil, err
		}
		for _, rowID := range goodsRows {
			row, present := goods.ByID[rowID]
			if !present {
				return nil, fmt.Errorf("Goods row %d was not found: %w", rowID, ErrRowNotFound)
			}
			snapshot.Goods[rowID] = readGoodsRow(goods.Data[row.DataOffset:])
		}
	}

	buddy, err := regulation.OpenTable(loaded.BND4, buddyLayout)
	if err != nil {
		return nil, err
	}
	snapshot.Buddy = make(map[int32]BuddyRow, len(buddy.Rows))
	for _, row := range buddy.Rows {
		snapshot.Buddy[row.ID] = readBuddyRow(buddy.Data[row.DataOffset:])
	}

	snapshot.NpcCloneRows, err = cloneNamespaceRows(loaded.BND4, npcLayout.Entry)
	if err != nil {
		return nil, err
	}
	snapshot.NpcThinkCloneRows, err = cloneNamespaceRows(loaded.BND4, npcThinkLayout.Entry)
	if err != nil {
		return nil, err
	}

	// The shipped summon rows' own values, for the rows just listed. Only
	// those: a world actor is never written by the strength switch, and
	// recording thousands of rows that can never be in question would make the
	// snapshot larger and no more able to answer anything.
	npc, err := regulation.OpenTable(loaded.BND4, npcLayout)
	if err != nil {
		return nil, err
	}
	snapshot.Npc = make(map[int32]NpcRow, len(snapshot.NpcCloneRows))
	for _, rowID := range snapshot.NpcCloneRows {
		row, present := npc.ByID[rowID]
		if !present {
			return nil, fmt.Errorf("NpcParam row %d was listed and not found: %w", rowID, ErrRowNotFound)
		}
		snapshot.Npc[rowID] = readNpcRow(npc.Data[row.DataOffset:])
	}

	stone, err := regulation.OpenTable(loaded.BND4, stoneLayout)
	if err != nil {
		return nil, err
	}
	if len(stone.Rows) == 0 {
		return nil, fmt.Errorf("BuddyStoneParam has no rows: %w", ErrUnsupportedRegulation)
	}
	snapshot.Stone = make(map[int32]StoneRow, len(stone.Rows))
	for _, row := range stone.Rows {
		snapshot.Stone[row.ID] = readStoneRow(stone.Data[row.DataOffset:])
	}
	return snapshot, nil
}

// BuddyFieldNames returns the four restorable BuddyParam field names in a
// stable order, for the same reason StoneFieldNames exists: a generated
// snapshot and the code that reads it must agree on column order without either
// hard-coding a list the other could drift from.
func GoodsFieldNames() []string { return fieldNames(goodsFields) }

// BuddyFieldNames is the same for BuddyParam.
func BuddyFieldNames() []string { return fieldNames(buddyFields) }

func readGoodsRow(row []byte) GoodsRow {
	values := make(GoodsRow, len(goodsFields))
	for name, field := range goodsFields {
		values[name] = readParamField(row, field, "goods", name)
	}
	return values
}

func readParamField(row []byte, field paramField, table, name string) int64 {
	return int64(readParamFieldValue(row, field, table, name))
}

// readParamFieldValue is the one decoder. readParamField narrows it for the
// integer tables; the buddy table keeps the float because three of its fields
// are one.
func readParamFieldValue(row []byte, field paramField, table, name string) float64 {
	switch field.kind {
	case "s32":
		return float64(int32(binary.LittleEndian.Uint32(row[field.offset:])))
	case "s16":
		return float64(int16(binary.LittleEndian.Uint16(row[field.offset:])))
	case "u8":
		return float64(row[field.offset])
	case "f32":
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(row[field.offset:])))
	}
	panic("unreachable: unknown " + table + " field kind " + field.kind + " for " + name)
}

func fieldNames[F any](fields map[string]F) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// cloneNamespaceRows lists one actor table's shipped rows inside the clone
// namespace, in ascending order.
//
// It reads row IDs only. Clone insertion opens these tables through npcLayout
// and npcThinkLayout, but a snapshot is taken of any save the player offers,
// including one this product will never write to, so asking which rows exist
// should not also demand that the table be shaped the way an insertion needs.
func cloneNamespaceRows(bnd4 []byte, entryName string) ([]int32, error) {
	_, present, err := regulation.LoadRowsByID(bnd4, entryName, "", 0)
	if err != nil {
		return nil, err
	}
	rows := make([]int32, 0, 128)
	for rowID := range present {
		if inCloneBand(rowID) {
			rows = append(rows, rowID)
		}
	}
	slices.Sort(rows)
	return rows, nil
}

func readBuddyRow(row []byte) BuddyRow {
	values := make(BuddyRow, len(buddyFields))
	for name, field := range buddyFields {
		values[name] = readParamFieldValue(row, field, "buddy", name)
	}
	return values
}

// StoneFieldNames returns the six expanded-area field names in a stable order,
// so a generated snapshot and the code that reads it agree on column order
// without either hard-coding a list the other could drift from.
func StoneFieldNames() []string { return fieldNames(stoneFields) }

func readStoneRow(row []byte) StoneRow {
	values := make(StoneRow, len(stoneFields))
	for name, field := range stoneFields {
		switch field.kind {
		case "u32":
			values[name] = int64(binary.LittleEndian.Uint32(row[field.offset:]))
		case "u16":
			values[name] = int64(binary.LittleEndian.Uint16(row[field.offset:]))
		case "s16":
			values[name] = int64(int16(binary.LittleEndian.Uint16(row[field.offset:])))
		default:
			panic("unreachable: unknown stone field kind " + field.kind + " for " + name)
		}
	}
	return values
}
