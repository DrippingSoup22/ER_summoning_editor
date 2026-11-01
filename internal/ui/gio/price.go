package gio

// What a summon costs.
//
// Two fields, and everything interesting about them is in when they stage.
// There is no Apply button: a price is one number a player types and then
// stops thinking about, and a field whose value is only real after a second
// click is a field that quietly loses edits. So every keystroke that leaves
// both fields readable stages the result, and StageCost replacing rather than
// accumulating is what makes that safe - typing "150" stages 1, then 15, then
// 150, and the session holds one price throughout.
//
// Typing the save's own price back is not an edit. It unstages instead, so a
// player who changes their mind by retyping the original number ends with
// nothing pending rather than with a change that writes the same bytes.
//
// Free is the third control: ticked, the Ash costs nothing (FP 0, HP -1, the
// game's own spelling of free) and the two fields only show it; unticked, the
// Ash is back on its own price. It is ticked whenever the Ash will cost
// nothing, however it came to, so a 0 typed in both fields ticks it too.
// Settings' All free ticks every Ash's box at once, or unticks them all when
// all are ticked (setAllFree).

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// noPrice is the goods family the fields hold when they hold none. Real
// families are positive, so this cannot collide with one.
const noPrice = int32(-1)

// layoutPrice is the container's cost: the two fields and Free, side by side
// in the options over the grid. What they have to say - the costs differ
// between levels, or what was typed cannot be staged - is priceNote's, shown
// under the Ash's name. While a worker holds the session nothing here can be
// asked of it, so the three only show what they last held.
func (s *State) layoutPrice(gtx layout.Context, th *material.Theme, ash *ashView) layout.Dimensions {
	busy := s.Busy()
	if !busy {
		s.loadPrice(ash)
		changed := false
		for _, editor := range []*widget.Editor{&s.fpField, &s.hpField} {
			for {
				event, ok := editor.Update(gtx)
				if !ok {
					break
				}
				switch event.(type) {
				case widget.ChangeEvent, widget.SubmitEvent:
					changed = true
				}
			}
		}
		if changed {
			s.applyPrice(ash)
		}
		if s.freeBox.Update(gtx) {
			s.setFree(ash, s.freeBox.Value)
		}
		s.freeBox.Value = s.isFree(ash)
		selectOnFocus(gtx, &s.priceKeys, &s.fpField, &s.hpField)
	}
	open := !busy && !s.freeBox.Value
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return priceField(gtx, th, &s.fpField, "FP", open)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return priceField(gtx, th, &s.hpField, "HP", open)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if busy {
				return fixedCheck(gtx, th, "Free", s.freeBox.Value)
			}
			box := material.CheckBox(th, &s.freeBox, "Free")
			box.Color, box.IconColor = colorFg, colorAccent
			return box.Layout(gtx)
		}),
	)
}

// priceOf is what an Ash will cost: the price staged for it, else the
// save's, which varies when its upgrade levels disagree. The session must be
// free to ask.
func (s *State) priceOf(ash *ashView) (price application.Price, varies bool) {
	if staged, ok := s.session.StagedPrice(ash.GoodsFamily); ok {
		return staged, false
	}
	return ash.Price, ash.PriceVaries
}

// isFree is whether an Ash will cost nothing: its Free box.
func (s *State) isFree(ash *ashView) bool {
	price, varies := s.priceOf(ash)
	return !varies && price.Free()
}

// setFree makes an Ash cost nothing, or puts it back on its own price - which
// for the one family the game ships free is free still.
func (s *State) setFree(ash *ashView, free bool) {
	if s.Busy() {
		return
	}
	s.priceErr = ""
	switch {
	case !free || !ash.PriceVaries && ash.Price.Free():
		if s.session.UnstageCost(ash.GoodsFamily) {
			s.setNotice(notice{text: fmt.Sprintf("%s is back to its own price.", ash.Name)})
		}
	default:
		if _, err := s.session.StageCost(strconv.Itoa(int(ash.GoodsFamily)), application.FreePrice); err != nil {
			s.setNotice(notice{text: err.Error(), bad: true})
			return
		}
		s.setNotice(notice{text: fmt.Sprintf("%s is free.", ash.Name)})
	}
	s.fillPrice(ash)
}

// allFree is whether every Ash of the open save will cost nothing: Settings'
// All free, which is on exactly when every Ash's Free box is ticked.
func (s *State) allFree() bool {
	loaded := s.LoadedSave()
	if loaded == nil || s.Busy() {
		return false
	}
	for index := range loaded.Ashes {
		if !s.isFree(&loaded.Ashes[index]) {
			return false
		}
	}
	return true
}

// setAllFree ticks every Ash's Free box - the ones not ticked yet; those that
// are stay as they are - or unticks them all, every Ash back on its own
// price, whatever was typed for it.
func (s *State) setAllFree(free bool) {
	loaded := s.LoadedSave()
	if loaded == nil || s.Busy() {
		return
	}
	count := 0
	for index := range loaded.Ashes {
		ash := &loaded.Ashes[index]
		switch {
		case free && !s.isFree(ash):
			if _, err := s.session.StageCost(strconv.Itoa(int(ash.GoodsFamily)), application.FreePrice); err != nil {
				s.setNotice(notice{text: err.Error(), bad: true})
				return
			}
			count++
		case !free && s.session.UnstageCost(ash.GoodsFamily):
			count++
		}
	}
	s.priceErr, s.priceFor = "", noPrice
	if free {
		s.setNotice(notice{text: fmt.Sprintf("Every Ash is free: %d changed.", count)})
		return
	}
	s.setNotice(notice{text: fmt.Sprintf("Every Ash is back to its own price: %d changed.", count)})
}

