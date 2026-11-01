package gio

// The spirit catalog panel: one grid of cards, opened in place, and the drag
// that starts anywhere in it.
//
// 249 cards over 413 entries over 686 movesets, narrowed by the loaded save to
// what it can actually deliver. That narrowing happens one layer down, so a
// card that reached here is one the save holds and nothing in the panel needs
// to be shown as unavailable.
//
// The grid holds the catalog, and it stays: clicking a card that has
// something inside it opens it IN PLACE, a frame under its row holding what
// it holds, and an entry inside opens the same way one frame deeper
// (over a Back button and a grid that replaced
// itself at each depth):
//
//	families  ->  entries  ->  variants
//	  cards       models       movesets
//
// One card is open at a time, and one entry inside it; a second click on
// either closes it. A depth is skipped where it holds one thing:
//
//   - 169 of the 249 cards hold one entry, so opening one shows that
//     entry's variants.
//   - 69 of those hold one variant as well, so clicking one opens nothing
//     and just marks the card.
//
// Every depth is a drag source, so nothing has to be opened before something
// can be taken: a card offers its root entry's first moveset, an entry offers
// its own first, and a variant offers itself.
//
// The cards themselves are built in spiritmodel.go, out of what curated.go
// reads.

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"slices"
	"strings"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// spiritMIME is the drag payload's type. The payload is an identity - the
// actor's two row numbers - and never a pointer or an index: a drag outlives
// the cell it started in, and the list underneath it can be filtered away
// while the pointer is still down.
const spiritMIME = "application/x-er-spirit"

// depth is how deep the open card is open: atFamilies for none (or a card
// that holds one moveset, only marked), atEntries for a card's entries,
// atVariants for movesets - an open entry's, or the one entry's of a card.
type depth int

const (
	atFamilies depth = iota
	atEntries
	atVariants
)

// visibleFamilies is the filtered index list, recomputed only when the query
// changes. Filtering 249 cards is cheap; doing it in every frame of a drag is
// the kind of cheap that adds up.
func (s *State) visibleFamilies(loaded *savedView) []int {
	query := strings.ToLower(strings.TrimSpace(s.search.Text()))
	if s.kind >= len(loaded.Categories) {
		s.kind = 0
	}
	category := ""
	if len(loaded.Categories) > 0 {
		category = loaded.Categories[s.kind]
	}
	subs := loaded.subcategoriesOf(category)
	if s.sub >= len(subs) {
		s.sub = 0
	}
	// The subcategory re-prunes the families in place: rows appear or go,
	// and the grid returns to the top because the indices below
	// moved. Without a subcategory the category plays no part in the pruning,
	// so changing the category alone does not re-prune.
	want := pruning{}
	if s.sub > 0 {
		want.category, want.subcategory = category, subs[s.sub]
	}
	if loaded.prunedFor != want {
		loaded.prunedFor = want
		loaded.Families = prune(loaded.all, want)
		loaded.Spirits = 0
		for _, family := range loaded.Families {
			loaded.Spirits += family.spirits()
		}
		s.filtered = nil
	}
	if s.query == query && s.shownKind == s.kind && s.shownSub == s.sub && s.filtered != nil {
		return s.filtered
	}
	// Whether the FILTER moved, which is not the same as whether this had to
	// recompute: the first frame after a load recomputes a cold cache under
	// the query it already had, and that is not a player narrowing anything.
	moved := s.query != query || s.shownKind != s.kind || s.shownSub != s.sub
	s.query, s.shownKind, s.shownSub = query, s.kind, s.sub
	s.filtered = make([]int, 0, len(loaded.Families))
	for index, family := range loaded.Families {
		if family.matches(query, category) {
			s.filtered = append(s.filtered, index)
		}
	}
	// Narrowing the catalog returns the grid to the top and closes the open
	// card: a query is a statement about the category, and the card may not
	// be in what it leaves.
	if moved {
		s.closeFamily()
	}
	return s.filtered
}

// closeFamily returns the grid to the catalog and forgets both depths below.
func (s *State) closeFamily() {
	s.family, s.entry = noFamily, noEntry
	s.entryBtns, s.entryDrags, s.variantBtns = nil, nil, nil
	s.cardOpened, s.builderWait = noCard, noCard
}

