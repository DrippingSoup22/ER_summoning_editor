package gio

// Which Ashes one character has, and changing that.
//
// This is the detail column of the Characters view, and it is the only place
// in the editor where an edit belongs to one person rather than to the save's
// regulation. See docs/DESIGN.md (The character slots): ownership covers inventory and the storage
// box both, so an Ash in the chest is one this will not offer to hand out
// again, and the button on its row says Remove instead.
//
// Nothing here writes. Every control stages, the pending list shows what is
// staged, and the save happens once, in save.go.

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// The Ash list's shape.
//
// It began as one full-width row per Ash, which put a 28dp icon and a short
// name against the left edge of a panel a thousand pixels wide and the Add
// button against the right edge, with nothing in between. Wrong twice over: the
// icon was too small to recognise the Ash by, which is the only thing an icon
// is for, and the row was mostly a gap.
//
// The icon's size is now a setting and the column count follows from it, the
// way the merchant editor's grids work: a cell is as wide as its contents need,
// the panel holds as many as fit, and a player who wants bigger pictures gets
// fewer of them across rather than a layout that argues with them. Two columns
// at the default 90dp on a wide window; four at 40dp.
//
// The default is under the 128px the masters are stored at, so nothing is
// upscaled there. Past about 128dp - or past 100dp at a 125% display scale -
// an icon is drawn larger than its source and softens. That is the player's
// choice to make and the slider lets them make it; re-vendoring larger masters
// would undo the first-paint work in icons.go for a size most people will not
// pick.
const (
	// The action sits in a slot of its own width at the right edge of the
	// cell, so every Add, Remove and Undo lines up down the column. 96dp less
	// the button's 12dp padding each side is 72dp of text, against about 50dp
	// for "Remove" at the row's text size.
	grantActionWidth = unit.Dp(96)
	// grantNameWidth is the least room a name is given: the longest Ash in the
	// catalogue over two lines. The name takes whatever the cell has beyond
	// it, so the slack sits between a name and its controls, inside the row's
	// rule, and the controls keep to the cell's right edge.
	grantNameWidth = unit.Dp(250)
)

// grantLevelWidth is the level slider every owned row carries, with the "+N"
// beside it.
const grantLevelWidth = levelSliderWidth

// ashLevels is +0 to +10, the whole of what a Spirit Ash has: the sliders run
// to ashLevels - 1. It matches internal/character's AshLevels, which is the
// regulation's own count; this view holds its own copy rather than reaching
// past the application boundary for it.
const ashLevels = 11

// grantCellWidth is the least one Ash needs: its icon, its name, its level and
// the button, with the gaps between them.
func grantCellWidth(gtx layout.Context, icon unit.Dp) int {
	return gtx.Dp(icon) + gtx.Dp(unit.Dp(12)) + gtx.Dp(grantNameWidth) +
		gtx.Dp(unit.Dp(8)) + gtx.Dp(grantLevelWidth) +
		gtx.Dp(unit.Dp(8)) + gtx.Dp(grantActionWidth)
}

// grantColumns is how many of those fit across the panel, and never fewer than
// one: a window too narrow for a single cell still has to draw them.
func grantColumns(gtx layout.Context, icon unit.Dp) int {
	cell, gap := grantCellWidth(gtx, icon), gtx.Dp(grantColumnGap)
	if columns := (gtx.Constraints.Max.X + gap) / (cell + gap); columns > 1 {
		return columns
	}
	return 1
}

// grantColumnGap is the space between columns. Wide enough that one column's
// action does not crowd the next column's icon.
const grantColumnGap = unit.Dp(24)

