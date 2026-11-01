package gio

// The Ash rail, and the snapshot the whole Editor reads from.
//
// Everything here is built once, in the worker, while it still holds the
// session - see state.go for why the frame may never call the session itself.
// So the rail is a list of plain structs and choosing an Ash costs a slice
// index rather than a read of the save. The rail itself is drawn by
// formation.go (layoutAshList).

import (
	"fmt"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// maxSlots is the ten. It is the application's constant rather than a second
// copy of the number, because the grid below is only correct while the two
// agree.
const maxSlots = application.MaxFormationSlots

// ashView is one Spirit Ash as the frame may describe it.
type ashView struct {
	Name        string
	GoodsFamily int32

	// Ships is how many rows this Ash's own formation block carries, which is
	// also how many of the ten slots exist in the save today. Slots past it are
	// rows a commit would insert.
	Ships int
	// RePointed means an earlier conversion left this Ash delivering another
	// family's formation, so what it summons in game is not what Slots says it
	// owns. Worth saying plainly rather than showing two truths at once.
	RePointed bool

	Price application.Price
	// PriceVaries means the eleven upgrade rows disagree, which nothing this
	// product writes can produce. One number would then be an invention.
	PriceVaries bool

	Slots [maxSlots]slotView
}

// slotView is one of the ten. An unfilled slot is the zero value, which is what
// makes the grid a fixed array rather than a list with a length.
type slotView struct {
	Filled bool
	// Npc and Think are the actor's rows, in whichever form the slot came by
	// them: a shipped formation names the summon-side clone and an authored one
	// names the world row. Both are accepted where a formation is composed, so
	// re-staging a shipped slot verbatim inserts nothing.
	Npc, Think int32
	// Person is the CharaInitParam row a person slot summons on the shell Npc
	// names; zero for a creature.
	Person int32
	// Name is the actor's own name and Family the model it belongs to, shown
	// only when the two differ. NoActor marks a shipped row that names no
	// actor at all - six of them exist - which is a real state and not an
	// empty slot.
	Name    string
	Family  string
	NoActor bool
	// Model is the family's model id, and is here for one thing: it is what
	// application.HurtsSummoner is keyed by, so a slot can say whether what is
	// in it has been seen to damage the player who summoned it. Empty for a
	// row nothing could be resolved for, which warns about nothing.
	Model string
	// Shipped marks a slot that summons rows the save already holds, as they
	// are: a row the host already holds, staged as PerceptionAsHeld (as the
	// file holds it, not a level) - or an Ashes card, staged on the
	// Follower, which FromSoftware's row is (application.AsShipped). Either
	// way it keeps Stance, the row's own, with Share agreeing with it, so
	// re-staging it beside a new slot changes nothing about it.
	Shipped bool
	Stance  application.Stance
	Level   application.Perception
	Share   application.Share
	// Enemy puts the slot on the enemy team, on the world branch, as the Loner
	// (the creature window's switch).
	Enemy bool
	// Look is the ghost or the creature's own colours, where its creature
	// lets the player choose (the creature window's switch).
	Look application.Look
	// Fighter summons the creature as itself rather than as a Helper (the
	// creature window's strength). A row the save holds - a shipped slot, a
	// person's shell - carries its own instead (State.strengthOf).
	Fighter bool
	// Place is where the slot stands and faces. Placed says it is the slot's
	// own - read from the file or put there on the grid; a slot without one
	// stands where the writer's ring puts it (application.RingPlacement).
	Place  application.Placement
	Placed bool
}

// buildAshes turns the session's read models into the rail.
//
// names labels a slot by actor row, under both of the rows an actor can be
// named by: the world row an authored slot carries, and the summon-side clone
// a shipped formation names. It comes from Session.ActorLabels rather than
// from the browsable Entities list, and deliberately so - Entities groups rows
// into one entry per spirit, and a shipped formation is free to name any row
// in a group, not only the one Entities chose to represent it. A lookup built
// from the grouped list would leave every folded-away row showing a number.
func buildAshes(states []application.AshState, names map[int32]application.ActorLabel) []ashView {
	views := make([]ashView, 0, len(states))
	for _, state := range states {
		view := ashView{
			Name:        state.Name,
			GoodsFamily: state.GoodsFamily,
			Ships:       len(state.Slots),
			RePointed:   state.RePointed,
			Price:       state.Price,
			PriceVaries: state.PriceVariesByLevel,
		}
		for index, slot := range state.Slots {
			if index >= maxSlots {
				break
			}
			cell := slotView{Filled: true, Npc: slot.Npc, Think: slot.Think, Person: slot.Person,
				Shipped: true, Stance: slot.Stance, Level: application.PerceptionAsHeld,
				Place: slot.Place, Placed: true}
			known, ok := names[slot.Npc]
			if !ok {
				// A creature on a second recipe is a clone in a higher band;
				// it is labelled as the creature it copies.
				if source, cloned := application.CloneSource(slot.Npc); cloned {
					known, ok = names[source]
				}
			}
			switch {
			case slot.Npc < 0:
				cell.NoActor = true
				cell.Name = "No actor"
			case ok:
				cell.Name, cell.Family, cell.Model = known.Name, known.Family, known.Model
			default:
				cell.Name = fmt.Sprintf("Actor %d", slot.Npc)
			}
			view.Slots[index] = cell
		}
		// A block longer than ten cannot be authored and is not something the
		// game ships, but reporting a count the grid cannot show would make the
		// panel lie about itself.
		if view.Ships > maxSlots {
			view.Ships = maxSlots
		}
		views = append(views, view)
	}
	return views
}

// nameHeld names every slot a formation holds as a drop of the same entry is
// named, so a reopened save reads as it was composed. A person is named by its
// CharaInitParam row - the shell or body it stands on names nobody - and one
// the catalog does not offer (a built character, which only a research seat
// makes) by that row. A creature on the game's own row is named by its Ashes
// card, and one on a clone by the world rows the clone copies. A creature the
// catalog does not offer keeps its actor label.
func (view *savedView) nameHeld() {
	for ash := range view.Ashes {
		for index, slot := range view.Ashes[ash].Slots {
			if !slot.Filled || slot.NoActor {
				continue
			}
			label, known := view.heldLabel(slot)
			if !known && slot.Person != 0 {
				label, known = application.ActorLabel{Name: fmt.Sprintf("Person %d", slot.Person)}, true
			}
			if known {
				cell := &view.Ashes[ash].Slots[index]
				cell.Name, cell.Family, cell.Model = label.Name, label.Family, label.Model
			}
		}
	}
}

// heldLabel is the catalog's name for a row a formation holds.
func (view *savedView) heldLabel(slot slotView) (application.ActorLabel, bool) {
	if label, known := view.dragged(slot.Npc, slot.Think, slot.Person); known || slot.Person != 0 {
		return label, known
	}
	// The game's own row, whatever think row its card names; its number may
	// look like a clone's (the DLC's Ashes sit where Commander Gaius's would),
	// so this comes first.
	if label, _, known := view.findVariant(func(variant variantView) bool {
		return variant.Shipped && variant.Npc == slot.Npc
	}); known {
		return label, true
	}
	npc, cloned := application.CloneSource(slot.Npc)
	think, thinkCloned := application.CloneSource(slot.Think)
	if !cloned || !thinkCloned {
		return application.ActorLabel{}, false
	}
	return view.dragged(npc, think, 0)
}

// selectedAsh is the Ash whose formation is open, or nil when none is.
func (s *State) selectedAsh(loaded *savedView) *ashView {
	if loaded == nil || s.selected < 0 || s.selected >= len(loaded.Ashes) {
		return nil
	}
	return &loaded.Ashes[s.selected]
}
