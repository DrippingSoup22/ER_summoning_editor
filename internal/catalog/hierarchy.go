package catalog

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// The browse hierarchy: four levels a player navigates, decided in research and
// vendored whole through the author's reference sync tool.
//
// Research decides every level: the category is read per entry from the game
// files, the card is the model family the author hand-shaped, the type and the
// variant are the collapse. This
// package does not re-derive any of it, and - the author's rule for the seam -
// it does not KNOW any of it either: the file is read as a table of TAGS, one
// row per entry, and the four levels are built from whatever the tags say. A
// new category, a new column, a renamed type all arrive on the next sync with
// no change here. The handful of columns this package reads by name are the
// ones a frontend cannot do without; every other column is kept on the entry
// as Tags, so a frontend that learns a new tag reads it the same way.
//
//	category   what the player MET       ashes, boss, enemy, wildlife, npc,
//	                                     environment - and whatever research adds
//	card       which creature it is      the model family; a person's block of ten
//	type       which version of it
//	variant    which one exactly         one entry: the row it ships
//
// A card may live under two categories (a Banished Knight's ordinary rows and,
// under boss, the Soldier of Godrick's boss placement), which is why a card is
// keyed by category and card together.

// HierarchyEntry is one leaf: the thing a player finally picks, and the row the
// product summons for it.
type HierarchyEntry struct {
	// Key is the entry's handle, unique in the file: scope:row.
	Key string
	// Trunk says how the row is delivered: "creature" (an NpcParam world row,
	// cloned) or "person" (a CharaInitParam row on one of the game's person
	// shells).
	Trunk string
	// Name is the variant label. It can be empty, which means the type is the
	// leaf and there was nothing to distinguish underneath it.
	Name string
	// Ships is the row the entry ships: an NpcParam id for a creature, a
	// CharaInitParam id for a person. Zero when the entry ships nothing (a
	// held entry).
	Ships int32
	// Think is a creature's think row, the map-stated one; zero for a person,
	// whose think row is the shell's.
	Think int32
	// Model is the creature's cXXXX model; c0000 for a person.
	Model string
	// HP is the shipped creature row's health as the regulation ships it.
	HP int32
	// Offer is research's answer to "may the product offer this": "ship",
	// "ship-with-warning" (Warning says why), "hold" (never offered: a seat
	// saw it stand, the row is broken, or the record refuses the family).
	Offer string
	// Warning is the record's caveat, empty when there is none.
	Warning string
	// Validity is research's word: valid / unused / broken / never for a
	// creature; valid / weak / inert for a person.
	Validity string
	// Subcategory is the finer word under the category where one is defined:
	// major / minor / character under boss; quest / invader / hostile /
	// co-op / class under npc; mount under wildlife.
	Subcategory string
	// DLC means the row belongs to Shadow of the Erdtree.
	DLC bool
	// Tags is every column of the file for this entry, the named ones
	// included. A frontend reads a tag it knows by name; a tag nobody knows
	// yet is carried rather than lost.
	Tags map[string]string
}

// HierarchyType is one version of a creature: the Banished Knight's halberd
// against its sword and shield.
type HierarchyType struct {
	Name    string
	Entries []HierarchyEntry
}

// HierarchyCard is one creature: the model family, or a person's block.
type HierarchyCard struct {
	Card, Name string
	// Artwork is research's picture key for the card: the card for a
	// creature, empty for a person. The panel draws a picture per variant
	// instead (internal/ui/gio/pictures.go) and does not read it.
	Artwork string
	Types   []HierarchyType
}

// HierarchyCategory is the top level: what the player met.
type HierarchyCategory struct {
	Name  string
	Cards []HierarchyCard
}

// requiredColumns are the columns a frontend cannot do without. Everything
// else is optional and arrives as a tag.
var requiredColumns = []string{"key", "trunk", "category", "card", "card_name", "type", "variant", "ships", "offer"}

// categoryOrder is the order the categories are handed over in: the author's
// six first, then anything research adds, alphabetically. A category is never
// dropped for being unknown. "ashes" leads it: the
// creatures of the game's own Spirit Ashes, one card each, are what the panel
// opens on.
var categoryOrder = []string{"ashes", "boss", "enemy", "wildlife", "npc", "environment"}

