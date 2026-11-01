package gio

// What a drop does.
//
// The rule underneath is that StageFormation replaces: it takes the whole
// formation, not a slot. So every edit here composes the ten as they should
// end up and stages that, and there is deliberately no incremental slot API to
// build against.
//
// The composed ten live in edits, which is frame state, beside the session's
// own staged set. Two copies of the same fact is a smell worth naming: the
// session holds what will be written and this holds what to draw, because the
// snapshot the frame draws is built by a worker and must not be mutated. They
// are written in one place - stage, below - and dropped together.
//
// The frame calls the session here, which the concurrency rule allows only
// while no worker holds it. Staging is a memory operation on data already
// read, so it is done in the frame rather than handed to a worker; a drop that
// took a goroutine and a repaint to appear would feel broken.

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// formationEdit is one Ash's ten slots as the player has composed them.
type formationEdit struct {
	Slots [maxSlots]slotView
}

// slotsFor is what the panel draws: the composed formation when there is one,
// and what the save ships otherwise.
func (s *State) slotsFor(ash *ashView) [maxSlots]slotView {
	if edit, editing := s.edits[ash.GoodsFamily]; editing {
		return edit.Slots
	}
	return ash.Slots
}

// drop puts the dragged actor in one slot and stages the result.
func (s *State) drop(ash *ashView, index int, slots [maxSlots]slotView, payload string) {
	filled, ok := s.arriving(payload, slots[index])
	if !ok {
		// A payload this application did not write. Nothing to do and nothing
		// worth telling the player, who dragged something from elsewhere.
		return
	}
	slots[index] = filled
	done := notice{text: fmt.Sprintf("%s added to %s.", filled.Name, ash.Name)}
	if why := filled.warning(); why != "" {
		// The warning replaces the confirmation rather than following it. The
		// footer is one line; the drop is already visible in the slot it
		// landed in; and of the two things that could be said here only one is
		// news. The slot goes on saying it after this line is gone - see
		// slotCell, which draws a warned slot in the warn colour and carries
		// the whole of it in the hover.
		done = notice{warn: true, text: fmt.Sprintf("%s can hurt you: %s.", filled.Name, why)}
	}
	s.stage(ash, slots, done)
}

// arriving is the slot one dragged actor fills, standing where at stood: the
// cell a grid drop landed on, or the place of whatever it replaces.
func (s *State) arriving(payload string, at slotView) (slotView, bool) {
	npc, think, person, ok := parseSpirit(payload)
	if !ok {
		return slotView{}, false
	}
	filled := slotView{Filled: true, Npc: npc, Think: think, Person: person, Name: fmt.Sprintf("Actor %d", npc),
		Place: at.Place, Placed: at.Placed}
	if loaded := s.LoadedSave(); loaded != nil {
		// The hierarchy names what was dragged, person or creature; the actor
		// labels name a creature row the hierarchy does not hold (a shipped
		// formation's own clone) and are the fallback.
		if known, variant, ok := loaded.draggedVariant(npc, think, person); ok {
			filled.Name, filled.Family, filled.Model = known.Name, known.Family, known.Model
			if variant.Shipped {
				// An Ashes card: FromSoftware's row is the Follower, on its
				// Ash's own stance.
				filled.Shipped, filled.Stance = true, variant.Stance
				filled.Level, filled.Share = application.PerceptionFollower, application.AshShare(variant.Stance)
			}
		} else if known, ok := loaded.labels[npc]; ok {
			filled.Name, filled.Family, filled.Model = known.Name, known.Family, known.Model
		}
	}
	if !filled.Shipped {
		// A creature the author limited to some roles arrives on its own
		// default (the large Shadow Commoner: the Hunter).
		filled.Level = s.session.CreatureRules(filled.authored()).Default
	}
	return filled, true
}

