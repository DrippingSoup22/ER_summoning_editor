package gio

// Everything staged, grouped by the part of the window it was made in, and the
// Drop behind each.
//
// The footer already says how many changes are pending. That number is the one
// thing this application must never let out of sight, and it is also the one
// number a player cannot act on: four pending changes across 84 Ashes is not a
// list of what they are. This is that list.
//
// It is grouped the way the window is: what the Characters view gave each
// character, what the Editor changed - each Ash's formation and price
// together, the custom characters, the game's own summons - and what Settings
// does to the whole save. The list was flat once, and it put one character's
// name on every Ash they were given, cut each line short to fit, and set an
// Ash's formation and its price rows apart. Every change has its own Drop; a
// group of more than one has a Drop all, and so has a section.
//
// A section and a group fold under their arrow, and say how many changes they
// hold while folded, with the Drop that takes them all: a player who changed
// thirty Ashes for one character and wants only to undo a price finds it
// without scrolling past the thirty. What is folded stays folded while the
// window is open (State.pendingFolded), keyed by what a section or group is
// about, so a drop that rebuilds the list keeps it.
//
// It is built from the session rather than from the panel's own state, because
// the session is what a commit writes. Anything that appears here will be
// written and anything written appears here, which is a property worth having
// literally rather than by keeping two sets in step. Conversions and the
// monument switch are not here: this editor never stages them, only the
// command line does.

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// pendingSection is one part of the window: its name and its groups. The name is also its key
// for folding; the title shows it in capitals.
type pendingSection struct {
	Name   string
	Groups []pendingGroup
}

// pendingGroup is every change to one thing: one character, one Ash, the
// custom characters, the game's own summons. A group without a name is its one
// change, which says what it is itself. Key is what the group is about -
// "character:3", "ash:1430000", "custom", "summons" - which folding keeps it
// by.
type pendingGroup struct {
	Key, Name string
	// Ash names the Ash the group is about, for its icon.
	Ash     string
	Entries []pendingEntry
}

// pendingEntry is one staged change. Drop calls whichever Unstage matches, and
// clears whatever frame state was drawing the same change.
type pendingEntry struct {
	Label, Detail string
	// Ash names the Ash the change is about, for its icon, where the group is
	// not one Ash: a character's.
	Ash  string
	Drop func()
}

// pendingSections is everything staged, by the part of the window it was made
// in.
//
// It asks the session, so it is empty while a worker holds it - which is
// correct rather than approximate, and the same rule PendingCount follows.
func (s *State) pendingSections() []pendingSection {
	if s.Busy() || !s.session.Loaded() {
		return nil
	}
	staged := s.session.Staged()
	var sections []pendingSection
	if groups := s.characterGroups(staged.Grants); len(groups) > 0 {
		sections = append(sections, pendingSection{Name: "Characters", Groups: groups})
	}
	if groups := s.editorGroups(staged); len(groups) > 0 {
		sections = append(sections, pendingSection{Name: "Editor", Groups: groups})
	}
	if area := staged.Area; area != nil {
		sections = append(sections, pendingSection{Name: "Spirit Ashes", Groups: []pendingGroup{{Entries: s.monumentEntries(area)}}})
	}
	if reset := staged.Restoration; reset != nil {
		sections = append(sections, pendingSection{Name: "This save", Groups: []pendingGroup{{
			Entries: []pendingEntry{{
				Label:  "Reset to vanilla",
				Detail: resetWords(reset),
				Drop:   func() { s.session.UnstageReset() },
			}},
		}}})
	}
	return sections
}

// characterGroups is one group per character, in slot order, every Ash given,
// levelled or taken a change of its own: the given first, then the levelled,
// then the taken, so changes of a kind sit together.
func (s *State) characterGroups(grants []*application.StagedGrant) []pendingGroup {
	// The session hands them over by slot and then by Ash, and a stable sort
	// keeps the Ashes in that order inside each kind.
	grants = slices.Clone(grants)
	slices.SortStableFunc(grants, func(a, b *application.StagedGrant) int {
		return cmp.Or(cmp.Compare(a.Slot, b.Slot), cmp.Compare(grantKind(a), grantKind(b)))
	})
	var groups []pendingGroup
	slot := -1
	for _, grant := range grants {
		if len(groups) == 0 || grant.Slot != slot {
			groups = append(groups, pendingGroup{Key: fmt.Sprintf("character:%d", grant.Slot), Name: grant.Character})
			slot = grant.Slot
		}
		group := &groups[len(groups)-1]
		group.Entries = append(group.Entries, pendingEntry{
			Label:  grant.Name,
			Detail: grantWords(grant),
			Ash:    grant.Name,
			Drop:   func() { s.session.UnstageGrant(grant.Slot, grant.GoodsFamily) },
		})
	}
	return groups
}

