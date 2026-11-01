package gio

// The creature window: everything about one summon on the grid, opened by
// clicking its token (placement.go). The rest of the window is dimmed under
// it; ×, Esc or a click outside closes it.
//
// It offers only what the summon allows (choices), and says why where it does
// not: a control it does not offer is drawn with no input under it, and a
// padlock and the reason beside it.
//
//   - a row kept as the save holds it, and a creature of the game's own Ashes on
//     its shipped row, keep the role and stance they carry (another role for
//     those needs a second copy of the row, application.ErrAshLevelOwed);
//   - a creature's strength, Helper or Fighter, is its own; a row the game ships
//     takes it row-wide, for every Ash that summons it, and a row kept as the
//     save holds it keeps its own (State.strengthOf);
//   - an enemy plays the Loner, so its role is not the player's to choose, and
//     it never takes the player's target; a person cannot be one yet;
//   - the role decides whether that share is a choice at all
//     (application.LevelShare): never on the Statue or the Loner;
//   - a creature's own rules (application.CreatureRules) decide its look -
//     own colours only where the ghost loses a part of its body, none to
//     choose where it carries a tint of its own - and, for the one creature
//     the author limited, which roles it takes.

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// creatureWidth caps the window: two columns, the roles three to a row in
// the first, each wide enough for its name, and the facing and the switches
// in the second, facingWidth wide.
const (
	creatureWidth = unit.Dp(780)
	facingWidth   = unit.Dp(300)
)

// role is one of the six buttons: its name, and what it does in one line. The
// distances are the ladder's (savecore's ladder profile), past the body.
type role struct {
	level application.Perception
	name  string
	words string
}

// roles are the buttons, from the still to the free.
var roles = [...]role{
	{application.PerceptionStatue, "Statue", "Stands where it is put and does nothing, even when hit."},
	{application.PerceptionFollower, "Follower", "Follows you and fights what you fight; by itself it notices only 2 m past its body."},
	{application.PerceptionGuard, "Guard", "Follows you and fights on its own eyes: 12 m past its body, in a 90° cone."},
	{application.PerceptionSentinel, "Sentinel", "Follows you and spots enemies far ahead: 30 m past its body, in a 45° cone."},
	{application.PerceptionHunter, "Hunter", "Free of you, it hunts: it sees 50 m past its body and smells 25 m all round."},
	{application.PerceptionLoner, "Loner", "Free of you, on its own senses, as you meet it in the world."},
}

// strength is one of the two strength buttons: Fighter or not, its name, and
// what it means in one line.
type strength struct {
	fighter bool
	name    string
	words   string
}

var strengths = [...]strength{
	{false, "Helper", "Built like the game's own Ashes: it deals a quarter of the creature's damage, takes half, " +
		"and has 2.7 times its health."},
	{true, "Fighter", "The creature as you meet it in the world: its own damage, poise and health."},
}

// roleName is a level's button name.
func roleName(level application.Perception) string {
	for _, one := range roles {
		if one.level == level {
			return one.name
		}
	}
	return ""
}

// openCreature opens the window on one slot, which leads the summons picked
// with it: the window is always one creature's.
func (s *State) openCreature(index int) {
	s.creatureSlot = index
	s.lead(index)
	s.creatureRefusal = ""
	s.helpPinned = noHelp
}