// priceNote is what the cost fields have to say, if anything: a refusal of
// what was typed, or that the eleven upgrade levels disagree - one number
// standing for eleven different ones would be the panel inventing a fact, so
// the fields start empty and a price typed applies to all of them.
func (s *State) priceNote(ash *ashView) (string, bool) {
	switch {
	case s.priceErr != "":
		return s.priceErr, true
	case ash.PriceVaries:
		return "Costs differ by level; a price here sets them all.", false
	}
	return "", false
}

// priceField is one labelled box: the label beside a field wide enough for
// the largest cost. Closed, it shows what it holds and is not a field: no
// editor is laid out, so it takes neither a click nor the keys.
func priceField(gtx layout.Context, th *material.Theme, editor *widget.Editor, label string, open bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(muted(th, label).Layout),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width, height := gtx.Dp(unit.Dp(72)), gtx.Dp(unit.Dp(30))
			gtx.Constraints = layout.Exact(image.Pt(width, height))
			if !open {
				return box(gtx, colorInputBg, colorInputBg, unit.Dp(4), func(gtx layout.Context) layout.Dimensions {
					return muted(th, editor.Text()).Layout(gtx)
				})
			}
			return box(gtx, colorInputBg, colorDivider, unit.Dp(4), func(gtx layout.Context) layout.Dimensions {
				style := material.Editor(th, editor, "0")
				style.Color = colorFg
				style.HintColor = colorMuted
				return style.Layout(gtx)
			})
		}),
	)
}

// loadPrice fills the fields when the player moves to another Ash, and leaves
// them alone otherwise - reloading on every frame would undo what is being
// typed.
func (s *State) loadPrice(ash *ashView) {
	if s.priceFor == ash.GoodsFamily {
		return
	}
	s.priceFor = ash.GoodsFamily
	s.priceErr = ""
	s.fillPrice(ash)
}

// fillPrice puts what the Ash will cost in the fields: the price staged for
// it, else the save's, empty where the save's levels disagree. What is held,
// including the -1 the game spells "free" with: the core normalises 0 and -1
// to one another on write, and showing something tidier than the file would
// be showing a value that is not in it.
func (s *State) fillPrice(ash *ashView) {
	price, varies := s.priceOf(ash)
	if varies {
		s.fpField.SetText("")
		s.hpField.SetText("")
		return
	}
	// -1 is the game's spelling of "none" (a price the Ash does not pay), so
	// it shows as 0; applyPrice writes it back unchanged.
	s.fpField.SetText(strconv.FormatInt(int64(max(price.FP, 0)), 10))
	s.hpField.SetText(strconv.FormatInt(int64(max(price.HP, 0)), 10))
}

// keepNone is a typed price with each 0 the save spells -1 left as -1, so a
// field shown as 0 and not changed writes nothing new.
func keepNone(typed, held application.Price) application.Price {
	if typed.FP == 0 && held.FP < 0 {
		typed.FP = held.FP
	}
	if typed.HP == 0 && held.HP < 0 {
		typed.HP = held.HP
	}
	return typed
}

// applyPrice stages what the fields now say, or explains why it cannot.
func (s *State) applyPrice(ash *ashView) {
	if s.Busy() {
		return
	}
	fp := strings.TrimSpace(s.fpField.Text())
	hp := strings.TrimSpace(s.hpField.Text())
	if fp == "" || hp == "" {
		// Mid-edit rather than wrong: a player clearing a field to retype it
		// has not made a mistake yet, so nothing is said and nothing stages.
		s.priceErr = ""
		return
	}
	fpValue, fpErr := strconv.ParseInt(fp, 10, 32)
	hpValue, hpErr := strconv.ParseInt(hp, 10, 32)
	if fpErr != nil || hpErr != nil {
		s.priceErr = "FP and HP have to be whole numbers."
		return
	}
	price := application.Price{FP: int32(fpValue), HP: int32(hpValue)}
	if !ash.PriceVaries {
		price = keepNone(price, ash.Price)
	}
	if err := price.Validate(); err != nil {
		s.priceErr = err.Error()
		return
	}
	s.priceErr = ""

	// The same price as the save's is not a change. Comparing Free to Free as
	// well as field to field is what makes 0/0 and 0/-1 count as the same
	// answer, which is what the core does when it writes them.
	if !ash.PriceVaries && (price == ash.Price || (price.Free() && ash.Price.Free())) {
		if s.session.UnstageCost(ash.GoodsFamily) {
			s.setNotice(notice{text: fmt.Sprintf("%s is back to its own price.", ash.Name)})
		}
		return
	}
	if _, err := s.session.StageCost(strconv.Itoa(int(ash.GoodsFamily)), price); err != nil {
		s.priceErr = err.Error()
		return
	}
	s.setNotice(notice{text: fmt.Sprintf("%s will %s.", ash.Name, priceWords(price))})
}

// priceWords is a price as a sentence, used by the footer's pending list as
// well as here, so the two describe one edit the same way.
func priceWords(price application.Price) string {
	if price.Free() {
		return "cost nothing"
	}
	if price.HP > 0 {
		return fmt.Sprintf("cost %d FP and %d HP", price.FP, price.HP)
	}
	return fmt.Sprintf("cost %d FP", price.FP)
}
