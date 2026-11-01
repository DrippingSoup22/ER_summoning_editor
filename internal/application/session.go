package application

// A session: one loaded save and everything the player has changed about it
// that has not been written yet.
//
// The three Service methods each perform a whole transaction - validate, read,
// plan, verify, write - and each refuses the other two. That is the right shape
// for a command line, where one invocation is one intent. It is the wrong shape
// for an editor, where one sitting is many intents and exactly one write, and
// where a change has to be reversible for as long as it has not been saved.
//
// So a session splits the transaction in three. Load reads the save once.
// Stage records what the player wants against what was read, in memory, and
// nothing reaches a file. PrepareCommit snapshots everything staged into an
// immutable plan and Commit writes it, once, to a new path.
//
// Concurrency: a Session is owned by one goroutine. Load is synchronous and
// slow enough that a desktop frontend must run it off its frame goroutine, and
// the result is a value that frontend then owns; nothing here is safe to call
// from two goroutines at a time. That is the same division the sibling merchant
// editor uses, where widget state belongs to the frame loop and workers return
// immutable results into it.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// Session is the source of truth for one loaded save and its pending changes.
//
// A frontend may hold selections, widgets and view state of its own, but what
// the player has actually changed lives here, so that a single Save writes all
// of it and dropping a change is a deletion rather than an inverse edit to
// compute.
type Session struct {
	service *Service
	loaded  *loadedSave

	// Staged work, keyed by what it targets rather than appended in order.
	// Editing the same Ash twice replaces rather than accumulates, which is
	// what makes a pending count a count of differences and an undo a
	// deletion.
	formations  map[int32]*StagedFormation
	conversions map[int32]*StagedConversion
	costs       map[int32]*StagedCost
	// restoration is the staged reset, of which there is at most one: it names
	// the whole save rather than any row of it.
	restoration *StagedRestoration
	// rowRoles is the role the player chose for each of the game's own summon
	// rows, Fighter true, and strength what that writes: nil when the save
	// already holds every one. See rowstrength.go.
	rowRoles map[int32]bool
	strength *StagedStrength
	// wideArea and noRestLimit are the monument switches, nil unless moved.
	// Unmoved writes nothing: a session that never mentions them must not
	// decide for the save either way. See area.go.
	wideArea, noRestLimit *bool

	// grants are the Ashes to put in a character's inventory. Kept apart from
	// the four above because they are the one staged change that writes
	// outside the regulation - see grants.go.
	grants map[grantKey]*StagedGrant
	// builds are the characters built from choices, by number, and dropped
	// the built characters the save holds that are being deleted (builds.go).
	builds  map[int]*StagedBuild
	dropped map[int]bool
}

// loadedSave is everything Load read, and the only thing staging is allowed to
// consult. A staged change planned against something other than the file in
// front of the player is a staged change that can be wrong by the time it is
// written.
type loadedSave struct {
	info SaveInfo
	// state holds every Spirit Ash goods row in the catalog, so the Ashes view
	// can say what each family currently delivers without reopening the save.
	state *savecore.State
	// formations holds every shipped block, keyed by trigger, plus the row-ID
	// sets an actor is checked against.
	formations *savecore.FormationState
	// people is the ten character slots, read once here rather than per call:
	// a read per keystroke costs seconds. See Characters.
	people []CharacterState
	// shipped is the shipped record of every summon row the game ships, which
	// a role chosen for one of them is written against (rowstrength.go).
	shipped map[int32]savecore.NpcRow
	// builds are the built characters the save holds, read back by number
	// (builds.go).
	builds map[int]character.Build
}

// NewSession returns an empty session over this service's catalog. It holds no
// save until Load succeeds, and every staging method refuses until then.
func (service *Service) NewSession() *Session {
	return &Session{
		service:     service,
		formations:  make(map[int32]*StagedFormation),
		conversions: make(map[int32]*StagedConversion),
		costs:       make(map[int32]*StagedCost),
	}
}

