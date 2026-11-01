package gio

// Which picture each tile shows.
//
// A picture belongs to a variant, the bottom of the catalog's tree, and is
// stored under the row the variant ships: assets/entities/43130030.png is the
// Leyndell Soldier with sword, shield and trumpet. The levels above it show a
// representative: the cover curation chose (curation/covers.csv), or a card's
// own picture where one is drawn for it (assets/entities/card_boss_c3560.png),
// and without either an entry its first variant with a picture, a card its
// first entry with one.
//
// A variant with no picture of its own borrows, in this order: the sibling
// curation gives it (curation/pictures.csv); a picture of another variant of
// its model, so a body drawn once shows on all its variants until each has
// its own; and the picture of the model curation draws it as (same_as, a DLC
// re-issue of a base creature). Where none exists the tile shows its entry's
// picture, then its card's (spirits.go). Read once; a frame must never touch
// a file.

import (
	"io/fs"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/curation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// pictures answers which picture a row shows.
type pictures struct {
	// drawn is every row with a picture of its own, and cards every card
	// picture, by its name (curation.CardPicture).
	drawn map[int32]bool
	cards map[string]bool
	// shows is curation's shared pictures: a variant to the sibling whose
	// picture it shows. covers is curation's choice of each card's and
	// entry's picture.
	shows  map[int32]int32
	covers map[curation.CoverOf]curation.Cover
	// model is, for each creature model, the first of its variants with a
	// picture of its own, in the hierarchy's order; sameAs is curation's
	// borrowed model.
	model  map[string]int32
	sameAs map[string]string
}

// newPictures reads what is drawn, curation's two judgements, and which
// variant of each model holds its first picture.
func newPictures(categories []application.HierarchyCategory) (pictures, error) {
	icons, err := fs.ReadDir(entityIcons, "assets/entities")
	if err != nil {
		return pictures{}, err
	}
	known := pictures{drawn: make(map[int32]bool, len(icons)), cards: make(map[string]bool, 8),
		model: make(map[string]int32, 256), sameAs: make(map[string]string, 128)}
	for _, icon := range icons {
		name := strings.TrimSuffix(icon.Name(), ".png")
		if row, err := strconv.ParseInt(name, 10, 32); err == nil {
			known.drawn[int32(row)] = true
		} else if strings.HasPrefix(name, "card_") {
			known.cards[name] = true
		}
	}
	if known.shows, err = curation.Pictures(); err != nil {
		return pictures{}, err
	}
	if known.covers, err = curation.Covers(); err != nil {
		return pictures{}, err
	}
	entities, err := curation.Load()
	if err != nil {
		return pictures{}, err
	}
	for _, entity := range entities {
		if entity.SameAs != "" {
			known.sameAs[entity.Model] = entity.SameAs
		}
	}
	for _, category := range categories {
		for _, card := range category.Cards {
			for _, one := range card.Types {
				for _, leaf := range one.Entries {
					if _, held := known.model[leaf.Model]; !held && leaf.Trunk == "creature" && known.drawn[leaf.Ships] {
						known.model[leaf.Model] = leaf.Ships
					}
				}
			}
		}
	}
	return known, nil
}

// of is the picture a row shows: its own, its look-alike's, or for a creature
// one of its model's. model is empty for a person, whose body names nobody.
// ofLeaf is the picture a hierarchy entry shows: by the row it ships, or a
// character's by its starting class (class_art).
func (known pictures) ofLeaf(leaf application.HierarchyEntry, model string) string {
	if class, err := strconv.Atoi(leaf.Tags["class_art"]); err == nil {
		return known.of(int32(class), "")
	}
	return known.of(leaf.Ships, model)
}

func (known pictures) of(row int32, model string) string {
	if known.drawn[row] {
		return strconv.Itoa(int(row))
	}
	if like, held := known.shows[row]; held && known.drawn[like] {
		return strconv.Itoa(int(like))
	}
	if model == "" {
		return ""
	}
	if drawn, held := known.model[model]; held {
		return strconv.Itoa(int(drawn))
	}
	if drawn, held := known.model[known.sameAs[model]]; held {
		return strconv.Itoa(int(drawn))
	}
	return ""
}

// ofLevel is the picture a card or an entry shows (entry empty for the card):
// its own where one is drawn for the card, else its cover's, else first, the
// picture of its first member with one. byRow is what each member variant
// shows. A cover with no picture yet gives way to first, so a choice made
// before the drawing never leaves a level blank.
func (known pictures) ofLevel(card, entry string, byRow map[int32]string, first string) string {
	cover := known.covers[curation.CoverOf{Card: card, Entry: entry}]
	if own := curation.CardPicture(card); cover.Own && known.cards[own] {
		return own
	}
	if art := byRow[cover.Row]; art != "" {
		return art
	}
	return first
}

// ofSlot is the picture a filled slot shows: the person it summons, or the
// creature by its world row - a shipped formation names the clone, which is
// read back to the row it copies. A row with a picture or a look-alike of its
// own keeps it: the game's Ash creatures live in band 1 too, and curation
// gives them the picture of their own look.
func (known pictures) ofSlot(slot slotView) string {
	if slot.Person != 0 {
		return known.of(slot.Person, "")
	}
	row := slot.Npc
	if source, cloned := application.CloneSource(row); cloned && !known.drawn[row] && !known.drawn[known.shows[row]] {
		row = source
	}
	return known.of(row, slot.Model)
}

// slotPicture is the picture a slot shows, on the save that is open.
func (s *State) slotPicture(slot slotView) string {
	loaded := s.LoadedSave()
	if loaded == nil {
		return ""
	}
	return loaded.pictures.ofSlot(slot)
}