// dropAt puts the dragged actor on the grid at place, stages the result and
// chooses it, so its knob is at hand: in the slot of the summon standing
// there, which it replaces - two never share a cell - or else in the first
// empty slot. An Ash holds ten at most, as the game's do. A card or an entry
// drags several (payloadOf), which dropMany places.
func (s *State) dropAt(ash *ashView, place application.Placement, payload string) {
	if several := strings.Split(payload, ";"); len(several) > 1 {
		s.dropMany(ash, place, several)
		return
	}
	slots := s.slotsFor(ash)
	free := standingAt(slots, place.X, place.Z, -1)
	for index, slot := range slots {
		if free < 0 && !slot.Filled {
			free = index
			break
		}
	}
	if free < 0 {
		s.setNotice(notice{text: ash.Name + " already summons ten: remove one first.", bad: true})
		return
	}
	slots[free].Place, slots[free].Placed = place, true
	s.drop(ash, free, slots, payload)
	s.pick(free, 0)
}

// dropMany places what a card or an entry drags, in its order, in the Ash's
// empty slots and no others: a whole card replaces nobody, and what does not
// fit in the ten is left out and counted. The first stands nearest the cell
// it was dropped on, and each next one on the nearest free cell where its
// body keeps clear of every body already standing (clearCell), facing the
// way the player faces.
func (s *State) dropMany(ash *ashView, place application.Placement, payloads []string) {
	slots := s.slotsFor(ash)
	var free []int
	for index, slot := range slots {
		if !slot.Filled {
			free = append(free, index)
		}
	}
	if len(free) == 0 {
		s.setNotice(notice{text: ash.Name + " already summons ten: remove one first.", bad: true})
		return
	}
	var bodies map[string]application.Body
	if loaded := s.LoadedSave(); loaded != nil {
		bodies = loaded.bodies
	}
	bodyOf := func(slot slotView) application.Body {
		if body, known := bodies[slot.Model]; known {
			return body
		}
		return personBody
	}
	places := cellPlaces(slots)
	taken := map[int]bool{cellOf(0, 0): true}
	standing := []capsule{onTheGround(personBody, application.Placement{})}
	for index, slot := range slots {
		if slot.Filled && onGround(places[index].X, places[index].Z) {
			taken[cellOf(places[index].X, places[index].Z)] = true
			if !slot.NoActor {
				standing = append(standing, onTheGround(bodyOf(slot), places[index]))
			}
		}
	}
	added, offered := 0, 0
	var first slotView
	var hurts, why string
	for _, one := range payloads {
		filled, ok := s.arriving(one, slotView{})
		if !ok {
			continue
		}
		offered++
		if added == len(free) {
			continue
		}
		body := bodyOf(filled)
		x, z := clearCell(body, place.X, place.Z, taken, standing)
		filled.Place, filled.Placed = application.Placement{X: x, Z: z}, true
		taken[cellOf(x, z)] = true
		standing = append(standing, onTheGround(body, filled.Place))
		slots[free[added]] = filled
		if added == 0 {
			first = filled
		}
		if warning := filled.warning(); hurts == "" && warning != "" {
			hurts, why = filled.Name, warning
		}
		added++
	}
	if added == 0 {
		return
	}
	text := fmt.Sprintf("%s added to %s.", first.Name, ash.Name)
	if added > 1 {
		text = fmt.Sprintf("%s and %d more added to %s.", first.Name, added-1, ash.Name)
	}
	if added < offered {
		text = fmt.Sprintf("%d of %d added to %s: it summons ten.", added, offered, ash.Name)
	}
	done := notice{text: text}
	if hurts != "" {
		done = notice{warn: true, text: fmt.Sprintf("%s %s can hurt you: %s.", text, hurts, why)}
	}
	s.stage(ash, slots, done)
	s.pick(free[0], 0)
}

