package gio

// An upgrade level on a slider. It replaced the lists of levels, which opened
// below their row and ran off the screen for a row near the window's foot
// (after the sibling merchant editor's weapon level).

import (
	"fmt"
	"math"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// The slider's track, and the room for the level printed beside it: "+25".
const (
	levelTrackWidth  = unit.Dp(90)
	levelLabelWidth  = unit.Dp(30)
	levelSliderWidth = levelTrackWidth + unit.Dp(8) + levelLabelWidth
)

type levelSlider struct {
	float widget.Float
	// moved is set by a press that moved the slider, until it is let go.
	moved bool
}

// Layout draws the slider on level, from +0 to top, with the level beside it,
// and returns the level the player let go on, or -1. While the slider is held
// the label follows it and nothing is chosen: choosing on every step of a drag
// would stage every level it crossed.
func (l *levelSlider) Layout(gtx layout.Context, th *material.Theme, level, top int) (layout.Dimensions, int) {
	if top <= 0 {
		return muted(th, "+0").Layout(gtx), -1
	}
	if l.float.Update(gtx) {
		l.moved = true
	}
	at := int(math.Round(float64(l.float.Value) * float64(top)))
	chose := -1
	if !l.float.Dragging() {
		if l.moved && at != level {
			chose = at
		}
		l.moved = false
		if chose < 0 {
			at = level
		}
		l.float.Value = float32(at) / float32(top)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(levelTrackWidth), gtx.Dp(levelTrackWidth)
			slider := material.Slider(th, &l.float)
			slider.Color = colorAccent
			return slider.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(levelLabelWidth)
			label := material.Body2(th, fmt.Sprintf("+%d", at))
			label.Color = colorFg
			return label.Layout(gtx)
		}),
	), chose
}