// layoutCreature draws the window when it is open, over everything.
func (s *State) layoutCreature(gtx layout.Context, th *material.Theme) {
	if s.creatureSlot == noToken {
		return
	}
	ash := s.selectedAsh(s.LoadedSave())
	index := s.creatureSlot
	if ash == nil || s.view != viewEditor || index < 0 || index >= maxSlots || !s.slotsFor(ash)[index].Filled {
		s.creatureSlot = noToken
		return
	}
	// Esc lets an open explanation go first, then closes the window.
	closing := s.creatureClose.Clicked(gtx) || s.creatureScrim.Clicked(gtx)
	if escaped(gtx) {
		if s.helpPinned != noHelp {
			s.helpPinned = noHelp
		} else {
			closing = true
		}
	}
	if closing {
		s.creatureSlot, s.helpPinned = noToken, noHelp
		return
	}
	slot := s.slotsFor(ash)[index]
	s.readCreature(gtx, ash, index, s.choicesFor(ash, slot, s.session.CreatureRules(slot.authored())))
	slots := s.slotsFor(ash)
	slot = slots[index]
	if !slot.Filled {
		// Removed this frame.
		s.creatureSlot = noToken
		return
	}
	place := s.placeOf(slots, index)
	rules := s.session.CreatureRules(slot.authored())
	o := s.choicesFor(ash, slot, rules)

	modal(gtx, &s.creatureScrim, creatureWidth, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutCreatureHead(gtx, th, slot, place, o)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(horizontalDivider),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutRoles(gtx, th, slot, rules, o)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutStrength(gtx, th, slot, o)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(28)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						width := min(gtx.Dp(facingWidth), gtx.Constraints.Max.X)
						gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutFacing(gtx, th, place, o)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutSwitches(gtx, th, slot, rules, o)
							}),
						)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(horizontalDivider),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutCreatureFoot(gtx, th, o)
			}),
		)
	})
}

// choices is what the window lets the player change on one slot, and why not
// where it does not. The window draws from it, and readCreature acts only on
// what it opens: a control it closes is drawn with no input of its own, so a
// click on it reaches nothing, and were one to arrive it is refused there.
type choices struct {
	// lock is why nothing in the window can change, or "".
	lock string
	// roles are the role buttons that take a click; roleLock is why none
	// does, or "". facing is whether the facing turns, which every summon of
	// an Ash that can be changed does.
	roles    [len(roles)]bool
	roleLock string
	facing   bool
	// fighter is the slot's strength, strength whether it can change, and
	// strengthNote how far a change reaches, or why there is none.
	fighter, strength bool
	strengthNote      string
	// Each switch: whether it can change, and why not in a few words. A row
	// kept as the save holds it carries its team on a row the window does not
	// read, so it shows no enemy switch.
	share, enemy, look             bool
	enemyShown                     bool
	shareNote, enemyNote, lookNote string
}

// choicesFor is what the window offers on one slot of the open Ash.
func (s *State) choicesFor(ash *ashView, slot slotView, rules application.CreatureRules) choices {
	o := choices{lock: ash.lockedNote(), enemyShown: slot.Level != application.PerceptionAsHeld}
	if o.lock != "" {
		// The one lock says it all; each field only shows what it holds.
		o.fighter, _, _ = s.strengthOf(slot, false)
		return o
	}
	o.roleLock, o.facing = slot.roleLocked(), true
	for at, one := range roles {
		o.roles[at] = o.roleLock == "" && rules.TakesLevel(one.level)
	}
	fixed := false
	o.fighter, fixed, o.strengthNote = s.strengthOf(slot, true)
	o.strength = !fixed

	on, choosable := application.ShareOffered(slot.Level)
	switch {
	case slot.Shipped:
		o.shareNote = "as its row carries it"
	case slot.Enemy:
		o.shareNote = "an enemy never does"
	case !choosable && on:
		o.shareNote = "the " + roleName(slot.Level) + " always does"
	case !choosable:
		o.shareNote = "the " + roleName(slot.Level) + " never does"
	default:
		o.share = true
	}
	switch {
	case slot.Shipped:
		o.enemyNote = "not built yet for the game's own Ash creatures"
	case slot.Person != 0:
		// The world branch needs a clone of the row, and a person's shell is
		// a summon-side row already.
		o.enemyNote = "not built yet for a person"
	default:
		o.enemy = o.enemyShown
	}
	o.look, o.lookNote = rules.LookChoosable, rules.LookFixed
	if o.look {
		o.lookNote = ""
	}
	return o
}

