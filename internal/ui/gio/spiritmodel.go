package gio

// Turning the browse hierarchy into the three levels the panel draws.
//
// The panel builds from application.Hierarchy() - the four levels research
// decides - and derives nothing of its own: the category is read per entry
// from the game files, the card is the model family the author hand-shaped,
// the type and the variant are the collapse, and every entry names the row it
// ships and whether it may be offered. The panel is a reader of tags, and a
// change research makes to any level reaches it on the next sync with no
// change here.
//
// The three levels the grid draws map onto the hierarchy's lower three:
//
//   - FAMILY is the CARD under one category: the models a player would call
//     one animal. A card that lives under two categories (a Banished Knight's
//     rows under enemy, the Soldier of Godrick's boss placement under boss) is
//     two tiles, one per category, each holding only the entries that earned
//     that category.
//   - ENTRY is the TYPE: which version of it - the Banished Knight's halberd
//     against its sword and shield; Gideon's questline row against his boss.
//   - VARIANT is the ENTRY the hierarchy ships: one row, the fullest or the
//     strongest valid one, with the record's offer on it.
//
// See archive/history/editor/SPIRITS.md and docs/CATALOGS.md. Nothing here draws anything.

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// DefaultPersonShell is the person shell a dragged person rides: Nepheli Loux
// Puppet's, 100000020, the shell research's person seats use. The shell
// carries no identity of its own; the
// CharaInitParam row the variant names supplies the person.
const DefaultPersonShell int32 = 100000020

type familyView struct {
	// Key identifies the tile: the category and the card together, because a
	// card may live under two categories and each tile holds different
	// entries.
	Key string
	// Card is the hierarchy's card id - a model family for a creature
	// (c2270), a block for a person (b23240), the class card - and Art the
	// picture it shows: its own or its cover's, else its first entry's with
	// one (pictures.go).
	Card string
	Name string
	// Category is the hierarchy's word, as research wrote it: boss, enemy,
	// wildlife, npc, environment, or whatever it adds next.
	Category string
	// Marks are what the tile shows as icons (marks.go): those every variant
	// on the card has, and a hurt or a note any of them has; Notes what its
	// note mark says.
	Marks mark
	Notes []string
	// Entries are the types on this card, in the hierarchy's order; always at
	// least one, each holding at least one variant.
	Entries []entryView
	// search is the lowercased text a query is matched against, folded once
	// when the view is built rather than on every keystroke of every frame.
	search string
	Art    string
	// Ash is the Spirit Ash an Ashes card's creature comes from, empty on
	// every other card. The card and everything under it show that Ash's
	// item icon rather than the creature's portrait, so a creature the game
	// ships as a summon is not mistaken for the enemy it is drawn from.
	Ash string
	// custom marks the custom characters' card (customFamily), which is
	// offered even empty: opening it offers the character builder.
	custom bool
}

// entryView is one type on a card: one version of the creature.
type entryView struct {
	Name string
	// Art is the picture this entry shows: its cover's, else its first
	// variant's with one.
	// Empty where none of them has a picture, and the card's is shown.
	Art string
	// Marks and Notes are the card's rule, over the entry's variants.
	Marks    mark
	Notes    []string
	Variants []variantView
	// Summary is a custom character's: what it is (level and main weapon).
	Summary string
}

type variantView struct {
	// Key is the hierarchy's handle for the entry (scope:row), unique.
	Key string
	// Name is the variant label, empty when the type is the leaf; Label is
	// what the chooser and a filled slot show: the type's name, and the
	// variant after it in parentheses ("Albinauric - Axe, armored
	// (Bloodbane)"), the catalog's one label style.
	Name  string
	Label string
	Art   string
	// Npc and Think are what a drag carries for a creature: the world row the
	// entry ships and its map-stated think row. For a person Npc is the shell
	// and Person the CharaInitParam row.
	Npc, Think, Person int32
	Trunk              string
	// Model is the creature's own model, what the hazard list
	// (application.HurtsSummoner) is keyed by; Art is the picture it shows,
	// its own or one it borrows (pictures.go). c0000 for a person.
	Model string
	HP    int32
	// Offer is the record's word: "ship" or "ship-with-warning" (Warning
	// says why). A held entry - a broken one included - never reaches the
	// panel.
	Offer   string
	Warning string
	// Subcategory is the finer word where the hierarchy has one: major /
	// minor under boss, quest / invader / hostile / co-op / class under npc.
	Subcategory string
	// Tags is every column of the hierarchy for this entry, for a tooltip or
	// a future control that learns a new one.
	Tags map[string]string
	// Marks are the icons the tile shows and Notes the words a note mark
	// stands for, shown when the pointer rests on it (marks.go).
	Marks mark
	Notes []string
	// Shipped marks an Ashes card, delivered as the game ships it - on the
	// Follower, which FromSoftware's row is - on Stance, its Ash's own
	// (application.AsShipped).
	Shipped bool
	Stance  application.Stance
}