// Load reads a save and makes it the one this session edits.
//
// It reads everything staging needs in one go - every Spirit Ash goods row and
// every shipped formation block - because a staged change is planned against
// what the save says, and asking the file again later would let the two drift
// apart while the player works.
//
// A successful load discards whatever was staged against the previous save. A
// failed one changes nothing at all, so a frontend that offers the picker again
// is still holding the save the player had.
func (session *Session) Load(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return ErrPathRequired
	}
	goodsRows := session.service.catalogGoodsRows()
	state, err := session.service.readState(ctx, path, goodsRows)
	if err != nil {
		return err
	}
	info := SaveInfo{
		Path: path, SHA256: state.SHA256,
		RegulationVersion: state.RegulationVersion,
		Supported:         RegulationSupported(state.RegulationVersion),
	}
	if !info.Supported {
		return &UnsupportedRegulationError{Found: info.RegulationVersion}
	}
	formations, err := savecore.ReadFormations(ctx, path, session.service.catalogTriggers())
	if err != nil {
		return fmt.Errorf("read the shipped formations: %w", err)
	}
	people, err := session.service.readCharacters(path)
	if err != nil {
		return err
	}
	// Two reads of one file, so they can disagree if it changed underneath
	// them. Everything staged afterwards is planned against one hash, and the
	// commit pins that hash, so a save swapped mid-load would otherwise be
	// caught only at the write - after the player had composed against it.
	if formations.SHA256 != info.SHA256 {
		return fmt.Errorf("%s changed while it was being read: %w", path, savecore.ErrSourceMismatch)
	}
	shipped, err := shippedSummonRows()
	if err != nil {
		return err
	}
	builds, err := readBuilds(path, state.CharacterRows)
	if err != nil {
		return err
	}

	session.loaded = &loadedSave{info: info, state: state, formations: formations, people: people, shipped: shipped,
		builds: builds}
	session.ClearStaged()
	return nil
}

// Loaded reports whether a save is open. Every other accessor and every staging
// method is meaningless until it is true.
func (session *Session) Loaded() bool { return session.loaded != nil }

// Save describes the loaded save, or the zero value when none is open.
func (session *Session) Save() SaveInfo {
	if session.loaded == nil {
		return SaveInfo{}
	}
	return session.loaded.info
}

// SlotState is one row of a formation block as the loaded save holds it.
//
// A row retired by an earlier authoring is not here at all: retiring sets the
// trigger to -1, and a block is the set of rows sharing one trigger, so the row
// leaves the block. What can still appear is a row naming no actor - npcParamId
// -1, as six shipped rows have - so Npc is reported as the save holds it rather
// than flattened to zero.
//
// Person and Stance are what the row carries besides its actor, so a frontend
// that re-stages the slot as it stands names the same summon: without them a
// Puppet re-staged beside a new slot lost its person (authoring writes
// NoNpcPlayerInit for a slot naming none) and every re-staged slot would take
// the product's follow stance over the one the file holds.
type SlotState struct {
	RowID      int32
	Npc, Think int32
	// Person is the CharaInitParam row the slot summons on the shell Npc
	// names; zero for a creature.
	Person int32
	// Stance is the row's follow type and target share.
	Stance Stance
	// Place is where the row stands relative to the player and which way it
	// faces, as the file holds it.
	Place Placement
}

// AshState is one Spirit Ash as the loaded save holds it: the catalog entry a
// picker shows, plus what this particular file says about it.
// Price is what one summon costs the player: FP, and HP for the few families
// that take it. It is an alias rather than a copy, so a Price built by a
// frontend is the Price the plan layer writes and there is no conversion to
// keep in step. What this package owns is the decision: if the layer beneath
// ever changes shape, this is where a frontend stops seeing it.
type Price = plan.Price

type AshState struct {
	AshOption
	// ShippedTrigger is the formation this Ash owns; CurrentTrigger is what its
	// base goods row points at now. They differ exactly when an earlier
	// conversion re-pointed it.
	ShippedTrigger int32
	CurrentTrigger int32
	// RePointed means this Ash no longer delivers its own formation, so it
	// cannot be authored into until a reset puts it back.
	RePointed bool
	// Price is what a summon currently costs, read from the base goods row.
	// PriceVariesByLevel means the eleven upgrade rows do not agree, which no
	// shipped family and nothing this product writes can produce, so a frontend
	// showing one number must say that it is showing one of several.
	Price              plan.Price
	PriceVariesByLevel bool
	// Slots is the block the Ash's own trigger delivers, in table order. It is
	// what the Ash ships with, not what it currently resolves: a re-pointed Ash
	// still owns its block and a reset gives it back.
	Slots []SlotState
}

