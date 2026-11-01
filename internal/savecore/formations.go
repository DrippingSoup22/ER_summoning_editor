package savecore

// Reading a host's formation, so that authoring can be asked for.
//
// Everything the compiler in internal/formation decides, it decides from what
// the save actually holds: which rows the host's block has, in what order, with
// which reinforcement tables, and which actor rows already exist. None of that
// is knowable from a catalog, because a save may already have been authored
// into - by an earlier run of this product, or by another tool - and a block
// read from a stale record would be a block that is not there.
//
// This is a reader in the same bounded sense as State and VanillaSnapshot: it
// reports the fields authoring writes plus the reinforcement table an inserted
// row inherits, and nothing else.

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

const (
	// buddyDopingOffset is dopingSpEffect_lv0; the eleven fields are contiguous
	// s32s. Measured on 11701000, where a shipped block row carries the
	// 290000..290010 series here, and asserted against the shipped regulation
	// by TestShippedBlocksCarryTheirReinforcementTable.
	buddyDopingOffset = 40
	// dopingLevels must equal formation.DopingLevels. Written twice and checked
	// once, for the reason given in clonenamespace_test.go: the save layer does
	// not import the model layer.
	dopingLevels = 11
)

// FormationRow is one BuddyParam row of a host's block, as the save holds it.
type FormationRow struct {
	RowID int32
	// Npc and Think are npcParamId and npcThinkParamId: what this row summons
	// and how it thinks.
	Npc   int32
	Think int32
	// AppearOnAroundSekihi is the placement flag: 1 spawns the formation at a
	// distant monument instead of beside the player.
	AppearOnAroundSekihi int32
	// DisablePCTargetShare and PcFollowType are the stance pair: whether the
	// formation follows the player or holds the ground it was called on. They
	// are read as well as written because a frontend offering the choice has
	// to be able to show which one the file already says - the same rule the
	// strength switch obeys.
	DisablePCTargetShare int32
	PcFollowType         int32
	// XOffset, ZOffset and YAngle are where this actor stands relative to the
	// player and which way it faces: lateral metres, forward metres, degrees.
	// Read as well as written for the reason the stance pair is - a reset has
	// to put back what authoring wrote, and a frontend showing a layout has to
	// be able to show the one the file already holds.
	XOffset float32
	ZOffset float32
	YAngle  float32
	// NpcPlayerInitParamId names a CharaInitParam row, and when it is set the
	// game builds that person instead of the creature Npc names. It is read so
	// a retired row can be put back holding it.
	NpcPlayerInitParamId int32
	// GenerateAnimId is the animation the summon arrives with.
	GenerateAnimId int32
	// Doping is dopingSpEffect_lv0..lv10, the reinforcement the row applies at
	// each upgrade level of the item that summoned it. An inserted row inherits
	// the table of the row it was copied from, so a caller needs to see them to
	// choose which row to copy.
	Doping [dopingLevels]int32
}

// Values is the row as the restorable fields, keyed the way a manifest keys
// them. A row retired by an authored formation keeps everything but its
// trigger, so the layer that builds that patch needs what the row holds now -
// and the block it already read is where that is.
//
// The reinforcement ladder is in it because authoring can write that too, and a
// patch that named an actor without naming the ladder would put a row back with
// somebody else's scaling on it.
func (row FormationRow) Values(trigger int32) map[string]float64 {
	values := map[string]float64{
		"triggerSpEffectId":    float64(trigger),
		"npcParamId":           float64(row.Npc),
		"npcThinkParamId":      float64(row.Think),
		"appearOnAroundSekihi": float64(row.AppearOnAroundSekihi),
		"disablePCTargetShare": float64(row.DisablePCTargetShare),
		"pcFollowType":         float64(row.PcFollowType),
		"npcPlayerInitParamId": float64(row.NpcPlayerInitParamId),
		"generateAnimId":       float64(row.GenerateAnimId),
		"x_offset":             float64(row.XOffset),
		"z_offset":             float64(row.ZOffset),
		"y_angle":              float64(row.YAngle),
	}
	for level, effect := range row.Doping {
		values[fmt.Sprintf("dopingSpEffect_lv%d", level)] = float64(effect)
	}
	return values
}