// buildFamilies turns the session's hierarchy into the panel's own.
//
// Every card under every category becomes a tile; the entries are the types;
// the variants are the hierarchy's entries. Nothing is dropped here except
// what the session already dropped (held entries and rows this save cannot
// deliver). Broken rows arrive marked and are pruned by the switch, not here.
// A variant shows its own picture and the levels above a representative
// (pictures.go).
func buildFamilies(categories []application.HierarchyCategory, art pictures) ([]familyView, error) {
	views := make([]familyView, 0, 256)
	for _, category := range categories {
		if category.Name == "" {
			return nil, fmt.Errorf("the hierarchy holds a category with no name")
		}
		for _, card := range category.Cards {
			view := familyView{
				Key: category.Name + "/" + card.Card, Card: card.Card, Name: card.Name,
				Category: category.Name,
				Entries:  make([]entryView, 0, len(card.Types)),
			}
			if category.Name == application.AshesCategory {
				view.Ash = ashOf(card.Name)
			}
			// What each variant shows, by the row it ships: the pool a cover
			// is chosen from (pictures.ofLevel).
			key := category.Name + ":" + card.Card
			cardRows := make(map[int32]string, 8)
			for _, one := range card.Types {
				entry := entryView{Name: one.Name}
				if entry.Name == "" {
					entry.Name = card.Name
				}
				entryRows := make(map[int32]string, len(one.Entries))
				for _, leaf := range one.Entries {
					variant, err := variantOf(leaf, entry.Name)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", view.Key, err)
					}
					model := leaf.Model
					if leaf.Trunk != "creature" {
						model = ""
					}
					variant.Art = art.ofLeaf(leaf, model)
					entryRows[leaf.Ships] = variant.Art
					entry.Art = cmp.Or(entry.Art, variant.Art)
					entry.Variants = append(entry.Variants, variant)
				}
				if len(entry.Variants) == 0 {
					return nil, fmt.Errorf("%s: type %q holds no variant", view.Key, one.Name)
				}
				entry.Art = art.ofLevel(key, entry.Name, entryRows, entry.Art)
				entry.Marks, entry.Notes = marksOver(entry.Variants)
				maps.Copy(cardRows, entryRows)
				view.Art = cmp.Or(view.Art, entry.Art)
				view.Entries = append(view.Entries, entry)
			}
			if len(view.Entries) == 0 {
				return nil, fmt.Errorf("%s: a card with no type", view.Key)
			}
			view.Art = art.ofLevel(key, "", cardRows, view.Art)
			view.Marks, view.Notes = marksOver(everyVariantOf(view))
			view.search = searchText(view)
			views = append(views, view)
		}
	}
	// Alphabetical within a category; the categories keep the hierarchy's
	// order, which is the author's.
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Name != views[j].Name {
			return views[i].Name < views[j].Name
		}
		return views[i].Key < views[j].Key
	})
	return views, nil
}

// ashOf is the Ash an Ashes card comes from: the card's name, up to the colon
// research puts after the Ash where it summons more than one creature
// ("Demi-Human Ashes: Demi-Human Shaman").
func ashOf(card string) string {
	ash, _, _ := strings.Cut(card, ": ")
	return ash
}

