package application

// Staging a Spirit Ash into a character's inventory.
//
// This is the one staged change that does not touch the regulation. Everything
// else here rewrites PARAM rows - which formation an Ash delivers, what it
// costs, which rows go back to shipped values - and is described by a manifest
// that names every byte. A grant writes a twelve-byte record and three counters
// inside one of the ten character slots, which the manifest format has no way
// to describe and which Reset has no way to take back.
//
// So it is deliberately kept beside the others rather than folded into them:
// its own map, its own staging call, and its own step at commit, after the
// regulation has been written. See docs/DESIGN.md (The character slots).

import (
	"fmt"
	"os"
	"sort"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
)

// ashByFamily is the catalog Ash with this goods family. Grants name a family
// rather than a name, because a name is what a player typed and a family is
// what the save record holds.
func (service *Service) ashByFamily(family int32) (catalog.Ash, error) {
	for _, ash := range service.ashes {
		if ash.GoodsFamily == family {
			return ash, nil
		}
	}
	return catalog.Ash{}, fmt.Errorf("no Spirit Ash with goods family %d: %w", family, ErrRowNotFound)
}

// StagedGrant is one Ash going to one character, at one upgrade level.
//
// Giving somebody an Ash and changing the level of one they already have are
// the same staged change, because they are the same sentence - "this character
// should have Lone Wolf Ashes at +5" - and which write it takes is a fact about
// the save rather than a decision the player made. Was is what says which of
// the two happened, for a pending list that has to describe it.
type StagedGrant struct {
	Slot int
	// Character is the name at the time it was staged, for a pending list that
	// should say who rather than which slot number.
	Character string
	// GoodsFamily is the Ash, and Name is what to call it.
	GoodsFamily int32
	Name        string
	// Level is the upgrade level the character will hold it at, +0 to +10. It
	// is the goods id's low digits and nothing else.
	Level int
	// Was is the level they hold it at now, and -1 when they do not hold it at
	// all. That -1 is what makes this a grant rather than an upgrade.
	Was int
	// Take is a removal rather than a grant: every copy of this Ash comes out
	// of the character's inventory and storage box.
	//
	// It shares the type and the key with a grant because one character cannot
	// both be given an Ash and have it taken away - they are answers to the
	// same question, and staging them separately would let a player queue both
	// and leave the commit to decide which they meant.
	Take bool
}

// Removal reports whether this takes the Ash away rather than giving it.
func (grant *StagedGrant) Removal() bool { return grant.Take }

// Upgrade reports whether this changes an Ash the character already holds
// rather than giving them one they do not.
func (grant *StagedGrant) Upgrade() bool { return grant.Was >= 0 }

// CharacterState is one of the ten slots, as a frontend may describe it.
type CharacterState struct {
	Slot         int
	Used         bool
	Name         string
	Level, Runes uint32
	// Class is the starting class's name, empty for a number the game does
	// not use.
	Class string
	// KnowsStorage reports whether the storage box could be read. When it is
	// false, Owns answers for inventory alone and a frontend must not claim
	// certainty it does not have.
	KnowsStorage bool
	// Owns is every Ash goods family this character already holds, in
	// inventory or in storage, at any upgrade level.
	Owns map[int32]bool
	// Levels is the upgrade level each of those is held at, keyed the same way.
	// Presence in this map means the same thing as a true in Owns; the two are
	// written together and a frontend may read either.
	Levels map[int32]int
}

// Characters is the ten slots of the loaded save.
//
// Answered from what Load read, not by reading again. Nothing the session holds
// knew about slots once - Load parsed the regulation and skipped over them - so
// this opened the file itself, and every caller paid a 26 MB read and a full
// parse. That was tolerable while a frontend called it once per load. It stopped
// being tolerable the moment staging a grant called it: setting the upgrade
// level on one character's whole collection stages one grant per Ash, so a
// single keystroke in the global level field read the save 53 times and took
// six seconds.
//
// Caching is also the more correct answer, not only the faster one. Everything
// staged is planned against the hash Load recorded, and the commit pins it, so
// a session that re-read the file mid-edit could stage two grants against two
// different versions of it and notice only at the write.
//
// The copy is defensive: the maps are the session's own, and a caller that
// mutated what it was handed would change what the session believes the save
// says.
func (session *Session) Characters() ([]CharacterState, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	people := make([]CharacterState, len(session.loaded.people))
	for index, person := range session.loaded.people {
		people[index] = person.clone()
	}
	return people, nil
}

