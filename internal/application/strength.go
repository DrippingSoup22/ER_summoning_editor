package application

// Summon strength: which role a summon plays.
//
// A summon is its body, times the Ash's level, times its role (docs/DESIGN.md,
// Summon strength; internal/strength). A Helper carries the game's helper
// package, which halves what it takes and quarters what it deals, and 2.7x its
// health; a Fighter is its body as it is. The Ash's level is the ladder each
// formation row ships with, whichever the role.
//
// The role is each creature's. A creature the player drops is cloned, and its
// role is part of the clone's recipe (AuthoredSlot.Fighter): a Helper and a
// Fighter of one creature are two rows. A row the save already holds is never
// cloned, so FromSoftware's own summon rows take the role row-wide
// (rowstrength.go).

import (
	"slices"
	"strconv"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/strength"
)

// authoredStrength is what the clones of each creature a formation names carry
// beyond their recipe, keyed by the world actor row the player dropped, which
// is what plan.BuildAuthor keys by: the health of each role, and the effects
// the creature is granted or loses.
//
// Residents a slot asked for ride on the same entry, ON TOP OF the actor's own
// granted set unless the slot asked for the bare row: a control effect added to
// a seat must not silently remove what the map gave the actor. A creature's
// granted and asked residents are the CREATURE's, not the slot's: they are
// added once however many slots summon it - a second copy is a second
// multiplier for a granted buff, and a resident slot spent either way.
//
// Removes are the resident effects a composite takes off the creature
// (catalog Variant.Removed - the God-Devouring Serpent's respawn state).
func (service *Service) authoredStrength(slots []AuthoredSlot) plan.Strength {
	byRow := service.familiesByRow()
	wanted := plan.Strength{}
	added := map[int32]map[int32]bool{}
	for _, slot := range slots {
		world := slot.resolve().Npc
		if world >= formation.CloneOffset {
			// Any clone band: the creature is the row's last eight digits.
			world %= formation.CloneOffset
		}
		entry, present := wanted[world]
		if !present {
			body := bodyHealth(byRow, world)
			entry = plan.ActorStrength{HelperHP: strength.Helper(body), FighterHP: body}
			added[world] = map[int32]bool{}
		}
		var residents, removes []int32
		if !slot.BareRow {
			residents = append(residents, service.grantedFor(world, byRow)...)
			removes = service.removedFor(world, byRow)
		}
		for _, asked := range slot.Residents {
			if slotOfEffect(residents, asked) < 0 {
				residents = append(residents, asked)
			}
		}
		for _, effect := range residents {
			if !added[world][effect] {
				added[world][effect] = true
				entry.Residents = append(entry.Residents, effect)
			}
		}
		if len(entry.Removes) == 0 {
			entry.Removes = append([]int32(nil), removes...)
		}
		wanted[world] = entry
	}
	return wanted
}

// bodyHealth is a creature's own health, its world row's - what a Fighter's
// clone carries, and what a Helper's is 2.7x of. Zero - the copied row's own -
// for a row the catalog does not list, which only a research seat names.
func bodyHealth(byRow map[int32]catalog.Family, world int32) int32 {
	for _, variant := range byRow[world].Variants {
		if variant.Npc == world && variant.HP > 0 {
			return variant.HP
		}
	}
	return 0
}

// removedFor is the catalog's removed set for a world row (a composite that
// takes a resident off the creature), or nil.
func (service *Service) removedFor(world int32, byRow map[int32]catalog.Family) []int32 {
	var removed []int32
	for _, variant := range byRow[world].Variants {
		if variant.Npc == world {
			removed = slices.Clone(variant.Removed)
		}
	}
	// A named character loses the tint of its own, in both looks (look.go).
	for _, effect := range service.rules[world].strip {
		if !slices.Contains(removed, effect) {
			removed = append(removed, effect)
		}
	}
	return removed
}

// grantedFor is the catalog's granted set for a world row, or nil.
func (service *Service) grantedFor(world int32, byRow map[int32]catalog.Family) []int32 {
	family, known := byRow[world]
	if !known {
		return nil
	}
	for _, variant := range family.Variants {
		if variant.Npc == world {
			return variant.Granted
		}
	}
	return nil
}

// hostSelector names an Ash the way StageFormation asks for one.
func hostSelector(goodsFamily int32) string { return strconv.Itoa(int(goodsFamily)) }

// familiesByRow is the catalog keyed by world actor row.
func (service *Service) familiesByRow() map[int32]catalog.Family {
	byRow := make(map[int32]catalog.Family, 8192)
	for _, family := range service.families {
		for _, variant := range family.Variants {
			byRow[variant.Npc] = family
		}
	}
	return byRow
}

// slotOfEffect is the index of effect in effects, or -1.
func slotOfEffect(effects []int32, effect int32) int {
	for index, held := range effects {
		if held == effect {
			return index
		}
	}
	return -1
}