// readCreature applies what the window's controls were just asked, and keeps
// a refusal to show in the window, beside what the player is looking at.
func (s *State) readCreature(gtx layout.Context, ash *ashView, index int, o choices) {
	slot := s.slotsFor(ash)[index]
	acted := true
	switch {
	case s.creatureRemove.Clicked(gtx) && o.lock == "":
		s.clearSlot(ash, index)
	case s.strengthBtns[0].Clicked(gtx) && o.strength && o.fighter:
		s.setFighter(ash, index, false)
	case s.strengthBtns[1].Clicked(gtx) && o.strength && !o.fighter:
		s.setFighter(ash, index, true)
	case s.shareBox.Update(gtx) && o.share:
		s.setShare(ash, index, s.shareBox.Value)
	case s.enemyBox.Update(gtx) && o.enemy:
		s.setEnemy(ash, index, s.enemyBox.Value)
	case s.lookBox.Update(gtx) && o.look:
		// A character's look is the body it stands on, so one the save holds
		// changes it too.
		s.setLook(ash, index, s.lookBox.Value)
	default:
		acted = false
		for at, one := range roles {
			if s.roleBtns[at].Clicked(gtx) && o.roles[at] && one.level != slot.Level {
				s.setRole(ash, index, one.level)
				acted = true
			}
		}
		angle := s.placeOf(s.slotsFor(ash), index).Angle
		turned := angle
		for at, one := range facings {
			if s.facingBtns[at].Clicked(gtx) {
				turned = one.angle
			}
		}
		if o.facing && turned != angle {
			s.turnSlot(ash, index, turned)
			acted = true
		}
	}
	if acted {
		s.creatureRefusal, s.helpPinned = "", noHelp
		if text, bad := s.Notice(); bad {
			s.creatureRefusal = text
		}
	}
}

// layoutCreatureHead is the portrait, the name, where it stands and what it
// can do to the player, and ×; under them, when nothing here can change, why.
func (s *State) layoutCreatureHead(gtx layout.Context, th *material.Theme, slot slotView,
	place application.Placement, o choices) layout.Dimensions {
	lines := make([]layout.FlexChild, 0, 4)
	lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		name := title(th, unit.Sp(18), slot.Name)
		name.MaxLines = 2
		return name.Layout(gtx)
	}))
	if slot.Family != "" && slot.Family != slot.Name {
		lines = append(lines, layout.Rigid(muted(th, slot.Family).Layout))
	}
	lines = append(lines, layout.Rigid(muted(th, standing(place)).Layout))
	if why := slot.warning(); why != "" {
		lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			warn := muted(th, "Can hurt you: "+why+".")
			warn.Color = colorWarn
			return warn.Layout(gtx)
		}))
	}
	head := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				side := unit.Dp(64)
				picture, has := s.entityArt.op(s.slotPicture(slot), gtx.Dp(side))
				return imageCell(gtx, side, picture, has && !slot.NoActor)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lines...)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				close := rowButton(th, &s.creatureClose, "×")
				close.TextSize = th.TextSize * 20 / 16
				close.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}
				return close.Layout(gtx)
			}),
		)
	}
	if o.lock == "" {
		return head(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(head),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return lockLine(gtx, th, o.lock) }),
	)
}

// standing says where a summon stands, as a sentence; which way it faces is
// its own field.
func standing(place application.Placement) string {
	where := describePlace(place)
	if where == "your own spot" {
		where = "on " + where
	}
	return "Stands " + where + "."
}

