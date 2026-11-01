package application

// Effect rows of our own, decided at Save over the whole save.
//
// A clone may carry a copy of the presence or the marker in place of the
// shipped one, or a resident of our own its creature's fix adds
// (savecore/owneffects.go), and the copy is a row the save holds only for it.
// So it is decided like the prune: a row is inserted while any
// clone in the written save will name it, and removed once none will. The
// clones are the ones the save keeps - less those the prune removes, and
// beside a staged reset only those a staged formation still names - and the
// ones this commit inserts. Free casting and the own-colours presence are
// named by the characters' bodies, and the characters' ladder by the formation
// rows that summon them, so they are held while the written save holds any
// character's body: a formation row naming a character keeps the character,
// and a retired one takes its Ash's ladder back (plan.BuildAuthor).

import (
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// ownEffectsPart is the insertions and removals, or nil when the save already
// holds exactly the rows it will name.
func (session *Session) ownEffectsPart(formations []*plan.Manifest, staged []*StagedFormation, pruned []int32, resetting bool) (*plan.Manifest, error) {
	state := session.loaded.state
	gone := map[int32]bool{}
	for _, row := range pruned {
		gone[row] = true
	}
	survives := map[int32]bool{}
	for _, one := range staged {
		for _, slot := range one.Formation.Slots {
			survives[slot.Npc] = true
		}
	}
	named := map[int32]bool{}
	for row, effects := range state.OwnEffectNames {
		if gone[row] || (resetting && !survives[row]) || isCharacterBody(row) {
			continue
		}
		for _, effect := range effects {
			named[effect] = true
		}
	}
	for _, manifest := range formations {
		for _, entry := range manifest.NpcCloneStrength {
			named[entry.Presence] = true
			named[entry.Marker] = true
			// A creature's own fix rides its residents (source own).
			for _, effect := range entry.Residents {
				named[effect] = true
			}
		}
	}
	if session.anyCharacterAfter(formations, resetting) {
		// Every character has bodies in its own colours beside its puppet
		// ones, and a formation row summoning it climbs the characters' ladder.
		named[savecore.FreeCasting], named[savecore.OwnColoursPresence] = true, true
		for _, rung := range savecore.CharacterLadder() {
			named[rung] = true
		}
	}
	var insert, remove []int32
	for _, own := range savecore.OwnEffects() {
		switch {
		case named[own.Row] && !state.OwnEffects[own.Row]:
			insert = append(insert, own.Row)
		case !named[own.Row] && state.OwnEffects[own.Row]:
			remove = append(remove, own.Row)
		}
	}
	if len(insert)+len(remove) == 0 {
		return nil, nil
	}
	return plan.BuildOwnEffects(session.loaded.info.SHA256, insert, remove)
}

// isCharacterBody reports whether an NpcParam row is in the character bodies'
// band, whose effects the character write decides.
func isCharacterBody(row int32) bool {
	return row >= character.BodyFloor && row < character.BuiltBodyOf(character.Builds)
}

// anyCharacterAfter reports whether the written save will hold a character's
// body, as the character write decides it (characters.go): a custom
// character's, or a save slot's a formation names - built from the slot, or
// kept as the save holds it when the slot was emptied since.
func (session *Session) anyCharacterAfter(formations []*plan.Manifest, resetting bool) bool {
	if len(session.buildsAfter(formations, resetting)) > 0 {
		return true
	}
	for row := range personsAfter(session.loaded.formations, formations, resetting) {
		slot, ok := character.SlotOf(row)
		if !ok {
			continue
		}
		body := savecore.CharacterRowKey{Table: "NpcParam", RowID: character.BodyOf(slot)}
		if _, held := session.loaded.state.CharacterRows[body]; held || session.loaded.people[slot].Used {
			return true
		}
	}
	return false
}