// FormationBlock is every BuddyParam row carrying one trigger, in the physical
// order the table holds them, which is the order the game summons them in.
//
// A block with no rows is reported rather than treated as an error: a trigger
// whose block is missing is a fact about the save, and the layer that knows
// what a caller intended is the one that should decide whether it is fatal.
type FormationBlock struct {
	Trigger int32
	Rows    []FormationRow
	// Inert are the block's own retired rows: the rows at the last row's ID
	// plus one, plus two and on, that exist, carry triggerSpEffectId -1 and
	// stay in the last row's hundred (row / 100) - the walk stops at the first
	// row that fails any of the three. A formation smaller than its block
	// retires its surplus rows this way, so they leave the block and stay
	// where they were; a formation that grows again re-fills them rather than
	// colliding with them (formation.Block.Inert).
	//
	// Why those rows are this block's and no other's (measured on the shipped
	// snapshot, reference/vanilla.json): no hundred holds
	// rows of two triggers, so a row in the last row's hundred belongs to no
	// other formation; and this editor only ever adds a block's rows directly
	// after its last one, so every row it added and later retired is in that
	// run. The game's own inert rows - row 0, 31800000 and the four of
	// 305000xx - sit in hundreds no live block holds, so the walk never
	// reaches one.
	Inert []FormationRow
}

// inertTrigger is how a retired row spells its trigger: plan.InertTrigger, and
// the value six of the game's own rows carry.
const inertTrigger = int32(-1)

// FormationState is what a save says about the formations a caller named, plus
// the row-ID sets that decide which actors can be summoned without inserting
// anything.
//
// The three key sets are complete, not bounded to the clone namespace the way a
// VanillaSnapshot's are. A snapshot records which rows the game shipped, and
// only clones can ever be in question there. This answers "does this row exist
// at all", which a world actor has to be asked about too - it is the row a
// clone would be copied from.
type FormationState struct {
	SHA256            string
	RegulationVersion uint32
	// Blocks holds one entry per requested trigger, present even when empty.
	Blocks map[int32]FormationBlock
	// BuddyRows, NpcRows and NpcThinkRows are every row ID each table holds.
	BuddyRows    map[int32]bool
	NpcRows      map[int32]bool
	NpcThinkRows map[int32]bool
	// Senses is every think row's own sight, for a frontend that draws what a
	// summon will notice (LevelSight).
	Senses map[int32]Senses
	// Actors is what EVERY BuddyParam row names, live or retired, whatever
	// trigger it carries: the rows a creature row is summoned through. Placing
	// clones at Save reads it to learn which creature rows the save still
	// uses, formation by formation (application's clonealloc.go).
	Actors map[int32]BuddyActors
	// Persons is the CharaInitParam row every BuddyParam row that names one
	// points at (npcPlayerInitParamId > 0), live or retired: the people the
	// save summons. The person writes are decided over it at Save
	// (application's sharedeffects.go).
	Persons map[int32]int32
}

// BuddyActors is the creature one BuddyParam row names: npcParamId and
// npcThinkParamId.
type BuddyActors struct {
	Npc, Think int32
}