// clearCell is the free cell nearest x, z where a body standing, facing ahead,
// keeps the clearance from every body in standing; nearer cells first, and
// among equally near ones the one further ahead, then further left. A row
// ahead or behind counts twice a cell aside, so the card stands in ranks:
// long bodies (a Crucible Knight's tail reaches 4.8 m behind it) shoulder to
// shoulder, small ones a block wider than deep. Where the whole ground has
// none, the nearest free cell, and the grid's tint shows the crowding.
func clearCell(body application.Body, x, z float64, taken map[int]bool, standing []capsule) (float64, float64) {
	x, z = math.Round(min(max(x, -groundReach), groundReach)), math.Round(min(max(z, -groundReach), groundReach))
	type cell struct{ x, z, far float64 }
	var cells []cell
	for cx := -groundReach; cx <= groundReach; cx++ {
		for cz := -groundReach; cz <= groundReach; cz++ {
			if !taken[cellOf(float64(cx), float64(cz))] {
				cells = append(cells, cell{float64(cx), float64(cz), math.Hypot(float64(cx)-x, 2*(float64(cz)-z))})
			}
		}
	}
	slices.SortFunc(cells, func(a, b cell) int {
		if a.far != b.far {
			return cmp.Compare(a.far, b.far)
		}
		if a.z != b.z {
			return cmp.Compare(b.z, a.z)
		}
		return cmp.Compare(a.x, b.x)
	})
	for _, c := range cells {
		one := onTheGround(body, application.Placement{X: c.x, Z: c.z})
		if !slices.ContainsFunc(standing, func(other capsule) bool { return one.gap(other) < clearance }) {
			return c.x, c.z
		}
	}
	if len(cells) > 0 {
		return cells[0].x, cells[0].z
	}
	return x, z
}

// moveSlot stands one slot somewhere else on the grid, keeping its facing. A
// summon already standing there leaves the formation: two never share a cell.
func (s *State) moveSlot(ash *ashView, index int, x, z float64) {
	slots := s.slotsFor(ash)
	place := s.placeOf(slots, index)
	place.X, place.Z = x, z
	done := notice{text: fmt.Sprintf("%s stands at %s.", slots[index].Name, describePlace(place))}
	if other := standingAt(slots, x, z, index); other >= 0 {
		done = notice{text: fmt.Sprintf("%s takes %s's place, at %s.", slots[index].Name, slots[other].Name, describePlace(place))}
		slots[other] = slotView{}
	}
	slots[index].Place, slots[index].Placed = place, true
	s.stage(ash, slots, done)
}

// moveSlots stands the summons in movers somewhere else together: lead where
// it is let go, at x, z, and the others as far and the same way, each keeping
// its facing. One alone takes a cell another stands in, and that one leaves
// (moveSlot); together they stand only on free ground, so a group never
// pushes out a summon the player did not pick.
func (s *State) moveSlots(ash *ashView, movers []int, lead int, x, z float64) {
	if len(movers) == 1 {
		s.moveSlot(ash, lead, x, z)
		return
	}
	slots := s.slotsFor(ash)
	places := cellPlaces(slots)
	dx, dz := x-places[lead].X, z-places[lead].Z
	if dx == 0 && dz == 0 {
		return
	}
	for _, index := range movers {
		if blocked, why := blockedAt(slots, places, movers, places[index].X+dx, places[index].Z+dz); blocked {
			s.setNotice(notice{text: fmt.Sprintf("%s would %s: move them where the ground is free.", slots[index].Name, why), bad: true})
			return
		}
	}
	for _, index := range movers {
		place := places[index]
		place.X, place.Z = place.X+dx, place.Z+dz
		slots[index].Place, slots[index].Placed = place, true
	}
	s.stage(ash, slots, notice{text: fmt.Sprintf("%d summons moved together.", len(movers))})
}

// blockedAt says whether a summon of a group moved to x, z would leave the
// ground or stand on one the group leaves behind, and which.
func blockedAt(slots [maxSlots]slotView, places [maxSlots]application.Placement, movers []int, x, z float64) (bool, string) {
	if !onGround(x, z) {
		return true, "leave the ground"
	}
	for index, slot := range slots {
		if slot.Filled && places[index].X == x && places[index].Z == z && !slices.Contains(movers, index) {
			return true, "stand on " + slot.Name
		}
	}
	return false, ""
}

