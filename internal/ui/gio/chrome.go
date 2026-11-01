package gio

// Shared surfaces, dividers, buttons and text styles.
//
// The layout rules here are the sibling merchant editor's, and they are
// carried over rather than rediscovered because the two traps they encode cost
// real time there: a panel background must force Min = Max to fill its flex
// region, and a bar must not, or a one-line bar balloons to fill the window.

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// panelSurface fills a panel background and pads its content. Correct only
// inside a Flexed region: forcing Min = Max is what makes it grow to fill the
// space it was given.
func panelSurface(gtx layout.Context, w layout.Widget) layout.Dimensions {
	paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.UniformInset(unit.Dp(12)).Layout(gtx, w)
}

// barSurface paints a background sized to its content's own height, for a
// Rigid bar in a vertical flex. At least one child of w must be Flexed so the
// bar's natural width comes out full-width.
func barSurface(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, w)
	call := macro.Stop()

	paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	return dims
}

// flexSpacer is an empty Flexed child that claims the space it was given.
// Returning a bare layout.Dimensions{} would report zero size instead, and the
// row would collapse around it.
func flexSpacer(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func horizontalDivider(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))
	paint.FillShape(gtx.Ops, colorDivider, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

func verticalDivider(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(1)), gtx.Constraints.Max.Y)
	paint.FillShape(gtx.Ops, colorDivider, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

// heading is a panel title: set in capitals, so the panels read as places
// rather than as more text. It does not pin a typeface - the window has one
// face at a time and Settings chooses it.
//
// It was 12sp in colorMuted, which is smaller and fainter than the body text
// under it - a title quieter than its own contents, which is backwards. It is
// now the size the Characters view already titles its columns at, in the
// editor's own foreground, so a panel announces itself.
func heading(th *material.Theme, label string) material.LabelStyle {
	style := material.Label(th, headingSize, label)
	style.Color = colorFg
	return style
}

// headingSize is the panel titles', and it is columnHeader's, because the two
// are the same thing on two views.
const headingSize = unit.Sp(15)

// panelHeader is the line at the top of an Editor panel: what the panel is on
// the left, what it holds on the right, and a rule under both.
//
// The count sits beside the title rather than on a line under it. A count on
// its own line is a whole row of the panel spent on one number, and it reads
// as a sentence about the panel when it is only a label on it - which is what
// the Ash rail's "84 in this save" and the spirit catalog's count-beside-the-
// search-field both were.
//
// count may be empty, for a panel whose contents are not a number anybody is
// looking for: the rail always holds every Ash in the game, and the formation
// always holds ten slots.
func panelHeader(gtx layout.Context, th *material.Theme, name, count string) layout.Dimensions {
	return panelHeaderWith(gtx, th, name, count, nil)
}

// panelHeaderWith is panelHeader with something after the name: a "?".
func panelHeaderWith(gtx layout.Context, th *material.Theme, name, count string, after layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(heading(th, name).Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if after == nil {
						return layout.Dimensions{}
					}
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, after)
				}),
				layout.Flexed(1, flexSpacer),
				layout.Rigid(muted(th, count).Layout),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(horizontalDivider),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
	)
}

// title is the application's own name and the selected Ash's, at reading size.
func title(th *material.Theme, size unit.Sp, label string) material.LabelStyle {
	style := material.Label(th, size, label)
	style.Color = colorFg
	return style
}

// muted is secondary text: counts, hints, and everything the player is not
// being asked to act on.
func muted(th *material.Theme, label string) material.LabelStyle {
	style := material.Body2(th, label)
	style.Color = colorMuted
	return style
}

// button is the one interactive button style: an accent ground under the
// editor's own light text. See colorAccentFill for why the ground is the deep
// accent and not the pale one.
func button(th *material.Theme, click *widget.Clickable, label string) material.ButtonStyle {
	style := material.Button(th, click, label)
	style.Background = colorAccentFill
	style.Color = colorFg
	style.CornerRadius = unit.Dp(4)
	return style
}

// quietButton is the same shape without the accent, for actions that are
// available but are not what the player came here to do.
func quietButton(th *material.Theme, click *widget.Clickable, label string) material.ButtonStyle {
	style := button(th, click, label)
	style.Background = colorControl
	return style
}