// grantKind orders a character's changes: given, levelled, taken.
func grantKind(grant *application.StagedGrant) int {
	switch {
	case grant.Removal():
		return 2
	case grant.Upgrade():
		return 1
	}
	return 0
}

// grantWords is what a staged grant does to one character's Ash. One staged
// change covers three edits - handing over an Ash, moving the level of one
// they hold, taking one away - so it says which.
func grantWords(grant *application.StagedGrant) string {
	switch {
	case grant.Removal():
		return "Taken away"
	case grant.Upgrade():
		return fmt.Sprintf("From +%d to +%d", grant.Was, grant.Level)
	}
	return fmt.Sprintf("Given at +%d", grant.Level)
}

// editorGroups is one group per Ash - its formation and its price together -
// then the custom characters, then the game's own summons.
func (s *State) editorGroups(staged application.Staged) []pendingGroup {
	byAsh := map[int32]*pendingGroup{}
	ashGroup := func(family int32, name string) *pendingGroup {
		if byAsh[family] == nil {
			byAsh[family] = &pendingGroup{Key: fmt.Sprintf("ash:%d", family), Name: name, Ash: name}
		}
		return byAsh[family]
	}
	for _, formation := range staged.Formations {
		group := ashGroup(formation.HostGoodsFamily, formation.HostName)
		group.Entries = append(group.Entries, pendingEntry{
			Label:  "Formation",
			Detail: s.formationWords(formation),
			Drop:   func() { s.unstageFormation(formation.HostGoodsFamily) },
		})
	}
	for _, cost := range staged.Costs {
		group := ashGroup(cost.HostGoodsFamily, cost.HostName)
		group.Entries = append(group.Entries, pendingEntry{
			Label: "Price",
			Detail: fmt.Sprintf("Will %s, instead of %s", priceWords(cost.Price),
				strings.TrimPrefix(priceWords(cost.Was), "cost ")),
			Drop: func() {
				s.session.UnstageCost(cost.HostGoodsFamily)
				// The fields are showing the price that was just dropped, so
				// they reload from the save on the next frame.
				s.priceFor = noPrice
			},
		})
	}

	var groups []pendingGroup
	for _, family := range slices.Sorted(maps.Keys(byAsh)) {
		groups = append(groups, *byAsh[family])
	}
	if custom := s.customEntries(staged); len(custom) > 0 {
		groups = append(groups, pendingGroup{Key: "custom", Name: "Custom characters", Entries: custom})
	}
	if strength := staged.Strength; strength != nil {
		groups = append(groups, pendingGroup{Key: "summons", Name: "The game's own summons", Entries: []pendingEntry{{
			Label:  strengthWords(strength),
			Detail: "Every Ash that summons them changes too.",
			Drop:   func() { s.session.DropStrength() },
		}}})
	}
	return groups
}

// monumentEntries are the monument switches that moved, each dropped by
// moving it back.
func (s *State) monumentEntries(area *application.StagedArea) []pendingEntry {
	var entries []pendingEntry
	add := func(name string, on *bool, move func(bool) error) {
		if on == nil {
			return
		}
		position, back := "off", true
		if *on {
			position, back = "on", false
		}
		entries = append(entries, pendingEntry{
			Label: name + ": " + position,
			Drop: func() {
				if err := move(back); err != nil {
					s.setNotice(notice{text: err.Error(), bad: true})
				}
			},
		})
	}
	add("Wider summoning area", area.WideArea, s.session.SetWideArea)
	add("No once-per-rest limit", area.NoRestLimit, s.session.SetNoRestLimit)
	return entries
}