// depth is how deep the open card is open, derived rather than stored.
//
// Storing it would let it disagree with what is open - a depth of atVariants
// with no entry chosen has nothing to draw - so the two numbers that a click
// actually sets are the whole state, and this reads them.
//
// The rule at every step is the same: go one deeper only where there is more
// than one thing to choose between. That is what keeps a click off a screen
// holding a single tile.
func (s *State) depth(loaded *savedView) depth {
	if s.family < 0 || s.family >= len(loaded.Families) {
		return atFamilies
	}
	// The custom characters' card always opens on its characters, beside
	// the tile that adds one: a character opens the builder, not a depth.
	if loaded.Families[s.family].custom {
		return atEntries
	}
	entries := loaded.Families[s.family].Entries
	if len(entries) > 1 {
		if s.entry < 0 || s.entry >= len(entries) {
			return atEntries
		}
		if len(entries[s.entry].Variants) > 1 {
			return atVariants
		}
		// A chosen entry with one moveset opens nothing. The grid stays on
		// the entries and marks it, which is the same answer a card with
		// nothing inside it gets one level up.
		return atEntries
	}
	if len(entries[0].Variants) > 1 {
		return atVariants
	}
	return atFamilies
}

// layoutSpirits is the right panel: the filters, and the grid.
func (s *State) layoutSpirits(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	visible := s.visibleFamilies(loaded)
	if len(s.familyBtns) != len(loaded.Families) {
		s.familyBtns = make([]widget.Clickable, len(loaded.Families))
		s.familyDrags = make([]widget.Draggable, len(loaded.Families))
	}
	s.stepDepth(gtx, loaded)
	// Cleared after stepDepth has read them and before the cells refill them,
	// so what a frame polls is always what the frame before it drew.
	s.drawnFamilies, s.drawnEntries = s.drawnFamilies[:0], s.drawnEntries[:0]

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// The count belongs to the title, not to the search field. It says
			// how much is on screen, which is a fact about the panel; sitting
			// at the end of the field it read as a property of the query, and
			// it pushed the field into being the whole width of the panel to
			// make room for itself.
			// The "?" is the legend of the tiles' marks (marks.go).
			return s.withHelp(gtx, th, helpMarks, func(gtx layout.Context) layout.Dimensions {
				return panelHeaderWith(gtx, th, "SPIRITS", fmt.Sprintf("%d of %d", len(visible), len(loaded.Families)),
					func(gtx layout.Context) layout.Dimensions { return s.helpBadge(gtx, th, helpMarks) })
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutFilters(gtx, th, loaded)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(visible) == 0 {
				return emptyState(gtx, th, s.nothingHere(loaded))
			}
			return s.layoutFamilyGrid(gtx, th, loaded, visible)
		}),
	)
}

// nothingHere is what an empty catalog says. A search reaches only the
// category chosen, so where other categories hold a match it names them and
// how many each holds, rather than leave the player to try every one.
func (s *State) nothingHere(loaded *savedView) string {
	if len(loaded.Categories) == 0 {
		return "Nothing matches that."
	}
	said := "Nothing in " + loaded.CategoryLabels[s.kind] + " matches that."
	if s.query == "" {
		return said
	}
	var elsewhere []string
	for index, category := range loaded.Categories {
		if index == s.kind {
			continue
		}
		count := 0
		for _, family := range loaded.Families {
			if family.matches(s.query, category) {
				count++
			}
		}
		if count > 0 {
			elsewhere = append(elsewhere, fmt.Sprintf("%s (%d)", loaded.CategoryLabels[index], count))
		}
	}
	if len(elsewhere) > 0 {
		said += "\nFound in " + strings.Join(elsewhere, ", ") + "."
	}
	return said
}

