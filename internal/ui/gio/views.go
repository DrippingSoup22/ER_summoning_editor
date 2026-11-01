package gio

// Three views, and the reason there are three.
//
// The Editor is the application. Characters and Settings exist so that the
// Editor does not have to make room for anything that is not editing.
//
// Characters is the landing view and it owns opening a file, which is the
// sibling merchant editor's shape and is right for the same reason: the two
// things a player does before editing - pick a save, pick who they are editing
// for - belong on one screen, and a Save view that carries a file card and
// nothing else spends a whole view on one button. There is no Save view here
// any more; what it used to say about the open file is said by the bar at the
// top of Characters.
//
// This is not a reversal of the decision to merge the old Ashes and Formation
// views. Those were two halves of one gesture - choose an Ash, fill its slots -
// and splitting them put a navigation step inside the gesture. Settings is the
// opposite: it is a thing you do once and leave.
//
// The footer belongs to all three. A staged change hidden behind a view switch
// is a staged change a player can forget they made, and the count is the one
// number this application must never let out of sight.

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type viewID int

const (
	viewCharacters viewID = iota
	viewEditor
	viewSettings
	viewCount int = 3
)

var viewNames = [viewCount]string{"Characters", "Editor", "Settings"}

// layoutTabs is the view switch, in the header. Every tab is always reachable:
// an Editor with no save open explains itself, which is better than a control
// that is there but refuses.
func (s *State) layoutTabs(gtx layout.Context, th *material.Theme) layout.Dimensions {
	children := make([]layout.FlexChild, 0, viewCount*2)
	for index, name := range viewNames {
		index, name := index, name
		if index > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.tab(gtx, th, &s.tabBtns[index], name, viewID(index))
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (s *State) tab(gtx layout.Context, th *material.Theme, click *widget.Clickable,
	name string, target viewID) layout.Dimensions {
	if click.Clicked(gtx) {
		s.view = target
	}
	if s.view == target {
		return button(th, click, name).Layout(gtx)
	}
	return quietButton(th, click, name).Layout(gtx)
}

// layoutEditor is the two halves: the containers - every Ash, or the open one
// on its grid - and the catalog of spirits that fill them, with a divider
// between them the player drags to share the width.
func (s *State) layoutEditor(gtx layout.Context, th *material.Theme) layout.Dimensions {
	loaded := s.LoadedSave()
	if loaded == nil {
		return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
			return emptyState(gtx, th, "No save open. Open one in Characters.")
		})
	}
	if s.builder != nil {
		return s.layoutBuilder(gtx, th, loaded)
	}
	// Whether a drag is in flight decides how the slots draw, and the slots
	// draw before the panel the drag starts in. Asking the sources directly is
	// what keeps the highlight in the same frame as the drag.
	s.dragging = false
	for index := range s.familyDrags {
		if dragged(gtx, &s.familyDrags[index]) {
			s.dragging = true
			break
		}
	}
	for index := range s.entryDrags {
		if dragged(gtx, &s.entryDrags[index]) {
			s.dragging = true
			break
		}
	}
	for index := range s.variantBtns {
		if dragged(gtx, &s.variantBtns[index]) {
			s.dragging = true
			break
		}
	}
	total, gap := gtx.Constraints.Max.X, gtx.Dp(splitGap)
	s.readSplit(gtx, total, gap)
	width := s.catalogPx(gtx, total)
	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutContainers(gtx, th, loaded)
			})
		}),
		layout.Rigid(layout.Spacer{Width: splitGap}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutSpirits(gtx, th, loaded)
			})
		}),
	)
	s.drawSplit(gtx, image.Rect(total-width-gap, 0, total-width, dims.Size.Y))
	// A click in the catalog that opened the builder did so after this frame
	// chose to draw the Editor, so the builder is drawn in the next one, now.
	if s.builder != nil {
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

const (
	// splitGap is the gap between the halves, and the divider sits in it.
	splitGap = unit.Dp(10)
	// gridLeast is the narrowest the containers' half is let go: a grid of a
	// few metres still, and the bar over it.
	gridLeast = unit.Dp(420)
	// cardName is the room a catalog card keeps for its name beside the
	// portrait, and cardGap the gap between two cards side by side.
	cardName = unit.Dp(176)
	cardGap  = unit.Dp(8)
	// catalogFrame is what the catalog panel spends outside its cards: its
	// padding either side (panelSurface) and the list's scrollbar.
	listScrollbar = unit.Dp(12)
	catalogFrame  = 2*unit.Dp(12) + listScrollbar
)

// catalogCard is the narrowest a catalog card is: its portrait and a name.
func (s *State) catalogCard() unit.Dp {
	return s.SpiritIconSize() + unit.Dp(8) + cardName
}

// defaultCatalogWidth is the catalog before the divider is ever dragged: two
// cards side by side, and a few dp over, since each length is rounded to
// pixels on its own and two cards measured exactly can come out a pixel short.
func (s *State) defaultCatalogWidth() unit.Dp {
	return 2*s.catalogCard() + cardGap + catalogFrame + unit.Dp(4)
}

// catalogPx is the catalog's width in this window: what the player chose, or
// the default, held between one card and what leaves the grid its least.
func (s *State) catalogPx(gtx layout.Context, total int) int {
	width := unit.Dp(s.settings.CatalogWidth)
	if width <= 0 {
		width = s.defaultCatalogWidth()
	}
	least := gtx.Dp(s.catalogCard() + catalogFrame)
	most := total - gtx.Dp(gridLeast) - gtx.Dp(splitGap)
	return max(min(gtx.Dp(width), most), least)
}

// readSplit follows the divider: the catalog's edge goes where the pointer
// is, and is stored when let go.
func (s *State) readSplit(gtx layout.Context, total, gap int) {
	for {
		got, ok := gtx.Event(pointer.Filter{Target: &s.splitTag,
			Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Enter | pointer.Leave})
		if !ok {
			break
		}
		e, isPointer := got.(pointer.Event)
		if !isPointer {
			continue
		}
		switch e.Kind {
		case pointer.Enter:
			s.splitHover = true
		case pointer.Leave:
			s.splitHover = false
		case pointer.Press:
			s.splitting, s.splitMoved = true, false
		case pointer.Drag:
			if s.splitting {
				s.settings.CatalogWidth = toDp(gtx, total-int(e.Position.X)-gap/2)
				s.splitMoved = true
			}
		case pointer.Release, pointer.Cancel:
			// A click is not a choice: only a drag is stored, so a click
			// leaves an unset width following the default.
			if s.splitting && s.splitMoved {
				s.settings.CatalogWidth = toDp(gtx, s.catalogPx(gtx, total))
				s.applySettings()
			}
			s.splitting = false
		}
	}
}

// toDp is a length in pixels as dp.
func toDp(gtx layout.Context, px int) float32 {
	if gtx.Metric.PxPerDp <= 0 {
		return float32(px)
	}
	return float32(px) / gtx.Metric.PxPerDp
}

// drawSplit is the divider: a grip in the gap, brighter under the pointer and
// lit while dragged, and an input area a little wider than the gap so it is
// easy to catch. Positions are the Editor's own, so a drag reads the same
// however the halves have moved under it.
func (s *State) drawSplit(gtx layout.Context, gap image.Rectangle) {
	ink := colorDivider
	switch {
	case s.splitting:
		ink = colorAccent
	case s.splitHover:
		ink = colorMuted
	}
	centre := gap.Min.Add(gap.Max).Div(2)
	half := image.Pt(max(gtx.Dp(unit.Dp(1.5)), 1), gtx.Dp(unit.Dp(22)))
	grip := image.Rectangle{Min: centre.Sub(half), Max: centre.Add(half)}
	paint.FillShape(gtx.Ops, ink, clip.UniformRRect(grip, half.X).Op(gtx.Ops))
	catch := gap.Inset(-gtx.Dp(unit.Dp(3)))
	catch.Min.Y, catch.Max.Y = gap.Min.Y, gap.Max.Y
	area := clip.Rect(catch).Push(gtx.Ops)
	pointer.CursorColResize.Add(gtx.Ops)
	event.Op(gtx.Ops, &s.splitTag)
	area.Pop()
}