// selectOnFocus selects the whole of whichever of editors has just gained the
// keys, so a number typed into it replaces the one it holds rather than
// joining it; a click into a field that already has them places the caret as
// usual. last is the one that had the keys the frame before.
func selectOnFocus(gtx layout.Context, last **widget.Editor, editors ...*widget.Editor) {
	var now *widget.Editor
	for _, editor := range editors {
		if gtx.Focused(editor) {
			now = editor
		}
	}
	if now != nil && now != *last {
		now.SetCaret(now.Len(), 0)
	}
	*last = now
}

// escaped reports whether Esc was pressed, which closes a window open over
// the rest - the same as its × and a click outside it.
func escaped(gtx layout.Context) bool {
	pressed := false
	for {
		got, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			return pressed
		}
		if e, isKey := got.(key.Event); isKey && e.State == key.Press {
			pressed = true
		}
	}
}

// modal draws a blocking dialog: a scrim over the whole window, and a panel
// centred across it, its top a fixed way down. The top stays where it is
// whatever the panel holds, so a click that opens a line or a fold moves only
// what is under it: a centred panel would recentre, and slide the very button
// the pointer is on away from it.
//
// A scrimmed dialog rather than a panel that opens in the layout, which is the
// merchant editor's own correction - it shipped the pending list as a corner
// dropdown first and replaced it, because an unscrimmed popup lets clicks
// through to the panels behind it. The version here also avoids the other half
// of that problem: a list that opens inside the flex shoves the editor upwards,
// so the thing a player was looking at moves when they ask what is staged.
//
// Everything is recorded and deferred, so the dialog composites over whatever
// the frame drew before it regardless of where it is called from. width caps
// the panel.
func modal(gtx layout.Context, scrim *widget.Clickable, width unit.Dp, content layout.Widget) {
	// The panel's own input layer, keyed apart from the scrim: a click on the
	// panel between its controls lands here and is dropped, where it would
	// otherwise fall through to the scrim and close the dialog.
	panel := (*modalPanel)(scrim)
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: panel, Kinds: pointer.Press | pointer.Release}); !ok {
			break
		}
	}

	deferred := op.Record(gtx.Ops)
	paint.FillShape(gtx.Ops, modalScrim, clip.Rect{Max: gtx.Constraints.Max}.Op())
	// The scrim is one opaque input layer over the whole window: it swallows
	// what it covers, and a click on it closes the dialog.
	scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})

	// A tenth of the window down, and never nearer its edges than a margin;
	// a Direction centres within the minimum size, which a caller is free to
	// leave at zero.
	// The bottom keeps clear of the footer, whose message line is drawn over
	// the scrim (notice.go) and must not land on the dialog.
	margin, bottom := gtx.Dp(unit.Dp(40)), gtx.Dp(dialogBottom)
	top := max(gtx.Constraints.Max.Y/10, margin)
	gtx.Constraints.Min = gtx.Constraints.Max
	layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Capped so the dialog does not fill a wide window, and floored so a
		// window narrower than the margins it wants still lays out rather than
		// asking for a negative width.
		gtx.Constraints.Max.X = max(min(gtx.Constraints.Max.X-gtx.Dp(unit.Dp(48)), gtx.Dp(width)), 1)
		gtx.Constraints.Max.Y = max(gtx.Constraints.Max.Y-top-bottom, 1)
		gtx.Constraints.Min = image.Point{}
		defer op.Offset(image.Pt(0, top)).Push(gtx.Ops).Pop()
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(14)).Layout(gtx, content)
		call := macro.Stop()

		paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: dims.Size}.Op())
		widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return dims })
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		event.Op(gtx.Ops, panel)
		area.Pop()
		call.Add(gtx.Ops)
		return dims
	})
	op.Defer(gtx.Ops, deferred.Stop())
}

// dialogBottom is how far above the window's bottom a dialog stops: past the
// footer and its message line.
const dialogBottom = unit.Dp(88)

// modalPanel is the scrim's address under another type, which makes it a tag
// of its own: an event tag is compared by type and pointer, so it cannot
// collide with the scrim's.
type modalPanel widget.Clickable

// disabledButton draws a button that is not one: no widget.Clickable is laid
// out at all, so it swallows nothing and reacts to nothing.
func disabledButton(gtx layout.Context, th *material.Theme, label string) layout.Dimensions {
	return inertButton(gtx, th, label,
		layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}, th.TextSize, unit.Dp(4))
}