// stepDepth consumes every click that can open or close a card, BEFORE the
// grid reads what is open.
//
// This is the whole of why a click has to be handled here rather than in the
// cell that owns the button. Gio draws a frame when there is INPUT, not when
// state changed during one. A click handled inside a cell is handled after the
// frame has already chosen what to draw, so the frame draws the old state and
// the new one waits for whatever makes the window redraw next - a mouse move,
// usually: hold the mouse still after clicking and the grid does not change at
// all. TestACardOpensInPlaceAndASecondClickClosesIt clicks as a hand does.
//
// It asks only the cells the last frame drew. A press can only have come from
// one of those, the grid is scrolled and most of the catalog is off screen, and
// asking all 249 cards instead costs 244us a frame against a 378us frame -
// nearly all of it spent on cards nobody could have clicked.
//
// A click opens a card at once. A touchpad's
// tap-and-drag arrives as a click and then a press, so for the system's
// double-click time the card keeps its place under the pointer, a second click
// on it leaves it open, and a drag from it closes it again (settleCards). A
// custom character is the exception: its click opens the builder, which takes
// the panel, so that one waits the double-click time first.
func (s *State) stepDepth(gtx layout.Context, loaded *savedView) {
	if s.addCharacterBtn.Clicked(gtx) {
		s.openBuilder(noBuild)
	}
	for _, index := range s.drawnFamilies {
		if index < len(s.familyBtns) && s.familyBtns[index].Clicked(gtx) {
			s.clickCard(gtx, cardRef{atFamilies, index})
		}
	}
	// After the families, so a double click that opened a card has already
	// dropped the buttons the old card's cells were using - and the bounds
	// check is what makes that safe rather than a read past the end of a
	// slice that was just replaced.
	for _, index := range s.drawnEntries {
		if index < len(s.entryBtns) && s.entryBtns[index].Clicked(gtx) {
			s.clickCard(gtx, cardRef{atEntries, index})
		}
	}
	s.settleCards(gtx)
}

// cardRef names a card at one depth of the catalog: a family, or an entry of
// the open family.
type cardRef struct {
	at    depth
	index int
}

var noCard = cardRef{index: -1}

// clickCard is a click on a card: it opens at once, keeping its place, unless
// it is the second click of a double click on the card just opened.
func (s *State) clickCard(gtx layout.Context, card cardRef) {
	if card == s.cardOpened && gtx.Now.Sub(s.cardOpenedAt) < s.doubleTap {
		return
	}
	if card.at == atEntries && s.customOpen() {
		s.builderWait, s.builderWaitAt = card, gtx.Now
		return
	}
	s.keepInPlace(card)
	s.openCard(card)
	s.cardOpened, s.cardOpenedAt = card, gtx.Now
}

// settleCards ends a click's grace. A drag from the card it opened, inside
// the double-click time, was a touchpad's tap-and-drag, so the card closes
// again under the drag; past the time the grace ends and the reveal may
// scroll the card into view. A custom character's builder opens once the time
// has passed with no drag from it. Until then it asks for a frame at that
// time, since nothing else may come to draw one.
func (s *State) settleCards(gtx layout.Context) {
	if card := s.builderWait; card != noCard {
		switch due := s.builderWaitAt.Add(s.doubleTap); {
		case s.draggingCard(gtx, card):
			s.builderWait = noCard
		case gtx.Now.Before(due):
			gtx.Execute(op.InvalidateCmd{At: due})
		default:
			s.builderWait = noCard
			s.openCard(card)
		}
	}
	card := s.cardOpened
	if card == noCard {
		return
	}
	due := s.cardOpenedAt.Add(s.doubleTap)
	switch {
	case !gtx.Now.Before(due):
		s.cardOpened = noCard
	case s.draggingCard(gtx, card):
		s.cardOpened, s.reveal = noCard, false
		if card.at == atFamilies && s.family == card.index {
			s.closeFamily()
		}
		if card.at == atEntries && s.entry == card.index {
			s.entry, s.variantBtns = noEntry, nil
		}
	default:
		gtx.Execute(op.InvalidateCmd{At: due})
	}
}

// draggingCard is whether a drag from the card has really started.
func (s *State) draggingCard(gtx layout.Context, card cardRef) bool {
	switch {
	case card.index < 0:
	case card.at == atFamilies && card.index < len(s.familyDrags):
		return dragged(gtx, &s.familyDrags[card.index])
	case card.at == atEntries && card.index < len(s.entryDrags):
		return dragged(gtx, &s.entryDrags[card.index])
	}
	return false
}

// customOpen is whether the open card is the custom characters'.
func (s *State) customOpen() bool {
	loaded := s.LoadedSave()
	return loaded != nil && s.family >= 0 && s.family < len(loaded.Families) && loaded.Families[s.family].custom
}