// layoutRoles is the six roles, three to a row, the slot's own lit, and under
// them why some cannot be chosen where they cannot. What a role does is
// behind Role's "?".
func (s *State) layoutRoles(gtx layout.Context, th *material.Theme, slot slotView, rules application.CreatureRules, o choices) layout.Dimensions {
	words := ""
	if o.roleLock == "" && o.lock == "" && rules.Levels != nil {
		words = "Takes only " + roleList(rules.Levels) + "."
	}
	gap := unit.Dp(6)
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.fieldTitle(gtx, th, "Role", helpRole)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := buttonWidth(gtx, gap)
			button := func(at int) layout.FlexChild {
				one := roles[at]
				return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					lit := one.level == slot.shownRole()
					if !o.roles[at] {
						return fixedButton(gtx, th, one.name, lit)
					}
					button := rowButton(th, &s.roleBtns[at], one.name)
					if lit {
						button.Background = colorAccentFill
					}
					return button.Layout(gtx)
				})
			}
			row := func(first int) layout.Widget {
				return func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						button(first), layout.Rigid(layout.Spacer{Width: gap}.Layout),
						button(first+1), layout.Rigid(layout.Spacer{Width: gap}.Layout),
						button(first+2))
				}
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(row(0)),
				layout.Rigid(layout.Spacer{Height: gap}.Layout),
				layout.Rigid(row(3)))
		}),
	}
	if words != "" {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(muted(th, words).Layout))
	}
	if o.roleLock != "" {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return lockLine(gtx, th, o.roleLock) }))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutStrength is the two strengths, the slot's own lit, and under them
// how far a choice reaches, or why there is none. What a strength means is
// behind Strength's "?".
func (s *State) layoutStrength(gtx layout.Context, th *material.Theme, slot slotView, o choices) layout.Dimensions {
	words := ""
	if o.strength {
		words = o.strengthNote
	}
	gap := unit.Dp(6)
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.fieldTitle(gtx, th, "Strength", helpStrength)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// As wide as a role: the two read as the same kind of choice.
			width := buttonWidth(gtx, gap)
			children := make([]layout.FlexChild, 0, 2*len(strengths))
			for at, one := range strengths {
				if at > 0 {
					children = append(children, layout.Rigid(layout.Spacer{Width: gap}.Layout))
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					lit := one.fighter == o.fighter
					if !o.strength {
						return fixedButton(gtx, th, one.name, lit)
					}
					button := rowButton(th, &s.strengthBtns[at], one.name)
					if lit {
						button.Background = colorAccentFill
					}
					return button.Layout(gtx)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
		}),
	}
	if words != "" {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(muted(th, words).Layout))
	}
	if !o.strength && o.strengthNote != "" {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return lockLine(gtx, th, o.strengthNote) }))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// buttonWidth is a role's or a strength's button: a third of the column.
func buttonWidth(gtx layout.Context, gap unit.Dp) int {
	return (gtx.Constraints.Max.X - 2*gtx.Dp(gap)) / 3
}

// fieldTitle is a field's name and its "?", with the explanation under them
// while it is open.
func (s *State) fieldTitle(gtx layout.Context, th *material.Theme, name string, topic helpTopic) layout.Dimensions {
	return s.withHelp(gtx, th, topic, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(title(th, unit.Sp(15), name).Layout),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.helpBadge(gtx, th, topic)
			}),
			layout.Flexed(1, flexSpacer),
		)
	})
}

// strengthOf is whether a slot summons a Fighter, whether that is fixed, and a
// note: how far a choice reaches, or why there is none. A creature the player
// dropped has a clone of its own; any other slot names a row the save holds,
// whose strength the session reports.
func (s *State) strengthOf(slot slotView, editable bool) (fighter, fixed bool, note string) {
	if !slot.Shipped && slot.Person == 0 {
		return slot.Fighter, !editable, ""
	}
	// One of the player's characters stands on the body of its role.
	if fighter, own := application.CharacterFighter(slot.Npc); own && slot.Person != 0 {
		return fighter, !editable, ""
	}
	if s.Busy() {
		// A worker holds the session, so the frame may not ask it anything.
		return false, true, ""
	}
	fighter, choosable := s.session.RowFighter(slot.Npc)
	switch {
	case !choosable && slot.Level == application.PerceptionAsHeld:
		return fighter, true, "Kept as the save holds it."
	case !choosable:
		return fighter, true, "It follows the body it is built on."
	case !editable:
		return fighter, true, ""
	case slot.Person != 0:
		return fighter, false, "Shared: every person built on this body changes too."
	}
	return fighter, false, "Shared: every Ash that summons it changes too."
}