// variantOf is one hierarchy entry as the panel drags it.
func variantOf(leaf application.HierarchyEntry, typeName string) (variantView, error) {
	variant := variantView{
		Key: leaf.Key, Name: leaf.Name, Label: typeName, Trunk: leaf.Trunk, Model: leaf.Model, HP: leaf.HP,
		Offer: leaf.Offer, Warning: leaf.Warning, Subcategory: leaf.Subcategory, Tags: leaf.Tags,
	}
	variant.Marks, variant.Notes = marksAndNotes(leaf)
	if leaf.Name != "" {
		variant.Label = typeName + " (" + leaf.Name + ")"
	}
	switch leaf.Trunk {
	case "creature":
		if leaf.Ships == 0 || leaf.Think == 0 {
			return variant, fmt.Errorf("%s ships no row", leaf.Key)
		}
		variant.Npc, variant.Think = leaf.Ships, leaf.Think
	case "person":
		if leaf.Ships == 0 {
			return variant, fmt.Errorf("%s ships no row", leaf.Key)
		}
		variant.Npc, variant.Think, variant.Person = DefaultPersonShell, DefaultPersonShell, leaf.Ships
		// An Ashes person arrives on its own shell, whose think row is its
		// behaviour - a Puppet on Nepheli's shell would think as Nepheli.
		shell, think, own, err := application.PersonShell(leaf)
		if err != nil {
			return variant, err
		}
		if own {
			variant.Npc, variant.Think = shell, think
		}
	default:
		return variant, fmt.Errorf("%s: unknown trunk %q", leaf.Key, leaf.Trunk)
	}
	if application.AsShipped(leaf) {
		stance, ok := application.ShippedStance(leaf)
		if !ok {
			return variant, fmt.Errorf("%s is an Ashes card with no stance", leaf.Key)
		}
		variant.Shipped, variant.Stance = true, stance
	}
	switch leaf.Offer {
	case "ship", "ship-with-warning":
	default:
		return variant, fmt.Errorf("%s: offer %q reached the panel", leaf.Key, leaf.Offer)
	}
	return variant, nil
}

// openingKind is the filter the panel opens on: the game's own Ashes where the
// save offers them, else the first category.
func openingKind(names []string) int {
	for index, name := range names {
		if name == application.AshesCategory {
			return index
		}
	}
	return 0
}

// categoryNames is the filter's vocabulary: each category in the order the
// hierarchy handed them over - the author's six first, the Ashes leading,
// then whatever research adds. There is no "every category": a catalog shown
// whole is no filter.
func categoryNames(categories []application.HierarchyCategory) []string {
	names := make([]string, 0, len(categories))
	for _, category := range categories {
		names = append(names, category.Name)
	}
	return names
}

// labelsOf is each word as a dropdown shows it (categoryLabel).
func labelsOf(words []string) []string {
	labels := make([]string, len(words))
	for index, word := range words {
		labels[index] = categoryLabel(word)
	}
	return labels
}

// allSubcategories is the second filter's first entry: no filter.
const allSubcategories = "All subcategories"

// subcategoryOrder is the order the author named the subcategories in: the
// bosses' principal fights, the rest, the bosses built on a person; the NPCs
// by how much the character matters; the enemies' kinds, people first.
// Any other word research writes follows them alphabetically - the same rule
// the categories are ordered by (catalog.orderedCategories). An order and not
// a vocabulary: a word missing here is still offered.
var subcategoryOrder = []string{
	"major", "minor", "character",
	"quest", "invader", "other", "class",
	"soldier", "mage", "folk", "undead", "monster", "construct",
}

// subcategoryNames is the second filter's vocabulary, per category: "All
// subcategories" then every subcategory an offered variant of that category
// carries. Read off the families, broken rows included, so the list does not
// change under the player when the switch is flipped. A category with no
// subcategory offers "All subcategories" alone.
func subcategoryNames(families []familyView) map[string][]string {
	present := map[string]map[string]bool{}
	for _, family := range families {
		for _, entry := range family.Entries {
			for _, variant := range entry.Variants {
				if variant.Subcategory == "" {
					continue
				}
				if present[family.Category] == nil {
					present[family.Category] = map[string]bool{}
				}
				present[family.Category][variant.Subcategory] = true
			}
		}
	}
	names := make(map[string][]string, len(present))
	for category, words := range present {
		list := []string{allSubcategories}
		for _, word := range subcategoryOrder {
			if words[word] {
				list = append(list, word)
				delete(words, word)
			}
		}
		rest := make([]string, 0, len(words))
		for word := range words {
			rest = append(rest, word)
		}
		sort.Strings(rest)
		names[category] = append(list, rest...)
	}
	return names
}