// standingAt is the slot whose summon stands in the cell at x, z, other than
// except, or -1.
func standingAt(slots [maxSlots]slotView, x, z float64, except int) int {
	places := cellPlaces(slots)
	for index, slot := range slots {
		if slot.Filled && index != except && places[index].X == x && places[index].Z == z {
			return index
		}
	}
	return -1
}

// turnSlot turns one slot to face angle, in degrees from the player's own
// facing, keeping where it stands.
func (s *State) turnSlot(ash *ashView, index int, angle float64) {
	slots := s.slotsFor(ash)
	place := s.placeOf(slots, index)
	place.Angle = angle
	slots[index].Place, slots[index].Placed = place, true
	s.stage(ash, slots, notice{text: fmt.Sprintf("%s faces %s.", slots[index].Name, describeFacing(angle))})
}

// turnSlots turns the summons in indices by the same angle, each from its own
// facing, keeping where they stand.
func (s *State) turnSlots(ash *ashView, indices []int, by float64) {
	slots := s.slotsFor(ash)
	places := cellPlaces(slots)
	if len(indices) == 1 {
		s.turnSlot(ash, indices[0], snapAngle(places[indices[0]].Angle+by))
		return
	}
	for _, index := range indices {
		place := places[index]
		place.Angle = snapAngle(place.Angle + by)
		slots[index].Place, slots[index].Placed = place, true
	}
	side := "right"
	if snapAngle(by) < 0 {
		side = "left"
	}
	s.stage(ash, slots, notice{text: fmt.Sprintf("%d summons turned %g° to the %s.", len(indices), math.Abs(snapAngle(by)), side)})
}

// placeOf is where a slot stands: its cell on the grid (cellPlaces).
func (s *State) placeOf(slots [maxSlots]slotView, index int) application.Placement {
	return cellPlaces(slots)[index]
}

// describePlace says where a placement is in words: metres ahead or behind and
// to either side of the player, to a tenth - the file holds float32, so a
// shipped 1.3 reads back as 1.2999999523162842.
func describePlace(place application.Placement) string {
	x, z := math.Round(place.X*10)/10, math.Round(place.Z*10)/10
	if x == 0 && z == 0 {
		return "your own spot"
	}
	parts := make([]string, 0, 2)
	switch {
	case z > 0:
		parts = append(parts, fmt.Sprintf("%g m ahead", z))
	case z < 0:
		parts = append(parts, fmt.Sprintf("%g m behind", -z))
	}
	switch {
	case x > 0:
		parts = append(parts, fmt.Sprintf("%g m to the right", x))
	case x < 0:
		parts = append(parts, fmt.Sprintf("%g m to the left", -x))
	}
	return strings.Join(parts, ", ")
}

// describeFacing names a facing relative to the player's, to a degree.
func describeFacing(angle float64) string {
	angle = math.Round(angle)
	switch angle {
	case 0:
		return "the way you face"
	case 180, -180:
		return "you"
	}
	side := "right"
	if angle < 0 {
		side = "left"
	}
	return fmt.Sprintf("%g° to your %s", math.Abs(angle), side)
}

// clearSlots empties the slots in indices together and stages what is left.
func (s *State) clearSlots(ash *ashView, indices []int) {
	if len(indices) == 1 {
		s.clearSlot(ash, indices[0])
		return
	}
	slots := s.slotsFor(ash)
	for _, index := range indices {
		slots[index] = slotView{}
	}
	s.stage(ash, slots, notice{text: fmt.Sprintf("%d summons removed from %s.", len(indices), ash.Name)})
}

// clearSlot empties one slot and stages what is left.
func (s *State) clearSlot(ash *ashView, index int) {
	slots := s.slotsFor(ash)
	emptied := slots[index]
	slots[index] = slotView{}
	s.stage(ash, slots, notice{text: fmt.Sprintf("%s removed from %s.", emptied.Name, ash.Name)})
}