// clone is one character's state with its own maps.
func (state CharacterState) clone() CharacterState {
	copied := state
	if state.Owns != nil {
		copied.Owns = make(map[int32]bool, len(state.Owns))
		for family, owns := range state.Owns {
			copied.Owns[family] = owns
		}
	}
	if state.Levels != nil {
		copied.Levels = make(map[int32]int, len(state.Levels))
		for family, level := range state.Levels {
			copied.Levels[family] = level
		}
	}
	return copied
}

// readCharacters is the read Load does once.
func (service *Service) readCharacters(path string) ([]CharacterState, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	people, err := character.Read(file)
	if err != nil {
		return nil, err
	}
	states := make([]CharacterState, 0, len(people))
	for _, person := range people {
		state := CharacterState{
			Slot: person.Slot, Used: person.Used, Name: person.Name,
			Level: person.Level, Runes: person.Runes, KnowsStorage: person.ReadStorage,
			Class: person.ClassName(),
		}
		if person.Used {
			state.Owns = make(map[int32]bool, 84)
			state.Levels = make(map[int32]int, 84)
			for _, ash := range service.ashes {
				// Asked by family and answered with a level, because the level
				// is in the id: a character holding Lone Wolf Ashes +5 holds
				// goods row 232005 and no row 232000 at all. Asking whether
				// they own 232000 would answer no.
				if level, owned := person.AshLevel(uint32(ash.GoodsFamily)); owned {
					state.Owns[ash.GoodsFamily] = true
					state.Levels[ash.GoodsFamily] = level
				}
			}
		}
		states = append(states, state)
	}
	return states, nil
}

// StageGrant records that one character should hold one Ash at one level.
//
// It refuses only what is not a change: an Ash they already hold at that level.
// Everything else stages - a level they do not hold it at is an upgrade, and an
// Ash they do not hold at all is a grant.
//
// The check is repeated against the file at the moment of the write, by
// internal/character, which is the only place that can be sure: the check and
// the write have to see the same bytes or the check is a guess. A frontend
// leaves the control alone for something it can see is already true; this
// refuses one it staged anyway, at commit.
func (session *Session) StageGrant(slot int, goodsFamily int32, level int) (*StagedGrant, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	if level < 0 || level >= character.AshLevels {
		return nil, fmt.Errorf("+%d: %w", level, character.ErrNoSuchLevel)
	}
	ash, err := session.service.ashByFamily(goodsFamily)
	if err != nil {
		return nil, err
	}
	// The session's own copy, not a fresh read: this runs once per Ash when the
	// global level field changes, and a read each time takes seconds.
	people := session.loaded.people
	if slot < 0 || slot >= len(people) || !people[slot].Used {
		return nil, fmt.Errorf("slot %d: %w", slot, character.ErrNoSuchCharacter)
	}
	was, owned := people[slot].Levels[goodsFamily]
	if owned && was == level {
		return nil, fmt.Errorf("%s already has %s at +%d: %w",
			people[slot].Name, ash.Name, level, character.ErrAlreadyOwned)
	}
	if !owned {
		was = -1
	}
	staged := &StagedGrant{
		Slot: slot, Character: people[slot].Name,
		GoodsFamily: goodsFamily, Name: ash.Name, Level: level, Was: was,
	}
	if session.grants == nil {
		session.grants = make(map[grantKey]*StagedGrant)
	}
	session.grants[grantKey{slot: slot, family: goodsFamily}] = staged
	return staged, nil
}