// roleLocked is why a slot's role cannot change, or "".
func (slot slotView) roleLocked() string {
	switch {
	case slot.Level == application.PerceptionAsHeld:
		return "Kept as the save holds it."
	case slot.Shipped:
		return "A shipped Ash creature fights as the Follower."
	case slot.Enemy:
		return "An enemy plays the Loner."
	}
	return ""
}

// shownRole is the button lit for a slot: its own role, which for a row kept
// as the save holds it is none of the six.
func (slot slotView) shownRole() application.Perception {
	if slot.Enemy {
		return application.PerceptionLoner
	}
	return slot.Level
}

// takesTarget is whether the slot shares the player's aggro: what its row
// carries, its choice, or its role's default.
func (slot slotView) takesTarget() bool {
	if slot.Shipped {
		return slot.Stance.DisablePCTargetShare == 0
	}
	switch {
	case slot.Enemy:
		return false
	case slot.Share == application.ShareOn:
		return true
	case slot.Share == application.ShareOff:
		return false
	}
	on, _ := application.ShareOffered(slot.Level)
	return on
}

// layoutSwitches is whether the summon shares the player's aggro, whether it
// is an enemy, and its look, each with its "?" and, where it is fixed, a
// padlock and the reason beside it.
func (s *State) layoutSwitches(gtx layout.Context, th *material.Theme, slot slotView, rules application.CreatureRules, o choices) layout.Dimensions {
	rows := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.switchRow(gtx, th, &s.shareBox, helpShare, slot.takesTarget(), o.share, "Share aggro", o.shareNote)
		}),
	}
	if o.enemyShown {
		rows = append(rows,
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.switchRow(gtx, th, &s.enemyBox, helpEnemy, slot.Enemy, o.enemy, "Enemy", o.enemyNote)
			}))
	}
	rows = append(rows,
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.switchRow(gtx, th, &s.lookBox, helpLook, slot.looks(rules) == application.LookOwnColours, o.look, "Own colours", o.lookNote)
		}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// looks is how the slot is drawn: one of the player's characters by the body
// it stands on, a creature by its own choice where it has one and its
// creature's look otherwise.
func (slot slotView) looks(rules application.CreatureRules) application.Look {
	if look, own := application.CharacterLook(slot.Npc); own && slot.Person != 0 {
		return look
	}
	if slot.Look != application.LookDefault && rules.LookChoosable {
		return slot.Look
	}
	return rules.Look
}

// roleList names roles for a sentence, in the buttons' order: "the Statue
// and the Hunter".
func roleList(levels []application.Perception) string {
	var names []string
	for _, one := range roles {
		if slices.Contains(levels, one.level) {
			names = append(names, "the "+one.name)
		}
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// switchRow is one checkbox and its "?", and, when it cannot change, a
// padlock and the reason under it, with the box itself drawn but not laid
// out as a control.
func (s *State) switchRow(gtx layout.Context, th *material.Theme, box *widget.Bool, topic helpTopic,
	value, open bool, label, note string) layout.Dimensions {
	line := s.withHelp(gtx, th, topic, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !open {
					return fixedCheck(gtx, th, label, value)
				}
				box.Value = value
				style := material.CheckBox(th, box, label)
				style.Color, style.IconColor = colorFg, colorAccent
				return style.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.helpBadge(gtx, th, topic)
			}),
			layout.Flexed(1, flexSpacer),
		)
	})
	if open || note == "" {
		return line
	}
	// Under the label, which starts past the box.
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(layout.Context) layout.Dimensions { return line }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(38), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return lockLine(gtx, th, note)
			})
		}))
}

