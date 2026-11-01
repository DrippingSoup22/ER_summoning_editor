package gio

// A dropdown: a header showing the current choice, and a list that opens over
// whatever is under it.
//
// Ported from the sibling merchant editor's components.Combo, which is the
// widget its catalog panel filters by category with, and carried over rather
// than rediscovered for the two traps it encodes:
//
//   - The list is RECORDED and DEFERRED, so it composites over the panels
//     drawn after this one. A list laid out in the flex would shove the grid
//     down every time it opened, which moves the thing the player is aiming at.
//   - The scrim is a huge transparent click-catcher drawn BEFORE the list and
//     also deferred, so a click anywhere else closes the dropdown instead of
//     falling through to whatever it landed on. Every queued scrim click is
//     drained, not just one, or a click that lands on the header in the same
//     frame leaks through and re-opens what it just closed.
//
// It is smaller than the merchant editor's: the options here are a fixed set
// of buckets rather than a catalog's categories, so there is no SetOptions, no
// separate menu and header labels, and the selection is an index rather than a
// string.

import (
	"image"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// comboListMax is how tall the opened list may get before it scrolls. Enough
// for the buckets there are, with room for one more.
const comboListMax = unit.Dp(300)

// comboPad is the room either side of a header's label, and between the label
// and its arrow.
const comboPad = unit.Dp(8)

type combo struct {
	header widget.Clickable
	scrim  widget.Clickable
	opts   []widget.Clickable
	list   widget.List
	open   bool
	// up opens the list above the header, for a dropdown near the window's
	// foot: a widget cannot know where on the window it is, so its caller
	// says.
	up bool
}

// Layout draws the header at the given width, and the list when it is open.
// It returns the index the player chose this frame, or -1 for no change, so a
// caller can invalidate whatever the choice feeds without polling a flag.
func (c *combo) Layout(gtx layout.Context, th *material.Theme,
	width unit.Dp, options []string, selected int) (layout.Dimensions, int) {
	if len(c.opts) != len(options) {
		c.opts = make([]widget.Clickable, len(options))
	}
	if c.header.Clicked(gtx) {
		c.open = !c.open
	}
	chose := -1
	for index := range options {
		if c.opts[index].Clicked(gtx) {
			if index != selected {
				chose = index
			}
			c.open = false
		}
	}
	// Every queued click, not just one: a second landing in the same frame
	// would otherwise re-toggle the header on the next.
	for c.scrim.Clicked(gtx) {
		c.open = false
	}
	// Esc closes an open list, before it reaches anything under it.
	if c.open && escaped(gtx) {
		c.open = false
	}

	label := ""
	if selected >= 0 && selected < len(options) {
		label = options[selected]
	}
	dims := c.header.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return c.layoutHeader(gtx, th, width, label)
	})
	if c.open && len(options) > 0 {
		c.layoutOverlay(gtx, th, dims.Size, options, selected)
	}
	return dims, chose
}

// layoutHeader is the closed dropdown, a text field's height so the two stand
// in one line.
func (c *combo) layoutHeader(gtx layout.Context, th *material.Theme,
	width unit.Dp, label string) layout.Dimensions {
	pinned := gtx.Dp(width)
	gtx.Constraints = layout.Exact(image.Pt(pinned, gtx.Dp(fieldHeight)))

	macro := op.Record(gtx.Ops)
	// Centred down the height by Center, which Flex does not do: it aligns
	// its children within the tallest of them, not within its constraints.
	dims := layout.Inset{Left: comboPad, Right: comboPad}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					style := material.Body2(th, label)
					style.Color = colorFg
					style.MaxLines = 1
					return style.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: comboPad}.Layout),
				layout.Rigid(muted(th, arrowGlyph(c.open)).Layout),
			)
		})
	})
	call := macro.Stop()

	// A control a player can press sits above the panel it is on, and lifts
	// again under the cursor - see colorControl.
	ground := colorControl
	if c.header.Hovered() {
		ground = colorSlotEdge
		pointer.CursorPointer.Add(gtx.Ops)
	}
	paint.FillShape(gtx.Ops, ground, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: unit.Dp(3)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	return dims
}

// fit is how wide the header must be to show the longest option whole: a
// dropdown as wide as what it offers, keeping that width whichever is chosen.
// The labels are measured into a recording that is thrown away.
func (c *combo) fit(gtx layout.Context, th *material.Theme, options []string) unit.Dp {
	measure := gtx
	measure.Constraints = layout.Constraints{Max: image.Pt(1<<24, 1<<24)}
	macro := op.Record(gtx.Ops)
	widest := 0
	for _, option := range options {
		style := material.Body2(th, option)
		style.MaxLines = 1
		widest = max(widest, style.Layout(measure).Size.X)
	}
	arrow := muted(th, arrowGlyph(false)).Layout(measure).Size.X
	macro.Stop()
	// Two pixels over, so a label measured exactly is never cut a pixel short.
	px := widest + arrow + 3*gtx.Dp(comboPad) + 2
	return unit.Dp(float32(px) / gtx.Metric.PxPerDp)
}