// ReadHierarchy parses the vendored browse hierarchy.
//
// Insertion order is kept for the types and entries under a card, so the
// file's own order (research's) survives into the product; cards are sorted by
// name within a category, and categories by categoryOrder.
func ReadHierarchy(source io.Reader) ([]HierarchyCategory, error) {
	reader := csv.NewReader(source)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read hierarchy: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("hierarchy catalog is empty")
	}
	header := make([]string, len(records[0]))
	index := map[string]int{}
	for position, name := range records[0] {
		header[position] = strings.TrimSpace(name)
		index[header[position]] = position
	}
	for _, required := range requiredColumns {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("hierarchy catalog has no %s column", required)
		}
	}

	type cardKey struct{ category, card string }
	type typeKey struct {
		cardKey
		name string
	}
	cards := map[cardKey]*HierarchyCard{}
	types := map[typeKey]*HierarchyType{}
	cardOrder := map[string][]cardKey{}
	typeOrder := map[cardKey][]string{}
	categories := map[string]bool{}
	seen := map[string]bool{}

	for line, record := range records[1:] {
		if len(record) == 0 || strings.HasPrefix(strings.TrimSpace(record[0]), "#") {
			continue
		}
		tags := make(map[string]string, len(header))
		for position, name := range header {
			if position < len(record) {
				tags[name] = strings.TrimSpace(record[position])
			} else {
				tags[name] = ""
			}
		}
		key := tags["key"]
		if key == "" {
			return nil, fmt.Errorf("hierarchy line %d has no key", line+2)
		}
		if seen[key] {
			return nil, fmt.Errorf("hierarchy line %d repeats key %s", line+2, key)
		}
		seen[key] = true
		category, card := tags["category"], tags["card"]
		if category == "" {
			return nil, fmt.Errorf("hierarchy line %d (%s) has no category", line+2, key)
		}
		entry, err := entryOf(tags)
		if err != nil {
			return nil, fmt.Errorf("hierarchy line %d (%s): %w", line+2, key, err)
		}

		ck := cardKey{category, card}
		if _, known := cards[ck]; !known {
			if !categories[category] {
				categories[category] = true
			}
			cardOrder[category] = append(cardOrder[category], ck)
			artwork := tags["artwork"]
			cards[ck] = &HierarchyCard{Card: card, Name: tags["card_name"], Artwork: artwork}
		}
		tk := typeKey{ck, tags["type"]}
		if _, known := types[tk]; !known {
			typeOrder[ck] = append(typeOrder[ck], tk.name)
			types[tk] = &HierarchyType{Name: tk.name}
		}
		types[tk].Entries = append(types[tk].Entries, entry)
	}

	out := make([]HierarchyCategory, 0, len(categories))
	for _, name := range orderedCategories(categories) {
		one := HierarchyCategory{Name: name}
		for _, ck := range cardOrder[name] {
			held := cards[ck]
			for _, typeName := range typeOrder[ck] {
				held.Types = append(held.Types, *types[typeKey{ck, typeName}])
			}
			one.Cards = append(one.Cards, *held)
		}
		sort.SliceStable(one.Cards, func(i, j int) bool {
			if one.Cards[i].Name != one.Cards[j].Name {
				return one.Cards[i].Name < one.Cards[j].Name
			}
			return one.Cards[i].Card < one.Cards[j].Card
		})
		out = append(out, one)
	}
	return out, nil
}

// entryOf reads the named fields off one row's tags.
func entryOf(tags map[string]string) (HierarchyEntry, error) {
	entry := HierarchyEntry{
		Key: tags["key"], Trunk: tags["trunk"], Name: tags["variant"],
		Model: tags["model"], Offer: tags["offer"], Warning: tags["warning"],
		Validity: tags["validity"], Subcategory: tags["subcategory"],
		DLC: tags["dlc"] == "yes", Tags: tags,
	}
	if entry.Trunk != "creature" && entry.Trunk != "person" {
		return entry, fmt.Errorf("unknown trunk %q", entry.Trunk)
	}
	var err error
	if entry.Ships, err = optionalInt32(tags["ships"]); err != nil {
		return entry, fmt.Errorf("unreadable ships: %w", err)
	}
	if entry.Think, err = optionalInt32(tags["think"]); err != nil {
		return entry, fmt.Errorf("unreadable think: %w", err)
	}
	if entry.HP, err = optionalInt32(tags["hp"]); err != nil {
		return entry, fmt.Errorf("unreadable hp: %w", err)
	}
	if entry.Offer == "" {
		return entry, fmt.Errorf("no offer")
	}
	if entry.Offer != "hold" && entry.Ships == 0 {
		return entry, fmt.Errorf("offered (%s) but ships no row", entry.Offer)
	}
	return entry, nil
}

func optionalInt32(raw string) (int32, error) {
	if raw == "" {
		return 0, nil
	}
	number, err := strconv.ParseInt(raw, 10, 32)
	return int32(number), err
}

// orderedCategories puts the author's six first, in that order, and whatever
// else the file holds after them, alphabetically.
func orderedCategories(present map[string]bool) []string {
	out := make([]string, 0, len(present))
	known := map[string]bool{}
	for _, name := range categoryOrder {
		known[name] = true
		if present[name] {
			out = append(out, name)
		}
	}
	rest := make([]string, 0)
	for name := range present {
		if !known[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}