// keepInPlace keeps the card a click opens under the pointer: an open card
// or entry above it closes as it opens, and the list scrolls up by the frame
// that goes, so a touchpad's second tap lands on the card it tapped.
func (s *State) keepInPlace(card cardRef) {
	columns := max(s.openColumns, 1)
	position := &s.familyList.Position
	switch card.at {
	case atFamilies:
		old, now := slices.Index(s.filtered, s.family), slices.Index(s.filtered, card.index)
		if s.family == card.index || old < 0 || now < 0 || old/columns >= now/columns {
			return
		}
		if frame := old/columns + 1; frame < position.First {
			position.First--
		} else {
			position.Offset -= s.familyFrame
		}
	case atEntries:
		if s.entry != noEntry && s.entry != card.index && s.entry/columns < card.index/columns {
			position.Offset -= s.entryFrame
		}
	}
}

// openCard opens a card in place: a family's entries, or an entry's variants
// - as deep as there is something to choose, see depth. A card already open
// closes, and opening another closes the one that was.
func (s *State) openCard(card cardRef) {
	switch card.at {
	case atFamilies:
		if s.family == card.index {
			s.closeFamily()
			return
		}
		s.closeFamily()
		s.family, s.reveal = card.index, true
	case atEntries:
		if loaded := s.LoadedSave(); loaded != nil && s.family >= 0 && s.family < len(loaded.Families) &&
			loaded.Families[s.family].custom {
			if entries := loaded.Families[s.family].Entries; card.index < len(entries) {
				s.openBuilder(entries[card.index].defaultVariant().build())
			}
			return
		}
		s.variantBtns = nil
		if s.entry == card.index {
			s.entry = noEntry
			return
		}
		s.entry = card.index
	}
}

// layoutSearch is the query field, the catalog's whole width.
func (s *State) layoutSearch(gtx layout.Context, th *material.Theme) layout.Dimensions {
	height := gtx.Dp(fieldHeight)
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return box(gtx, colorInputBg, colorDivider, unit.Dp(4), func(gtx layout.Context) layout.Dimensions {
		editor := material.Editor(th, &s.search, "Search spirits")
		editor.Color = colorFg
		editor.HintColor = colorMuted
		return editor.Layout(gtx)
	})
}

// chooseKind puts the category filter on index, and the subcategory back on
// "All subcategories": the index it held named a word of the old category.
func (s *State) chooseKind(index int) {
	s.kind, s.sub = index, 0
}

// subcategoriesOf is the subcategory dropdown's list for a category, "All
// subcategories" alone where the category has none.
func (loaded *savedView) subcategoriesOf(category string) []string {
	if names := loaded.Subcategories[category]; len(names) > 0 {
		return names
	}
	return []string{allSubcategories}
}

// layoutFilters is what is over the catalog: what to search in, and what to
// search for, laid out by filterBar - a dropdown as wide as its longest option,
// the search beside them where it fits.
//
// The category is a dropdown rather than a row of buttons. Six buttons are six
// things to read every time the panel is looked at, in order to choose one of
// them; a dropdown is one thing to read, and it says which is in force without
// the other five having to be on screen to say it. It is the sibling merchant
// editor's category filter, which is the same control doing the same job - see
// combo.go.
//
// The second dropdown is the category's subcategory - major / minor /
// character under the bosses, the npc's quest / invader / hostile, wildlife's
// mounts - the same control doing the same job one level finer, and only
// where the category has one to choose. Unlike a category it narrows INSIDE a
// card, so a card holding two subcategories shows only the chosen one's
// entries.
//
// There is no "All categories": the panel opens on the game's own Ashes and a
// category is always chosen.
//
// The two depths below them are deliberately not two more dropdowns. A filter
// narrows the world; a family is a place to go into, and those are different
// gestures.
func (s *State) layoutFilters(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	category := ""
	if s.kind < len(loaded.Categories) {
		category = loaded.Categories[s.kind]
	}
	// The categories are the hierarchy's own words, in its order: nothing
	// here knows them, so a category research adds is a filter on the next
	// sync. Only their look is the panel's (categoryLabel).
	kindWidth := s.kindCombo.fit(gtx, th, loaded.CategoryLabels)
	dropdowns := []dropdown{{width: kindWidth, draw: func(gtx layout.Context) layout.Dimensions {
		dims, chose := s.kindCombo.Layout(gtx, th, kindWidth, loaded.CategoryLabels, s.kind)
		if chose >= 0 {
			s.chooseKind(chose)
			s.kindChosen = true
		}
		return dims
	}}}
	// A category with no subcategory has no second dropdown, since one with
	// nothing to choose is not one.
	if subs := loaded.SubcategoryLabels[category]; len(subs) > 1 {
		subWidth := s.subCombo.fit(gtx, th, subs)
		dropdowns = append(dropdowns, dropdown{width: subWidth, draw: func(gtx layout.Context) layout.Dimensions {
			dims, chose := s.subCombo.Layout(gtx, th, subWidth, subs, s.sub)
			if chose >= 0 {
				s.sub = chose
			}
			return dims
		}})
	}
	return filterBar(gtx, dropdowns, func(gtx layout.Context) layout.Dimensions {
		return s.layoutSearch(gtx, th)
	})
}