// fixedButton is a role or strength button that takes no click: recessed,
// as a control that is not available is, and no widget.Clickable is laid out
// under it, so the pointer neither lights it nor reaches it. Lit, it is the
// slot's own, which stays as it is: outlined in the accent rather than filled
// with it, so it does not read as a button.
func fixedButton(gtx layout.Context, th *material.Theme, label string, lit bool) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			name := material.Label(th, th.TextSize*14/16, label)
			name.Color, name.Alignment = colorMuted, text.Middle
			if lit {
				name.Color = colorFg
			}
			return name.Layout(gtx)
		})
	call := macro.Stop()
	corner := gtx.Dp(unit.Dp(4))
	shape := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: corner, SW: corner, NE: corner, NW: corner}
	paint.FillShape(gtx.Ops, colorInputBg, shape.Op(gtx.Ops))
	if lit {
		paint.FillShape(gtx.Ops, colorAccent,
			clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	}
	call.Add(gtx.Ops)
	return dims
}

// fixedCheck is a checkbox that takes no click, drawn where material's would
// be and as large: its box and label recede, and no widget is laid out, so
// nothing reaches it.
func fixedCheck(gtx layout.Context, th *material.Theme, label string, value bool) layout.Dimensions {
	icon := th.Icon.CheckBoxUnchecked
	if value {
		icon = th.Icon.CheckBoxChecked
	}
	const size = unit.Dp(26)
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// material's box sits in a square a third larger, for its glow.
			side := gtx.Dp(size) * 4 / 3
			gtx.Constraints = layout.Exact(image.Pt(side, side))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Pt(gtx.Dp(size), 0)
				icon.Layout(gtx, colorMuted)
				return layout.Dimensions{Size: image.Pt(gtx.Dp(size), gtx.Dp(size))}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				name := material.Label(th, th.TextSize*14/16, label)
				name.Color = colorMuted
				return name.Layout(gtx)
			})
		}),
	)
}

// lockLine is why a field cannot change: a padlock, and the reason in the
// window's own text, so it is read rather than passed over.
func lockLine(gtx layout.Context, th *material.Theme, why string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return padlock(gtx, gtx.Dp(unit.Dp(13)), colorAccent)
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			words := material.Body2(th, why)
			words.Color = colorFg
			return words.Layout(gtx)
		}),
	)
}

// padlock is a small closed padlock, side pixels high: a body, and a shackle
// arched over it.
func padlock(gtx layout.Context, side int, ink color.NRGBA) layout.Dimensions {
	width := side * 13 / 16
	stroke := max(float32(side)/8, 1)
	body := image.Rect(0, side*7/16, width, side)
	corner := max(side/10, 1)
	paint.FillShape(gtx.Ops, ink, clip.RRect{Rect: body, SE: corner, SW: corner, NE: corner, NW: corner}.Op(gtx.Ops))
	// Two quarter circles as cubics, k the usual 0.5523 of the radius.
	left, right := float32(width)*0.22, float32(width)*0.78
	radius, middle := (right-left)/2, float32(width)/2
	foot, crown := float32(body.Min.Y), stroke/2+(right-left)/2
	const k = 0.5523
	var shackle clip.Path
	shackle.Begin(gtx.Ops)
	shackle.MoveTo(f32.Pt(left, foot))
	shackle.LineTo(f32.Pt(left, crown))
	shackle.CubeTo(f32.Pt(left, crown-k*radius), f32.Pt(middle-k*radius, crown-radius), f32.Pt(middle, crown-radius))
	shackle.CubeTo(f32.Pt(middle+k*radius, crown-radius), f32.Pt(right, crown-k*radius), f32.Pt(right, crown))
	shackle.LineTo(f32.Pt(right, foot))
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: shackle.End(), Width: stroke}.Op())
	return layout.Dimensions{Size: image.Pt(width, side)}
}

