package gio

// The container half of the Editor: every Ash, and - once one is opened - its
// formation on the placement grid (placement.go), which replaces the list.
//
// An open Ash has its options in a strip over the grid, in two groups: its
// cost (the two fields and Free, price.go), and the grid's own switches -
// whether it shows what each summon sees, and its turning knobs. Everything
// about one summon is in its own window (creature.go), opened by clicking its
// token.

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// ashTileMinWidth is the narrowest an Ash's tile is: wide enough for most
// names on two lines under the icon.
const ashTileMinWidth = unit.Dp(116)

// layoutContainers is the left half: the list, or the open Ash.
func (s *State) layoutContainers(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	if s.allAshesBtn.Clicked(gtx) {
		s.closeAsh()
	}
	ash := s.selectedAsh(loaded)
	if ash == nil {
		return s.layoutAshList(gtx, th, loaded)
	}
	return s.layoutFormationPage(gtx, th, loaded, ash)
}

// closeAsh goes back to the list, forgetting everything the grid held about
// the Ash that was open.
func (s *State) closeAsh() {
	s.selected = noAsh
	s.chosenSlot, s.hoverSlot, s.pressSlot, s.creatureSlot = noToken, noToken, noToken, noToken
	s.together = nil
	s.knobHover, s.turning = noToken, noToken
	s.moving, s.panning = false, false
	s.gridView = homeView
}

// openAsh opens one Ash's formation, the grid on its home view.
func (s *State) openAsh(index int) {
	s.closeAsh()
	s.selected = index
}

// layoutAshList is every container, as tiles: the item's icon and its name.
func (s *State) layoutAshList(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	if len(s.ashBtns) != len(loaded.Ashes) {
		s.ashBtns = make([]widget.Clickable, len(loaded.Ashes))
	}
	changed := 0
	for _, ash := range loaded.Ashes {
		if s.edits[ash.GoodsFamily] != nil {
			changed++
		}
	}
	count := ""
	if changed > 0 {
		count = fmt.Sprintf("%d changed", changed)
	}
	tile := max(s.EditorIconSize()+unit.Dp(24), ashTileMinWidth)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return panelHeader(gtx, th, "SPIRIT ASHES", count)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// As many tiles as fit, then stretched to fill the row, so the slack
			// is spread between them rather than left at the end.
			available := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(12))
			least, gap := gtx.Dp(tile), gtx.Dp(gridGap)
			columns := max((available+gap)/(least+gap), 1)
			width := unit.Dp(float32((available-(columns-1)*gap)/columns) / gtx.Metric.PxPerDp)
			return layoutColumns(gtx, th, &s.ashList, len(loaded.Containers), columns, false,
				func(gtx layout.Context, at int) layout.Dimensions {
					index := loaded.Containers[at]
					return s.ashTile(gtx, th, index, loaded.Ashes[index], width)
				})
		}),
	)
}

// ashTile is one Ash in the list: its icon over its name, and a line in ember
// when its formation cannot be edited. Clicking it opens the Ash.
func (s *State) ashTile(gtx layout.Context, th *material.Theme, index int, ash ashView, width unit.Dp) layout.Dimensions {
	click := &s.ashBtns[index]
	if click.Clicked(gtx) {
		s.openAsh(index)
		// This frame chose to draw the list before the click was read, so
		// the grid is drawn in the next one, now rather than at whatever
		// redraws the window next - the pointer's next move.
		gtx.Execute(op.InvalidateCmd{})
	}
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(width), gtx.Dp(width)
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					var border *color.NRGBA
					if s.edits[ash.GoodsFamily] != nil {
						border = &colorGlow
					}
					size := s.EditorIconSize()
					icon, has := s.art.op(ash.Name, gtx.Dp(size))
					return iconCell{Img: icon, Empty: !has, Size: size, Border: border}.Layout(gtx, th, nil)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					name := material.Body2(th, ash.Name)
					name.Color, name.MaxLines, name.Alignment = colorFg, 2, text.Middle
					if s.edits[ash.GoodsFamily] != nil {
						name.Color = colorGlow
					}
					return name.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					why := ash.locked()
					if why == "" {
						return layout.Dimensions{}
					}
					note := muted(th, why)
					note.Color, note.MaxLines, note.Alignment = colorWarn, 1, text.Middle
					return note.Layout(gtx)
				}),
			)
		})
		call := macro.Stop()
		if click.Hovered() {
			corner := gtx.Dp(unit.Dp(4))
			paint.FillShape(gtx.Ops, colorSlotFill,
				clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: corner, SW: corner, NE: corner, NW: corner}.Op(gtx.Ops))
			pointer.CursorPointer.Add(gtx.Ops)
		}
		call.Add(gtx.Ops)
		return dims
	})
}

// locked is why an Ash's formation cannot be edited, in a few words, or "".
func (ash ashView) locked() string {
	if ash.RePointed {
		return "converted"
	}
	return ""
}

