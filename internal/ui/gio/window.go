package gio

// One frame: a header, the current view, and a footer.
//
// The header says which save is open and switches views; the footer says what
// a commit would write. Both are present on every view, and everything between
// them belongs to whichever view is showing. See views.go for why there are
// three of them.

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// newTheme builds the material theme every widget style derives from. The face
// and the palette are set from Settings immediately afterwards.
func newTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(fontCollection()))
	th.Face = bodyFace
	return th
}

// applyPalette puts the chosen theme's colours in place, on the material
// theme as well, in place: a fresh one would discard the shaper and its
// caches (see applySettings).
func (s *State) applyPalette() {
	usePalette(s.settings.Theme)
	s.th.Palette = material.Palette{
		Bg: colorBg, Fg: colorFg,
		ContrastBg: colorAccentFill, ContrastFg: colorFg,
	}
}

// Layout renders one frame.
func (s *State) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Gio clears to white, so the ground is painted before anything else.
	paint.FillShape(gtx.Ops, colorBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	s.readHelp(gtx)
	s.readPresses(gtx)

	if just, saved := s.consumeJustLoaded(); just {
		// The view is left where the player put it. Opening a save happens in
		// the Characters view now, so jumping to the Editor would take them off
		// the screen they opened it from, mid-gesture - the old Save view had
		// nothing to stay for and this one does.
		//
		// A new save has its own Ashes, so the old selection means nothing.
		// The file just saved has the same ones, so after a save the open Ash,
		// the builder and the chosen character stay. Clearing the clickables
		// keeps them the same length as the list either way.
		if !saved {
			s.selected = noAsh
			// The builder was building a character of the save that was open.
			s.builder = nil
		}
		s.ashBtns = nil
		if loaded := s.LoadedSave(); loaded != nil && !s.kindChosen {
			s.chooseKind(openingKind(loaded.Categories))
		}
		// A commit clears what was staged, and a new save never had any. Either
		// way the composed formations describe a file that is no longer open.
		s.edits = nil
		s.pendingOpen = false
		s.dropBtns = nil
		// The price fields hold the old save's numbers for an Ash that may not
		// even be the same one, so they reload rather than carry across.
		s.priceFor = noPrice
		s.priceErr = ""
		s.family, s.entry = noFamily, noEntry
		s.entryBtns, s.entryDrags = nil, nil
		s.drawnFamilies, s.drawnEntries = nil, nil
		s.filtered = nil
		// A new save has its own ten slots, so the old choice means nothing:
		// the view opens on the save's first character instead.
		if loaded := s.LoadedSave(); !saved && loaded != nil {
			s.person = firstCharacter(loaded.Characters)
		}
		s.personBtns = nil
		s.grantBtns = nil
		// The level choosers answered about the old save's characters. levelFor
		// is what makes the next chosen character reset them, and it is cleared
		// here rather than left equal to a slot number that now names somebody
		// else.
		s.grantLevelSliders = nil
		s.levelFor = noPerson
	}
	// The whole frame sits inside an inset rather than bleeding to the window
	// edge, which is the merchant editor's shape: a panel touching the frame
	// reads as clipped by it rather than as sitting in it.
	dims := layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutHeader(gtx, th)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(horizontalDivider),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return s.layoutView(gtx, th)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return barSurface(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutFooter(gtx, th)
					})
				}),
			)
		})

	// Last, so the dialog composites over everything the frame drew. It is a
	// blocking overlay rather than part of the layout above: see chrome.go's
	// modal for why the panel it replaced was the wrong shape.
	s.layoutPending(gtx, th)
	s.layoutCreature(gtx, th)
	// An explanation whose "?" this frame did not draw - another view, a
	// closed dialog - is let go, so it cannot take the next Esc unseen.
	if !s.helpShown {
		s.helpPinned = noHelp
	}
	// After everything else that answers Esc, so it opens only on an Esc
	// nothing else took.
	s.layoutQuit(gtx, th)
	s.catchPresses(gtx)
	return dims
}

// layoutView draws whichever view is current.
func (s *State) layoutView(gtx layout.Context, th *material.Theme) layout.Dimensions {
	switch s.view {
	case viewEditor:
		return s.layoutEditor(gtx, th)
	case viewSettings:
		return s.layoutSettings(gtx, th)
	default:
		return s.layoutCharacters(gtx, th)
	}
}

// layoutHeader is the application's name and the view switch, and nothing else.
//
// It carried the open file's name for a while, on the reasoning that the Editor
// must never leave any doubt about which save is being changed. The name is now
// where a player looks for it - at the end of the bar that opened it, beside
// Load and Browse, which is where the sibling merchant editor puts it and it is
// right for the same reason: a filename belongs with the controls that change
// it, not beside a title that never changes. The cost is that the Editor and
// Settings do not name the open file; the merchant editor accepts the same
// trade.
func (s *State) layoutHeader(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(title(th, unit.Sp(19), "ER Summoning Editor").Layout),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutTabs(gtx, th)
		}),
	)
}

// layoutFooter carries what a commit would write and the controls that decide
// it, on every view.
//
// Ordered as the merchant editor's: the primary action on the left, what just
// happened in the middle, and the review control on the right. Save leads
// because it is the thing the whole application is for; a player scanning the
// bar finds it in the same place whatever else is on it.
//
// The staged count is folded into Save's own label rather than printed beside
// it. One number in one place - a bar that says "3 pending changes" next to a
// button that writes three changes is saying it twice, and the button is where
// it is actually load-bearing.
func (s *State) layoutFooter(gtx layout.Context, th *material.Theme) layout.Dimensions {
	pending := s.PendingCount()
	working := s.BusyMsg() != ""

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Inert until there is something to write. A Save that opens a
			// dialog and then reports that nothing was staged has wasted two
			// clicks to say what the button could have said by being quiet.
			if working {
				return disabledButton(gtx, th, s.BusyMsg())
			}
			if pending == 0 {
				return disabledButton(gtx, th, "Save file as…")
			}
			if s.saveBtn.Clicked(gtx) {
				s.StartSaveAs()
			}
			return button(th, &s.saveBtn, fmt.Sprintf("Save file as… (%s)", changes(pending))).Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutNotice(gtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Nothing staged is nothing to review, and an inert control says
			// that more quietly than a dialog reading "no pending changes".
			if pending == 0 {
				return disabledButton(gtx, th, "Pending (0)")
			}
			if s.pendingBtn.Clicked(gtx) {
				s.pendingOpen = !s.pendingOpen
			}
			review := quietButton(th, &s.pendingBtn, fmt.Sprintf("Pending (%d)", pending))
			if s.pendingOpen {
				review.Background = colorAccentFill
			} else {
				review.Color = colorGlow
			}
			return review.Layout(gtx)
		}),
	)
}

// changes is the staged count as the Save button says it.
func changes(count int) string {
	if count == 1 {
		return "1 change"
	}
	return fmt.Sprintf("%d changes", count)
}