// columns is how many cards sit side by side in a width: as many as the
// catalog holds (catalogCard), so dragging the divider wider adds a column
// rather than stretching the names. All three depths are laid out with it.
func (s *State) columns(gtx layout.Context, width int) int {
	gap := gtx.Dp(cardGap)
	return max((width+gap)/(gtx.Dp(s.catalogCard())+gap), 1)
}

// cardRow is one row of a grid of count cards.
func cardRow(gtx layout.Context, columns, row, count int, cell func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	children := make([]layout.FlexChild, 0, columns*2)
	for column := 0; column < columns; column++ {
		if column > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: cardGap}.Layout))
		}
		at := row*columns + column
		if at >= count {
			// An empty half-row still claims its column, so the last row's
			// one card is the width of every other card rather than double.
			children = append(children, layout.Flexed(1, flexSpacer))
			continue
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return cell(gtx, at)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

// grid is a scrolling grid of count cards: the builder's items.
func (s *State) grid(gtx layout.Context, th *material.Theme, list *widget.List, count int,
	cell func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	columns := s.columns(gtx, gtx.Constraints.Max.X-gtx.Dp(listScrollbar))
	rows := (count + columns - 1) / columns
	return material.List(th, list).Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		return cardRow(gtx, columns, row, count, cell)
	})
}

// cardRows is a grid of count cards inside an open card, every row drawn,
// with opened under the row of cell open where open is one. It keeps the
// catalog's columns (openColumns), so a frame's inset does not cost one.
func (s *State) cardRows(gtx layout.Context, count, open int, opened layout.Widget,
	cell func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	columns := s.openColumns
	if columns < 1 {
		columns = s.columns(gtx, gtx.Constraints.Max.X)
	}
	rows := (count + columns - 1) / columns
	children := make([]layout.FlexChild, 0, rows+1)
	for row := 0; row < rows; row++ {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return cardRow(gtx, columns, row, count, cell)
		}))
		if open >= 0 && row == open/columns {
			children = append(children, layout.Rigid(opened))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutFamilyGrid is the catalog: the cards in rows, and under the open
// card's row what it holds (layoutOpened). Nothing is replaced, so the rest of
// the catalog stays where it was.
func (s *State) layoutFamilyGrid(gtx layout.Context, th *material.Theme, loaded *savedView, visible []int) layout.Dimensions {
	columns := s.columns(gtx, gtx.Constraints.Max.X-gtx.Dp(listScrollbar))
	s.openColumns = columns
	rows := (len(visible) + columns - 1) / columns
	open := -1
	if s.depth(loaded) != atFamilies {
		if at := slices.Index(visible, s.family); at >= 0 {
			open = at / columns
		}
	}
	elements := rows
	if open >= 0 {
		elements++
		// An opened card whose frame would start below the panel's foot
		// scrolls up to the top, its frame under it.
		// Not while a click's grace keeps the card under the pointer.
		if position := &s.familyList.Position; s.reveal && s.cardOpened == noCard && open+1 >= position.First+position.Count-1 {
			position.First, position.Offset = open, 0
		}
	}
	if s.cardOpened == noCard {
		s.reveal = false
	}
	return material.List(th, &s.familyList).Layout(gtx, elements, func(gtx layout.Context, element int) layout.Dimensions {
		row := element
		if open >= 0 && element > open {
			if element == open+1 {
				dims := s.layoutOpened(gtx, th, loaded.Families[s.family])
				s.familyFrame = dims.Size.Y
				return dims
			}
			row--
		}
		return cardRow(gtx, columns, row, len(visible), func(gtx layout.Context, at int) layout.Dimensions {
			index := visible[at]
			return s.familyCard(gtx, th, index, loaded.Families[index])
		})
	})
}

// layoutOpened is what the open card holds, framed under its row: its entries
// - the custom card's with the tile that adds one - or, for a card holding one
// entry, that entry's movesets.
func (s *State) layoutOpened(gtx layout.Context, th *material.Theme, family familyView) layout.Dimensions {
	return openedFrame(gtx, colorInputBg, func(gtx layout.Context) layout.Dimensions {
		if family.custom || len(family.Entries) > 1 {
			return s.layoutEntryGrid(gtx, th, family)
		}
		return s.layoutVariantGrid(gtx, th, family, family.Entries[0])
	})
}

// openedFrame is the frame around what an open card or entry holds: a well a
// shade apart from the catalog, under the row it opened from.
func openedFrame(gtx layout.Context, fill color.NRGBA, w layout.Widget) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(6)).Layout(gtx, w)
		call := macro.Stop()
		radius := gtx.Dp(unit.Dp(4))
		paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rectangle{Max: dims.Size}, radius).Op(gtx.Ops))
		call.Add(gtx.Ops)
		widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return dims })
		return dims
	})
}

