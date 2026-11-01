package application

// The colour switch an earlier build wrote, put back.
//
// A summon renders through PhantomParam 200, whose shipped name is "Spirit
// Summon", reached through SpEffectVfxParam 57000. Earlier builds offered a
// switch for the whole save that pointed 57000 at a profile with no coat (410,
// or 0 before it), so every summon the file could deliver rendered in its own
// colours. The look is each creature's now (look.go): the ghost as the game
// ships it, or its own colours on a presence of our own. So a save that still carries the switch has the game's spirit
// look put back by the next Save that writes the regulation anyway - as a
// ladder an earlier build changed goes back - and a reset puts back a profile
// this editor did not write as well.
//
// See plan.Tint for the write and why it is one field.

import "github.com/DrippingSoup22/ER_summoning_editor/internal/plan"

// tintPart is the spirit look put back where the save holds another: one an
// earlier build of this editor wrote, or any other beside a staged reset; nil
// otherwise.
func (session *Session) tintPart(resetting bool) (*plan.Manifest, error) {
	tint, known := plan.TintOf(session.loaded.state.PhantomProfile)
	if !session.coloursChanged() || !resetting && (!known || tint != plan.OriginalColours) {
		return nil, nil
	}
	return plan.BuildTint(session.loaded.info.SHA256, plan.SpiritTint)
}

// coloursChanged reports whether the loaded save draws summons other than
// through the game's own spirit look, which a reset puts back.
func (session *Session) coloursChanged() bool {
	return session.loaded.state.PhantomProfile != plan.SpiritPhantomProfile
}