// layoutGrants is the right column: every Ash, marked by whether this character
// has it, with the ones they do not offered.
func (s *State) layoutGrants(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	chosen := s.person >= 0 && s.person < len(loaded.Characters) && loaded.Characters[s.person].Used
	person := characterView{}
	addable, staged := 0, 0
	if chosen {
		person = loaded.Characters[s.person]
		s.loadLevels(gtx, person, loaded)
		pending := s.stagedFor(person.Slot)
		for index, ash := range loaded.Ashes {
			if s.grantBtns[index].Clicked(gtx) {
				// The button says what the click does to the character's
				// collection (rowAction). On a staged addition, Remove takes the
				// addition off; on a staged removal, Add takes the removal off -
				// whether the change was made here or by the toolbar. On an Ash
				// they hold, Remove replaces any level change staged on it.
				staged, owned := pending[ash.GoodsFamily], person.Owns[ash.GoodsFamily]
				switch {
				case staged != nil && (staged.Removal() || !owned):
					s.undoGrant(person, ash)
				case owned:
					s.takeAway(person, ash)
				default:
					s.grant(person, ash)
				}
			}
			switch {
			case person.Owns[ash.GoodsFamily]:
			case pending[ash.GoodsFamily] != nil:
				staged++
			default:
				addable++
			}
		}
		if s.grantAllBtn.Clicked(gtx) {
			if addable > 0 {
				s.grantEveryMissing(person, loaded)
			} else {
				s.dropGrants(person)
			}
		}

	}

	name, detail, count := "Spirit Ashes", "", ""
	if chosen {
		name, detail = person.Name, person.summary()
		count = fmt.Sprintf("%d of %d Spirit Ashes", len(person.Owns), len(loaded.Ashes))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return columnHeader(gtx, th, name, detail, count)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !chosen {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutGrantActions(gtx, th, person, loaded, addable, staged)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !chosen || person.KnowsStorage {
				return layout.Dimensions{}
			}
			// The one caveat kept on screen, because it is conditional and a
			// player has no other way to find it out. That an added item cannot
			// be taken back used to sit here too, on every frame; it is said by
			// the notice every Add raises, which is where a warning about an
			// action belongs.
			note := muted(th, "Storage box unreadable: an Ash shown missing may be in the chest.")
			note.Color = colorWarn
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, note.Layout)
		}),
		// The rule over the list closes the toolbar, so it goes with it.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !chosen {
				return layout.Dimensions{}
			}
			return horizontalDivider(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			switch {
			case !chosen && firstCharacter(loaded.Characters) == noPerson:
				return emptyState(gtx, th, "This save holds no characters.")
			case !chosen:
				return emptyState(gtx, th, "Choose a character on the left.")
			case addable == 0 && staged == 0:
				return emptyState(gtx, th, person.Name+" already has every Spirit Ash.")
			}
			return s.layoutGrantGrid(gtx, th, person, loaded)
		}),
	)
}

// layoutGrantActions is the toolbar over the list: the level every Ash should
// be at, and the button that adds the missing ones.
//
// The two sit together because they are the same kind of thing - one gesture,
// eighty-four rows - and a player looking for "make them all +10" looks where
// "add them all" is. They are the size of the rows' own controls, because they
// are the same controls applied to every row.
func (s *State) layoutGrantActions(gtx layout.Context, th *material.Theme,
	person characterView, loaded *savedView, addable, staged int) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(muted(th, "Level for every Ash").Layout),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims, chose := s.grantAllSlider.Layout(gtx, th, s.grantAll, ashLevels-1)
			if chose >= 0 {
				s.grantAll = chose
				s.setEveryLevel(person, loaded)
			}
			return dims
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutGrantAll(gtx, th, addable, staged)
		}),
		layout.Flexed(1, flexSpacer),
	)
}

// layoutGrantAll is the one control that acts on the whole list, and it turns
// around into the control that undoes itself.
//
// The turn-around is not decoration. Adding every missing Ash stages dozens of
// changes at once, and taking them back one at a time through the pending
// dialog would be a worse job than the one the button just saved.
func (s *State) layoutGrantAll(gtx layout.Context, th *material.Theme, addable, staged int) layout.Dimensions {
	label := ""
	switch {
	case addable > 0:
		label = fmt.Sprintf("Add the %d missing", addable)
	case staged > 0:
		label = fmt.Sprintf("Remove the %d added", staged)
	default:
		return layout.Dimensions{}
	}
	return rowButton(th, &s.grantAllBtn, label).Layout(gtx)
}

// rowButton is every button in the list and its toolbar: the level chooser's
// text size and padding, so a button and the chooser beside it are one height
// and their words sit on one line.
func rowButton(th *material.Theme, click *widget.Clickable, label string) material.ButtonStyle {
	style := quietButton(th, click, label)
	style.TextSize = th.TextSize * 14 / 16
	style.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}
	return style
}

func (s *State) layoutGrantGrid(gtx layout.Context, th *material.Theme,
	person characterView, loaded *savedView) layout.Dimensions {
	icon := s.IconSize()
	columns := grantColumns(gtx, icon)
	rows := (len(loaded.Ashes) + columns - 1) / columns
	pending := s.stagedFor(person.Slot)
	return material.List(th, &s.grantList).Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		children := make([]layout.FlexChild, 0, columns*2)
		for column := 0; column < columns; column++ {
			if column > 0 {
				children = append(children, layout.Rigid(layout.Spacer{Width: grantColumnGap}.Layout))
			}
			at := row*columns + column
			if at >= len(loaded.Ashes) {
				// An empty cell still claims its column, so the last row's
				// entries are the width of every other row's.
				children = append(children, layout.Flexed(1, flexSpacer))
				continue
			}
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return s.grantRow(gtx, th, at, person, loaded, pending, icon)
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	})
}

