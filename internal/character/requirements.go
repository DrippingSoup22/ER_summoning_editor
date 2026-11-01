package character

// What a weapon or a spell requires of the attributes. The builder warns of a
// requirement a character does not meet and builds it anyway: a console test cast a spell whatever the stats, and how often follows
// the spell's own AI judgement.

// Needs is the value each attribute must reach, by its index in
// Build.Attributes; zero is no requirement.
type Needs [8]uint8

// The attributes a requirement names, by their index in Build.Attributes.
const (
	Strength     = 3
	Dexterity    = 4
	Intelligence = 5
	Faith        = 6
	Arcane       = 7
)

// Unmet is one requirement a character falls short of: the attribute, by its
// index in Build.Attributes, and the value it needs.
type Unmet struct {
	Attribute int
	Need      uint8
}

// Shortfalls are the requirements a build does not meet, by hand slot and by
// spell slot.
type Shortfalls struct {
	Hands  [handSlots][]Unmet
	Spells [rowSpells][]Unmet
}

// Count is how many requirements are unmet.
func (s Shortfalls) Count() int {
	count := 0
	for _, unmet := range append(s.Hands[:], s.Spells[:]...) {
		count += len(unmet)
	}
	return count
}

// Shortfalls are the requirements a build does not meet, the weapon's at its
// affinity. A requirement is the one-handed one: a player two-handing a weapon
// counts Strength half again, and nothing says a summon does.
func (c Choices) Shortfalls(b Build) Shortfalls {
	short := func(needs Needs) []Unmet {
		var out []Unmet
		for attribute, need := range needs {
			if need > b.Attributes[attribute] {
				out = append(out, Unmet{Attribute: attribute, Need: need})
			}
		}
		return out
	}
	var out Shortfalls
	for hand, weapon := range b.Hands {
		if weapon.Base != 0 && weapon.Affinity < len(Affinities) {
			out.Hands[hand] = short(c.Weapons[weapon.Base].Needs[weapon.Affinity])
		}
	}
	for slot, spell := range b.Spells {
		if spell != 0 {
			out.Spells[slot] = short(c.Spells[spell])
		}
	}
	return out
}