func inertButton(gtx layout.Context, th *material.Theme, label string,
	inset layout.Inset, size unit.Sp, radius unit.Dp) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		style := material.Label(th, size, label)
		style.Color = colorMuted
		// Centred, because the thing this stands in for is a material button
		// and those centre their label. It only shows where the caller has
		// pinned a width - in a list of rows - and is a no-op in a bar, where
		// the label sizes to itself.
		style.Alignment = text.Middle
		return style.Layout(gtx)
	})
	call := macro.Stop()

	corner := gtx.Dp(radius)
	shape := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: corner, SW: corner, NE: corner, NW: corner}
	paint.FillShape(gtx.Ops, colorInputBg, shape.Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// emptyState is what a panel says when it has nothing in it yet. Centred, one
// line, and never an error: an application that starts with no save is doing
// what it is supposed to.
func emptyState(gtx layout.Context, th *material.Theme, label string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		style := muted(th, label)
		style.Alignment = text.Middle
		return style.Layout(gtx)
	})
}

// box paints a rounded, outlined region at the constraints it was given and
// lays its content inside. The caller pins the size, because every box in this
// application is part of a grid and a grid whose cells size themselves to their
// contents is not one.
func box(gtx layout.Context, fill, edge color.NRGBA, radius unit.Dp, w layout.Widget) layout.Dimensions {
	size := gtx.Constraints.Max
	corner := gtx.Dp(radius)
	shape := clip.RRect{Rect: image.Rectangle{Max: size}, SE: corner, SW: corner, NE: corner, NW: corner}
	paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))

	// The stroke is centred on its path, so the path is inset by half of it to
	// keep the whole line inside the cell.
	line := gtx.Dp(unit.Dp(1))
	inner := clip.RRect{
		Rect: image.Rectangle{Min: image.Pt(line, line), Max: size.Sub(image.Pt(line, line))},
		SE:   corner, SW: corner, NE: corner, NW: corner,
	}
	paint.FillShape(gtx.Ops, edge,
		clip.Stroke{Path: inner.Path(gtx.Ops), Width: float32(line)}.Op())

	layout.UniformInset(unit.Dp(6)).Layout(gtx, w)
	return layout.Dimensions{Size: size}
}

// imageCell is an entity's portrait, or the square one will occupy.
//
// Both cases are drawn at exactly the same size, which was the point of
// reserving the square before any artwork existed: a picture arriving changes
// what is in the cell and moves nothing around it. 30 of the 282 exist today,
// so the empty case is the common one and will be for a long time.
//
// Nothing is invented for a family with no picture. There is no placeholder
// glyph, no initial, no silhouette - a made-up mark on 250 cells would be 250
// pieces of misinformation, and the name beside the cell is already the answer.
func imageCell(gtx layout.Context, side unit.Dp, art paint.ImageOp, has bool) layout.Dimensions {
	px := gtx.Dp(side)
	gtx.Constraints = layout.Exact(image.Pt(px, px))
	if !has {
		return box(gtx, colorInputBg, alpha(colorSlotEdge, 0x66), unit.Dp(3),
			func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
	}
	// Clipped to the same rounded square the empty cell draws, so a row of
	// portraits and a row of empties are the same shape.
	corner := gtx.Dp(unit.Dp(3))
	shape := clip.RRect{Rect: image.Rectangle{Max: image.Pt(px, px)}, SE: corner, SW: corner, NE: corner, NW: corner}
	defer shape.Push(gtx.Ops).Pop()
	art.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(px, px)}
}

// selectableRow is one entry in a list: full width, its own height, and marked
// when it is the current one. Unselected rows paint nothing at all, so a long
// list is a column of text rather than a column of boxes.
func selectableRow(gtx layout.Context, selected bool, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 6, Bottom: 6, Left: 8, Right: 8}.Layout(gtx, w)
	call := macro.Stop()

	size := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
	if selected {
		corner := gtx.Dp(unit.Dp(4))
		shape := clip.RRect{Rect: image.Rectangle{Max: size}, SE: corner, SW: corner, NE: corner, NW: corner}
		paint.FillShape(gtx.Ops, colorSlotFill, shape.Op(gtx.Ops))
		// The accent edge, rather than an accent ground: the selected Ash is
		// pointed at, not lit up.
		paint.FillShape(gtx.Ops, colorAccent,
			clip.Rect{Max: image.Pt(gtx.Dp(unit.Dp(2)), size.Y)}.Op())
	}
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: size}
}