// layoutEntryGrid is the open card's models, with the open entry's movesets
// framed under its row.
func (s *State) layoutEntryGrid(gtx layout.Context, th *material.Theme, family familyView) layout.Dimensions {
	if len(s.entryBtns) != len(family.Entries) {
		s.entryBtns = make([]widget.Clickable, len(family.Entries))
		s.entryDrags = make([]widget.Draggable, len(family.Entries))
	}
	if family.custom {
		// The tile that adds a character first, where it is found on the
		// first visit, when it is all the card holds.
		return s.cardRows(gtx, len(family.Entries)+1, -1, nil, func(gtx layout.Context, at int) layout.Dimensions {
			if at == 0 {
				return s.addCharacterCard(gtx, th)
			}
			return s.entryCard(gtx, th, at-1, family, family.Entries[at-1])
		})
	}
	open := -1
	if s.entry >= 0 && s.entry < len(family.Entries) && len(family.Entries[s.entry].Variants) > 1 {
		open = s.entry
	}
	s.entryFrame = 0
	opened := func(gtx layout.Context) layout.Dimensions {
		dims := openedFrame(gtx, colorSlotFill, func(gtx layout.Context) layout.Dimensions {
			return s.layoutVariantGrid(gtx, th, family, family.Entries[open])
		})
		s.entryFrame = dims.Size.Y
		return dims
	}
	return s.cardRows(gtx, len(family.Entries), open, opened, func(gtx layout.Context, at int) layout.Dimensions {
		return s.entryCard(gtx, th, at, family, family.Entries[at])
	})
}

// layoutVariantGrid is one entry's movesets, in a grid of their own.
func (s *State) layoutVariantGrid(gtx layout.Context, th *material.Theme,
	family familyView, entry entryView) layout.Dimensions {
	if len(s.variantBtns) != len(entry.Variants) {
		s.variantBtns = make([]widget.Draggable, len(entry.Variants))
	}
	// What a tile falls back to, resolved once for the whole depth: the
	// entry's picture where there is one and the card's otherwise.
	//
	// The tile's own picture is the variant's (pictures.go), and it is asked
	// first. Where it has none, the entry's picture is the honest answer,
	// because the variant really is that creature holding something else.
	fallback := entry.Art
	if _, has := s.entityArt.op(fallback, gtx.Dp(s.SpiritIconSize())); !has {
		fallback = family.Art
	}
	return s.cardRows(gtx, len(entry.Variants), -1, nil, func(gtx layout.Context, at int) layout.Dimensions {
		variant := entry.Variants[at]
		variant.Label = family.short(variant.Label)
		return s.variantCard(gtx, th, at, family.Ash, fallback, family.tileName(entry, variant), variant)
	})
}