// StageRemoval records that one character should no longer hold one Ash.
//
// Refused when they do not have it, for the reason Remove itself refuses: a
// removal that quietly did nothing would leave the player believing an item is
// gone. The check is repeated against the file at the moment of the write,
// which is the only place it can be certain.
func (session *Session) StageRemoval(slot int, goodsFamily int32) (*StagedGrant, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	ash, err := session.service.ashByFamily(goodsFamily)
	if err != nil {
		return nil, err
	}
	people := session.loaded.people
	if slot < 0 || slot >= len(people) || !people[slot].Used {
		return nil, fmt.Errorf("slot %d: %w", slot, character.ErrNoSuchCharacter)
	}
	was, owned := people[slot].Levels[goodsFamily]
	if !owned {
		return nil, fmt.Errorf("%s does not have %s: %w",
			people[slot].Name, ash.Name, character.ErrNotOwned)
	}
	staged := &StagedGrant{
		Slot: slot, Character: people[slot].Name,
		GoodsFamily: goodsFamily, Name: ash.Name, Level: was, Was: was, Take: true,
	}
	if session.grants == nil {
		session.grants = make(map[grantKey]*StagedGrant)
	}
	session.grants[grantKey{slot: slot, family: goodsFamily}] = staged
	return staged, nil
}

// UnstageGrant drops one staged grant and reports whether one was there.
func (session *Session) UnstageGrant(slot int, goodsFamily int32) bool {
	key := grantKey{slot: slot, family: goodsFamily}
	_, had := session.grants[key]
	delete(session.grants, key)
	return had
}

// grantKey is what makes a grant unique: one Ash, for one character. Staging
// the same pair twice is one grant, which is what stops a double click from
// asking for two copies of something the rule says there is one of.
type grantKey struct {
	slot   int
	family int32
}

// stagedGrants is every grant, ordered by slot and then by Ash so two commits
// of one staged set describe themselves the same way.
func (session *Session) stagedGrants() []*StagedGrant {
	grants := make([]*StagedGrant, 0, len(session.grants))
	for _, grant := range session.grants {
		grants = append(grants, grant)
	}
	sort.Slice(grants, func(i, j int) bool {
		if grants[i].Slot != grants[j].Slot {
			return grants[i].Slot < grants[j].Slot
		}
		return grants[i].GoodsFamily < grants[j].GoodsFamily
	})
	return grants
}

// applyGrants writes the staged grants into a save this session just produced.
//
// It runs on the output rather than the input, after the regulation has been
// written, and that ordering is the whole design. The input is never opened for
// writing by anything in this product; the output is a file this commit created
// moments ago, so rewriting it is not an edit to anybody's save.
//
// A grant that fails does not fail the commit. The regulation edits are already
// correct and already written, and throwing away a good save because an item
// could not be added would cost the player the thing they came for. The
// failures are collected and reported, and the caller says what happened.
func applyGrants(outputPath string, grants []*StagedGrant) (granted int, failures []error) {
	if len(grants) == 0 {
		return 0, nil
	}
	file, err := os.ReadFile(outputPath)
	if err != nil {
		return 0, []error{fmt.Errorf("reopen %s to add items: %w", outputPath, err)}
	}
	for _, grant := range grants {
		write := func() error {
			if grant.Take {
				return character.Remove(file, grant.Slot, uint32(grant.GoodsFamily))
			}
			return character.Set(file, grant.Slot, uint32(grant.GoodsFamily), grant.Level)
		}
		if err := write(); err != nil {
			failures = append(failures, fmt.Errorf("%s for %s: %w", grant.Name, grant.Character, err))
			continue
		}
		granted++
	}
	if granted == 0 {
		return 0, failures
	}
	// Written whole and in place. The file is this commit's own output and its
	// length has not changed - a grant fills a record that was already there,
	// and a level change rewrites four bytes of one - so there is nothing to
	// move and nothing to truncate.
	if err := os.WriteFile(outputPath, file, 0o644); err != nil {
		return 0, append(failures, fmt.Errorf("write the added items into %s: %w", outputPath, err))
	}
	return granted, failures
}