// formationWords is what a staged formation summons, each creature once with
// how many: "4 summons: Wolf ×3, Regal Ancestor Spirit (enemy)". It reads the
// slots the grid draws, which name what the session only numbers.
func (s *State) formationWords(formation *application.StagedFormation) string {
	loaded := s.LoadedSave()
	at := slices.IndexFunc(loaded.Ashes, func(ash ashView) bool { return ash.GoodsFamily == formation.HostGoodsFamily })
	if at < 0 {
		return plural(len(formation.Requested), "summon")
	}
	var names []string
	counts := map[string]int{}
	filled := 0
	for _, slot := range s.slotsFor(&loaded.Ashes[at]) {
		if !slot.Filled {
			continue
		}
		filled++
		name := slot.Name
		if name == "" {
			name = "no creature"
		}
		if slot.Enemy {
			name += " (enemy)"
		}
		if counts[name] == 0 {
			names = append(names, name)
		}
		counts[name]++
	}
	for index, name := range names {
		if counts[name] > 1 {
			names[index] = fmt.Sprintf("%s ×%d", name, counts[name])
		}
	}
	return plural(filled, "summon") + ": " + strings.Join(names, ", ")
}

// customEntries is the custom characters: each one made, changed or deleted.
func (s *State) customEntries(staged application.Staged) []pendingEntry {
	var entries []pendingEntry
	for _, build := range staged.Builds {
		state := "Changed"
		if build.New {
			state = "New"
		}
		entries = append(entries, pendingEntry{
			Label:  build.Name,
			Detail: state + " · " + build.Summary,
			Drop: func() {
				s.session.UnstageBuild(build.N)
				s.refreshCustom()
			},
		})
	}
	names := map[int]string{}
	if len(staged.DroppedBuilds) > 0 {
		for _, one := range s.session.CustomCharacters() {
			names[one.N] = one.Name
		}
	}
	for _, n := range staged.DroppedBuilds {
		entries = append(entries, pendingEntry{
			Label:  names[n],
			Detail: "Deleted from the save",
			Drop: func() {
				s.session.UnstageBuild(n)
				s.refreshCustom()
			},
		})
	}
	return entries
}

// strengthWords is how many of the game's own summon rows change role.
func strengthWords(strength *application.StagedStrength) string {
	var turned []string
	for _, one := range []struct {
		rows int
		role string
	}{{strength.Fighters, "Fighter"}, {strength.Helpers, "Helper"}} {
		switch {
		case one.rows == 1:
			turned = append(turned, "1 row made a "+one.role)
		case one.rows > 1:
			turned = append(turned, fmt.Sprintf("%d rows made %ss", one.rows, one.role))
		}
	}
	return strings.Join(turned, ", ")
}

// resetWords is what a staged reset puts back.
func resetWords(reset *application.StagedRestoration) string {
	restored := reset.GoodsRows + reset.BuddyRows + reset.StoneRows
	words := fmt.Sprintf("%d rows back to shipped values", restored)
	if reset.RowsRemoved > 0 {
		words += fmt.Sprintf(", %d added rows removed", reset.RowsRemoved)
	}
	if reset.SummonRows > 0 {
		words += fmt.Sprintf(", %d of the game's own summons back to Helpers", reset.SummonRows)
	}
	if reset.Colours {
		words += ", the spirit look back"
	}
	if reset.Characters > 0 {
		words += fmt.Sprintf(", %d custom characters deleted", reset.Characters)
	}
	return words
}

// plural is a count and its noun: "1 summon", "4 summons".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// pendingLine is one line of the dialog's list: a section's title, a group's
// name, or one change.
//
// A title or a name is a fold: key is what it folds under, folded whether it
// is, and count how many changes it holds, which it says while folded. drop is
// what its button does - nil for a line without one - and said is what the
// footer says after.
type pendingLine struct {
	section *pendingSection
	group   *pendingGroup
	entry   *pendingEntry
	key     string
	folded  bool
	count   int
	// first is the first section, which needs no space above it, and nested a
	// change inside a named group, set in under the name.
	first, nested bool
	drop          func()
	said          string
}

