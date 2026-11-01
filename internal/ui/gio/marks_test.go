package gio

import (
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// A warning is research's shorthand; a tile shows marks and the player's
// words, with no row, animation or hazard code left in them, and only what is
// true of this row: a family hazard true of some of the model's rows (stands,
// vanishes, spawner) says nothing here, while research's own clause for the
// row does.
func TestMarksSayAWarningInThePlayersWords(t *testing.T) {
	leaf := application.HierarchyEntry{
		Trunk: "creature", Tags: map[string]string{"tag": "extra", "dlc": "yes"},
		Warning: "rides a mount in the world: summoned, it arrives on foot; " +
			"not every placement's knight: the blood slash (3024; 3025), which only some placements give; " +
			"its casts exist, but the witness saw it do nothing; vanishes; family hazard: stands|vanishes|spawner|scale",
	}
	marks, notes := marksAndNotes(leaf)
	if want := markExtra | markOnFoot | markNote; marks != want {
		t.Errorf("marks %05b, want %05b", marks, want)
	}
	want := []string{
		"Has the blood slash, which only some placements give.",
		"Vanishes soon after it arrives.",
		"A large body: summon it in the open.",
	}
	if !slices.Equal(notes, want) {
		t.Errorf("notes %q,\nwant %q", notes, want)
	}
}

// A card is a statue only if all of it is, and has a note if any of it does.
func TestACardsMarksAreItsVariantsRule(t *testing.T) {
	statue := variantView{Marks: markStatue | markOnFoot}
	noted := variantView{Marks: markOnFoot | markNote, Notes: []string{"Runs away from enemies."}}
	if marks, notes := marksOver([]variantView{statue, noted}); marks != markOnFoot|markNote || len(notes) != 1 || notes[0] == noted.Notes[0] {
		t.Errorf("a mixed card shows %05b %q: want on foot and a note pointing inside", marks, notes)
	}
	if marks, notes := marksOver([]variantView{noted, noted}); marks != markOnFoot|markNote || !slices.Equal(notes, noted.Notes) {
		t.Errorf("a card of one note shows %05b %q: want that note", marks, notes)
	}
}
