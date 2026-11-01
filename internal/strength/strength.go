// Package strength holds the numbers a summon's role writes.
//
// A summon is three layers, multiplied (docs/DESIGN.md, Summon strength):
//
//   - its body: the creature's own row, before any region multiplies it;
//   - the Ash's level: the ladder the Ash ships with, which for a creature's
//     Ash is rung for rung the world's region tiers (+10 is 7200, the game's
//     top), so this package writes no ladder at all;
//   - its role, each creature's: the Fighter is the body as it is, the Helper
//     carries the game's helper package and more health.
//
// One number is the product's own: how much health a Helper carries.
package strength

import "math"

// Levels is how many upgrade levels an Ash has, and so how long a ladder is.
// It must equal formation.DopingLevels; the packages do not import each other
// and internal/formation asserts the pair.
const Levels = 11

// HelperHealth is what a Helper's health is over the creature's own: the median
// of FromSoftware's own summons over the world rows they come from (94 summons
// whose world row the catalog holds, measured 2026-10-25; the middle 80% lie
// between x1.0 and x5.0). With the package's half damage taken, a Helper
// survives like the game's own Ashes and hits like them.
const HelperHealth = 2.7

// Helper is the health a Helper carries for a body of this much.
func Helper(body int32) int32 { return int32(math.Round(float64(body) * HelperHealth)) }

// Body is the health a Fighter carries where only a Helper's health is known:
// FromSoftware's own summon rows, whose health is a Helper's.
func Body(helper int32) int32 { return int32(math.Round(float64(helper) / HelperHealth)) }

// Taxes are the shipped effects that reduce what a summon deals or takes, and
// so the effects the Fighter goes without.
//
// By meaning rather than by number range, which is the whole point of the list.
// A rule of "strip the 296xxx band" would take the Mausoleum Knight's
// `+25% damage` off, making one creature weaker as a Fighter than the base
// game gives it, and would take target priority off 106 rows - a change to how
// enemies choose what to attack that nobody asked the role for.
//
// Measured from SpEffectParam on regulation 11701000; the whole table of what
// the 115 shipped summon rows carry is in the workspace's
// archive/history/editor/SCALING.md.
var Taxes = []int32{
	// On all 115: -75% damage, +50% absorption, -95% poise damage.
	296000,
	// A second cut on top of it, on one row each: the Depraved Perfumer at
	// -25% and the Battlemage at -30%.
	296117,
	296122,
}

// Taxed reports whether an effect is one of them.
func Taxed(effect int32) bool {
	for _, tax := range Taxes {
		if tax == effect {
			return true
		}
	}
	return false
}
