package application

import "github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"

// Sight is what one summon notices by itself, in metres from its centre: an eye
// cone Eye long and Cone degrees wide - 180 is the half circle in front, the
// widest a cone gets - whose apex sits Behind metres back, and a nose all round
// to Nose. Zero eye and nose is a summon that notices nothing until it is hit.
type Sight = savecore.Senses

// SightOf is what a slot will notice once it is written: its level's profile
// over its body, as the writer computes it, or the row's own senses for the
// Loner and for a row kept as the file holds it. A frontend draws it; nothing
// is staged.
func (session *Session) SightOf(slot AuthoredSlot) (Sight, error) {
	if session.loaded == nil {
		return Sight{}, ErrNoSaveLoaded
	}
	actor, err := slot.withBody(session.service.families)
	if err != nil {
		return Sight{}, err
	}
	own := session.loaded.formations.Senses[actor.Think]
	return savecore.LevelSight(actor.Perception.Name(), actor.Body, actor.Front, own, actor.NoTurn), nil
}