// pendingLines lays the sections out as the list's lines, leaving out what is
// folded.
//
// A title or a name has a button when there is something it alone can drop: a
// Drop all over more than one change, or, folded over one, that change's Drop
// - its own line being hidden. Open over one change it has none, the change's
// own Drop being the same button.
func pendingLines(sections []pendingSection, folded map[string]bool) []pendingLine {
	var lines []pendingLine
	fold := func(line pendingLine, entries []pendingEntry, name string) pendingLine {
		line.folded, line.count = folded[line.key], len(entries)
		if line.count > 1 || line.folded {
			line.drop = func() {
				for _, entry := range entries {
					entry.Drop()
				}
			}
			line.said = "No longer pending: every change to " + name + "."
		}
		return line
	}
	for at := range sections {
		section := &sections[at]
		var all []pendingEntry
		for _, group := range section.Groups {
			all = append(all, group.Entries...)
		}
		head := fold(pendingLine{section: section, key: section.Name, first: at == 0}, all, "the "+section.Name+" section")
		if head.count > 1 || head.folded {
			head.said = "No longer pending: every change in " + section.Name + "."
		}
		lines = append(lines, head)
		if head.folded {
			continue
		}
		for g := range section.Groups {
			group := &section.Groups[g]
			if group.Name != "" {
				line := fold(pendingLine{group: group, key: group.Key}, group.Entries, group.Name)
				lines = append(lines, line)
				if line.folded {
					continue
				}
			}
			for e := range group.Entries {
				entry := &group.Entries[e]
				said := "Dropped: " + entry.Label
				if group.Name != "" {
					said += " (" + group.Name + ")"
				}
				lines = append(lines, pendingLine{entry: entry, nested: group.Name != "", drop: entry.Drop, said: said + "."})
			}
		}
	}
	return lines
}

// pendingWidth caps the dialog: wide enough for an Ash's name and a formation
// of ten summons on two lines beside its Drop, and narrow enough that the
// editor behind it stays visible as the context the dialog is about.
const pendingWidth = unit.Dp(640)

// layoutPending is the dialog: a scrim over the window and the staged changes
// centred on it, the way the creature window opens.
//
// A blocking dialog rather than a panel that opens in the layout. The merchant
// editor shipped this as a corner dropdown first and replaced it because clicks
// fell through to the panels behind; opening it inside the flex instead has the
// other half of the problem, which is that asking what is staged shoves the
// editor upward and moves the thing the player was looking at.
func (s *State) layoutPending(gtx layout.Context, th *material.Theme) {
	if !s.pendingOpen {
		return
	}
	sections := s.pendingSections()
	if len(sections) == 0 {
		// Dropping the last change closes the dialog, rather than leaving an
		// empty panel open over a window with nothing staged.
		s.pendingOpen = false
		return
	}
	if s.pendingFolded == nil {
		s.pendingFolded = map[string]bool{}
	}
	lines := pendingLines(sections, s.pendingFolded)
	if len(s.dropBtns) != len(lines) {
		s.dropBtns = make([]widget.Clickable, len(lines))
		s.foldBtns = make([]widget.Clickable, len(lines))
	}
	// Esc lets an open explanation go first, then closes the list.
	if s.pendingClose.Clicked(gtx) {
		s.pendingOpen = false
	}
	if escaped(gtx) {
		if s.helpPinned != noHelp {
			s.helpPinned = noHelp
		} else {
			s.pendingOpen = false
		}
	}
	for s.pendingScrim.Clicked(gtx) {
		s.pendingOpen = false
	}
	for index, line := range lines {
		changed := false
		if line.key != "" && s.foldBtns[index].Clicked(gtx) {
			s.pendingFolded[line.key] = !line.folded
			changed = true
		}
		if line.drop != nil && s.dropBtns[index].Clicked(gtx) {
			line.drop()
			s.setNotice(notice{text: line.said})
			changed = true
		}
		if !changed {
			continue
		}
		// The lines are rebuilt and drawn in this same frame, buttons and
		// all: a frame without the dialog would show the window undimmed,
		// a flash over the whole screen at every fold and drop.
		if sections = s.pendingSections(); len(sections) == 0 {
			s.pendingOpen = false
			return
		}
		lines = pendingLines(sections, s.pendingFolded)
		s.dropBtns = make([]widget.Clickable, len(lines))
		s.foldBtns = make([]widget.Clickable, len(lines))
		break
	}
	count := s.PendingCount()

	modal(gtx, &s.pendingScrim, pendingWidth, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		// The list scrolls under the title, which stays where it is.
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutPendingHead(gtx, th, count)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.List(th, &s.pendingList).Layout(gtx, len(lines),
					func(gtx layout.Context, index int) layout.Dimensions {
						return s.layoutPendingLine(gtx, th, index, lines[index])
					})
			}),
		)
	})
}

