package gio

// The creature window's facing: which way a summon stands when it arrives,
// turned from the way the player faces. Eight buttons around a needle set the
// eight ways, ahead up as on the grid - every facing the editor gives, as the
// knob on the grid and the arrow keys give them (turnStep).

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// facing is one of the eight ways: its angle, its name for the button, and
// where it sits on the pad, the middle being the needle.
type facing struct {
	angle       float64
	name        string
	row, column int
}

var facings = [...]facing{
	{-45, "Face ahead-left", 0, 0}, {0, "Face ahead", 0, 1}, {45, "Face ahead-right", 0, 2},
	{-90, "Face left", 1, 0}, {90, "Face right", 1, 2},
	{-135, "Face back-left", 2, 0}, {180, "Face back", 2, 1}, {135, "Face back-right", 2, 2},
}

// padCell is one button of the pad, and padGap the gap between two.
const (
	padCell = unit.Dp(40)
	padGap  = unit.Dp(6)
)

// layoutFacing is the field: its title and "?", the pad, and beside it which
// way the summon faces in words.
func (s *State) layoutFacing(gtx layout.Context, th *material.Theme, place application.Placement, o choices) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.fieldTitle(gtx, th, "Facing", helpFacing)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutPad(gtx, place.Angle, o.facing)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Flexed(1, muted(th, "Faces "+describeFacing(place.Angle)+".").Layout),
			)
		}),
	)
}

// layoutPad is the eight ways around a needle pointing the summon's own,
// the way it faces lit. Closed, the buttons are drawn and not laid out.
func (s *State) layoutPad(gtx layout.Context, angle float64, open bool) layout.Dimensions {
	cell, gap := gtx.Dp(padCell), gtx.Dp(padGap)
	side := 3*cell + 2*gap
	at := func(row, column int) image.Point { return image.Pt(column*(cell+gap), row*(cell+gap)) }
	square := image.Rectangle{Max: image.Pt(cell, cell)}
	corner := gtx.Dp(unit.Dp(4))
	shape := clip.RRect{Rect: square, SE: corner, SW: corner, NE: corner, NW: corner}

	for index, one := range facings {
		lit := math.Abs(math.Remainder(angle-one.angle, 360)) < 0.5
		offset := op.Offset(at(one.row, one.column)).Push(gtx.Ops)
		draw := func(gtx layout.Context, hovered bool) {
			fill, ink := colorControl, colorFg
			switch {
			case lit:
				fill = colorAccentFill
			case !open:
				fill, ink = colorInputBg, colorMuted
			case hovered:
				fill = colorSlotEdge
			}
			paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
			drawArrow(gtx, layout.FPt(square.Max).Mul(0.5), facingVector(one.angle), float32(cell)*0.32, ink)
		}
		if open {
			click := &s.facingBtns[index]
			gtx := gtx
			gtx.Constraints = layout.Exact(square.Max)
			click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.LabelOp(one.name).Add(gtx.Ops)
				draw(gtx, click.Hovered())
				return layout.Dimensions{Size: square.Max}
			})
		} else {
			draw(gtx, false)
		}
		offset.Pop()
	}

	// The needle: the exact facing, which a step may leave between two ways.
	offset := op.Offset(at(1, 1)).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, colorInputBg, clip.Ellipse(square).Op(gtx.Ops))
	drawArrow(gtx, layout.FPt(square.Max).Mul(0.5), facingVector(angle), float32(cell)*0.38, colorAccent)
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(side, side)}
}

// drawArrow is an arrow through centre along dir, reach long each way: a
// shaft and a head.
func drawArrow(gtx layout.Context, centre, dir f32.Point, reach float32, ink color.NRGBA) {
	tail, tip := centre.Sub(dir.Mul(reach)), centre.Add(dir.Mul(reach*0.35))
	var shaft clip.Path
	shaft.Begin(gtx.Ops)
	shaft.MoveTo(tail)
	shaft.LineTo(tip)
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: shaft.End(), Width: max(reach/5, 1.5)}.Op())
	triangle(gtx, centre.Add(dir.Mul(reach*0.55)), dir, reach*0.5, ink)
}

// facingButtons are the pad's eight ways.
type facingButtons struct {
	facingBtns [len(facings)]widget.Clickable
}