// layoutCreatureFoot is the last refusal, if the window's last change was
// refused, and the way to take the summon out, where it can be.
func (s *State) layoutCreatureFoot(gtx layout.Context, th *material.Theme, o choices) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if s.creatureRefusal == "" {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
			}
			note := muted(th, s.creatureRefusal)
			note.Color = colorError
			return note.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if o.lock != "" {
				return layout.Dimensions{}
			}
			return rowButton(th, &s.creatureRemove, "Remove from the formation").Layout(gtx)
		}),
	)
}

// setRole gives one slot a role, with its share back on that role's default.
func (s *State) setRole(ash *ashView, index int, level application.Perception) {
	slots := s.slotsFor(ash)
	slots[index].Level, slots[index].Share = level, application.ShareDefault
	s.stage(ash, slots, notice{text: fmt.Sprintf("%s is now the %s.", slots[index].Name, roleName(level))})
}

// setFighter makes one slot a Fighter or a Helper: on its own clone for a
// creature the player dropped, on the body of that role for one of the
// player's characters, and on the row for any other, which every slot
// summoning that row shares.
func (s *State) setFighter(ash *ashView, index int, fighter bool) {
	slots := s.slotsFor(ash)
	words := "a Helper"
	if fighter {
		words = "a Fighter"
	}
	_, character := application.CharacterFighter(slots[index].Npc)
	switch {
	case !slots[index].Shipped && slots[index].Person == 0:
		slots[index].Fighter = fighter
	case character && slots[index].Person != 0:
		slots[index].Npc = application.CharacterBodyAs(slots[index].Npc, fighter)
	default:
		if s.Busy() {
			return
		}
		if err := s.session.SetRowFighter(slots[index].Npc, fighter); err != nil {
			s.setNotice(notice{text: err.Error(), bad: true})
			return
		}
		s.setNotice(notice{text: fmt.Sprintf("%s is now %s, in every Ash.", slots[index].Name, words)})
		return
	}
	s.stage(ash, slots, notice{text: fmt.Sprintf("%s is now %s.", slots[index].Name, words)})
}

// setShare chooses whether one slot shares the player's aggro.
func (s *State) setShare(ash *ashView, index int, on bool) {
	slots := s.slotsFor(ash)
	slots[index].Share = application.ShareOff
	words := "no longer shares your aggro"
	if on {
		slots[index].Share, words = application.ShareOn, "shares your aggro"
	}
	s.stage(ash, slots, notice{text: fmt.Sprintf("%s %s.", slots[index].Name, words)})
}

// setEnemy puts one slot on the enemy team as the Loner, or back on the
// player's side as the Follower.
func (s *State) setEnemy(ash *ashView, index int, enemy bool) {
	slots := s.slotsFor(ash)
	slots[index].Enemy, slots[index].Share = enemy, application.ShareDefault
	slots[index].Level = s.session.CreatureRules(slots[index].authored()).Default
	done := notice{text: fmt.Sprintf("%s is an ally again.", slots[index].Name)}
	if enemy {
		slots[index].Level = application.PerceptionLoner
		done = notice{text: fmt.Sprintf("%s is now an enemy.", slots[index].Name), warn: true}
	}
	s.stage(ash, slots, done)
}

// setLook draws one slot in its own colours, or in the game's ghost - one of
// the player's characters in the Puppets' coat, on the body that wears it.
func (s *State) setLook(ash *ashView, index int, own bool) {
	slots := s.slotsFor(ash)
	slots[index].Look = application.LookGhost
	words := "is drawn in the game's ghost"
	if own {
		slots[index].Look, words = application.LookOwnColours, "is drawn in its own colours"
	}
	if slots[index].Person != 0 {
		slots[index].Npc = application.CharacterBodyIn(slots[index].Npc, slots[index].Look)
		if !own {
			words = "wears the Puppets' coat"
		}
	}
	s.stage(ash, slots, notice{text: fmt.Sprintf("%s %s.", slots[index].Name, words)})
}
