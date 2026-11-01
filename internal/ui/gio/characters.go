package gio

// The Characters view: which save is open, and who is in it.
//
// This is the landing view and it owns opening a file, which is the sibling
// merchant editor's shape and is copied from it deliberately - the bar across
// the top, the columns under it, the fixed-width chooser column and the flexed
// detail column are all its measurements. See docs/GUI.md.
//
// It is also the one view about the ten character slots rather than about the
// regulation, and the distinction is worth saying out loud on screen: every
// other edit in this editor applies to all ten characters at once, because a
// formation is a property of the game's data and not of a save file. Only an
// added item belongs to one person.
//
// The right-hand column - which Ashes the chosen person has, and adding or
// removing one - is in grants.go.

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// The open bar's measurements, ported from the merchant editor's own.
//
// The path field is capped at a fraction of the bar rather than given the flex,
// because uncapped it swallows every pixel the buttons do not take and leaves
// the message with nowhere to go. It is also forced to that width rather than
// merely allowed it: an Editor holding a short path shrinks to its content
// otherwise, so the bar's shape would change as the player typed.
const (
	pathFieldNum, pathFieldDen = 1, 4
	fieldHeight                = unit.Dp(34)
	// The open file's name sits at the end of the bar, in a slot of its own
	// width so it does not move as the name changes.
	filenameSlotWidth = unit.Dp(220)
	// A Rigid child of a vertical flex has an unbounded height, and
	// verticalDivider trusts what it is given, so a divider inside a one-line
	// bar has to be told how tall the line is.
	dividerBarHeight = unit.Dp(28)
)

// charColWidth is the chooser column: wide enough for the slot number, a
// character's name and the line under it ("Level 150 · Confessor"), and no
// wider, because the Ashes beside it are what a wide window should be spending
// its width on.
const charColWidth = unit.Dp(240)

// The chooser's rows. Every row is the same height, an empty slot included, so
// the ten read as one column; the number sits in a column of its own so the
// names start on one line.
const (
	personRowHeight = unit.Dp(50)
	slotNumberWidth = unit.Dp(22)
)

// columnHeaderHeight is both columns' header line. Fixed, so the two rules under
// them sit at one height whatever each header holds.
const columnHeaderHeight = unit.Dp(34)

// characterView is one slot as the frame may describe it.
type characterView struct {
	Slot         int
	Used         bool
	Name         string
	Class        string
	Level, Runes uint32
	// KnowsStorage is false when the storage box could not be read, in which
	// case Owns answers for inventory alone. Said on screen rather than
	// swallowed: an offer to hand out a copy must not imply a certainty it
	// does not have.
	KnowsStorage bool
	Owns         map[int32]bool
	// Levels is the upgrade level each owned Ash is held at, keyed the same
	// way. Presence means the same thing as a true in Owns.
	Levels map[int32]int
}

func buildCharacters(states []application.CharacterState) []characterView {
	views := make([]characterView, 0, len(states))
	for _, state := range states {
		views = append(views, characterView{
			Slot: state.Slot, Used: state.Used, Name: state.Name, Class: state.Class,
			Level: state.Level, Runes: state.Runes,
			KnowsStorage: state.KnowsStorage, Owns: state.Owns, Levels: state.Levels,
		})
	}
	return views
}

// summary is the line under a character's name: their level and starting
// class, which is what tells them apart. A save may hold six characters with
// one name, and two of them at one level.
//
// How much of the catalogue they hold is not here: it is the count over the
// column beside it for whichever character is chosen.
func (person characterView) summary() string {
	if !person.Used {
		return ""
	}
	if person.Class == "" {
		return fmt.Sprintf("Level %d", person.Level)
	}
	return fmt.Sprintf("Level %d · %s", person.Level, person.Class)
}

// firstCharacter is the slot chosen when a save opens: the first one somebody
// is in, so the view opens on a character's Ashes rather than on an
// instruction to pick one.
func firstCharacter(people []characterView) int {
	for index, person := range people {
		if person.Used {
			return index
		}
	}
	return noPerson
}

