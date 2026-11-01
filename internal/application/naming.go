package application

// What to call an actor.
//
// The catalog carries the name the game authored for every one of its 6,279
// rows, and those names do two jobs at once. They say what the actor is - a
// spear skeleton rather than a crossbow one - and they say where it was
// standing, which for a summon is not a fact about it at all. This splits the
// two, keeping the first and dropping the second.

import (
	"regexp"
	"strings"
)

// meaningfulQualifiers are the parenthesised words that describe the actor, so
// the ones that survive into the name a player reads.
//
// It is an allow-list rather than a list of places to remove, and that is the
// whole design. The words worth keeping are a closed set - weapons, states,
// sizes - while place names are a long tail that grows with every region the
// game adds. An unrecognised qualifier is dropped, so a new region's enemies
// join their family instead of arriving as a new entry each.
var meaningfulQualifiers = map[string]bool{
	// What it is carrying, which is what it fights with.
	"Bow": true, "Crossbow": true, "Curved Swords": true, "Dagger": true,
	"Fists": true, "Flail": true, "Glaive": true, "Greatsword": true,
	"Greatsword + Poison Breath": true, "Hammer": true, "Ringblade": true,
	"Scythe": true, "Spear": true, "Staff": true, "Sword": true,
	"Sword and Shield": true, "Torchpole": true, "Torchpole, Cloak": true,
	"Twinblade": true, "Banner": true, "Chest": true, "Candles": true,
	"Pot": true, "Jellyfish": true,
	// What it does, or what has been done to it.
	"Accursed": true, "Blood": true, "Burning": true, "Burrowed": true,
	"Death-serving": true, "Frenzy": true, "Ice Lightning": true,
	"Maggots": true, "Magic": true, "Mirage": true, "Mounted": true,
	"Mushroom": true, "Poison Breath": true, "Poisoned": true, "Puppet": true,
	"Replete": true, "Rotten": true, "Sorcerer": true, "Spirit": true,
	"Stinger": true, "Stinger, Large Abdomen": true, "Stranded Soul": true,
	// What it looks like, where that is the actor and not an effect.
	"Dragon Head": true, "Lion Head": true, "Golden Wings": true,
	"Green": true, "Horned": true, "Winged": true, "Surcoat": true,
	"Phantom": true, "Regal": true, "Nonregal": true,
	// How big it is.
	"Large": true, "Small": true,
}

// arenaTail matches the place a name trails off into: "(Limgrave) - Fringefolk
// Hero's Grave". It is only ever removed after a dropped qualifier, so the
// weapon tails - "Man-Serpent - Magma Blade", "Clayman - Spear" - survive.
var arenaTail = regexp.MustCompile(`\s+-\s+.*$`)

// bracketed is the "[Boss]" some placements carry, which says which fight the
// row belongs to rather than what the actor is.
var bracketed = regexp.MustCompile(`\s*\[[^\]]*\]`)

// qualifier matches one parenthesised group.
var qualifier = regexp.MustCompile(`\s*\(([^)]*)\)`)

// duplicateSuffix is the trailing " A" / " B" on names the catalog had to tell
// apart and a player does not.
var duplicateSuffix = regexp.MustCompile(` [AB]$`)

// internalStates are annotations from the game's own tooling that reached the
// names: a battle state and a healthbar owner. Neither is an actor.
var internalStates = []string{" -Passive Battle State-", " Healthbar"}

// DisplayName is the name to show for an actor.
//
// Scaling and arena qualifiers - Boss, Unscaled, Base - are dropped with the
// places. They mark the same enemy re-tuned for a fight rather than a
// different enemy, and without this a single moveset's rows could carry
// "Crucible Knight", "Crucible Knight (Boss)" and "Crucible Knight
// (Unscaled)" depending on which one happened to be its representative.
func DisplayName(name string) string {
	for _, state := range internalStates {
		name = strings.ReplaceAll(name, state, "")
	}
	name = duplicateSuffix.ReplaceAllString(name, "")

	kept := make([]string, 0, 2)
	dropped := false
	stripped := qualifier.ReplaceAllStringFunc(name, func(match string) string {
		word := strings.TrimSpace(qualifier.FindStringSubmatch(match)[1])
		words, unknown := classify(word)
		for _, one := range words {
			kept = appendOnce(kept, one)
		}
		if unknown {
			dropped = true
		}
		return ""
	})
	if dropped {
		stripped = arenaTail.ReplaceAllString(stripped, "")
	}
	stripped = bracketed.ReplaceAllString(stripped, "")
	stripped = strings.Join(strings.Fields(stripped), " ")

	if len(kept) == 0 {
		return stripped
	}
	return stripped + " (" + strings.Join(kept, ", ") + ")"
}

// classify decides whether one parenthesised group is meaningful.
//
// A group is usually one word - "(Spear)" - but this function also has to
// make sense of its own output fed back in: two kept qualifiers are joined
// with ", " into one group, "(Large, Maggots)", so that a family with several
// weapon variants does not carry a run of empty parentheses. Read back, that
// joined group is not itself in the allow-list, so it is split on ", " and
// accepted whole only when every part is - which is what makes DisplayName
// idempotent rather than merely convergent on the first pass.
func classify(word string) (kept []string, unknown bool) {
	if meaningfulQualifiers[word] {
		return []string{word}, false
	}
	parts := strings.Split(word, ", ")
	if len(parts) < 2 {
		return nil, true
	}
	for _, part := range parts {
		if !meaningfulQualifiers[part] {
			return nil, true
		}
	}
	return parts, false
}

func appendOnce(words []string, word string) []string {
	for _, seen := range words {
		if seen == word {
			return words
		}
	}
	return append(words, word)
}
