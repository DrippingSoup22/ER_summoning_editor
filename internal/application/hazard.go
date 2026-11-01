package application

// The actors seen to hurt the player who summoned them. Every entry was
// seen in game, and the formation panel shows it as a warning, never a
// refusal: what it damages is a decision the player is entitled to make.

// hurtsSummoner is what has actually been seen to hurt its own summoner, and
// how. Curated, seen, and deliberately short: nothing is on this list
// because it looks like it might be.
//
// The mechanism is not the obvious one, and curation/TESTING.md section 10 has
// it in full. Ally-to-ally ATTACK damage is already blocked: research threw a
// Cursed-Blood Pot with both AtkParam target bits set at the player's own
// spirit summon and it did nothing, because the bits decide which actors a
// hitbox registers on and a second team gate still zeroes the payload. Standing
// next to the Furnace Golem is safe for exactly that reason. What got through
// was not an attack - one of these runs the player over as it turns, and the
// other sets the ground alight and the fire does the rest. Mass and a placed
// area effect are not attacks and no team gate covers them.
//
// Which is also why this cannot be turned off. The matrix that decides who is
// allied with whom is in the executable; the bits that do express it live in
// AtkParam and BulletParam, which are per attack and shared with every other
// actor that uses them; and NpcParam, the whole of what the clone rewrites,
// holds none of it. A warning is what is left, and it is what the formation
// panel shows.
//
// An actor NOT on this list is not a promise that it is safe. It means nobody
// has seen it hurt its summoner, which across most of the catalog means nobody
// has yet tried.
var hurtsSummoner = map[string]string{
	"c5170": "its stomp sets the ground alight, and the fire burns whoever is standing in it",
	"c8120": "it runs over whatever it turns into, and nothing can kill it to stop it",
}

// HurtsSummoner is how this actor has been seen to damage the player who
// summoned it, and whether it has been seen to at all.
//
// Keyed by model, and the frontend's way in: a slot holding one of these is
// worth a word before the player finds out in a boss arena. An empty result
// says only that nothing has been seen - see hurtsSummoner.
func HurtsSummoner(model string) (string, bool) {
	why, found := hurtsSummoner[model]
	return why, found
}