// AshStates returns every Spirit Ash the catalog knows, in catalog order, as
// the loaded save holds it. It is the read model an Ashes view renders.
func (session *Session) AshStates() ([]AshState, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	states := make([]AshState, 0, len(session.service.ashes))
	for _, ash := range session.service.ashes {
		state, err := session.ashState(ash)
		if err != nil {
			return nil, err
		}
		states = append(states, *state)
	}
	return states, nil
}

// AshState resolves one Ash by exact case-insensitive name or numeric
// goods-family ID, exactly as a conversion names one.
func (session *Session) AshState(selector string) (*AshState, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	ash, err := catalog.Resolve(session.service.ashes, selector)
	if err != nil {
		return nil, &UnknownAshError{Role: "host", Selector: selector, cause: err}
	}
	return session.ashState(ash)
}

func (session *Session) ashState(ash catalog.Ash) (*AshState, error) {
	current, known := session.loaded.state.Goods[ash.GoodsFamily]
	if !known {
		return nil, fmt.Errorf("goods row %d for %s: %w", ash.GoodsFamily, ash.Name, ErrRowNotFound)
	}
	shipped := ash.TriggerFamilies[0]
	state := &AshState{
		AshOption:      optionFor(ash),
		ShippedTrigger: shipped,
		CurrentTrigger: current.Trigger,
		RePointed:      current.Trigger != shipped,
		Price:          plan.Price{FP: current.ConsumeMP, HP: current.ConsumeHP},
	}
	// The price is read from every upgrade row and not only the base one. All
	// 84 shipped families charge the same at every level and this product
	// writes all eleven together, so a save where they disagree was edited by
	// something else - and one number standing for eleven different ones would
	// be this layer inventing a fact.
	for level := 1; level < ash.UpgradeLevels; level++ {
		row, known := session.loaded.state.Goods[ash.GoodsFamily+int32(level)]
		if !known {
			return nil, fmt.Errorf("goods row %d for %s +%d: %w",
				ash.GoodsFamily+int32(level), ash.Name, level, ErrRowNotFound)
		}
		if row.ConsumeMP != current.ConsumeMP || row.ConsumeHP != current.ConsumeHP {
			state.PriceVariesByLevel = true
		}
	}
	for _, row := range session.loaded.formations.Blocks[shipped].Rows {
		slot := SlotState{RowID: row.RowID, Npc: row.Npc, Think: row.Think,
			Stance: Stance{DisablePCTargetShare: int(row.DisablePCTargetShare), PcFollowType: int(row.PcFollowType)},
			Place:  formation.PlacementOfRow(row.XOffset, row.ZOffset, row.YAngle)}
		if row.NpcPlayerInitParamId > 0 {
			slot.Person = row.NpcPlayerInitParamId
		}
		state.Slots = append(state.Slots, slot)
	}
	return state, nil
}

// catalogGoodsRows is every goods row the catalog names, ascending. A session
// reads all of them because the Ashes view describes all of them, and because
// reading the file once is what keeps staging a memory operation.
func (service *Service) catalogGoodsRows() []int32 {
	rows := make([]int32, 0, len(service.ashes)*11)
	for _, ash := range service.ashes {
		for level := 0; level < ash.UpgradeLevels; level++ {
			rows = append(rows, ash.GoodsFamily+int32(level))
		}
	}
	slices.Sort(rows)
	return rows
}

// catalogTriggers is every formation family the catalog names, ascending and
// deduplicated. Giant Rat Ashes contributes three; every other family one.
func (service *Service) catalogTriggers() []int32 {
	var triggers []int32
	for _, ash := range service.ashes {
		triggers = append(triggers, ash.TriggerFamilies...)
	}
	slices.Sort(triggers)
	return slices.Compact(triggers)
}

// blockFor returns the shipped formation block of one trigger, refusing a save
// that holds none. A block with no rows is not an empty formation: it is a save
// this catalog does not describe.
func (session *Session) blockFor(ash catalog.Ash, trigger int32) (savecore.FormationBlock, error) {
	block := session.loaded.formations.Blocks[trigger]
	if len(block.Rows) == 0 {
		return savecore.FormationBlock{}, fmt.Errorf(
			"%s delivers formation %d, which this save holds no rows for: %w",
			ash.Name, trigger, ErrRowNotFound)
	}
	return block, nil
}