// grantRow is one Ash: its picture, its name, its level when the character has
// it, and the one control for the row, with a rule under it.
//
// An Ash the character has is drawn in full and one they lack is veiled: the
// list is a picture of their collection, so what they own is what stands out.
// It was the other way round, which drew a character's own Ashes greyed like
// unavailable things. A row with a change staged on it is lit in the glow.
func (s *State) grantRow(gtx layout.Context, th *material.Theme, index int,
	person characterView, loaded *savedView, pending map[int32]*application.StagedGrant,
	icon unit.Dp) layout.Dimensions {
	ash := loaded.Ashes[index]
	owned := person.Owns[ash.GoodsFamily]
	staged := pending[ash.GoodsFamily]

	// The level chooser belongs to an Ash the character HAS. On one they do
	// not, the level is the toolbar's - it is what Add will use - and a second
	// chooser saying so on every unowned row would be eighty-odd controls
	// asking a question one control already answers.
	row := func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					picture, has := s.art.op(ash.Name, gtx.Dp(icon))
					return iconCell{Img: picture, Empty: !has, Size: icon, Dim: !owned && staged == nil}.Layout(gtx, th, nil)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				// The name takes what the cell has left, so the chooser and the
				// button sit against the cell's right edge, one line down the
				// column; the cell is never narrower than grantCellWidth.
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					name := material.Body1(th, ash.Name)
					name.MaxLines = 2
					switch {
					case staged != nil:
						name.Color = colorGlow
					case owned:
						name.Color = colorFg
					default:
						name.Color = colorMuted
					}
					return name.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// The slot is claimed either way, so the Add buttons of the
					// rows above and below an owned one stay in one line.
					gtx.Constraints.Min.X = gtx.Dp(grantLevelWidth)
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					if !levelled(owned, staged) {
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}
					shown := s.levelShown(person, ash.GoodsFamily, pending)
					dims, chose := s.grantLevelSliders[index].Layout(gtx, th, shown, ashLevels-1)
					if chose >= 0 {
						s.setLevel(person, ash, chose)
					}
					return dims
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Both bounds, not just the floor. With only Min pinned the
					// slot still had the whole rest of the cell as its ceiling,
					// and a button told to fill its width filled all of that.
					gtx.Constraints.Max.X = gtx.Dp(grantActionWidth)
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					// One slot, one verb, and always a control: an Ash to add,
					// an Ash to take away, or a change to undo. The player is
					// answering the same question about the row in all three
					// cases, so it is answered in one place.
					return rowButton(th, &s.grantBtns[index], rowAction(owned, staged)).Layout(gtx)
				}),
			)
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(row),
		layout.Rigid(horizontalDivider),
	)
}

// grant stages one Ash for one character, at the level the header is asking
// for. An Ash nobody owns has no level of its own, so the one control that
// speaks for the whole list speaks for the new one too.
func (s *State) grant(person characterView, ash ashView) {
	if s.Busy() || s.grantStaged(person.Slot, ash.GoodsFamily) {
		return
	}
	level := s.globalLevel()
	if _, err := s.session.StageGrant(person.Slot, ash.GoodsFamily, level); err != nil {
		s.setNotice(notice{text: err.Error(), bad: true})
		return
	}
	s.setNotice(notice{text: fmt.Sprintf(
		"%s gets %s at +%d.", person.Name, ash.Name, level)})
}

// undoGrant drops the change staged against one row, whether it was staged
// from that row or by the button over the whole column.
func (s *State) undoGrant(person characterView, ash ashView) {
	if s.Busy() {
		return
	}
	if !s.session.UnstageGrant(person.Slot, ash.GoodsFamily) {
		return
	}
	if _, owns := person.Levels[ash.GoodsFamily]; owns {
		s.setNotice(notice{text: fmt.Sprintf("No change to %s for %s.", ash.Name, person.Name)})
		return
	}
	s.setNotice(notice{text: fmt.Sprintf("%s won't get %s.", person.Name, ash.Name)})
}

// grantStaged reports whether this Ash is already staged for this character.
func (s *State) grantStaged(slot int, family int32) bool {
	return s.stagedFor(slot)[family] != nil
}

