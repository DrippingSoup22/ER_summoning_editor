package curation_test

// The curation file is hand-written, so what is worth testing is the ways a
// hand can get it wrong.

import (
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/curation"
)

// The shipped file loads, and it is one row per family with nothing repeated.
// The join against the catalog lives in internal/ui/gio, where the catalog is
// already loaded; what belongs here is the file's own shape.
func TestTheShippedFileLoads(t *testing.T) {
	entities, err := curation.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(entities) != 445 {
		t.Errorf("the curation file holds %d families, want 445", len(entities))
	}
	seen := make(map[string]bool, len(entities))
	for _, entity := range entities {
		if seen[entity.Model] {
			t.Errorf("%s appears twice", entity.Model)
		}
		seen[entity.Model] = true
		if !strings.HasPrefix(entity.Model, "c") {
			t.Errorf("%q is not a cXXXX model", entity.Model)
		}
	}
}

// A category outside the vocabulary fails the load rather than reaching the
// panel. Reaching it would mean a family filed under no filter, showing up in
// the grid only when nothing is selected, with nothing anywhere saying why.
func TestDecodeRefusesAFileItCannotTrust(t *testing.T) {
	const header = "model,name,provenance,kind,standing,family,card,same_as,delivers,note\n"
	const row = "c3661,Putrid Corpse,base_placed,creature,enemy,,,,,\n"

	for _, test := range []struct {
		name, body, want string
	}{
		{"a kind nobody defined",
			header + "c3661,Putrid Corpse,base_placed,monster,enemy,,,,,\n", "not a kind"},
		{"an empty kind",
			header + "c3661,Putrid Corpse,base_placed,,enemy,,,,,\n", "not a kind"},
		{"a standing nobody defined",
			header + "c3661,Putrid Corpse,base_placed,creature,hostile,,,,,\n", "not a standing"},
		{"an empty standing",
			header + "c3661,Putrid Corpse,base_placed,creature,,,,,,\n", "not a standing"},
		{"the same family twice",
			header + row + row, "twice"},
		{"a row with no model",
			header + ",Putrid Corpse,base_placed,creature,enemy,,,,,\n", "no model"},
		{"a family nobody lists",
			header + "c3661,Putrid Corpse,base_placed,creature,enemy,c9999,,,,\n", "does not list"},
		{"a family browsed under itself",
			header + "c3661,Putrid Corpse,base_placed,creature,enemy,c3661,,,,\n", "under itself"},
		{"a chain of two hops",
			header +
				"c3660,Commoner,base_placed,creature,enemy,,,,,\n" +
				"c3661,Putrid Corpse,base_placed,creature,enemy,c3660,,,,\n" +
				"c3662,Putrid Corpse,base_placed,creature,enemy,c3661,,,,\n",
			"itself browsed under"},
		{"a person browsed under a creature",
			header +
				"c3660,Commoner,base_placed,creature,enemy,,,,,\n" +
				"c3665,Gatekeeper Gostoc,base_placed,person,passive,c3660,,,,\n",
			"never share a card"},
		{"a creature browsed under a person",
			header +
				"c3665,Gatekeeper Gostoc,base_placed,person,passive,,,,,\n" +
				"c3660,Commoner,base_placed,creature,enemy,c3665,,,,\n",
			"never share a card"},
		// The rule is about kinds and not about people: a machine may not be
		// merged into an animal either, which the old npc-only rule allowed.
		{"a construct browsed under a creature",
			header +
				"c3660,Commoner,base_placed,creature,enemy,,,,,\n" +
				"c8100,Ballista,base_placed,construct,enemy,c3660,,,,\n",
			"never share a card"},
		{"a card name on a row that is not a card",
			header +
				"c3660,Commoner,base_placed,creature,enemy,,Commoner,,,\n" +
				"c3661,Putrid Corpse,base_placed,creature,enemy,c3660,Corpse,,,\n",
			"only the row that IS a card can name one"},
		{"a delivers column saying something else",
			header + "c3661,Putrid Corpse,base_placed,creature,enemy,,,,maybe,\n",
			"the only thing that column ever says"},
		{"a missing column",
			"model,kind\nc3661,creature\n", "missing"},
		{"a file with a header and nothing else",
			header, "no rows"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := curation.Decode(strings.NewReader(test.body)); err == nil {
				t.Fatal("accepted a curation file that should have been refused")
			} else if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refused with %q, which does not mention %q", err, test.want)
			}
		})
	}
}

// The pictures file loads, and refuses a variant listed twice, one that shows
// itself, and one that shows a picture which is itself borrowed. That the rows
// are siblings in the catalog is checked in internal/ui/gio.
func TestThePicturesFileLoadsAndRefusesAChain(t *testing.T) {
	if _, err := curation.Pictures(); err != nil {
		t.Fatal(err)
	}
	const header = "variant,shows,name,why\n"
	for name, body := range map[string]string{
		"twice":  "1,2,,\n1,3,,\n",
		"itself": "1,1,,\n",
		"chain":  "1,2,,\n2,3,,\n",
		"no row": "x,2,,\n",
	} {
		if _, err := curation.DecodePictures(strings.NewReader(header + body)); err == nil {
			t.Errorf("%s: the file loaded", name)
		}
	}
}

// The covers file loads, and refuses a level listed twice, a cover that is no
// row, and an entry claiming a picture of its own. That each cover sits in its
// card and entry is checked in internal/ui/gio.
func TestTheCoversFileLoadsAndRefusesWhatCannotBeShown(t *testing.T) {
	if _, err := curation.Covers(); err != nil {
		t.Fatal(err)
	}
	const header = "card,entry,shows,name\n"
	for name, body := range map[string]string{
		"twice":         "boss:c1,,1,\nboss:c1,,2,\n",
		"no row":        "boss:c1,,x,\n",
		"no card key":   "c1,,1,\n",
		"an entry, own": "boss:c1,Entry,own,\n",
	} {
		if _, err := curation.DecodeCovers(strings.NewReader(header + body)); err == nil {
			t.Errorf("%s: the file loaded", name)
		}
	}
}