// layoutCharacters is the whole view: the open bar, then the ten slots on the
// left and the chosen one's Ashes on the right.
func (s *State) layoutCharacters(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Every control on this view is drained before anything is drawn, so a
	// click's effect shows in the frame it was made in rather than the one
	// after it. The merchant editor's ordering, and its reason.
	s.readOpenBar(gtx)
	loaded := s.LoadedSave()
	if loaded != nil {
		if len(s.personBtns) != len(loaded.Characters) {
			s.personBtns = make([]widget.Clickable, len(loaded.Characters))
		}
		for index := range loaded.Characters {
			if loaded.Characters[index].Used && s.personBtns[index].Clicked(gtx) {
				s.person = index
			}
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return barSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutOpenBar(gtx, th, loaded)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutColumns(gtx, th, loaded)
			})
		}),
	)
}

// readOpenBar drains the bar's two controls. Enter in the field loads what is
// typed, because a player who has just pasted a path is not looking for the
// button beside it.
func (s *State) readOpenBar(gtx layout.Context) {
	if s.openBtn.Clicked(gtx) {
		s.askBeforeOpening(s.StartOpenSave)
	}
	typed := s.loadBtn.Clicked(gtx)
	for {
		event, ok := s.pathField.Update(gtx)
		if !ok {
			break
		}
		if _, submitted := event.(widget.SubmitEvent); submitted {
			typed = true
		}
	}
	if !typed {
		return
	}
	if path := strings.TrimSpace(s.pathField.Text()); path != "" {
		s.askBeforeOpening(func() { s.StartLoadSave(path) })
	}
}

// layoutOpenBar is one row: a typed path and Load, a Browse button that opens
// the system picker, whatever the editor is doing right now, and the name of
// the file that is actually open.
//
// The two ways of naming a file are both here on purpose. Browse is what a
// player uses once; the field is what someone with the path already in their
// clipboard uses, and it is also the only way to reach a file a picker's
// sandbox will not show.
//
// It carried a line under the row with the path, the regulation and the counts
// and no longer does: the old Save view arriving here by habit, two rows tall
// to say what a player cannot act on.
//
// The filename slot is the one thing on the bar that is not a control, and it
// earns the place. It spent a revision in the header beside the application's
// title, which is the wrong end of the window from the buttons that change it.
func (s *State) layoutOpenBar(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	barWidth := gtx.Constraints.Max.X
	working := s.BusyMsg() != ""
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPathField(gtx, th, barWidth)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if working {
				return disabledButton(gtx, th, "Load")
			}
			return quietButton(th, &s.loadBtn, "Load").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if working {
				return disabledButton(gtx, th, "Browse…")
			}
			return button(th, &s.openBtn, "Browse…").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		// What the editor is doing right now. The two buttons keep their own
		// labels while it runs, so the bar does not change width as work starts
		// and stops.
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			busy := muted(th, s.BusyMsg())
			busy.MaxLines, busy.Alignment = 1, text.End
			return busy.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.Y = gtx.Dp(dividerBarHeight)
			return verticalDivider(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(filenameSlotWidth)
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			slot := muted(th, "no save open")
			if loaded != nil {
				slot = title(th, th.TextSize, loaded.Name)
			}
			slot.MaxLines, slot.Alignment = 1, text.Middle
			return slot.Layout(gtx)
		}),
	)
}

// layoutPathField is the typed path, capped and then forced to that width. A
// method rather than an inline closure so a test can measure it directly.
func (s *State) layoutPathField(gtx layout.Context, th *material.Theme, barWidth int) layout.Dimensions {
	gtx.Constraints.Max.X = barWidth * pathFieldNum / pathFieldDen
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	height := gtx.Dp(fieldHeight)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return box(gtx, colorInputBg, colorDivider, unit.Dp(4), func(gtx layout.Context) layout.Dimensions {
		field := material.Editor(th, &s.pathField, "Path to memory.dat…")
		field.Color = colorFg
		field.HintColor = colorMuted
		return field.Layout(gtx)
	})
}

// layoutColumns is the drill-down under the bar: a fixed chooser column, a
// divider, and the Ashes taking whatever is left.
func (s *State) layoutColumns(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	if loaded == nil {
		return emptyState(gtx, th, "No save open. Type its path above, or Browse.")
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(charColWidth)
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return s.layoutPeople(gtx, th, loaded)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(verticalDivider),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutGrants(gtx, th, loaded)
		}),
	)
}