// layoutPendingHead is the title, how many changes, its "?" and the ×.
func (s *State) layoutPendingHead(gtx layout.Context, th *material.Theme, count int) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.withHelp(gtx, th, helpPending, func(gtx layout.Context) layout.Dimensions {
				return s.layoutPendingTitle(gtx, th, count)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(horizontalDivider),
	)
}

// layoutPendingTitle is the head's one line.
func (s *State) layoutPendingTitle(gtx layout.Context, th *material.Theme, count int) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(title(th, unit.Sp(18), "Pending changes").Layout),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(muted(th, plural(count, "change")).Layout),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.helpBadge(gtx, th, helpPending) }),
				layout.Flexed(1, flexSpacer),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					close := rowButton(th, &s.pendingClose, "×")
					close.TextSize = th.TextSize * 20 / 16
					close.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}
					return close.Layout(gtx)
				}),
			)
		}),
	)
}

// The icons beside an Ash's group and beside a character's Ash: big enough to
// tell one Ash from another, small enough to stay a mark beside a line of text.
const (
	pendingGroupIcon = unit.Dp(32)
	pendingEntryIcon = unit.Dp(28)
)

// layoutPendingLine draws one line: a section's title over its rule, a group's
// name, or one change with its Drop. A change's detail wraps rather than being
// cut short, because the detail is what the change is.
func (s *State) layoutPendingLine(gtx layout.Context, th *material.Theme, index int, line pendingLine) layout.Dimensions {
	switch {
	case line.section != nil:
		top := unit.Dp(20)
		if line.first {
			top = unit.Dp(14)
		}
		return layout.Inset{Top: top, Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutFold(gtx, th, index, line, heading(th, strings.ToUpper(line.section.Name)).Layout)
			})}
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(horizontalDivider))
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})

	case line.group != nil:
		return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutFold(gtx, th, index, line, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.pendingIcon(gtx, line.group.Ash, pendingGroupIcon)
					}),
					layout.Rigid(material.Body1(th, line.group.Name).Layout),
				)
			})
		})
	}

	indent := unit.Dp(0)
	if line.nested {
		indent = foldIndent
	}
	return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: indent}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.pendingIcon(gtx, line.entry.Ash, pendingEntryIcon)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{layout.Rigid(material.Body2(th, line.entry.Label).Layout)}
				if line.entry.Detail != "" {
					children = append(children, layout.Rigid(muted(th, line.entry.Detail).Layout))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Rigid(rowButton(th, &s.dropBtns[index], "Drop").Layout),
		)
	})
}

// foldIndent is the arrow's column: a group's name and a change sit in under
// the arrow of what holds them.
const foldIndent = unit.Dp(24)

// layoutFold is a title or a name that folds: the arrow and the name are one
// button, followed by how many changes it holds, and at the far edge its Drop
// all - or its Drop, folded over one change.
func (s *State) layoutFold(gtx layout.Context, th *material.Theme, index int, line pendingLine,
	name layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.foldBtns[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				pointer.CursorPointer.Add(gtx.Ops)
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(foldIndent)
						arrow := muted(th, foldArrow(line.folded))
						arrow.TextSize = th.TextSize
						return arrow.Layout(gtx)
					}),
					layout.Rigid(name),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(muted(th, plural(line.count, "change")).Layout),
				)
			})
		}),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			switch {
			case line.drop == nil:
				return layout.Dimensions{}
			case line.count == 1:
				return rowButton(th, &s.dropBtns[index], "Drop").Layout(gtx)
			}
			return rowButton(th, &s.dropBtns[index], "Drop all").Layout(gtx)
		}),
	)
}

// foldArrow is the arrow beside a title or a name: down while it is open, the
// way the dropdowns point (arrowGlyph), and to the right while it is folded.
func foldArrow(folded bool) string {
	if folded {
		return "▸"
	}
	return "▾"
}

// pendingIcon is an Ash's icon and the gap after it, or nothing for a line
// that is not about one Ash.
func (s *State) pendingIcon(gtx layout.Context, ash string, side unit.Dp) layout.Dimensions {
	if ash == "" {
		return layout.Dimensions{}
	}
	icon, has := s.art.op(ash, gtx.Dp(side))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return imageCell(gtx, side, icon, has) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
	)
}