// cardFace is the inside of a cell at any depth: a portrait, a name and one
// line under it.
//
// ash is the Ash an Ashes card comes from (familyView.Ash), whose icon comes
// first. art is the cell's own portrait key and fallback is the one above it,
// so a picture is inherited downward: an entry with nothing drawn for it shows
// its card's picture, which is honest, because the entry really is that
// creature.
func (s *State) cardFace(gtx layout.Context, th *material.Theme,
	ash, art, fallback, name, subtitle string, marks tileMarks, lit bool) layout.Dimensions {
	side := s.SpiritIconSize()
	picture, has := s.art.op(ash, gtx.Dp(side))
	if !has {
		picture, has = s.entityArt.op(art, gtx.Dp(side))
	}
	if !has {
		picture, has = s.entityArt.op(fallback, gtx.Dp(side))
	}
	return s.cardFaceOf(gtx, th, func(gtx layout.Context) layout.Dimensions {
		return imageCell(gtx, side, picture, has)
	}, name, subtitle, marks, lit)
}

// cardFaceOf is cardFace with the picture drawn by the caller: an item's
// icon, or the sign that adds a character.
func (s *State) cardFaceOf(gtx layout.Context, th *material.Theme, picture layout.Widget,
	name, subtitle string, marks tileMarks, lit bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(picture),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			style := material.Body2(th, name)
			style.MaxLines = 1
			if lit {
				style.Color = colorGlow
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(style.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(muted(th, subtitle).Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutMarks(gtx, th, marks, subtitle != "")
						}),
					)
				}),
			)
		}),
	)
}

// familyCard is one card in the catalog, and a drag source.
//
// Clicking it opens it as deep as there is something to see and no deeper,
// and the subtitle says what that is: "6 entries" opens six creatures under
// its row, "4 variants" four movesets, and a card saying neither only marks
// itself.
//
// Dragging it places each of its entries once, as its first moveset, in the
// card's order (familyView.firstOfEach): opening it is how one is placed.
//
// The press is not read here. It is read in stepDepth, before the panel decides
// what is open; all this does is show whether this card is the one that is.
func (s *State) familyCard(gtx layout.Context, th *material.Theme, index int, family familyView) layout.Dimensions {
	click := &s.familyBtns[index]
	s.drawnFamilies = append(s.drawnFamilies, index)
	chosen := s.family == index

	drag := &s.familyDrags[index]
	drag.Type = spiritMIME
	if mime, ok := drag.Update(gtx); ok && len(family.Entries) > 0 {
		drag.Offer(gtx, mime, io.NopCloser(strings.NewReader(payloadOf(family.firstOfEach()))))
	}

	// What the card says under its name, plus why it is on screen at all when
	// the query matched something inside it rather than its own name.
	subtitle := family.subtitle()
	if inside := family.matched(s.query); inside != "" {
		subtitle = "matches " + inside
	}

	render := func(gtx layout.Context) layout.Dimensions {
		return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return selectableRow(gtx, chosen || dragged(gtx, drag), func(gtx layout.Context) layout.Dimensions {
				marks := tileMarks{family.Marks, family.Notes, noteTag{"card", family.Key}}
				return s.cardFace(gtx, th, family.Ash, family.Art, "", family.Name, subtitle, marks, chosen)
			})
		})
	}

	var ghost layout.Widget
	if dragged(gtx, drag) {
		ghost = func(gtx layout.Context) layout.Dimensions {
			return dragGhost(gtx, th, family.Name)
		}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	defer pass.Pop()
	return drag.Layout(gtx, render, ghost)
}

// entryCard is one model inside an open card, and a drag source.
//
// Clicking it opens its movesets under its row where it has more than one,
// and marks it otherwise. Dragging it places each of its movesets, as a card
// places each of its entries; an entry of one places that one.
func (s *State) entryCard(gtx layout.Context, th *material.Theme, index int,
	family familyView, entry entryView) layout.Dimensions {
	click := &s.entryBtns[index]
	s.drawnEntries = append(s.drawnEntries, index)
	chosen := s.entry == index || s.builderWait == cardRef{atEntries, index}

	drag := &s.entryDrags[index]
	drag.Type = spiritMIME
	if mime, ok := drag.Update(gtx); ok && len(entry.Variants) > 0 {
		drag.Offer(gtx, mime, io.NopCloser(strings.NewReader(payloadOf(entry.Variants))))
	}

	render := func(gtx layout.Context) layout.Dimensions {
		return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return selectableRow(gtx, chosen || dragged(gtx, drag), func(gtx layout.Context) layout.Dimensions {
				marks := tileMarks{entry.Marks, entry.Notes, noteTag{"entry", family.Key + "/" + entry.Name}}
				return s.cardFace(gtx, th, family.Ash, entry.Art, family.Art, family.short(entry.Name), entry.subtitle(), marks, chosen)
			})
		})
	}

	var ghost layout.Widget
	if dragged(gtx, drag) {
		ghost = func(gtx layout.Context) layout.Dimensions {
			return dragGhost(gtx, th, entry.Name)
		}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	defer pass.Pop()
	return drag.Layout(gtx, render, ghost)
}