// dropdown is one of a picker's dropdowns as filterBar lays it out: how wide it
// is, and what draws it.
type dropdown struct {
	width unit.Dp
	draw  layout.Widget
}

// searchLeast is the narrowest the search field stands beside the dropdowns;
// with less room left it takes a line of its own.
const searchLeast = unit.Dp(180)

// filterBar lays a picker's filters out: its dropdowns left to right, each as
// wide as its longest option, then the search field taking the rest of the
// line - or a line of its own where less than searchLeast is left. Every
// control is a text field's height and a line wraps where the next does not
// fit. The dropdowns come first, so the one just used stays where it is when
// the one after it comes or goes.
func filterBar(gtx layout.Context, dropdowns []dropdown, search layout.Widget) layout.Dimensions {
	width, height, gap := gtx.Constraints.Max.X, gtx.Dp(fieldHeight), gtx.Dp(unit.Dp(8))
	x, y := 0, 0
	place := func(w int, draw layout.Widget) {
		if x > 0 && x+gap+w > width {
			x, y = 0, y+height+gap
		}
		if x > 0 {
			x += gap
		}
		at := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		child := gtx
		child.Constraints = layout.Exact(image.Pt(min(w, width), height))
		draw(child)
		at.Pop()
		x += w
	}
	for _, one := range dropdowns {
		place(gtx.Dp(one.width), one.draw)
	}
	rest := width - x - gap
	if x == 0 || rest < gtx.Dp(searchLeast) {
		rest = width
	}
	place(rest, search)
	return layout.Dimensions{Size: image.Pt(width, y+height)}
}

func (c *combo) layoutOverlay(gtx layout.Context, th *material.Theme,
	header image.Point, options []string, selected int) {
	// The scrim first, so it sits under the list it closes.
	macro := op.Record(gtx.Ops)
	const span = 8000
	offset := op.Offset(image.Pt(-span/2, -span/2)).Push(gtx.Ops)
	scrimGtx := gtx
	scrimGtx.Constraints = layout.Exact(image.Pt(span, span))
	c.scrim.Layout(scrimGtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(span, span)}
	})
	offset.Pop()
	op.Defer(gtx.Ops, macro.Stop())

	macro = op.Record(gtx.Ops)
	listGtx := gtx
	listGtx.Constraints.Min = image.Pt(header.X, 0)
	listGtx.Constraints.Max = image.Pt(header.X, gtx.Dp(comboListMax))
	inner := op.Record(gtx.Ops)
	list := c.layoutList(listGtx, th, options, selected)
	drawn := inner.Stop()
	below := header.Y
	if c.up {
		below = -list.Size.Y
	}
	offset = op.Offset(image.Pt(0, below)).Push(gtx.Ops)
	drawn.Add(gtx.Ops)
	offset.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}

func (c *combo) layoutList(gtx layout.Context, th *material.Theme,
	options []string, selected int) layout.Dimensions {
	c.list.Axis = layout.Vertical
	macro := op.Record(gtx.Ops)
	dims := material.List(th, &c.list).Layout(gtx, len(options),
		func(gtx layout.Context, index int) layout.Dimensions {
			return c.opts[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.row(gtx, th, index, options[index], index == selected)
			})
		})
	call := macro.Stop()

	paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	widget.Border{Color: colorDivider, Width: unit.Dp(1)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	return dims
}

// row is one option. It is recorded and replayed rather than stacked, because
// layout.Stack zeroes Constraints.Min before laying out its children, so a
// Stack-based ground would only ever cover the label's own text width instead
// of the row - the merchant editor's own note, and the same fix.
func (c *combo) row(gtx layout.Context, th *material.Theme,
	index int, label string, current bool) layout.Dimensions {
	full := gtx.Constraints.Max.X
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 6, Bottom: 6, Left: 8, Right: 8}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			style := material.Body2(th, label)
			style.Color = colorFg
			style.MaxLines = 1
			return style.Layout(gtx)
		})
	call := macro.Stop()
	dims.Size.X = full

	switch {
	case current:
		paint.FillShape(gtx.Ops, colorAccentFill, clip.Rect{Max: dims.Size}.Op())
	case c.opts[index].Hovered():
		paint.FillShape(gtx.Ops, colorControl, clip.Rect{Max: dims.Size}.Op())
	}
	if c.opts[index].Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	call.Add(gtx.Ops)
	return dims
}

func arrowGlyph(open bool) string {
	if open {
		return "▴"
	}
	return "▾"
}
