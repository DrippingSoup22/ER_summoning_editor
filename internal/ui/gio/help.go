package gio

// The editor's explanations. The window itself carries none: a thing that
// needs explaining carries a "?" after its name, and they all work one way - a
// click on it opens what the thing means in a bubble under it, until the "?" is
// clicked again, another is, the bubble itself is, or Esc. Pointing at a "?"
// only lights it: nothing opens that the player did not ask for. One bubble
// shows at a time (a clean window, explanations on
// request).
//
// A short line beside a thing says only what state it is in - why it cannot
// change, what it needs, what it risks; what it means is here.

import (
	"image"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// helpTopic is one field the window explains.
type helpTopic int

const (
	helpRole helpTopic = iota
	helpStrength
	helpShare
	helpEnemy
	helpLook
	helpFacing
	helpFullScreen
	helpWideArea
	helpNoRestLimit
	helpAllFree
	helpReset
	helpName
	helpAttributes
	helpNeeds
	helpAshOfWar
	helpGrid
	helpPending
	helpMarks
	helpTopics
	noHelp helpTopic = -1
)

// helpLine is one line of an explanation: a paragraph, or with a lead, an
// entry of a list - a role and what it does - and with a mark, its icon
// before the lead.
type helpLine struct {
	mark        mark
	lead, words string
}

// explanation is a field's title and what it means.
func explanation(topic helpTopic) (string, []helpLine) {
	switch topic {
	case helpRole:
		lines := []helpLine{{words: "How the summon picks its fights: how far it notices enemies by itself, " +
			"and whether it stays with you."}}
		for _, one := range roles {
			lines = append(lines, helpLine{lead: one.name, words: one.words})
		}
		return "Role", lines
	case helpStrength:
		lines := []helpLine{{words: "How hard the summon hits and how much it takes."}}
		for _, one := range strengths {
			lines = append(lines, helpLine{lead: one.name, words: one.words})
		}
		return "Strength", append(lines,
			helpLine{words: "The Ash's upgrade level scales either one: at +10 the creature is as strong as in " +
				"the game's last regions."},
			helpLine{words: "A creature of the game's own Ashes, and a person, use one row for every Ash that " +
				"summons them, so their strength changes everywhere at once."},
			helpLine{words: "One of your characters is chosen slot by slot. At +10 it is as strong as you built " +
				"it; each level below takes a tenth of its health and damage, down to a tenth at +0."})
	case helpShare:
		return "Share aggro", []helpLine{
			{words: "On, the summon shares your aggro: it attacks whatever you hit or lock on to, and any enemy " +
				"fighting you, and stays on it even when something else hits it."},
			{words: "Off, it chooses its own fights with its role's senses, may turn on whatever hits it, and " +
				"lets a target go 8 seconds after losing it."},
			{words: shareRule()},
		}
	case helpEnemy:
		return "Enemy", []helpLine{
			{words: "On, the summon fights against you: it arrives on the enemy's side, hostile to you and to " +
				"your other summons, and plays the Loner - on its own senses, as in the world."},
			{words: "Not built yet for a person, or for the creatures of the game's own Ashes."},
		}
	case helpFacing:
		return "Facing", []helpLine{
			{words: "Which way the summon stands when it arrives, turned from the way you face: up is ahead of " +
				"you, as on the grid."},
			{words: "The eight buttons set the eight ways, the only ones the editor gives, as do the knob on " +
				"the grid and the left and right arrow keys. The needle in the middle points the way it faces " +
				"now, which a formation the save holds may set between two of them."},
		}
	case helpLook:
		return "Own colours", []helpLine{
			{words: "On, the summon is drawn in its own colours, with no spirit coat. Off, it wears the game's " +
				"spirit look, as every Ash does - and one of your characters the coat of the game's Puppets."},
			{words: "Some have no choice: a body the spirit look would draw with a part missing is always in " +
				"its own colours; a creature with a tint of its own shows it whatever the look; the game's own " +
				"people and the creatures of the game's own Ashes arrive as the game draws them."},
		}
	case helpFullScreen:
		return "Full screen", []helpLine{
			{words: "On, the editor fills the screen and has no title bar: Esc asks whether to close it. Off, " +
				"it is a window. Kept for the next start."},
		}
	case helpWideArea:
		return "Wider summoning area", []helpLine{
			{words: "On, an Ash can be summoned farther from a Rebirth Monument: the small areas the game " +
				"draws around them are dropped, and 39 monuments the game places but never switches on are " +
				"woken - near Agheel Lake North, Murkwater Cave, Mistwood Outskirts, the Artist's Shack, " +
				"Seaside Ruins, Saintsbridge, Altus Highway Junction, in Caelid and in the Land of Shadow."},
			{words: "A monument must still be near: how far one reaches is set by the game. Off puts the " +
				"game's areas back and the 39 to sleep."},
		}
	case helpNoRestLimit:
		return "No once-per-rest limit", []helpLine{
			{words: "On, an Ash can be summoned again near the same monument once the last summon is gone, " +
				"without resting at a Site of Grace. Off, the game's limit is back: once per monument until " +
				"you rest."},
		}
	case helpAllFree:
		return "All free", []helpLine{
			{words: "Ticks every Ash's Free box: no Ash costs FP or HP to summon. Off, each is back on its " +
				"own price."},
		}
	case helpReset:
		return "Reset to vanilla", []helpLine{
			{words: "Puts back, as the game ships them, the rows this editor writes: formations, prices, " +
				"roles, the summons it added and the monument switches."},
			{words: "It is staged like any change: nothing is written until you save, and it can be dropped. " +
				"Spirit Ashes given to your characters stay."},
		}
	case helpName:
		return "Name", []helpLine{
			{words: "The editor shows it, to tell your characters apart. In the game a summoned character " +
				"shows no name."},
		}
	case helpAttributes:
		return "Attributes", []helpLine{
			{words: "No attribute goes below the starting class's own: choosing a class raises any that are " +
				"lower."},
			{words: "Spells and skills cost a summoned character no FP; Mind still raises its focus."},
		}
	case helpNeeds:
		return "Requirements", []helpLine{
			{words: "The character is built anyway, but the game may not let it use what it lacks the " +
				"attributes for."},
		}
	case helpAshOfWar:
		return "Ash of War", []helpLine{
			{words: "Choose turns the list on the right to the Ashes of War that go on this weapon, and only " +
				"those. The weapon's affinity follows the Ash."},
			{words: "A weapon with a skill of its own takes none."},
		}
	case helpGrid:
		return "Grid", []helpLine{
			{words: "Up is the way you face, and the triangle is you."},
			{lead: "Mouse", words: "Drag a spirit from the catalog onto the grid; a whole card brings the first " +
				"of each of its entries, and an entry all its variants, into the empty slots. Drag a summon to move it, or " +
				"off the grid to remove it, and its knob to turn it. Right-click a summon for its options."},
			{lead: "Several", words: "Ctrl-click summons to pick them together, or hold Shift and pass the pointer " +
				"over them: dragging or turning one moves or turns them all, and dragging one off the grid removes " +
				"them all. Ctrl-click a picked one to let it go, or click the ground to let all go. A right-click " +
				"still opens one."},
			{lead: "View", words: "Drag the ground to look around, and scroll to zoom."},
			{lead: "Cells", words: "A summon covers its own cell and every cell its body fills half of, seen from above. " +
				"Yellow: two bodies so near they may arrive inside each other."},
			{lead: "Sight", words: "With Show sight on, click a summon to see what it notices by itself: bright " +
				"where its eyes see ahead, faint all round where only its nose notices what comes close."},
			{lead: "Touchpad", words: "Tap, then tap again and drag."},
			{words: "A formation that cannot be changed is shown as the save holds it."},
		}
	case helpPending:
		return "Pending changes", []helpLine{
			{words: "Nothing here is written until you save a copy of the save. Drop takes a change back."},
			{words: "Reset to vanilla does not undo the Spirit Ashes given to your characters."},
		}
	case helpMarks:
		return "Labels", markLegend
	}
	return "", nil
}

// shareRule is which roles share aggro, read from what the window offers, so
// it stays true whatever the roles are given.
func shareRule() string {
	var always, never, choice []string
	for _, one := range roles {
		on, choosable := application.ShareOffered(one.level)
		switch {
		case choosable:
			choice = append(choice, "the "+one.name)
		case on:
			always = append(always, "the "+one.name)
		default:
			never = append(never, "the "+one.name)
		}
	}
	join := func(names []string, last string) string {
		if len(names) < 2 {
			return strings.Join(names, "")
		}
		return strings.Join(names[:len(names)-1], ", ") + " " + last + " " + names[len(names)-1]
	}
	parts := []string{}
	if len(always) > 0 {
		parts = append(parts, "Always on for "+join(always, "and")+", which notices almost nothing by itself.")
	}
	parts = append(parts, "Never for "+join(append(never, "an enemy"), "or")+".")
	if len(choice) > 0 {
		parts = append(parts, "Your choice for "+join(choice, "and")+", off at first.")
	}
	return strings.Join(parts, " ")
}

// readHelp takes the "?" clicks and a click on the open bubble: a "?" keeps
// its bubble open or lets it go, and the bubble closes itself. Once a frame,
// before anything is drawn.
func (s *State) readHelp(gtx layout.Context) {
	s.helpShown = false
	for topic := range s.helpBtns {
		if s.helpBtns[topic].Clicked(gtx) {
			if s.helpPinned == helpTopic(topic) {
				s.helpPinned = noHelp
			} else {
				s.helpPinned = helpTopic(topic)
			}
		}
	}
	for {
		got, ok := gtx.Event(pointer.Filter{Target: &s.helpTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, isPointer := got.(pointer.Event); isPointer {
			s.helpPinned = noHelp
		}
	}
}

// helpBadge is a field's "?": a small ring, lit under the pointer and filled
// while its bubble is open.
func (s *State) helpBadge(gtx layout.Context, th *material.Theme, topic helpTopic) layout.Dimensions {
	click := &s.helpBtns[topic]
	side := gtx.Dp(unit.Dp(18))
	gtx.Constraints = layout.Exact(image.Pt(side, side))
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		ring := image.Rectangle{Max: image.Pt(side, side)}
		ink := colorMuted
		switch {
		case s.helpPinned == topic:
			paint.FillShape(gtx.Ops, colorAccentFill, clip.Ellipse(ring).Op(gtx.Ops))
			ink = colorFg
		case click.Hovered():
			ink = colorAccent
		}
		paint.FillShape(gtx.Ops, ink,
			clip.Stroke{Path: clip.Ellipse(ring).Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1.2)))}.Op())
		pointer.CursorPointer.Add(gtx.Ops)
		mark := material.Caption(th, "?")
		mark.Color, mark.Alignment = ink, text.Middle
		layout.Center.Layout(gtx, mark.Layout)
		return layout.Dimensions{Size: ring.Max}
	})
}

