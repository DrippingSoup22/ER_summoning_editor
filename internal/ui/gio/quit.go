package gio

// Leaving the editor, and the window it opens in. A full-screen window, which
// Settings offers, has no title bar to close it from, so Esc asks, in a small
// dialog, whether to close the editor.

import (
	"fmt"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// The window the editor opens in when it is not full screen, and the least it
// may be made. The two rails are rigid, so the window's width decides how much
// is left for the spirits catalog; below the minimum it drops to a single
// column of spirits, which is a list rather than a catalog.
const (
	windowWidth, windowHeight       = unit.Dp(1280), unit.Dp(800)
	windowMinWidth, windowMinHeight = unit.Dp(1180), unit.Dp(760)
)

// FullScreen reports whether the editor fills the screen: off by default, on
// every system, until the player turns it on in Settings.
func (s *State) FullScreen() bool { return s.settings.FullScreen }

// WindowOptions are the window's options at creation. Full screen comes after
// Size, because the options apply in order and Size puts the mode back to
// Windowed.
func (s *State) WindowOptions() []app.Option {
	options := []app.Option{
		app.Title("ER Summoning Editor"),
		app.Size(s.scaled(windowWidth), s.scaled(windowHeight)),
		app.MinSize(s.scaled(windowMinWidth), s.scaled(windowMinHeight)),
	}
	if s.FullScreen() {
		options = append(options, app.Fullscreen.Option())
	}
	return options
}

// applyWindowMode puts the window in the mode Settings now says: full screen,
// or a window of the default size, which Size sets.
func (s *State) applyWindowMode() {
	if s.win == nil {
		return
	}
	if s.FullScreen() {
		s.win.Option(app.Fullscreen.Option())
		return
	}
	s.win.Option(app.Size(s.scaled(windowWidth), s.scaled(windowHeight)))
}

// scaled is a window size at the scale the frames are drawn at. Gio sizes a
// window with the system's scale alone, so without this a window drawn at 1.25
// would hold a fifth less of the layout than it was designed for.
func (s *State) scaled(size unit.Dp) unit.Dp {
	if s.windowScale <= 0 {
		return size
	}
	return size * unit.Dp(s.windowScale)
}

// quitWidth is the dialog: a question, what closing loses, and two buttons.
const quitWidth = unit.Dp(440)

// unsaved is what leaving the open save loses, said only when it loses
// something: what is staged and not saved, and a character in the builder not
// yet staged.
func (s *State) unsaved() []string {
	var lost []string
	if count := s.PendingCount(); count > 0 {
		lost = append(lost, fmt.Sprintf("%s staged and not saved.", plural(count, "change")))
	}
	if s.builder != nil && s.builder.draft != s.builder.original {
		lost = append(lost, "The character in the builder is not staged.")
	}
	return lost
}

// askBeforeOpening opens another save at once when nothing would be lost, and
// otherwise asks first: opening one replaces whatever is staged.
func (s *State) askBeforeOpening(open func()) {
	if len(s.unsaved()) == 0 {
		open()
		return
	}
	s.openAfter = open
}

// layoutQuit is the dialog that asks whether to close the editor, or to open
// another save over unsaved work. It is laid out last, so an Esc it reads is
// one nothing else took: a dropdown's list, the creature window, the Pending
// list and a chosen summon let theirs go first, and an open explanation goes
// before the dialog opens. Esc again, Cancel or a click outside it close the
// dialog; its other button does what it asked.
func (s *State) layoutQuit(gtx layout.Context, th *material.Theme) {
	if escaped(gtx) {
		switch {
		case s.openAfter != nil:
			s.openAfter = nil
		case s.quitOpen:
			s.quitOpen = false
		case s.helpPinned != noHelp:
			s.helpPinned = noHelp
		default:
			s.quitOpen = true
		}
	}
	if s.quitCancel.Clicked(gtx) {
		s.quitOpen, s.openAfter = false, nil
	}
	for s.quitScrim.Clicked(gtx) {
		s.quitOpen, s.openAfter = false, nil
	}
	if s.quitClose.Clicked(gtx) {
		switch open := s.openAfter; {
		case open != nil:
			s.openAfter = nil
			open()
		case s.quitOpen:
			s.quitOpen = false
			if s.quit != nil {
				s.quit()
			}
		}
	}
	if !s.quitOpen && s.openAfter == nil {
		return
	}
	lost := s.unsaved()
	question, confirm, lose := "Close the editor?", "Close", "Close without saving"
	if s.openAfter != nil {
		question, confirm, lose = "Open another save?", "Open", "Open without saving"
	}
	modal(gtx, &s.quitScrim, quitWidth, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		children := []layout.FlexChild{
			layout.Rigid(title(th, unit.Sp(18), question).Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		}
		for _, line := range lost {
			warn := muted(th, line)
			warn.Color = colorWarn
			children = append(children, layout.Rigid(warn.Layout))
		}
		if len(lost) > 0 {
			confirm = lose
		}
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, flexSpacer),
					layout.Rigid(rowButton(th, &s.quitCancel, "Cancel").Layout),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(button(th, &s.quitClose, confirm).Layout),
				)
			}),
		)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// closer is how the editor closes its window: through the window, or not at
// all with none (tests).
func closer(win *app.Window) func() {
	return func() { win.Perform(system.ActionClose) }
}
