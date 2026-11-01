package catalog

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// The vendored hierarchy, read as tags. These tests pin what a frontend can
// rely on - the levels are there, every entry is one row, every offered entry
// ships a row - and deliberately NOT the vocabulary: research owns the
// categories, the cards, the types and the words, and the whole point of the
// seam is that any of them can change without a test here.

func read(t *testing.T) []HierarchyCategory {
	t.Helper()
	out, err := ReadHierarchy(reference.HierarchyCatalog())
	if err != nil {
		t.Fatalf("ReadHierarchy: %v", err)
	}
	return out
}

func TestEveryEntryAppearsOnceAndEveryOfferedEntryShipsARow(t *testing.T) {
	seen := map[string]bool{}
	offers := map[string]int{}
	for _, category := range read(t) {
		for _, card := range category.Cards {
			for _, one := range card.Types {
				for _, entry := range one.Entries {
					if seen[entry.Key] {
						t.Errorf("entry %s appears twice", entry.Key)
					}
					seen[entry.Key] = true
					offers[entry.Offer]++
					switch entry.Offer {
					case "ship", "ship-with-warning":
						if entry.Ships == 0 {
							t.Errorf("%s is offered (%s) and ships no row", entry.Key, entry.Offer)
						}
						if entry.Trunk == "creature" && entry.Think == 0 {
							t.Errorf("%s is a creature offered without a think row", entry.Key)
						}
						if entry.Offer == "ship-with-warning" && entry.Warning == "" {
							t.Errorf("%s warns and says nothing", entry.Key)
						}
					case "hold":
					default:
						t.Errorf("%s has an offer this package does not know: %q", entry.Key, entry.Offer)
					}
					if entry.Trunk == "person" && entry.Think != 0 {
						t.Errorf("%s is a person with a think row of its own", entry.Key)
					}
				}
			}
		}
	}
	t.Logf("offers: %v", offers)
	for _, must := range []string{"ship", "ship-with-warning", "hold"} {
		if offers[must] == 0 {
			t.Errorf("no entry is offered %q; the file does not exercise the vocabulary", must)
		}
	}
}