// columnHeader is the line at the top of each column: what it is on the left,
// a muted detail beside it, its count on the right, and a rule under all three.
//
// Both columns get one at one fixed height, and that is the point of it: the
// two rules line up and the two lists start at the same height. Controls do
// not go in it - a header holding a chooser grew taller than its neighbour and
// pushed its rule out of line - so a column's actions are a toolbar under it.
func columnHeader(gtx layout.Context, th *material.Theme, name, detail, count string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(columnHeaderHeight)
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			// Centre, not W: W keeps the height as the row's floor, and the
			// labels then sat on the rule rather than in the middle of the line.
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(title(th, headingSize, name).Layout),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(muted(th, detail).Layout),
					layout.Flexed(1, flexSpacer),
					layout.Rigid(muted(th, count).Layout),
				)
			})
		}),
		layout.Rigid(horizontalDivider),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
	)
}

// layoutPeople is the chooser: ten slots, empty ones included, because a save
// has ten and a view that hid the empty ones would be hiding the shape of the
// thing it describes.
func (s *State) layoutPeople(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	used := 0
	for _, person := range loaded.Characters {
		if person.Used {
			used++
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return columnHeader(gtx, th, "Characters", "",
				fmt.Sprintf("%d of %d", used, len(loaded.Characters)))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// The scrollbar overlays rather than taking a gutter: ten rows
			// rarely scroll, and a gutter kept every row short of the rule
			// over them.
			list := material.List(th, &s.personList)
			list.AnchorStrategy = material.Overlay
			return list.Layout(gtx, len(loaded.Characters),
				func(gtx layout.Context, index int) layout.Dimensions {
					return s.personRow(gtx, th, index, loaded)
				})
		}),
	)
}

// personRow is one slot: its number, the character's name, and their level and
// class under it, marked with the accent edge when it is the chosen one - the
// Editor's Ash list marks its selection the same way.
//
// The number is the slot's, counted from one as the game's load menu lists
// them, and it is what tells apart two characters with one name.
func (s *State) personRow(gtx layout.Context, th *material.Theme, index int, loaded *savedView) layout.Dimensions {
	person := loaded.Characters[index]
	row := func(gtx layout.Context, hovered bool) layout.Dimensions {
		return characterRow(gtx, th, person, s.person == index, hovered)
	}
	return layout.Inset{Bottom: unit.Dp(2)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			if !person.Used {
				// An empty slot is drawn and is not a control: there is nobody
				// there to give anything to, and a row that lit under the
				// pointer would say otherwise.
				return row(gtx, false)
			}
			click := &s.personBtns[index]
			return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return row(gtx, click.Hovered())
			})
		})
}

// characterRow draws one chooser row at its fixed height.
func characterRow(gtx layout.Context, th *material.Theme, person characterView,
	chosen, hovered bool) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(personRowHeight))
	corner := gtx.Dp(unit.Dp(4))
	shape := clip.RRect{Rect: image.Rectangle{Max: size}, SE: corner, SW: corner, NE: corner, NW: corner}
	switch {
	case chosen:
		paint.FillShape(gtx.Ops, colorSlotFill, shape.Op(gtx.Ops))
		paint.FillShape(gtx.Ops, colorAccent, clip.Rect{Max: image.Pt(gtx.Dp(unit.Dp(2)), size.Y)}.Op())
	case hovered:
		paint.FillShape(gtx.Ops, alpha(colorSlotFill, 0xA0), shape.Op(gtx.Ops))
	}
	if hovered {
		pointer.CursorPointer.Add(gtx.Ops)
	}

	gtx.Constraints = layout.Exact(size)
	layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(slotNumberWidth)
					return muted(th, fmt.Sprint(person.Slot+1)).Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if !person.Used {
						return muted(th, "Empty").Layout(gtx)
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							name := material.Body1(th, person.Name)
							name.Color, name.MaxLines = colorFg, 1
							return name.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							line := muted(th, person.summary())
							line.MaxLines = 1
							return line.Layout(gtx)
						}),
					)
				}),
			)
		})
	})
	return layout.Dimensions{Size: size}
}