// variantCard is one moveset at the bottom depth, and a drag source.
//
// It is the only depth with nothing under it, so it does not click - it is
// dragged. The drag is offered on every frame the widget asks for one and the
// payload is built then rather than held, so a drag always carries the actor
// the tile showed when it started.
//
// ash is the card's Ash, if it is an Ashes card, and fallback the picture to
// use when this moveset has none of its own, which today is all of them.
// name is what the tile says (familyView.tileName); the drag carries the
// variant's whole label.
func (s *State) variantCard(gtx layout.Context, th *material.Theme, index int,
	ash, fallback, name string, variant variantView) layout.Dimensions {
	drag := &s.variantBtns[index]
	drag.Type = spiritMIME
	if mime, ok := drag.Update(gtx); ok {
		drag.Offer(gtx, mime, io.NopCloser(strings.NewReader(payload(variant))))
	}

	render := func(gtx layout.Context) layout.Dimensions {
		return selectableRow(gtx, dragged(gtx, drag), func(gtx layout.Context) layout.Dimensions {
			marks := tileMarks{variant.Marks, variant.Notes, noteTag{"variant", variant.Key}}
			return s.cardFace(gtx, th, ash, variant.Art, fallback, name, variant.detail(), marks, false)
		})
	}

	// A ghost only once the drag has really started (dragged), so a press
	// that never moves shows nothing and behaves as a press.
	var ghost layout.Widget
	if dragged(gtx, drag) {
		ghost = func(gtx layout.Context) layout.Dimensions {
			return dragGhost(gtx, th, variant.Label)
		}
	}
	// PassOp: the drag area is registered on top of the tile and input areas
	// are opaque by default. Without pass-through, anything underneath - here
	// the list's own scrolling - stops seeing the pointer.
	pass := pointer.PassOp{}.Push(gtx.Ops)
	defer pass.Pop()
	return drag.Layout(gtx, render, ghost)
}

// dragSlop is how far a press on a card must move before it is a drag. Gio's
// Draggable says it is dragging from the press itself, before the pointer has
// moved, so without this every click would read as a drag for the frames the
// button is held - and a drag from a card just opened closes it again
// (settleCards): a click whose press and release came in two frames, which is
// every click a hand makes, would have closed its own card.
const dragSlop = unit.Dp(6)

// dragged is whether a card's drag has really started.
func dragged(gtx layout.Context, drag *widget.Draggable) bool {
	pos, slop := drag.Pos(), float32(gtx.Dp(dragSlop))
	return drag.Dragging() && pos.X*pos.X+pos.Y*pos.Y > slop*slop
}

// payload is what a drag carries: the actor's rows - the world row and its
// think row for a creature, the shell, the shell's think row and the
// CharaInitParam row for a person - and never an index into a list that can be
// filtered away while the pointer is down.
func payload(variant variantView) string {
	return fmt.Sprintf("%d,%d,%d", variant.Npc, variant.Think, variant.Person)
}

// payloadOf is several actors' payloads in order, ";"-joined: what a card or
// an entry drags.
func payloadOf(variants []variantView) string {
	each := make([]string, len(variants))
	for index, variant := range variants {
		each[index] = payload(variant)
	}
	return strings.Join(each, ";")
}

// dragGhost is what follows the pointer during a drag.
func dragGhost(gtx layout.Context, th *material.Theme, label string) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		style := material.Body2(th, label)
		style.Color = colorContrastFg
		style.MaxLines = 1
		return style.Layout(gtx)
	})
	call := macro.Stop()

	corner := gtx.Dp(unit.Dp(4))
	shape := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: corner, SW: corner, NE: corner, NW: corner}
	paint.FillShape(gtx.Ops, colorGlow, shape.Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}