// lockedNote is the same reason as a sentence, for the open Ash.
func (ash ashView) lockedNote() string {
	if ash.RePointed {
		return "Converted: delivers another Ash's formation until reset."
	}
	return ""
}

// layoutFormationPage is one open Ash: the bar with the way back and its name,
// its options, then the grid.
func (s *State) layoutFormationPage(gtx layout.Context, th *material.Theme, loaded *savedView, ash *ashView) layout.Dimensions {
	editable := ash.locked() == ""
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutFormationBar(gtx, th, ash)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.withHelp(gtx, th, helpGrid, func(gtx layout.Context) layout.Dimensions {
				return s.layoutAshOptions(gtx, th, ash, editable)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(horizontalDivider),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutPlacement(gtx, th, loaded, ash, editable)
		}),
	)
}

// layoutFormationBar is the page's first line: back to the list, and the Ash,
// with a note under its name when there is one.
func (s *State) layoutFormationBar(gtx layout.Context, th *material.Theme, ash *ashView) layout.Dimensions {
	note, bad := s.priceNote(ash)
	if why := ash.lockedNote(); why != "" {
		note, bad = why, false
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			back := rowButton(th, &s.allAshesBtn, "‹  All Ashes")
			return back.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := unit.Dp(36)
			icon, has := s.art.op(ash.Name, gtx.Dp(size))
			return iconCell{Img: icon, Empty: !has, Size: size}.Layout(gtx, th, nil)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					name := title(th, unit.Sp(17), ash.Name)
					name.MaxLines = 1
					return name.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if note == "" {
						return layout.Dimensions{}
					}
					style := muted(th, note)
					style.MaxLines = 1
					style.Color = colorWarn
					if bad {
						style.Color = colorError
					}
					return style.Layout(gtx)
				}),
			)
		}),
	)
}

// optionsWidth is what the strip needs to hold both groups on one line; a
// narrower one puts the grid's switches under the cost.
const optionsWidth = unit.Dp(640)

// layoutAshOptions is the strip of the Ash's own options: COST - the fields
// and Free - and GRID - Show sight and the turning knobs, which an Ash whose
// formation cannot be changed does not offer, having none to turn.
func (s *State) layoutAshOptions(gtx layout.Context, th *material.Theme, ash *ashView, editable bool) layout.Dimensions {
	sight, knobs := s.showSight.Update(gtx), s.showKnobs.Update(gtx)
	if sight || knobs {
		s.settings.ShowSight, s.settings.HideKnobs = s.showSight.Value, !s.showKnobs.Value
		s.applySettings()
	}
	switchBox := func(box *widget.Bool, label string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			style := material.CheckBox(th, box, label)
			style.Color, style.IconColor = colorFg, colorAccent
			return style.Layout(gtx)
		}
	}
	group := func(name string, controls ...layout.FlexChild) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := heading(th, name)
					label.TextSize = th.TextSize * 13 / 16
					return label.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, append(children, controls...)...)
		}
	}
	cost := group("COST", layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return s.layoutPrice(gtx, th, ash)
	}))
	gridControls := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.helpBadge(gtx, th, helpGrid) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
		layout.Rigid(switchBox(&s.showSight, "Show sight")),
	}
	if editable {
		gridControls = append(gridControls,
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Rigid(switchBox(&s.showKnobs, "Turning knobs")))
	}
	grid := group("GRID", gridControls...)
	if gtx.Constraints.Max.X < gtx.Dp(optionsWidth) {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(cost),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(grid))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(cost),
		layout.Rigid(layout.Spacer{Width: unit.Dp(18)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// A rule between the groups, as tall as a field.
			size := image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(26)))
			paint.FillShape(gtx.Ops, colorDivider, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(18)}.Layout),
		layout.Rigid(grid),
		layout.Flexed(1, flexSpacer),
	)
}

// tooltip is who is in a slot, and what it will do to the player if that is
// anything.
func (slot slotView) tooltip() string {
	if why := slot.warning(); why != "" {
		return slot.Name + "\nCan hurt you: " + why + ".\nIt is a warning, not a refusal: drag it off the grid to clear it."
	}
	return slot.Name
}

// warning is what this slot's occupant has been seen to do to the player who
// summoned it, and "" for everything nobody has seen do anything - which is
// almost all of the catalog, and is not the same thing as safe.
//
// A summon cannot hit its owner with an ATTACK: research settled that in game,
// and internal/application/hazard.go carries the mechanism and the measurement.
// What the two on that list do instead is run the player over and set the
// ground alight, neither of which is an attack and neither of which can be
// turned off from a save. So the grid says so, and the player decides.
func (slot slotView) warning() string {
	if !slot.Filled || slot.NoActor || slot.Model == "" {
		return ""
	}
	why, _ := application.HurtsSummoner(slot.Model)
	return why
}
