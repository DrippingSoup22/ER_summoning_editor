package curation

// Which picture a card and an entry show.
//
// A picture belongs to a variant, the bottom of the catalog's tree; a card
// and an entry show one of their variants' pictures, their cover. Which one
// is a matter of taste - the Crucible Knight card is best shown by its tail,
// not by Devonia who happens to come first - so the author chose them, and
// this is where the choice is written. A card may also have a picture of its
// own, drawn for the card and for no variant: the Godskin card shows the two
// Godskins together.

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CoverOf names the level a cover is for: a card by its catalog key
// (category:card, "boss:c3560"), and an entry by its card's key and its name.
// Entry is empty for the card itself.
type CoverOf struct {
	Card, Entry string
}

// Cover is the picture a level shows: the row of the variant whose picture
// it is, or Own for a card with a picture drawn for itself.
type Cover struct {
	Row int32
	Own bool
}

// Covers is the author's choice of cover for each card and entry that has
// one.
func Covers() (map[CoverOf]Cover, error) {
	return DecodeCovers(bytes.NewReader(mustRead("covers.csv")))
}

// DecodeCovers parses a covers file. Exported so a test can hand it a bad one.
func DecodeCovers(source io.Reader) (map[CoverOf]Cover, error) {
	records, err := csv.NewReader(source).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse the covers file: %w", err)
	}
	if len(records) < 1 {
		return nil, fmt.Errorf("the covers file has no header")
	}
	columns := make(map[string]int, len(records[0]))
	for index, name := range records[0] {
		columns[name] = index
	}
	// name is for the person editing the file and read by nothing.
	for _, needed := range []string{"card", "entry", "shows", "name"} {
		if _, held := columns[needed]; !held {
			return nil, fmt.Errorf("the covers file is missing %q", needed)
		}
	}
	covers := make(map[CoverOf]Cover, len(records)-1)
	for _, record := range records[1:] {
		of := CoverOf{Card: record[columns["card"]], Entry: record[columns["entry"]]}
		if category, card, ok := strings.Cut(of.Card, ":"); !ok || category == "" || card == "" {
			return nil, fmt.Errorf("the covers file names %q, which is not a category:card key", of.Card)
		}
		if _, twice := covers[of]; twice {
			return nil, fmt.Errorf("the covers file lists %s %q twice", of.Card, of.Entry)
		}
		shows := record[columns["shows"]]
		if shows == "own" {
			if of.Entry != "" {
				return nil, fmt.Errorf("%s %q: only a card has a picture of its own", of.Card, of.Entry)
			}
			covers[of] = Cover{Own: true}
			continue
		}
		row, err := strconv.ParseInt(shows, 10, 32)
		if err != nil || row <= 0 {
			return nil, fmt.Errorf("the covers file names %q, which is not a row", shows)
		}
		covers[of] = Cover{Row: int32(row)}
	}
	return covers, nil
}

// CardPicture is the name of a card's own picture: card_boss_c3560 for
// boss:c3560. The icon is that name with .png, and its master that name, a
// dash and a readable name - card_boss_c3560-godskin.png.
func CardPicture(card string) string {
	return "card_" + strings.ReplaceAll(card, ":", "_")
}