// withHelp lays a field out and, while its explanation shows, the bubble just
// under it, over whatever follows, as wide as the field.
func (s *State) withHelp(gtx layout.Context, th *material.Theme, topic helpTopic, field layout.Widget) layout.Dimensions {
	dims := field(gtx)
	if s.helpPinned != topic {
		return dims
	}
	s.helpShown = true
	macro := op.Record(gtx.Ops)
	offset := op.Offset(image.Pt(0, dims.Size.Y+gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
	bubble := gtx
	bubble.Constraints = layout.Constraints{Min: image.Pt(gtx.Constraints.Max.X, 0), Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(600)))}
	s.layoutHelp(bubble, th, topic)
	offset.Pop()
	op.Defer(gtx.Ops, macro.Stop())
	return dims
}

// layoutHelp is one bubble: a raised panel with an accent edge, the field's
// name, and what it means. A press on it closes it, and goes no further, so a
// control under it is never clicked through it.
func (s *State) layoutHelp(gtx layout.Context, th *material.Theme, topic helpTopic) layout.Dimensions {
	title, lines := explanation(topic)
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(14)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				name := material.Body1(th, title)
				name.Color = colorGlow
				return name.Layout(gtx)
			})}
			for _, line := range lines {
				children = append(children,
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						words := material.Body2(th, line.words)
						words.Color = colorFg
						if line.lead == "" {
							return words.Layout(gtx)
						}
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if line.mark == 0 {
									return layout.Dimensions{}
								}
								dims := drawMark(gtx, th, line.mark)
								dims.Size.X = gtx.Dp(unit.Dp(32))
								return dims
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Dp(unit.Dp(76))
								lead := material.Body2(th, line.lead)
								lead.Color = colorAccent
								return lead.Layout(gtx)
							}),
							layout.Flexed(1, words.Layout),
						)
					}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	call := macro.Stop()

	rect := image.Rectangle{Max: dims.Size}
	corner := gtx.Dp(unit.Dp(6))
	shape := clip.RRect{Rect: rect, SE: corner, SW: corner, NE: corner, NW: corner}
	paint.FillShape(gtx.Ops, colorControl, shape.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, colorSlotEdge,
		clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	edge := clip.RRect{Rect: image.Rect(0, 0, gtx.Dp(unit.Dp(4)), rect.Max.Y), NW: corner, SW: corner}
	paint.FillShape(gtx.Ops, colorAccent, edge.Op(gtx.Ops))
	call.Add(gtx.Ops)
	area := clip.Rect(rect).Push(gtx.Ops)
	event.Op(gtx.Ops, &s.helpTag)
	area.Pop()
	return dims
}
