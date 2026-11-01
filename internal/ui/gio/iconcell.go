package gio

// The icon cell, and the grid it sits in.
//
// Ported from the sibling merchant editor's components.IconCell, trimmed to
// what this editor uses and recoloured to this palette. Two details in it are
// carried over deliberately rather than rediscovered, because both cost real
// time there:
//
// Hovered() is read INSIDE click.Layout. Reading it beforehand renders a frame
// stale, and if nothing else asks for a redraw the highlight and tooltip only
// appear when the cursor leaves - the leave event being the thing that finally
// forces a frame.
//
// The tooltip is immediate-mode: drawn if and only if the cell is hovered this
// frame. No delay timer and no retained state, so there is nothing that can
// leave a stale popup behind after the pointer has gone.

import (
	"image"
	"image/color"

	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// iconCellSize is the side of a cell in the Ash grid. Icons are stored at
// 256px, so anything up to that stays sharp.
const iconCellSize = unit.Dp(64)

// gridGap is the space between cells, horizontally and vertically, and it is
// none.
//
// The cells sit against each other and every one draws a 1dp border, so what
// separates two of them is a grey hairline rather than a channel of panel
// background - the sibling merchant editor's grids, and the game's own
// inventory. Six dp of gap between eighty-four icons is most of a column's
// width spent on nothing, and it made the rail read as scattered tiles instead
// of one sheet.
const gridGap = unit.Dp(0)

// tooltipWidth caps a hover label, so a long name wraps rather than running
// off the panel it is drawn over.
const tooltipWidth = unit.Dp(240)

// iconCell is one icon button: an image, a border that carries its state, and
// the name that appears under the cursor.
//
// A by-value style with the caller holding the click state, which is what lets
// one cell keep its hover across frames while the grid around it is rebuilt.
type iconCell struct {
	// Img is the already-decoded icon. The zero value draws the empty cell,
	// which is what an actor with no artwork gets.
	Img   paint.ImageOp
	Empty bool
	// Size overrides the side; 0 means iconCellSize.
	Size unit.Dp
	// Border, when set, replaces the default contour - selection, a staged
	// edit, or an active drop target.
	Border *color.NRGBA
	// Tooltip is the hover text, and the reason a grid of pictures is
	// browsable at all.
	Tooltip string
	// Label is what a clickable cell says to a screen reader, and to a test's
	// hands; empty for none.
	Label string
	// Dim veils the icon. It is how the Characters view says "this character
	// does not have it" without spending a word on it - the picture is still
	// there to be recognised, and what they own stands out beside it.
	Dim bool
}

// Layout draws the cell. click is the caller's retained per-cell state, and
// may be nil for a cell that is only ever looked at.
func (cell iconCell) Layout(gtx layout.Context, th *material.Theme, click *widget.Clickable) layout.Dimensions {
	if click == nil {
		return cell.content(gtx, th, false)
	}
	dims := click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if cell.Label != "" {
			semantic.LabelOp(cell.Label).Add(gtx.Ops)
		}
		return cell.content(gtx, th, click.Hovered())
	})
	if cell.Tooltip != "" && click.Hovered() {
		hoverLabel(gtx, th, cell.Tooltip, dims, false)
	}
	return dims
}

func (cell iconCell) content(gtx layout.Context, th *material.Theme, hovered bool) layout.Dimensions {
	size := cell.Size
	if size == 0 {
		size = iconCellSize
	}
	side := gtx.Dp(size)
	square := image.Pt(side, side)

	gtx.Constraints = layout.Exact(square)

	ground := colorInputBg
	if hovered {
		ground = colorSlotFill
		pointer.CursorPointer.Add(gtx.Ops)
	}
	paint.FillShape(gtx.Ops, ground, clip.Rect{Max: square}.Op())

	if !cell.Empty {
		inner := gtx
		inner.Constraints = layout.Exact(square)
		widget.Image{Src: cell.Img, Fit: widget.Contain, Position: layout.Center}.Layout(inner)
	}
	if cell.Dim {
		// A scrim in the panel's own colour rather than a second copy of the
		// icon at low alpha: one fill, no extra cache entry, and it veils the
		// cell's ground along with the picture on it.
		paint.FillShape(gtx.Ops, alpha(colorPanelBg, 0xB0), clip.Rect{Max: square}.Op())
	}
	// The border is the whole of a cell's state: selected, staged, or the slot
	// a drag is over. It is also what separates one cell from the next, now
	// that the cells sit against each other - see gridGap.
	edge, width := colorDivider, unit.Dp(1)
	if cell.Border != nil {
		edge, width = *cell.Border, unit.Dp(2)
	}
	widget.Border{Color: edge, Width: width}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: square} })

	return layout.Dimensions{Size: square}
}

// hoverLabel draws a hover label under what was just drawn, deferred so that
// it overlays whatever is painted after it rather than being covered by the
// next row of the grid. leftward ends it at the right edge of what is under
// it, for a mark near the window's right edge.
func hoverLabel(gtx layout.Context, th *material.Theme, words string, under layout.Dimensions, leftward bool) {
	deferred := op.Record(gtx.Ops)
	gtx.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(tooltipWidth), 1e6)}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			label := material.Body2(th, words)
			label.Color = colorFg
			return label.Layout(gtx)
		})
	call := macro.Stop()

	at := image.Pt(0, under.Size.Y+gtx.Dp(unit.Dp(2)))
	if leftward {
		at.X = under.Size.X - dims.Size.X
	}
	op.Offset(at).Add(gtx.Ops)

	paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: dims.Size}.Op())
	widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: unit.Dp(2)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	call.Add(gtx.Ops)

	op.Defer(gtx.Ops, deferred.Stop())
}

// layoutColumns is the same grid at a column count the caller has decided.
//
// down fills the columns rather than the rows: with it, ten cells in two
// columns are 1 to 5 down the left and 6 to 10 down the right, so the numbers
// read as one sequence down the panel instead of zig-zagging across it. That is
// only worth having where the cells are numbered, which is the formation.
func layoutColumns(gtx layout.Context, th *material.Theme, list *widget.List, count, columns int,
	down bool, cell func(gtx layout.Context, index int) layout.Dimensions) layout.Dimensions {
	list.Axis = layout.Vertical
	if columns < 1 {
		columns = 1
	}
	rows := (count + columns - 1) / columns
	at := func(row, column int) int { return row*columns + column }
	if down {
		at = func(row, column int) int { return column*rows + row }
	}
	return material.List(th, list).Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		children := make([]layout.FlexChild, 0, columns*2)
		for column := 0; column < columns; column++ {
			index := at(row, column)
			if index >= count {
				break
			}
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return cell(gtx, index) }),
				layout.Rigid(layout.Spacer{Width: gridGap}.Layout),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			}),
			layout.Rigid(layout.Spacer{Height: gridGap}.Layout),
		)
	})
}
