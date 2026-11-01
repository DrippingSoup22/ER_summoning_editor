package gio

// Two faces, one of them chosen.
//
// Lora is the default and carries the whole window; Cinzel is the
// inscriptional alternative, offered in Settings and applied everywhere at
// once. Both are Google Fonts under the SIL Open Font License - the licences
// are vendored beside the files - and both were already chosen and lived with
// in the sibling merchant editor, where Cinzel was picked after establishing
// that Elden Ring's own UI serif is proprietary and not redistributable.
//
// Cinzel started out here as the face for titles and panel headings, which was
// wrong: its lowercase glyphs are cap-height by design, so a mixed-case title
// set in it reads as a mistake rather than as a flourish. It is a whole-window
// choice or nothing.
//
// The collection is additive to Gio's system-font lookup, so a glyph neither
// face covers still resolves. One trap, found the hard way in the sibling and
// worth carrying: text.Shaper treats the FIRST face in the collection as the
// fallback for an EMPTY typeface query, so a theme whose Face is "" silently
// renders in whichever face happens to be first. th.Face is always set from
// typefaceFor, which never returns the empty string, and nothing else pins a
// typeface of its own.

import (
	_ "embed"
	"fmt"

	"gioui.org/font"
	"gioui.org/font/opentype"
)

//go:embed assets/fonts/Lora.ttf
var loraTTF []byte

//go:embed assets/fonts/Cinzel.ttf
var cinzelTTF []byte

const (
	bodyFace    font.Typeface = "Lora"
	displayFace font.Typeface = "Cinzel"
)

// fontCollection parses the embedded faces for text.NewShaper. It panics on a
// bad parse because the files are embedded in the binary: a failure here means
// the build is broken, not that the player did something.
func fontCollection() []font.FontFace {
	var faces []font.FontFace
	for _, ttf := range [][]byte{loraTTF, cinzelTTF} {
		face, err := opentype.Parse(ttf)
		if err != nil {
			panic(fmt.Errorf("parse embedded font: %w", err))
		}
		faces = append(faces, font.FontFace{Font: face.Font(), Face: face})
	}
	return faces
}

// fontKeys and fontNames are what Settings offers, in the order the dropdown
// lists them: the stored value, and the label shown for it. Two parallel
// slices rather than a map, because a list a player reads has an order and a
// map does not.
var (
	fontKeys  = []string{"lora", "cinzel"}
	fontNames = []string{"Lora", "Cinzel"}
)

// fontChoice is the index of a stored value, for seeding the dropdown.
// Anything unrecognised lands on Lora, which is the fallback typefaceFor makes
// for the same value.
func fontChoice(setting string) int {
	for index, key := range fontKeys {
		if key == setting {
			return index
		}
	}
	return 0
}

// typefaceFor resolves a Settings.Font value to the face th.Face is set to.
// Anything unrecognised - an older config, a hand-edited one - resolves to
// Lora rather than to the empty string, which would land on whichever face the
// shaper happens to hold first.
func typefaceFor(setting string) font.Typeface {
	if setting == "cinzel" {
		return displayFace
	}
	return bodyFace
}
