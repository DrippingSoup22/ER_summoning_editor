package application_test

// What a name becomes. DisplayName cleans a raw catalog name for display.

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// What a label keeps decides what a player reads on a card, and what it drops
// decides whether two rows of one moveset vote for the same name.
func TestADisplayNameKeepsTheActorAndDropsThePlace(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		// What it fights with survives, because that is the choice.
		"Large Skeleton (Spear)":         "Large Skeleton (Spear)",
		"Crystalian (Ringblade)":         "Crystalian (Ringblade)",
		"Man-Serpent - Magma Blade Hot":  "Man-Serpent - Magma Blade Hot",
		"Wandering Noble (Sorcerer)":     "Wandering Noble (Sorcerer)",
		"Putrid Corpse (Large, Maggots)": "Putrid Corpse (Large, Maggots)",

		// Where it stood does not.
		"Fingercreeper (Leyndell)":                               "Fingercreeper",
		"Gargoyle (Leyndell - Royal Capital)":                    "Gargoyle",
		"Black Blade Kindred (Dragonbarrow - Bestial Sanctum)":   "Black Blade Kindred",
		"Merciless Chariot (Limgrave) - Fringefolk Hero's Grave": "Merciless Chariot",
		"Fallingstar Beast (Scaduview) - Hinterland [Boss]":      "Fallingstar Beast",

		// Nor does how the fight was tuned.
		"Crucible Knight (Boss)":     "Crucible Knight",
		"Crucible Knight (Unscaled)": "Crucible Knight",
		"Crystalian (Boss)":          "Crystalian",

		// Nor do the game's own annotations.
		"Gargoyle -Passive Battle State-":                 "Gargoyle",
		"Gargoyle (Greatsword) -Passive Battle State-":    "Gargoyle (Greatsword)",
		"Dragonkin Soldier of Nokstella Healthbar (Boss)": "Dragonkin Soldier of Nokstella",
		"Twinsage Glintstone Sorcerer A":                  "Twinsage Glintstone Sorcerer",

		// A name with nothing to strip comes back as it went in.
		"Banished Knight": "Banished Knight",
	} {
		if got := application.DisplayName(name); got != want {
			t.Errorf("%q became %q, want %q", name, got, want)
		}
	}
}