// pruning is what the families on the panel were narrowed for: a
// subcategory of one category. Categories are not pruned - the
// category filter picks whole tiles - but a subcategory narrows INSIDE a
// tile, since one card may hold entries of two (Millicent's invader rows and
// her co-op rows are one npc card).
type pruning struct {
	category, subcategory string
}

// keeps reports whether a variant of a family survives the pruning.
func (by pruning) keeps(family familyView, variant variantView) bool {
	return by.subcategory == "" || family.Category != by.category || variant.Subcategory == by.subcategory
}

// prune returns the families as the subcategory shows them: only the chosen
// subcategory's variants on its category's cards, and no type or card that
// had nothing else.
func prune(families []familyView, by pruning) []familyView {
	if by.subcategory == "" {
		return families
	}
	kept := make([]familyView, 0, len(families))
	for _, family := range families {
		pruned := family
		pruned.Entries = make([]entryView, 0, len(family.Entries))
		for _, entry := range family.Entries {
			kept_ := make([]variantView, 0, len(entry.Variants))
			for _, variant := range entry.Variants {
				if by.keeps(family, variant) {
					kept_ = append(kept_, variant)
				}
			}
			if len(kept_) == 0 {
				continue
			}
			entry.Variants = kept_
			entry.Marks, entry.Notes = marksOver(kept_)
			pruned.Entries = append(pruned.Entries, entry)
		}
		if len(pruned.Entries) == 0 && !family.custom {
			continue
		}
		pruned.Marks, pruned.Notes = marksOver(everyVariantOf(pruned))
		pruned.search = searchText(pruned)
		kept = append(kept, pruned)
	}
	return kept
}

// searchText is everything a query is matched against, folded once when the
// view is built rather than on every keystroke of every frame: the card's
// name and id, every type's name, every variant's label, its subcategory and
// its tag. A card called Knight has to answer to "Cuckoo"; Millicent's boss
// row to "invader"; Ranni to "statue".
func searchText(family familyView) string {
	var text strings.Builder
	text.WriteString(family.Name)
	text.WriteByte(' ')
	text.WriteString(family.Card)
	text.WriteByte(' ')
	text.WriteString(family.Category)
	for _, entry := range family.Entries {
		text.WriteByte(' ')
		text.WriteString(entry.Name)
		for _, variant := range entry.Variants {
			text.WriteByte(' ')
			text.WriteString(variant.Label)
			text.WriteByte(' ')
			text.WriteString(variant.Subcategory)
			text.WriteByte(' ')
			text.WriteString(variant.Tags["tag"])
		}
	}
	return strings.ToLower(text.String())
}

// matches reports whether a family answers the current query in the chosen
// category. An empty query matches the whole category.
func (family familyView) matches(query, category string) bool {
	if family.Category != category {
		return false
	}
	return query == "" || strings.Contains(family.search, query)
}

// matched is the name of the thing inside the card that answered the query,
// when the card's own name did not.
func (family familyView) matched(query string) string {
	if query == "" || strings.Contains(strings.ToLower(family.Name), query) {
		return ""
	}
	for _, entry := range family.Entries {
		if strings.Contains(strings.ToLower(entry.Name), query) {
			return entry.Name
		}
	}
	for _, entry := range family.Entries {
		for _, variant := range entry.Variants {
			if strings.Contains(strings.ToLower(variant.Label), query) {
				return variant.Label
			}
		}
	}
	return ""
}

// spirits is how many actors this card can put in a slot, across every entry.
func (family familyView) spirits() int {
	total := 0
	for _, entry := range family.Entries {
		total += len(entry.Variants)
	}
	return total
}

// defaultVariant is what an entry stands for: its first variant, in
// research's order (card-orders.csv), which the builder opens on.
func (entry entryView) defaultVariant() variantView {
	return entry.Variants[0]
}

// firstOfEach is what dragging the card itself places: each entry once, as
// its first variant, in the card's order.
func (family familyView) firstOfEach() []variantView {
	firsts := make([]variantView, 0, len(family.Entries))
	for _, entry := range family.Entries {
		if len(entry.Variants) > 0 {
			firsts = append(firsts, entry.defaultVariant())
		}
	}
	return firsts
}

