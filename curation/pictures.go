package curation

// Which variant shows another variant's picture.
//
// A picture belongs to a variant, the bottom of the catalog's tree, and is
// stored under the row the variant ships. Many variants show a sibling's
// instead: one that differs only in what it does - a Kindred of Rot that only
// throws, a Scarab that explodes - since a picture cannot show a move, and
// any other the author chose to give a card's picture to (the Picture
// Choices page, 2026-10-28: every Godrick Soldier shows the one drawing).
// The sibling is on the same card, or is the same body on another (a boss
// Black Knight showing the ordinary one's picture), or a look-alike on
// another card the author named (the Big turtle shows the Turtle, Nataan the
// Sanguine Noble). That is a judgement about pictures, not a fact about the
// game, which is why it is written here.

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// Pictures is that judgement: the row a variant ships, to the row of the
// sibling whose picture it shows.
func Pictures() (map[int32]int32, error) {
	return DecodePictures(bytes.NewReader(mustRead("pictures.csv")))
}

// DecodePictures parses a pictures file. Exported so a test can hand it a bad
// one.
func DecodePictures(source io.Reader) (map[int32]int32, error) {
	records, err := csv.NewReader(source).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse the pictures file: %w", err)
	}
	if len(records) < 1 {
		return nil, fmt.Errorf("the pictures file has no header")
	}
	columns := make(map[string]int, len(records[0]))
	for index, name := range records[0] {
		columns[name] = index
	}
	// name and why are for the person editing the file and read by nothing.
	for _, needed := range []string{"variant", "shows", "name", "why"} {
		if _, held := columns[needed]; !held {
			return nil, fmt.Errorf("the pictures file is missing %q", needed)
		}
	}
	row := func(text string) (int32, error) {
		value, err := strconv.ParseInt(text, 10, 32)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("the pictures file names %q, which is not a row", text)
		}
		return int32(value), nil
	}
	shows := make(map[int32]int32, len(records)-1)
	for _, record := range records[1:] {
		variant, err := row(record[columns["variant"]])
		if err != nil {
			return nil, err
		}
		sibling, err := row(record[columns["shows"]])
		if err != nil {
			return nil, err
		}
		if _, twice := shows[variant]; twice {
			return nil, fmt.Errorf("the pictures file lists %d twice", variant)
		}
		if variant == sibling {
			return nil, fmt.Errorf("%d shows its own picture", variant)
		}
		shows[variant] = sibling
	}
	// One step only: a variant shows a picture of its own, never one that is
	// itself borrowed.
	for variant, sibling := range shows {
		if _, borrows := shows[sibling]; borrows {
			return nil, fmt.Errorf("%d shows %d, which shows another's picture", variant, sibling)
		}
	}
	return shows, nil
}