// stage records the composed ten with the session, and keeps them to draw.
//
// A slot naming no actor is left out of what is staged. Six shipped rows are
// like that; they summon nothing, and a formation cannot name an actor that
// does not exist. Everything else goes in the order the grid shows, each in
// the cell the grid shows it in.
func (s *State) stage(ash *ashView, slots [maxSlots]slotView, done notice) {
	// The frame may only call the session while no worker holds it. The
	// controls that lead here are all disabled during one, so this is the
	// backstop for a drag that was already in flight when a load began.
	if s.Busy() {
		return
	}
	places := cellPlaces(slots)
	authored := make([]application.AuthoredSlot, 0, maxSlots)
	for index := range slots {
		if !slots[index].Filled {
			continue
		}
		slots[index].Place, slots[index].Placed = places[index], true
		if !slots[index].NoActor {
			authored = append(authored, slots[index].authored())
		}
	}
	if len(authored) == 0 {
		// The save has no way to say "summons nothing": a formation is at least
		// one row. So the last slot cannot be emptied, and saying why is better
		// than a cell that refuses to clear without explaining itself.
		s.setNotice(notice{
			text: "An Ash must summon something: replace this one instead.",
			bad:  true,
		})
		return
	}
	host := strconv.Itoa(int(ash.GoodsFamily))
	if _, err := s.session.StageFormation(host, authored); err != nil {
		s.setNotice(notice{text: err.Error(), bad: true})
		return
	}
	if s.edits == nil {
		s.edits = make(map[int32]*formationEdit)
	}
	s.edits[ash.GoodsFamily] = &formationEdit{Slots: slots}
	s.setNotice(done)
}

// authored is the slot as the session stages it.
func (slot slotView) authored() application.AuthoredSlot {
	one := application.AuthoredSlot{Npc: slot.Npc, Think: slot.Think, Person: slot.Person}
	if slot.Placed {
		place := slot.Place
		one.Placement = &place
	}
	if slot.Shipped {
		// Rows the save holds, as they are: nothing is written on them, and
		// they keep the stance they carry.
		one.Perception, one.Share, one.Stance = slot.Level, slot.Share, slot.Stance
		return one
	}
	// The player's own: the role, share and look chosen in the creature
	// window, the team, and the stance those call for. A creature's strength
	// rides on its clone; a person's is its shell's, chosen on the row
	// (setFighter).
	one.Perception, one.Share, one.Look = slot.Level, slot.Share, slot.Look
	one.Fighter = slot.Fighter && slot.Person == 0
	if slot.Enemy {
		one.Team, one.WorldBranch = application.TeamEnemy, true
	}
	if stance, known := application.LevelStance(one); known {
		one.Stance = stance
	}
	return one
}

// unstageFormation drops one Ash's staged formation, which puts the panel back
// to what the save holds. Nothing is written either way, so this is symmetrical
// with staging rather than an undo of it.
//
// The formation panel used to carry a "Drop these changes" button that did this
// and said so in the footer. The footer's own pending list already lists the
// edit and drops it, so the button was a second way to do one thing in the
// narrowest panel of the three. What is left is this, with the pending list as
// its only caller.
func (s *State) unstageFormation(family int32) {
	if s.Busy() {
		return
	}
	s.session.Unstage(family)
	delete(s.edits, family)
}

// parseSpirit reads a drag payload: the actor's two row numbers.
func parseSpirit(payload string) (npc, think, person int32, ok bool) {
	rows := strings.Split(payload, ",")
	if len(rows) != 3 {
		return 0, 0, 0, false
	}
	values := make([]int32, 3)
	for at, raw := range rows {
		number, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
		if err != nil {
			return 0, 0, 0, false
		}
		values[at] = int32(number)
	}
	return values[0], values[1], values[2], true
}