// subtitle is what a card says under the name: what is behind it. What it
// is - a statue, an extra, the DLC's, a hurt, a note - its marks say
// (marks.go).
func (family familyView) subtitle() string {
	if family.custom {
		switch len(family.Entries) {
		case 0:
			return "Add a character"
		case 1:
			return "1 character"
		}
		return fmt.Sprintf("%d characters", len(family.Entries))
	}
	if family.Category == application.CustomCategory {
		return fmt.Sprintf("%d characters", family.spirits())
	}
	switch {
	case len(family.Entries) > 1:
		return fmt.Sprintf("%d entries", len(family.Entries))
	case len(family.Entries) == 1:
		return family.Entries[0].subtitle()
	}
	return ""
}

// subtitle is the same line for an entry card, one level down.
func (entry entryView) subtitle() string {
	if entry.Summary != "" {
		return entry.Summary
	}
	switch count := len(entry.Variants); {
	case count > 1:
		return fmt.Sprintf("%d variants", count)
	case count == 1:
		return entry.Variants[0].detail()
	}
	return ""
}

// everyVariantOf is every variant on a card.
func everyVariantOf(family familyView) []variantView {
	var all []variantView
	for _, entry := range family.Entries {
		all = append(all, entry.Variants...)
	}
	return all
}

// short is a name as its own card shows it: "Omen - Greatsword" is
// "Greatsword" on the Omen card, which already says Omen. Anything else is kept whole. Only what the card draws is
// shortened; a summon dragged out keeps its whole name.
func (family familyView) short(name string) string {
	if rest, cut := strings.CutPrefix(name, family.Name+" - "); cut && rest != "" {
		return rest
	}
	return name
}

// tileName is what a variant's own tile says: its own words, which its entry
// above does not already say ("Hooded" under Omen's Axe), else the entry's
// name.
func (family familyView) tileName(entry entryView, variant variantView) string {
	if variant.Name == "" {
		return family.short(entry.Name)
	}
	return strings.ToUpper(variant.Name[:1]) + variant.Name[1:]
}

// detail is the one thing worth knowing about a variant beside its name: its
// tags (research's broken, statue and extra, `|`-joined), its subcategory
// where the hierarchy has one, its caveat, or its health.
func (variant variantView) detail() string {
	if variant.HP > 0 {
		return fmt.Sprintf("%d HP", variant.HP)
	}
	return ""
}

// customFamily is the custom characters' card as the panel draws it: one
// entry per character, each a person on its own body pictured as its starting
// class, and none before the first is built - the card then shows the
// Vagabond, the class a new character starts as.
func customFamily(card application.HierarchyCard, art pictures) (familyView, error) {
	view := familyView{
		Key: application.CustomCategory + "/" + card.Card, Card: card.Card, Name: card.Name,
		Category: application.CustomCategory, custom: true,
	}
	for _, one := range card.Types {
		entry := entryView{Name: one.Name}
		for _, leaf := range one.Entries {
			variant, err := variantOf(leaf, one.Name)
			if err != nil {
				return view, err
			}
			variant.Art = art.ofLeaf(leaf, "")
			entry.Summary, entry.Art = leaf.Tags["summary"], cmp.Or(entry.Art, variant.Art)
			entry.Variants = append(entry.Variants, variant)
		}
		view.Art = cmp.Or(view.Art, entry.Art)
		view.Entries = append(view.Entries, entry)
	}
	if view.Art == "" {
		if vagabond, err := strconv.Atoi(application.ClassArt(0)); err == nil {
			view.Art = art.of(int32(vagabond), "")
		}
	}
	view.search = searchText(view)
	return view, nil
}

// withCustom is the families with the custom characters' card put in, or
// put back: last in its category, after the save's own characters.
func withCustom(families []familyView, custom familyView) []familyView {
	out := slices.DeleteFunc(slices.Clone(families), func(family familyView) bool { return family.custom })
	at := len(out)
	for index, family := range out {
		if family.Category == custom.Category {
			at = index + 1
		}
	}
	return slices.Insert(out, at, custom)
}