// loadLevels matches the per-row fields to the list, and starts them again when
// the player moves to another character.
//
// The reset is the whole reason this is a function. A level typed against one
// character is an answer about that character, and carrying it onto the next
// one would be the editor quietly proposing an edit nobody asked for.
func (s *State) loadLevels(gtx layout.Context, person characterView, loaded *savedView) {
	if len(s.grantBtns) != len(loaded.Ashes) {
		s.grantBtns = make([]widget.Clickable, len(loaded.Ashes))
	}
	if len(s.grantLevelSliders) != len(loaded.Ashes) {
		s.grantLevelSliders = make([]levelSlider, len(loaded.Ashes))
	}
	if s.levelFor == person.Slot {
		return
	}
	s.levelFor = person.Slot
	// Back to +0 for the next character, and that is the whole reset. A chooser
	// is told what to show every frame, so there is nothing else of the last
	// character's to clear - where the fields it replaced had to be re-seeded
	// row by row, and the change each seeding raised swallowed with it.
	s.grantAll = 0
}

// levelShown is the level one row should be displaying, most specific answer
// first.
//
// A staged change is the most specific thing there is: it is what this editor
// will write, so it outranks what the save currently says. Then what the
// character holds. Then the header's level, which is what an Ash nobody owns is
// worth saying about - it is the level Add will give it.
func (s *State) levelShown(person characterView, family int32,
	pending map[int32]*application.StagedGrant) int {
	if staged := pending[family]; staged != nil {
		return staged.Level
	}
	if level, owned := person.Levels[family]; owned {
		return level
	}
	return s.globalLevel()
}

// globalLevel is what the header's chooser is asking for, which is also the
// level a new Add uses. An Ash nobody owns has no level of its own to show.
func (s *State) globalLevel() int { return s.grantAll }

// setLevel is what one row's chooser now says.
//
// Two outcomes. The level the character already holds the Ash at drops whatever
// was staged, because the row is asking for what is already true; any other
// level stages it - as an upgrade on an Ash they hold, and as the level to add
// it at on one they do not, which is the case a row staged by Add all is in.
func (s *State) setLevel(person characterView, ash ashView, level int) {
	if s.Busy() {
		return
	}
	if held, owns := person.Levels[ash.GoodsFamily]; owns && held == level {
		if s.session.UnstageGrant(person.Slot, ash.GoodsFamily) {
			s.setNotice(notice{text: fmt.Sprintf("%s stays at +%d.", ash.Name, level)})
		}
		return
	}
	if _, err := s.session.StageGrant(person.Slot, ash.GoodsFamily, level); err != nil {
		s.setNotice(notice{text: err.Error(), bad: true})
		return
	}
	s.setNotice(notice{text: fmt.Sprintf("%s's %s at +%d.", person.Name, ash.Name, level)})
}

// setEveryLevel is what the header's field now says, applied to the whole list.
//
// It broadcasts: every Ash the character holds takes it, including ones set
// individually beforehand, and every Ash they do not is added at it from here
// on. That is the gesture - "all of them at +10" - and a global control that
// skipped the rows somebody had touched would be a control whose effect
// depended on history nobody can see. Setting one row afterwards is how one row
// differs again.
func (s *State) setEveryLevel(person characterView, loaded *savedView) {
	if s.Busy() {
		return
	}
	// No validation, and none to write: the chooser holds one of the eleven
	// levels the game has and cannot hold anything else.
	level := s.grantAll
	pending := s.stagedFor(person.Slot)
	upgraded, dropped, refused := 0, 0, 0
	var first error
	for _, ash := range loaded.Ashes {
		held, owns := person.Levels[ash.GoodsFamily]
		switch {
		case owns && held == level:
			if s.session.UnstageGrant(person.Slot, ash.GoodsFamily) {
				dropped++
			}
		case owns || pending[ash.GoodsFamily] != nil:
			if _, err := s.session.StageGrant(person.Slot, ash.GoodsFamily, level); err != nil {
				refused++
				if first == nil {
					first = err
				}
				continue
			}
			upgraded++
		}
	}
	switch {
	case upgraded == 0 && dropped == 0 && first != nil:
		s.setNotice(notice{text: first.Error(), bad: true})
	case upgraded == 0:
		s.setNotice(notice{text: fmt.Sprintf(
			"%s's Ashes are already +%d.", person.Name, level)})
	case refused > 0:
		s.setNotice(notice{text: fmt.Sprintf(
			"%d of %s's Ashes at +%d; %d refused.", upgraded, person.Name, level, refused), bad: true})
	default:
		s.setNotice(notice{text: fmt.Sprintf(
			"%d of %s's Ashes at +%d.", upgraded, person.Name, level)})
	}
}