// ReadFormations opens a save read-only and reports the blocks named by
// triggers, together with the row-ID sets of the three tables authoring
// touches. It writes nothing.
func ReadFormations(ctx context.Context, path string, triggers []int32) (*FormationState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := loadSave(path)
	if err != nil {
		return nil, err
	}
	state := &FormationState{
		SHA256:            regulation.Sha256Hex(loaded.FileBytes),
		RegulationVersion: loaded.Version,
		Blocks:            make(map[int32]FormationBlock, len(triggers)),
	}
	for _, trigger := range triggers {
		state.Blocks[trigger] = FormationBlock{Trigger: trigger}
	}

	buddy, err := regulation.OpenTable(loaded.BND4, buddyLayout)
	if err != nil {
		return nil, err
	}
	state.BuddyRows = make(map[int32]bool, len(buddy.Rows))
	state.Actors = make(map[int32]BuddyActors, len(buddy.Rows))
	state.Persons = map[int32]int32{}
	inert := map[int32]FormationRow{}
	triggerOffset := buddyFields["triggerSpEffectId"].offset
	npcOffset := buddyFields["npcParamId"].offset
	thinkOffset := buddyFields["npcThinkParamId"].offset
	for _, row := range buddy.Rows {
		state.BuddyRows[row.ID] = true
		body := buddy.Data[row.DataOffset:]
		state.Actors[row.ID] = BuddyActors{
			Npc:   int32(binary.LittleEndian.Uint32(body[npcOffset:])),
			Think: int32(binary.LittleEndian.Uint32(body[thinkOffset:])),
		}
		if person := int32(binary.LittleEndian.Uint32(body[buddyPlayerInitOffset:])); person > 0 {
			state.Persons[row.ID] = person
		}
		trigger := int32(binary.LittleEndian.Uint32(body[triggerOffset:]))
		block, wanted := state.Blocks[trigger]
		if !wanted && trigger != inertTrigger {
			continue
		}
		read := FormationRow{
			RowID:                row.ID,
			Npc:                  int32(binary.LittleEndian.Uint32(body[npcOffset:])),
			Think:                int32(binary.LittleEndian.Uint32(body[thinkOffset:])),
			AppearOnAroundSekihi: int32(body[buddyFields["appearOnAroundSekihi"].offset]),
			DisablePCTargetShare: int32(body[buddyTargetShareOffset]),
			PcFollowType:         int32(body[buddyFollowTypeOffset]),
			NpcPlayerInitParamId: int32(binary.LittleEndian.Uint32(body[buddyPlayerInitOffset:])),
			GenerateAnimId:       int32(binary.LittleEndian.Uint32(body[buddyGenerateAnimOffset:])),
			XOffset:              math.Float32frombits(binary.LittleEndian.Uint32(body[buddyXOffset:])),
			ZOffset:              math.Float32frombits(binary.LittleEndian.Uint32(body[buddyZOffset:])),
			YAngle:               math.Float32frombits(binary.LittleEndian.Uint32(body[buddyYAngleOffset:])),
		}
		for level := range read.Doping {
			read.Doping[level] = int32(binary.LittleEndian.Uint32(body[buddyDopingOffset+4*level:]))
		}
		if trigger == inertTrigger {
			inert[row.ID] = read
		}
		if !wanted {
			continue
		}
		block.Rows = append(block.Rows, read)
		state.Blocks[trigger] = block
	}
	for trigger, block := range state.Blocks {
		if len(block.Rows) == 0 {
			continue
		}
		last := block.Rows[len(block.Rows)-1].RowID
		for id := last + 1; id/100 == last/100; id++ {
			row, retired := inert[id]
			if !retired {
				break
			}
			block.Inert = append(block.Inert, row)
		}
		state.Blocks[trigger] = block
	}

	if state.NpcRows, err = rowIDSet(loaded.BND4, npcLayout); err != nil {
		return nil, err
	}
	if state.NpcThinkRows, err = rowIDSet(loaded.BND4, npcThinkLayout); err != nil {
		return nil, err
	}
	if state.Senses, err = readSenses(loaded.BND4); err != nil {
		return nil, err
	}
	return state, nil
}

func rowIDSet(bnd4 []byte, layout regulation.TableLayout) (map[int32]bool, error) {
	table, err := regulation.OpenTable(bnd4, layout)
	if err != nil {
		return nil, err
	}
	ids := make(map[int32]bool, len(table.Rows))
	for _, row := range table.Rows {
		ids[row.ID] = true
	}
	return ids, nil
}
