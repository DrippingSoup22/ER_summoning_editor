package application

// What one creature allows, read off the catalog: its spirit look, and the
// roles it takes.
//
// The look is the author's:
// a summon is drawn in the game's ghost or in its own colours with no
// overwrite at all, and the player chooses - except on a body the ghost loses
// a part of (own colours only), a creature with a tint of its own (the tint
// shows whatever the look, so it has none to choose), a person the game ships
// (it arrives on its shell, as shipped) and a row the game ships as an Ash
// (never written). A named character with a tint loses it, in both looks. The
// player's own characters choose too, between the coat of the Puppet shell
// they copy and their own colours (nothing between),
// on their two bodies (characters.go).
//
// The roles are the ladder unless the author limited a creature: the large Shadow Commoner takes the Statue and the Hunter
// only, on the quiet marker, because under the shipped marker its battle
// script never attacks.
//
// A creature may arrive on an animation of its own (St. Trina is drawn only on her 30000), and one is delivered as a pair
// (partners.csv): the mimic tear holds its spot as the Statue and turns into
// the Silver Tear person, who arrives on the same spot, as the Mimic Tear Ash's
// own two rows do, and plays the slot's role. The pair is two rows from one
// slot; a save read back shows them as the two rows they are.

import (
	"slices"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// Look is how a summon is drawn.
type Look uint8

const (
	// LookDefault is the creature's own: the ghost, or its own colours where
	// the ghost cannot draw it whole.
	LookDefault Look = iota
	// LookGhost is the game's spirit coat (295000, as shipped) - on one of
	// the player's characters, the coat of its Puppet shell (295200).
	LookGhost
	// LookOwnColours is the creature with no overwrite at all
	// (savecore.OwnColoursPresence).
	LookOwnColours
)

// CreatureRules is what one creature allows.
type CreatureRules struct {
	// Look is the look it has when the player chooses none, and
	// LookChoosable whether he may; LookFixed says why not, in a few words.
	Look          Look
	LookChoosable bool
	LookFixed     string
	// Levels are the roles it takes, Default first; nil is every role of the
	// ladder, on the Follower by default.
	Levels  []Perception
	Default Perception
	// marker is the companion marker its clone carries in place of 297000;
	// strip the tint effects it loses; arrival the animation it arrives on,
	// zero for the spirit arrival; partner the person a pair brings.
	marker  int32
	strip   []int32
	arrival int32
	partner partner
}

// partner is the person a pair brings onto its creature's spot: its
// CharaInitParam row, the person shell it stands on and the animation it
// arrives on. Zero is no pair.
type partner struct{ person, shell, arrival int32 }

// TakesLevel reports whether the creature takes a role.
func (rules CreatureRules) TakesLevel(level Perception) bool {
	return rules.Levels == nil || slices.Contains(rules.Levels, level)
}

// everyRole is the rules of a creature nobody limited: the player's choice of
// look, and every role.
var everyRole = CreatureRules{Look: LookGhost, LookChoosable: true, Default: PerceptionFollower}

// creatureRulesOf reads the rules off every creature entry of the catalog, by
// each world row the entry holds.
func creatureRulesOf(categories []catalog.HierarchyCategory) map[int32]CreatureRules {
	out := map[int32]CreatureRules{}
	for _, category := range categories {
		for _, card := range category.Cards {
			for _, one := range card.Types {
				for _, entry := range one.Entries {
					if entry.Trunk != "creature" || AsShipped(entry) {
						continue
					}
					rules := rulesOf(entry.Tags)
					for _, row := range append([]string{strconv.Itoa(int(entry.Ships))}, strings.Split(entry.Tags["members"], "|")...) {
						if id, err := strconv.ParseInt(row, 10, 32); err == nil && id > 0 {
							out[int32(id)] = rules
						}
					}
				}
			}
		}
	}
	return out
}

func rulesOf(tags map[string]string) CreatureRules {
	rules := everyRole
	switch tags["look"] {
	case "own-colours":
		rules.Look, rules.LookChoosable, rules.LookFixed = LookOwnColours, false, "own colours only: the ghost loses a part of its body"
	case "own-tint":
		rules.Look, rules.LookChoosable, rules.LookFixed = LookGhost, false, "it carries a tint of its own"
	}
	for _, effect := range strings.Split(tags["look_strip"], "|") {
		if id, err := strconv.ParseInt(effect, 10, 32); err == nil {
			rules.strip = append(rules.strip, int32(id))
		}
	}
	if levels := tags["levels"]; levels != "" {
		rules.Levels = nil
		for _, name := range strings.Split(levels, "|") {
			rules.Levels = append(rules.Levels, formation.Perception(name))
		}
		rules.Default = formation.Perception(tags["level_default"])
		slices.SortStableFunc(rules.Levels, func(a, b Perception) int {
			if a == rules.Default {
				return -1
			}
			if b == rules.Default {
				return 1
			}
			return 0
		})
	}
	if marker, err := strconv.ParseInt(tags["marker"], 10, 32); err == nil {
		rules.marker = int32(marker)
	}
	if arrival, err := strconv.ParseInt(tags["arrival"], 10, 32); err == nil {
		rules.arrival = int32(arrival)
	}
	if fields := strings.Split(tags["partner"], ":"); len(fields) == 3 {
		var ids [3]int32
		for index, field := range fields {
			id, err := strconv.ParseInt(field, 10, 32)
			if err != nil {
				return rules
			}
			ids[index] = int32(id)
		}
		rules.partner = partner{person: ids[0], shell: ids[1], arrival: ids[2]}
	}
	return rules
}

// CreatureRules is what the creature a slot names allows. A person and a row
// the game ships as an Ash arrive as shipped: no look to choose.
func (session *Session) CreatureRules(slot AuthoredSlot) CreatureRules {
	return session.service.rulesFor(slot)
}

func (service *Service) rulesFor(slot AuthoredSlot) CreatureRules {
	switch {
	case slot.Person > 0 && isOwnBody(formation.Actor{Npc: slot.Npc, Person: slot.Person}):
		return CreatureRules{Look: LookGhost, LookChoosable: true, Default: PerceptionFollower}
	case slot.Person > 0:
		return CreatureRules{Look: LookGhost, LookFixed: "a person arrives on its shell, as shipped", Default: PerceptionFollower}
	case slot.Npc >= formation.CloneOffset:
		return CreatureRules{Look: LookGhost, LookFixed: "the game's own summon, as shipped", Default: PerceptionFollower}
	}
	if rules, known := service.rules[slot.Npc]; known {
		return rules
	}
	return everyRole
}

// applyRules holds every slot to its creature's rules and writes what they
// decide onto it: the presence its look asks, the marker its creature
// carries, the animation it arrives on - and a pair's partner, as a slot of
// its own right after it. A research seat's own presence stands.
func (service *Service) applyRules(slots []AuthoredSlot) ([]AuthoredSlot, error) {
	out := slices.Clone(slots)
	if slices.ContainsFunc(out, func(slot AuthoredSlot) bool { return service.rulesFor(slot).partner.person > 0 }) {
		// A pair's two rows stand on one spot, so the ring a slot without one
		// would stand on is fixed as the player saw it, before the partner
		// lengthens the formation.
		for index := range out {
			if out[index].Placement == nil {
				place := RingPlacement(index, len(out))
				out[index].Placement = &place
			}
		}
	}
	ruled := make([]AuthoredSlot, 0, len(out))
	for index := range out {
		slot := &out[index]
		rules := service.rulesFor(*slot)
		if slot.Look != LookDefault && slot.Look != rules.Look && !rules.LookChoosable {
			return nil, &SlotRuleError{Slot: index + 1, Npc: slot.Npc, Why: rules.LookFixed, cause: ErrLookFixed}
		}
		enemy := slot.WorldBranch && slot.Team != TeamSummon
		if !enemy && slot.Perception != PerceptionAsHeld && !rules.TakesLevel(slot.Perception) {
			return nil, &SlotRuleError{Slot: index + 1, Npc: slot.Npc, Why: "it takes " + levelNames(rules.Levels), cause: ErrLevelNotOffered}
		}
		look := slot.Look
		if look == LookDefault || !rules.LookChoosable {
			look = rules.Look
		}
		switch {
		case slot.Person > 0:
			// A character's look is the body it stands on, chosen or kept.
			if rules.LookChoosable && slot.Look != LookDefault {
				slot.Npc = CharacterBodyIn(slot.Npc, slot.Look)
			}
		case look == LookOwnColours && slot.Presence == 0:
			slot.Presence = savecore.OwnColoursPresence
		}
		if !enemy {
			slot.marker = rules.marker
		}
		slot.arrival = rules.arrival
		if rules.partner.person == 0 || enemy {
			ruled = append(ruled, *slot)
			continue
		}
		person := AuthoredSlot{Npc: rules.partner.shell, Think: rules.partner.shell, Person: rules.partner.person,
			Perception: slot.Perception, Share: slot.Share, Stance: slot.Stance, Placement: slot.Placement,
			arrival: rules.partner.arrival}
		slot.Perception, slot.Share = PerceptionStatue, ShareDefault
		slot.Stance, _ = LevelStance(*slot)
		ruled = append(ruled, *slot, person)
	}
	return ruled, nil
}

// CharacterLook is how a character on one of its own bodies is drawn -
// LookGhost, its Puppet shell's coat, or LookOwnColours - and false for an
// actor that is no character's body.
func CharacterLook(npc int32) (Look, bool) {
	_, own, _, ok := character.BodyAt(npc)
	switch {
	case !ok:
		return LookDefault, false
	case own:
		return LookOwnColours, true
	}
	return LookGhost, true
}

// CharacterBodyIn is the body a character on its own body stands on in a
// look, in the same role: the own-colours twin, or the Puppet coat's for any
// other look. A row that is no character's body is returned as it is.
func CharacterBodyIn(npc int32, look Look) int32 {
	body, _, fighter, ok := character.BodyAt(npc)
	if !ok {
		return npc
	}
	return character.BodyFor(body, look == LookOwnColours, fighter)
}

// CharacterFighter reports whether a character on one of its own bodies is
// the Fighter, and false for an actor that is no character's body. A
// character's role is the body it stands on, chosen in the creature window as
// its look is (characters.go).
func CharacterFighter(npc int32) (fighter, ok bool) {
	_, _, fighter, ok = character.BodyAt(npc)
	return fighter, ok
}

// CharacterBodyAs is the body a character on its own body stands on in a role,
// in the same look. A row that is no character's body is returned as it is.
func CharacterBodyAs(npc int32, fighter bool) int32 {
	body, own, _, ok := character.BodyAt(npc)
	if !ok {
		return npc
	}
	return character.BodyFor(body, own, fighter)
}

func levelNames(levels []Perception) string {
	names := make([]string, len(levels))
	for index, level := range levels {
		names[index] = level.Name()
	}
	return strings.Join(names, " and ")
}
