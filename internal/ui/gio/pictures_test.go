package gio

// The pictures are joined to the catalog by row, and research's curation can
// move a row (a merge ships another member): a picture, a shared picture or a
// cover left naming a row nobody offers would never be shown again, with
// nothing saying so. These tests hold the join, and the order a variant and a
// level borrow in.

import (
	"cmp"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/curation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// offered is every variant the whole catalog offers, by the row it ships, and
// the card (category:card, curation's key) and entry it sits in, the entry
// named as the panel names it.
func offered(t *testing.T) ([]catalog.HierarchyCategory, map[int32]catalog.HierarchyEntry, map[int32]curation.CoverOf) {
	t.Helper()
	whole, err := catalog.ReadHierarchy(reference.HierarchyCatalog())
	if err != nil {
		t.Fatal(err)
	}
	leaves := make(map[int32]catalog.HierarchyEntry, 1500)
	places := make(map[int32]curation.CoverOf, 1500)
	for _, category := range whole {
		for _, card := range category.Cards {
			for _, one := range card.Types {
				for _, leaf := range one.Entries {
					if leaf.Offer != "hold" {
						leaves[leaf.Ships] = leaf
						places[leaf.Ships] = curation.CoverOf{Card: category.Name + ":" + card.Card,
							Entry: cmp.Or(one.Name, card.Name)}
					}
				}
			}
		}
	}
	return whole, leaves, places
}

// Every picture is a variant or a card the catalog offers; every shared
// picture is another offered variant's, never in place of a variant's own
// (which would be drawn and never shown); and every cover is a member of its
// card and entry.
func TestEveryPictureShareAndCoverNamesWhatTheCatalogOffers(t *testing.T) {
	_, leaves, places := offered(t)
	cards := make(map[string]bool, 300)
	cardPictures := make(map[string]bool, 300)
	entries := make(map[curation.CoverOf]bool, 700)
	for _, place := range places {
		cards[place.Card] = true
		cardPictures[curation.CardPicture(place.Card)] = true
		entries[place] = true
	}
	icons, err := fs.ReadDir(entityIcons, "assets/entities")
	if err != nil {
		t.Fatal(err)
	}
	drawn := make(map[int32]bool, len(icons))
	for _, icon := range icons {
		name := strings.TrimSuffix(icon.Name(), ".png")
		row, err := strconv.ParseInt(name, 10, 32)
		switch {
		case err == nil:
			drawn[int32(row)] = true
			if _, held := leaves[int32(row)]; !held {
				t.Errorf("%s pictures row %d, which no variant offers", icon.Name(), row)
			}
		case strings.HasPrefix(name, "card_"):
			if !cardPictures[name] {
				t.Errorf("%s pictures no card the catalog offers", icon.Name())
			}
		default:
			t.Errorf("%s is named for no row and no card", icon.Name())
		}
	}
	shows, err := curation.Pictures()
	if err != nil {
		t.Fatal(err)
	}
	for variant, sibling := range shows {
		_, held := leaves[variant]
		_, also := leaves[sibling]
		switch {
		case !held || !also:
			t.Errorf("%d shows %d: one of them is no offered variant", variant, sibling)
		case drawn[variant]:
			t.Errorf("%d shows %d's picture, but has its own", variant, sibling)
		}
	}
	covers, err := curation.Covers()
	if err != nil {
		t.Fatal(err)
	}
	for of, cover := range covers {
		place, held := places[cover.Row]
		switch {
		case of.Entry == "" && !cards[of.Card]:
			t.Errorf("a cover for %s, which is no card", of.Card)
		case of.Entry != "" && !entries[of]:
			t.Errorf("a cover for %s %q, which is no entry", of.Card, of.Entry)
		case cover.Own:
		case !held || place.Card != of.Card || (of.Entry != "" && place.Entry != of.Entry):
			t.Errorf("%s %q shows %d, which is not one of its variants", of.Card, of.Entry, cover.Row)
		}
	}
}

// A card shows its own picture where one is drawn, then its cover's, then its
// first member's; a cover with nothing to show yet gives way.
func TestALevelShowsItsOwnPictureThenItsCoversThenItsFirst(t *testing.T) {
	known := pictures{
		cards: map[string]bool{"card_boss_c1": true},
		covers: map[curation.CoverOf]curation.Cover{
			{Card: "boss:c1"}: {Own: true}, {Card: "boss:c2"}: {Own: true},
			{Card: "boss:c3"}: {Row: 30}, {Card: "boss:c3", Entry: "Three"}: {Row: 31},
		},
	}
	byRow := map[int32]string{30: "30", 31: ""}
	for _, one := range []struct {
		card, entry, want string
	}{
		{"boss:c1", "", "card_boss_c1"}, // its own
		{"boss:c2", "", "first"},        // its own, not drawn yet
		{"boss:c3", "", "30"},           // its cover's
		{"boss:c3", "Three", "first"},   // a cover with no picture yet
		{"boss:c4", "", "first"},        // no cover
	} {
		if got := known.ofLevel(one.card, one.entry, byRow, "first"); got != one.want {
			t.Errorf("%s %q shows %q, want %q", one.card, one.entry, got, one.want)
		}
	}
}

// A variant shows its own picture; with none, its look-alike's; with neither,
// the first picture of its body, then of the body curation draws it as. A
// person borrows nothing. On a made-up roster, because on the real one a
// look-alike's picture is usually also its body's first, which would let the
// steps swap places unseen.
func TestAVariantShowsItsOwnPictureThenItsLookAlikesThenItsBodys(t *testing.T) {
	known := pictures{
		drawn:  map[int32]bool{1: true, 5: true, 8: true},
		shows:  map[int32]int32{2: 8, 3: 4},
		model:  map[string]int32{"c100": 5},
		sameAs: map[string]string{"c200": "c100"},
	}
	for _, one := range []struct {
		row   int32
		model string
		want  string
	}{
		{1, "c100", "1"}, // its own
		{2, "c100", "8"}, // its look-alike's, before its body's
		{3, "c100", "5"}, // a look-alike with no picture: its body's
		{6, "c200", "5"}, // the body curation draws it as
		{7, "c300", ""},  // nothing anywhere
		{9, "", ""},      // a person
	} {
		if got := known.of(one.row, one.model); got != one.want {
			t.Errorf("row %d of %q shows %q, want %q", one.row, one.model, got, one.want)
		}
	}
}

// A slot holding a band-1 row shows the look-alike curation gives that row -
// a game's Ash creature (Rotten Stray Ashes showed
// nothing in its container) - and without one, the picture of the world row
// it is read back to.
func TestASlotShowsItsRowsLookAlikeBeforeItsSources(t *testing.T) {
	known := pictures{
		drawn: map[int32]bool{41670040: true, 30100034: true},
		shows: map[int32]int32{141660000: 41670040},
	}
	for _, one := range []struct {
		npc  int32
		want string
	}{
		{141660000, "41670040"}, // its own look-alike
		{130100034, "30100034"}, // none: its source's picture
	} {
		if got := known.ofSlot(slotView{Filled: true, Npc: one.npc}); got != one.want {
			t.Errorf("a slot holding %d shows %q, want %q", one.npc, got, one.want)
		}
	}
}
