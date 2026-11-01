package character

// A summoned character casts a spell's NPC copy where the game has one: the
// same spell with no requirement, 1 FP and the AI judgement FromSoftware gave
// its own people, which a summon often casts more freely (on the console: the
// player's Comet once in a long fight, its copy again and again). The builder
// offers the player spell and the row holds the copy.

import (
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// spellCopies maps a player spell to its copy; copiedSpells is the inverse.
var spellCopies, copiedSpells = readSpellCopies()

// CastAs is the Magic row a character's row holds for spell: its NPC copy,
// or the spell itself where it has none.
func CastAs(spell int32) int32 {
	if copy, ok := spellCopies[spell]; ok {
		return copy
	}
	return spell
}

// offeredAs is CastAs' inverse: the player spell a row's copy stands for.
func offeredAs(held int32) int32 {
	if spell, ok := copiedSpells[held]; ok {
		return spell
	}
	return held
}

// readSpellCopies reads reference/spell-copies.csv: spell,copy,... with a
// header. The table is embedded, so a bad one is a build defect.
func readSpellCopies() (map[int32]int32, map[int32]int32) {
	records, err := csv.NewReader(reference.SpellCopyCatalog()).ReadAll()
	if err != nil || len(records) < 2 || records[0][0] != "spell" || records[0][1] != "copy" {
		panic(fmt.Sprintf("reference/spell-copies.csv is not a spell,copy table: %v", err))
	}
	copies, spells := map[int32]int32{}, map[int32]int32{}
	for _, record := range records[1:] {
		spell, err1 := strconv.ParseInt(record[0], 10, 32)
		copy, err2 := strconv.ParseInt(record[1], 10, 32)
		if err1 != nil || err2 != nil || spells[int32(copy)] != 0 {
			panic(fmt.Sprintf("reference/spell-copies.csv: bad row %v", record))
		}
		copies[int32(spell)], spells[int32(copy)] = int32(copy), int32(spell)
	}
	return copies, spells
}