// stagedFor is every Ash staged for one character, as a set.
//
// Built once per frame and passed down, rather than asked per Ash: the list
// draws 84 rows and a scan of the staged set for each of them is 84 scans of a
// slice that the Add-all button can leave 84 entries long.
func (s *State) stagedFor(slot int) map[int32]*application.StagedGrant {
	pending := make(map[int32]*application.StagedGrant)
	if s.Busy() || !s.session.Loaded() {
		return pending
	}
	for _, staged := range s.session.Staged().Grants {
		if staged.Slot == slot {
			pending[staged.GoodsFamily] = staged
		}
	}
	return pending
}

// grantEveryMissing stages every Ash this character does not hold.
//
// A refusal does not stop the rest. The session is the authority on what can be
// granted and it answers per Ash; one that cannot be added is a reason to say
// so at the end, not a reason to abandon the eighty that could.
func (s *State) grantEveryMissing(person characterView, loaded *savedView) {
	if s.Busy() {
		return
	}
	pending := s.stagedFor(person.Slot)
	added, refused := 0, 0
	var first error
	for _, ash := range loaded.Ashes {
		if person.Owns[ash.GoodsFamily] || pending[ash.GoodsFamily] != nil {
			continue
		}
		if _, err := s.session.StageGrant(person.Slot, ash.GoodsFamily, s.globalLevel()); err != nil {
			refused++
			if first == nil {
				first = err
			}
			continue
		}
		added++
	}
	switch {
	case added == 0 && first != nil:
		s.setNotice(notice{text: first.Error(), bad: true})
	case refused > 0:
		s.setNotice(notice{text: fmt.Sprintf(
			"%s gets %d Ashes; %d refused: %v", person.Name, added, refused, first), bad: true})
	default:
		s.setNotice(notice{text: fmt.Sprintf(
			"%s gets %d Ashes.", person.Name, added)})
	}
}

// dropGrants unstages every Ash staged for one character.
//
// The families are collected before any of them is dropped: UnstageGrant
// changes the set this is reading.
func (s *State) dropGrants(person characterView) {
	if s.Busy() {
		return
	}
	var families []int32
	for _, staged := range s.session.Staged().Grants {
		if staged.Slot == person.Slot {
			families = append(families, staged.GoodsFamily)
		}
	}
	dropped := 0
	for _, family := range families {
		if s.session.UnstageGrant(person.Slot, family) {
			dropped++
		}
	}
	if dropped > 0 {
		s.setNotice(notice{text: fmt.Sprintf(
			"%s won't get the %d Ashes.", person.Name, dropped)})
	}
}

// takeAway stages the removal of one Ash from one character.
//
// The notice says what Reset cannot undo, exactly as the grant's does and for
// the same reason: an item leaving somebody's inventory is the one edit this
// product makes that Reset to vanilla has no answer for, and a player should
// hear that before they save rather than after.
func (s *State) takeAway(person characterView, ash ashView) {
	if s.Busy() {
		return
	}
	if _, err := s.session.StageRemoval(person.Slot, ash.GoodsFamily); err != nil {
		s.setNotice(notice{text: err.Error(), bad: true})
		return
	}
	s.setNotice(notice{text: fmt.Sprintf(
		"%s loses %s.", person.Name, ash.Name), warn: true})
}

// rowAction is the word on one row's button, and it is only ever Add or
// Remove (the user, 2026-10-26: an Add that turns into Undo makes no
// sense). The word follows what the character WILL hold once the save is
// written: an Ash they will have can be removed, one they will not can be
// added. So pressing Add turns the button into Remove, and pressing it again
// takes the addition back. That a change is staged is said by the row's name,
// lit in the glow.
func rowAction(owned bool, staged *application.StagedGrant) string {
	willHold := owned
	if staged != nil {
		willHold = !staged.Removal()
	}
	if willHold {
		return "Remove"
	}
	return "Add"
}

// levelled reports whether a row shows an upgrade chooser.
//
// An Ash the character holds has a level, and so does one staged to be added -
// that second case is what Add all leaves eighty-odd rows in, and they were
// showing an empty slot where the level they were being added at should be. An
// Ash staged to be taken away has none: a level is a fact about something the
// character is going to have.
func levelled(owned bool, staged *application.StagedGrant) bool {
	if staged != nil {
		return !staged.Removal()
	}
	return owned
}
