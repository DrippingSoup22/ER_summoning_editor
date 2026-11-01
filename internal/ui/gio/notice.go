package gio

// The message line: the editor answering what was just done.
//
// One line in the middle of the footer, short - what happened, not why; the
// "?"s say why - and marked by kind: a dot in the accent for news, in ember
// for a warning, in red for a refusal. News fades after a few seconds, so
// nothing stale is left on screen; a warning or a refusal stays until the
// next press anywhere in the window, so it cannot fade before it is read. While a dialog is open the line is drawn over its
// scrim, so what a change made in the dialog did stays readable.

import (
	"image"
	"image/color"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const (
	// noticeLife is how long news stays, the last noticeFade of it fading.
	noticeLife = 6 * time.Second
	noticeFade = time.Second
	// noticeSize is a size up from body text, so the one line the editor
	// answers in is not the quietest thing in the window.
	noticeSize = unit.Sp(16)
)

// currentNotice is the message and its number, which tells a new message from
// the same one said again.
func (s *State) currentNotice() (notice, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notice, s.noticeSeq
}

// readPresses settles a warning or a refusal on the first press after it.
// Read before anything is drawn, so the press that leads to a new message
// cannot settle that one.
func (s *State) readPresses(gtx layout.Context) {
	pressed := false
	for {
		got, ok := gtx.Event(pointer.Filter{Target: &s.pressTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, isPointer := got.(pointer.Event); isPointer {
			pressed = true
		}
	}
	if message, seq := s.currentNotice(); pressed && (message.bad || message.warn) {
		s.noticeSettled = seq
	}
}

// catchPresses lays the catcher readPresses reads: the whole window, deferred
// so it is above every dialog, and passing every press on to what is under it.
func (s *State) catchPresses(gtx layout.Context) {
	macro := op.Record(gtx.Ops)
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.pressTag)
	pass.Pop()
	area.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}

// dialogOpen is whether a dialog dims the window.
func (s *State) dialogOpen() bool {
	return s.pendingOpen || s.creatureSlot != noToken || s.quitOpen || s.openAfter != nil
}

// layoutNotice is the line, centred in the width it is given.
func (s *State) layoutNotice(gtx layout.Context, th *material.Theme) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	nothing := layout.Dimensions{Size: gtx.Constraints.Min}
	message, seq := s.currentNotice()
	if seq != s.noticeShown {
		s.noticeShown, s.noticeSince = seq, gtx.Now
	}
	if message.text == "" || seq == s.noticeSettled {
		return nothing
	}
	ink, mark, strength := colorFg, colorAccent, float32(1)
	switch {
	case message.bad:
		ink, mark = colorError, colorError
	case message.warn:
		// A warning is not a refusal and is not coloured like one: ember, as
		// a warned slot is drawn.
		ink, mark = colorWarn, colorWarn
	default:
		age := gtx.Now.Sub(s.noticeSince)
		switch {
		case age >= noticeLife:
			return nothing
		case age >= noticeLife-noticeFade:
			strength = float32(noticeLife-age) / float32(noticeFade)
			gtx.Execute(op.InvalidateCmd{})
		default:
			gtx.Execute(op.InvalidateCmd{At: s.noticeSince.Add(noticeLife - noticeFade)})
		}
	}
	ink.A, mark.A = uint8(float32(ink.A)*strength), uint8(float32(mark.A)*strength)
	line := func(gtx layout.Context) layout.Dimensions {
		words := title(th, noticeSize, message.text)
		words.Color, words.MaxLines = ink, 1
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, flexSpacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return noticeMark(gtx, mark) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Rigid(words.Layout),
			layout.Flexed(1, flexSpacer),
		)
	}
	if !s.dialogOpen() {
		return line(gtx)
	}
	// Deferred twice: a dialog is deferred once, and a deferral inside a
	// deferred macro runs after every one of those, so the line is drawn over
	// the scrim, where it stands in the footer.
	macro := op.Record(gtx.Ops)
	dims := line(gtx)
	call := macro.Stop()
	outer := op.Record(gtx.Ops)
	op.Defer(gtx.Ops, call)
	op.Defer(gtx.Ops, outer.Stop())
	return dims
}

// noticeMark is the dot that says what kind of message the line is.
func noticeMark(gtx layout.Context, ink color.NRGBA) layout.Dimensions {
	side := gtx.Dp(unit.Dp(8))
	paint.FillShape(gtx.Ops, ink, clip.Ellipse(image.Rectangle{Max: image.Pt(side, side)}).Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(side, side)}
}
