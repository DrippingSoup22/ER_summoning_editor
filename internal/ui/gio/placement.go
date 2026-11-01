package gio

// The placement grid: where each summon of the open Ash stands and which way it
// faces, around the player.
//
// One cell is a metre, and a cell is where a summon will stand: +z straight
// ahead of the player, +x to the right, a positive facing turning right
// (seen on the console). The row holds z 3.2 m short, since the game stands a summon that
// much further ahead (formation.Ahead), and the application converts both ways.
//
// The game sets no distance limit; the map does: a summon travels along a line
// from the player and stops where something blocks it. The ground here runs
// groundReach metres every way, and the panel is a view onto it: it opens on
// the player with five metres to its nearer edge, and zooms and pans from there.
//
// What the pointer does, the same with a mouse and a touchpad:
//
//   - a left click on a token chooses it, and a right click - a touchpad's
//     two-finger tap - opens its window; the left and right arrows turn the
//     chosen one, and Esc lets it go;
//   - a token dragged to another cell moves, and dragged out of the grid is
//     removed; the knob on its edge turns it; a catalog drag dropped on a cell
//     adds a summon there. Each of these chooses the summon it touched. Two
//     summons never share a cell: one dropped or moved onto another's takes
//     its place, and the other leaves the formation (staging.go);
//   - the chosen summon's knob stays on it whatever the pointer does, and a
//     token under the pointer shows its own beside it, to turn it at once;
//   - a press anywhere on the grid, and a drop on it, take the keys from a
//     text field, so the arrows turn the chosen summon again (takeKeys);
//   - a click on the ground does nothing, and a drag on it looks around;
//   - the wheel zooms about the pointer - a touchpad's pinch arrives as Ctrl
//     and the wheel - and the buttons in the corner zoom and come home.
//
// A touchpad drags with a tap and then a second tap held down: the tap chooses
// the summon, and the second drags it. The window opens on the right button
// alone, which never starts a drag, so nothing waits to tell the two apart.
//
// The drawing is in gridpaint.go.

import (
	"image"
	"io"
	"math"
	"slices"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

const (
	// groundReach is how far the ground runs from the player, every way.
	groundReach = 25
	groundSide  = 2*groundReach + 1
	// The view's reach is how many metres the panel shows from its centre to
	// its nearer edge: homeReach is five metres and the cell they end in;
	// nearestReach is a few cells, farthestReach the whole ground.
	homeReach     = 5.5
	nearestReach  = 2.5
	farthestReach = groundReach + 0.5
	// zoomStep is one notch of the wheel - wheelNotch on Windows - and one
	// press of a zoom button.
	zoomStep   = 1.25
	wheelNotch = 120
	// turnStep is what a facing snaps to: the eight ways - ahead, behind,
	// either side and the four between.
	turnStep = 45.0
	// noCell and noToken are the grid's "nothing" for a cell and a slot.
	noCell  = -1
	noToken = -1
	// dropGrace is how many frames the cells are still asked for a drop once
	// a catalog drag has ended: the drop arrives a frame or two after the
	// release, and a cell nobody asks is a cell the drop is lost on.
	dropGrace = 3
)

// knobRadius is the turn knob on a token's edge.
const knobRadius = unit.Dp(9)

// gridView is where the grid looks: the ground point at the panel's centre, and
// how far it reaches.
type gridView struct{ x, z, reach float64 }

var homeView = gridView{reach: homeReach}

// clamped keeps a view on the ground and its zoom in range.
func (v gridView) clamped() gridView {
	if !(v.reach > 0) {
		v.reach = homeReach
	}
	v.reach = min(max(v.reach, nearestReach), farthestReach)
	v.x = min(max(v.x, -groundReach), groundReach)
	v.z = min(max(v.z, -groundReach), groundReach)
	return v
}

// gridGeometry is one frame's grid: the panel's size in pixels - its top-left
// is the origin - a metre in pixels, and the view.
type gridGeometry struct {
	size image.Point
	cell float32
	view gridView
}

func newGridGeometry(size image.Point, view gridView) gridGeometry {
	view = view.clamped()
	short := max(min(size.X, size.Y), 1)
	return gridGeometry{size: size, cell: float32(float64(short) / 2 / view.reach), view: view}
}

func (g gridGeometry) centre() f32.Point {
	return f32.Pt(float32(g.size.X)/2, float32(g.size.Y)/2)
}

// toPx is a point on the ground in the panel's pixels.
func (g gridGeometry) toPx(x, z float64) f32.Point {
	c := g.centre()
	return f32.Pt(c.X+float32(x-g.view.x)*g.cell, c.Y-float32(z-g.view.z)*g.cell)
}

// toGround is a pixel in the panel as a point on the ground.
func (g gridGeometry) toGround(p f32.Point) (x, z float64) {
	c := g.centre()
	return g.view.x + float64((p.X-c.X)/g.cell), g.view.z - float64((p.Y-c.Y)/g.cell)
}

// inside reports whether a pixel is in the panel.
func (g gridGeometry) inside(p f32.Point) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < float32(g.size.X) && p.Y < float32(g.size.Y)
}

// zoomAt is the view zoomed by factor - above 1 nearer - about a pixel of the
// panel, keeping the ground under that pixel where it is.
func (g gridGeometry) zoomAt(p f32.Point, factor float64) gridView {
	x, z := g.toGround(p)
	view := g.view
	view.reach /= factor
	zoomed := newGridGeometry(g.size, view)
	view = zoomed.view
	c := g.centre()
	view.x = x - float64((p.X-c.X)/zoomed.cell)
	view.z = z + float64((p.Y-c.Y)/zoomed.cell)
	return view.clamped()
}

// onGround reports whether a point is on the ground.
func onGround(x, z float64) bool {
	return math.Abs(x) <= groundReach+0.5 && math.Abs(z) <= groundReach+0.5
}

// cellOf is the cell under a point on the ground; the point must be on it.
func cellOf(x, z float64) int {
	column := int(math.Round(x)) + groundReach
	row := groundReach - int(math.Round(z))
	return row*groundSide + column
}

// groundOf is a cell's centre on the ground.
func groundOf(cell int) (x, z float64) {
	return float64(cell%groundSide - groundReach), float64(groundReach - cell/groundSide)
}

// span is the cells the panel shows, in ground metres: columns left to right
// and rows bottom to top, cut to the ground. An empty span has left > right.
func (g gridGeometry) span() (left, right, bottom, top int) {
	l, t := g.toGround(f32.Pt(0, 0))
	r, b := g.toGround(layout.FPt(g.size))
	left = max(int(math.Floor(l+0.5)), -groundReach)
	right = min(int(math.Floor(r+0.5)), groundReach)
	bottom = max(int(math.Floor(b+0.5)), -groundReach)
	top = min(int(math.Floor(t+0.5)), groundReach)
	return left, right, bottom, top
}

// eachCell calls do for every cell the panel shows.
func (g gridGeometry) eachCell(do func(cell int, x, z float64)) {
	left, right, bottom, top := g.span()
	for z := top; z >= bottom; z-- {
		for x := left; x <= right; x++ {
			do(cellOf(float64(x), float64(z)), float64(x), float64(z))
		}
	}
}

// pxX and pxZ are a line of ground across and along, in whole pixels. Every
// edge is rounded on its own, so two neighbouring cells share theirs exactly.
func (g gridGeometry) pxX(x float64) int {
	return int(math.Round(float64(g.toPx(x, 0).X)))
}

func (g gridGeometry) pxZ(z float64) int {
	return int(math.Round(float64(g.toPx(0, z).Y)))
}

// rectOf is a rectangle of ground in pixels.
func (g gridGeometry) rectOf(left, top, right, bottom float64) image.Rectangle {
	return image.Rect(g.pxX(left), g.pxZ(top), g.pxX(right), g.pxZ(bottom))
}

func (g gridGeometry) cellRect(cell int) image.Rectangle {
	x, z := groundOf(cell)
	return g.rectOf(x-0.5, z+0.5, x+0.5, z-0.5)
}

// tokenSide is a summon's square, sized from the cell and held readable.
func (g gridGeometry) tokenSide(gtx layout.Context) float32 {
	return min(max(g.cell*0.82, float32(gtx.Dp(unit.Dp(18)))), float32(gtx.Dp(unit.Dp(56))))
}

// knobAt is where a token's turn knob sits: on the edge of its square, out
// along its facing, where the arrow is.
func (g gridGeometry) knobAt(gtx layout.Context, place application.Placement) f32.Point {
	return edgePoint(g.toPx(place.X, place.Z), g.tokenSide(gtx)/2, facingVector(place.Angle))
}

// facingVector is the unit vector of a facing on screen: 0 is up, the way the
// player faces, and a positive angle turns right.
func facingVector(angle float64) f32.Point {
	radians := angle * math.Pi / 180
	return f32.Pt(float32(math.Sin(radians)), float32(-math.Cos(radians)))
}

// edgePoint is where a ray from a square's centre along dir leaves it.
func edgePoint(centre f32.Point, half float32, dir f32.Point) f32.Point {
	along := half / max(float32(math.Abs(float64(dir.X))), float32(math.Abs(float64(dir.Y))))
	return centre.Add(dir.Mul(along))
}

// snapAngle rounds a facing to turnStep and keeps it in -180..180.
func snapAngle(angle float64) float64 {
	snapped := math.Round(angle/turnStep) * turnStep
	if snapped > 180 {
		snapped -= 360
	}
	if snapped <= -180 {
		snapped += 360
	}
	return snapped
}

// portraitTurn is how far a token's portrait is turned for a facing, in quarter
// turns so the picture stays square in its square: upright to 45 degrees
// either way, on its side to 135, upside down - looking back at the player -
// beyond.
func portraitTurn(angle float64) float64 {
	angle = math.Remainder(angle, 360)
	switch a := math.Abs(angle); {
	case a <= 45:
		return 0
	case a < 135:
		return math.Copysign(90, angle)
	}
	return 180
}

// cellPlaces is where each summon stands on the grid: in one cell of its own,
// always, and the cells are the only places the grid offers. A place the grid
// gave is a cell already and stays where the player put it. A place the save
// gave between cells, a second summon on a cell the save gave twice, or the
// ring a slot without one stands on, goes to the nearest cell no other summon
// - and not the player - stands in; it is drawn there, and the first edit to
// the formation writes it there.
func cellPlaces(slots [maxSlots]slotView) [maxSlots]application.Placement {
	count := 0
	for _, slot := range slots {
		if slot.Filled {
			count++
		}
	}
	var places [maxSlots]application.Placement
	for index, slot := range slots {
		places[index] = application.RingPlacement(index, count)
		if slot.Placed {
			places[index] = slot.Place
		}
	}
	whole := func(place application.Placement) bool {
		return place.X == math.Round(place.X) && place.Z == math.Round(place.Z)
	}
	// The first summon in a cell keeps it; a later one is handed another.
	taken := map[int]bool{}
	settled := [maxSlots]bool{}
	for index, slot := range slots {
		place := places[index]
		if !slot.Filled || !whole(place) || !onGround(place.X, place.Z) || taken[cellOf(place.X, place.Z)] {
			continue
		}
		taken[cellOf(place.X, place.Z)], settled[index] = true, true
	}
	// The player's own cell is handed to nobody, though a summon put there
	// stays.
	taken[cellOf(0, 0)] = true
	for index, slot := range slots {
		place := places[index]
		if !slot.Filled || settled[index] {
			continue
		}
		place.X, place.Z = nearestFreeCell(place.X, place.Z, taken)
		taken[cellOf(place.X, place.Z)] = true
		places[index] = place
	}
	return places
}

// nearestFreeCell is the cell nearest a point that nothing has taken. Ten
// summons and the player cannot fill the cells a few metres round a point, so
// the search stays near; the ground's edge pulls a far point in.
func nearestFreeCell(x, z float64, taken map[int]bool) (float64, float64) {
	const near = 3
	x = min(max(x, -groundReach), groundReach)
	z = min(max(z, -groundReach), groundReach)
	bestX, bestZ, best := math.Round(x), math.Round(z), math.Inf(1)
	for dx := -near; dx <= near; dx++ {
		for dz := -near; dz <= near; dz++ {
			cx, cz := math.Round(x)+float64(dx), math.Round(z)+float64(dz)
			if !onGround(cx, cz) || taken[cellOf(cx, cz)] {
				continue
			}
			if d := math.Hypot(cx-x, cz-z); d < best {
				bestX, bestZ, best = cx, cz, d
			}
		}
	}
	return bestX, bestZ
}

// capsule is a body on the ground: radius around the line down its middle,
// from x0, z0 to x1, z1.
type capsule struct{ x0, z0, x1, z1, radius float64 }

// onTheGround lays a body standing at place on the ground, lying along its
// facing: Side around the line down its middle, which runs from Side short of
// where its tail ends to Side short of where its head ends.
func onTheGround(body application.Body, place application.Placement) capsule {
	radians := place.Angle * math.Pi / 180
	right, ahead := math.Sin(radians), math.Cos(radians)
	head, tail := body.Front-body.Side, body.Back-body.Side
	return capsule{place.X - right*tail, place.Z - ahead*tail, place.X + right*head, place.Z + ahead*head, body.Side}
}

// cellShare is how much of a cell a body must fill to cover it, seen from
// above: half (after the whole cell left Commander
// Gaius's boar a line). One rule for every creature; a nose or a tail tip
// reaching in does not take a cell.
const cellShare = 0.5

// share is how much of the cell at x, z the body fills, counted on a 10 by 10
// lattice of points across it, so to a tenth; nought where it does not reach
// in at all.
func (body capsule) share(x, z float64) float64 {
	if cellGap(body.x0-x, body.z0-z, body.x1-x, body.z1-z) >= body.radius {
		return 0
	}
	const side = 10
	inside := 0
	for i := range side {
		for j := range side {
			px, pz := x-0.5+(float64(i)+0.5)/side, z-0.5+(float64(j)+0.5)/side
			if toSegment(px, pz, body.x0, body.z0, body.x1, body.z1) <= body.radius {
				inside++
			}
		}
	}
	return float64(inside) / (side * side)
}

// gap is how far apart two bodies stand, edge to edge: below nought they
// stand in each other.
func (body capsule) gap(other capsule) float64 {
	between := 0.0
	if !crossing(body, other) {
		between = min(
			toSegment(body.x0, body.z0, other.x0, other.z0, other.x1, other.z1),
			toSegment(body.x1, body.z1, other.x0, other.z0, other.x1, other.z1),
			toSegment(other.x0, other.z0, body.x0, body.z0, body.x1, body.z1),
			toSegment(other.x1, other.z1, body.x0, body.z0, body.x1, body.z1))
	}
	return between - body.radius - other.radius
}

// crossing reports whether the lines down two bodies cross: each one's ends
// lie on either side of the other.
func crossing(a, b capsule) bool {
	side := func(line capsule, x, z float64) float64 {
		return (line.x1-line.x0)*(z-line.z0) - (line.z1-line.z0)*(x-line.x0)
	}
	return side(a, b.x0, b.z0)*side(a, b.x1, b.z1) < 0 && side(b, a.x0, a.z0)*side(b, a.x1, a.z1) < 0
}

// cellGap is how far the segment x0,z0 - x1,z1 passes from the cell centred
// at nought: nothing where it crosses the cell, and otherwise the nearest of
// its ends to the cell, or of the cell's corners to it.
func cellGap(x0, z0, x1, z1 float64) float64 {
	if crossesCell(x0, z0, x1, z1) {
		return 0
	}
	gap := min(toCell(x0, z0), toCell(x1, z1))
	for _, corner := range [4][2]float64{{-0.5, -0.5}, {-0.5, 0.5}, {0.5, -0.5}, {0.5, 0.5}} {
		gap = min(gap, toSegment(corner[0], corner[1], x0, z0, x1, z1))
	}
	return gap
}

// crossesCell reports whether the segment x0,z0 - x1,z1 passes through the
// cell centred at nought, clipping it to the cell one axis at a time.
func crossesCell(x0, z0, x1, z1 float64) bool {
	from, to := 0.0, 1.0
	for _, axis := range [2][2]float64{{x0, x1 - x0}, {z0, z1 - z0}} {
		start, step := axis[0], axis[1]
		if step == 0 {
			if math.Abs(start) > 0.5 {
				return false
			}
			continue
		}
		enter, leave := (-0.5-start)/step, (0.5-start)/step
		from, to = max(from, min(enter, leave)), min(to, max(enter, leave))
	}
	return from <= to
}

// toCell is how far the point x, z is from the cell centred at nought.
func toCell(x, z float64) float64 {
	return math.Hypot(max(math.Abs(x)-0.5, 0), max(math.Abs(z)-0.5, 0))
}

// toSegment is how far the point x, z is from the segment x0,z0 - x1,z1.
func toSegment(x, z, x0, z0, x1, z1 float64) float64 {
	dx, dz := x1-x0, z1-z0
	along := 0.0
	if length := dx*dx + dz*dz; length > 0 {
		along = min(max(((x-x0)*dx+(z-z0)*dz)/length, 0), 1)
	}
	return math.Hypot(x-x0-along*dx, z-z0-along*dz)
}

// What a cell holds: nothing, a body, or two bodies standing too near each
// other - the size warning.
const (
	cellEmpty = iota
	cellCovered
	cellCrowded
)

// personBody is the player's body, and any summon's the catalog has no family
// for (a person): a person's collision radius, the Godrick Soldier's.
var personBody = application.Body{Front: 0.4, Back: 0.4, Side: 0.4}

// clearance is how near two bodies may be placed before they warn: a summon
// lands within about a tenth of a metre of where it is placed (seen on the console).
const clearance = 0.1

// footprint says what every cell holds. A body covers its own cell and each
// cell around it that it fills half of (cellShare), as far as its width
// and length go, turned with its facing; the player's body covers the
// player's cell. Height is not counted: a tall body covers the ground under
// it. Two bodies nearer each other than the clearance crowd the cells both
// cover and their own two: they may arrive inside each other.
func footprint(bodies map[string]application.Body, slots [maxSlots]slotView,
	places [maxSlots]application.Placement) []int {
	held := make([]int, groundSide*groundSide)
	everyone := []capsule{onTheGround(personBody, application.Placement{})}
	claims := [][]int{{cellOf(0, 0)}}
	for index, slot := range slots {
		place := places[index]
		if !slot.Filled || slot.NoActor || !onGround(place.X, place.Z) {
			continue
		}
		body, known := bodies[slot.Model]
		if !known {
			body = personBody
		}
		one := onTheGround(body, place)
		own := cellOf(place.X, place.Z)
		cells := []int{own}
		// Only the cells the body could reach are tested.
		reach := math.Max(body.Front, math.Max(body.Back, body.Side)) + 1
		for x := math.Floor(place.X - reach); x <= place.X+reach; x++ {
			for z := math.Floor(place.Z - reach); z <= place.Z+reach; z++ {
				if cell := cellOf(x, z); cell != own && onGround(x, z) && one.share(x, z) >= cellShare {
					cells = append(cells, cell)
				}
			}
		}
		everyone, claims = append(everyone, one), append(claims, cells)
	}
	for _, cells := range claims {
		for _, cell := range cells {
			held[cell] = cellCovered
		}
	}
	for i := range everyone {
		for j := i + 1; j < len(everyone); j++ {
			if everyone[i].gap(everyone[j]) >= clearance {
				continue
			}
			held[claims[i][0]], held[claims[j][0]] = cellCrowded, cellCrowded
			for _, cell := range claims[i] {
				if slices.Contains(claims[j], cell) {
					held[cell] = cellCrowded
				}
			}
		}
	}
	return held
}

// layoutPlacement is the grid for one Ash. editable is false for an Ash whose
// formation cannot be changed, which is drawn, zoomed and panned, and not
// offered.
func (s *State) layoutPlacement(gtx layout.Context, th *material.Theme, loaded *savedView,
	ash *ashView, editable bool) layout.Dimensions {
	size := gtx.Constraints.Max
	// What arrives this frame happened on the last frame's grid, so it is read
	// against that view, and the view it leaves is the one drawn.
	geo := newGridGeometry(size, s.gridView)
	slots := s.slotsFor(ash)
	places := cellPlaces(slots)
	if len(s.cellTags) != groundSide*groundSide {
		s.cellTags = make([]int, groundSide*groundSide)
		s.dropCell = noCell
	}
	s.settlePicked(slots)

	targets := s.dragging && editable
	s.claimed = false
	if editable {
		s.readGridDrops(gtx, ash, geo, targets)
	}
	s.readTokens(gtx, ash, geo, slots, editable)
	if editable {
		s.readKnob(gtx, ash, geo, places)
	}
	s.readKeys(gtx, ash, places, editable)
	s.readGround(gtx, geo)
	s.readViewButtons(gtx, geo)
	geo = newGridGeometry(size, s.gridView)
	// What the input staged this frame is drawn this frame: a summon let go
	// off the grid is gone at once, not when the pointer next moves.
	slots = s.slotsFor(ash)
	places = cellPlaces(slots)
	s.settlePicked(slots)
	s.settleKnob(slots)

	// A move in flight is drawn where the pointer is, and a turn at the angle
	// the knob points to - the summons picked with it alike; neither is staged
	// until it is let go.
	var movers []int
	if s.moving && s.pressSlot != noToken {
		movers = s.movers()
		x, z := geo.toGround(s.moveAt)
		dx, dz := math.Round(x)-places[s.pressSlot].X, math.Round(z)-places[s.pressSlot].Z
		for _, index := range movers {
			places[index].X, places[index].Z = places[index].X+dx, places[index].Z+dz
		}
	}
	if s.turning != noToken {
		by := s.turnAngle - places[s.turning].Angle
		for _, index := range s.picked() {
			places[index].Angle = snapAngle(places[index].Angle + by)
		}
	}

	// Everything on the grid is inside the ground's own input area, so the
	// wheel zooms over a token as well as beside one. A press reaches both a
	// token and the ground; the ground lets go of one a token or the knob took.
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.gridTag)
	if s.panning && s.panMoved {
		pointer.CursorGrabbing.Add(gtx.Ops)
	}
	drawGround(gtx, geo)
	drawFootprint(gtx, geo, footprint(loaded.bodies, slots, places))
	if s.showSight.Value {
		s.drawSight(gtx, geo, slots, places)
	}
	s.drawDropTargets(gtx, geo, targets)
	drawRings(gtx, th, geo)
	drawPlayer(gtx, geo)
	knobs := s.knobsShown(slots, editable)
	for index, slot := range slots {
		if slot.Filled {
			s.drawToken(gtx, geo, index, slot, places[index], editable, slices.Contains(knobs, index))
		}
	}
	if s.moving && s.pressSlot != noToken {
		s.drawMoveTarget(gtx, geo, slots, places, movers)
	}
	for _, index := range knobs {
		s.drawKnob(gtx, geo, index, places[index])
	}
	// The name stays while the pointer is on the summon's knob too: the knob
	// sits on the token's edge, and leaving the token for it is not leaving
	// the summon. Nothing is said while it turns: the arrow says where it
	// faces, and the creature window says it in words.
	named := s.hoverSlot
	if named == noToken {
		named = s.knobHover
	}
	if named != noToken && named < maxSlots && slots[named].Filled && !s.moving && s.turning == noToken {
		drawGridLabel(gtx, th, geo, places[named], slots[named].tooltip())
	}
	area.Pop()
	// The buttons are outside the ground's area, so pressing one is not a drag
	// on the ground under it.
	s.drawViewButtons(gtx, geo)
	return layout.Dimensions{Size: size}
}

// readGridDrops takes a catalog drag dropped on a cell, and tracks the cell a
// drag is over so it can be marked. The cells are asked while a drag is in
// flight and for a few frames after, which is when a drop can arrive.
func (s *State) readGridDrops(gtx layout.Context, ash *ashView, geo gridGeometry, targets bool) {
	if targets {
		s.dropFrames = dropGrace
	} else if s.dropFrames > 0 {
		s.dropFrames--
	} else {
		s.dropCell = noCell
		return
	}
	geo.eachCell(func(cell int, x, z float64) {
		tag := &s.cellTags[cell]
		for {
			got, ok := gtx.Event(transfer.TargetFilter{Target: tag, Type: spiritMIME})
			if !ok {
				break
			}
			data, dropped := got.(transfer.DataEvent)
			if !dropped {
				continue
			}
			reader := data.Open()
			payload, _ := io.ReadAll(reader)
			reader.Close()
			s.dropAt(ash, application.Placement{X: x, Z: z}, string(payload))
			s.dropCell = noCell
			takeKeys(gtx)
			s.invalidate()
		}
		for {
			got, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Enter | pointer.Leave})
			if !ok {
				break
			}
			moved, isPointer := got.(pointer.Event)
			if !isPointer {
				continue
			}
			switch moved.Kind {
			case pointer.Enter:
				s.dropCell = cell
			case pointer.Leave:
				if s.dropCell == cell {
					s.dropCell = noCell
				}
			}
		}
	})
	if !targets {
		s.dropCell = noCell
	}
}

// readTokens is what the pointer does to the summons: press, drag and let go
// on a token moves it - and the summons picked with it - or removes them when
// let go outside the grid; a left click chooses it (pick), and a right click
// opens its window. With Shift held, the pointer passing over a summon adds
// it to the summons picked, no button pressed. An Ash that cannot be edited
// opens the window and moves nothing.
func (s *State) readTokens(gtx layout.Context, ash *ashView, geo gridGeometry,
	slots [maxSlots]slotView, editable bool) {
	slop := float32(gtx.Dp(unit.Dp(6)))
	for index := range s.tokenTags {
		for {
			got, ok := gtx.Event(pointer.Filter{Target: &s.tokenTags[index],
				Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel |
					pointer.Enter | pointer.Leave | pointer.Move})
			if !ok {
				break
			}
			e, isPointer := got.(pointer.Event)
			if !isPointer || !slots[index].Filled {
				continue
			}
			// A Move as well as an Enter, so Shift pressed over a summon
			// takes it at the pointer's next step.
			if (e.Kind == pointer.Enter || e.Kind == pointer.Move) &&
				e.Modifiers.Contain(key.ModShift) && e.Buttons == 0 {
				s.join(index)
			}
			switch e.Kind {
			case pointer.Enter:
				s.hoverSlot = index
			case pointer.Leave:
				if s.hoverSlot == index {
					s.hoverSlot = noToken
				}
			case pointer.Press:
				s.claimed, s.claimedAt = true, e.Time
				takeKeys(gtx)
				if e.Buttons.Contain(pointer.ButtonSecondary) {
					s.openCreature(index)
					continue
				}
				s.pressSlot, s.pressAt, s.moving = index, e.Position, false
			case pointer.Drag:
				if s.pressSlot != index || !editable {
					continue
				}
				if !s.moving && distance(e.Position, s.pressAt) > slop {
					s.moving = true
				}
				s.moveAt = e.Position
			case pointer.Release:
				if s.pressSlot != index {
					continue
				}
				switch {
				case s.moving:
					movers := s.movers()
					x, z := geo.toGround(s.moveAt)
					if geo.inside(s.moveAt) && onGround(x, z) {
						s.moveSlots(ash, movers, index, math.Round(x), math.Round(z))
						s.lead(index)
					} else {
						s.clearSlots(ash, movers)
					}
				default:
					s.pick(index, e.Modifiers)
				}
				s.pressSlot, s.moving = noToken, false
			case pointer.Cancel:
				s.pressSlot, s.moving = noToken, false
			}
		}
	}
}

// readKnob turns the summon whose knob is dragged: it faces the pointer, in
// turnStep steps, and the summons picked with it turn as far; they are staged
// when let go. Taking hold of a knob chooses its summon, so the knob stays
// where the player left it.
func (s *State) readKnob(gtx layout.Context, ash *ashView, geo gridGeometry,
	places [maxSlots]application.Placement) {
	for index := range s.knobTags {
		for {
			got, ok := gtx.Event(pointer.Filter{Target: &s.knobTags[index],
				Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Enter | pointer.Leave})
			if !ok {
				break
			}
			e, isPointer := got.(pointer.Event)
			if !isPointer {
				continue
			}
			switch e.Kind {
			case pointer.Enter:
				s.knobHover = index
			case pointer.Leave:
				if s.knobHover == index {
					s.knobHover = noToken
				}
			case pointer.Press:
				s.claimed, s.claimedAt = true, e.Time
				takeKeys(gtx)
				if e.Buttons.Contain(pointer.ButtonPrimary) {
					s.turning, s.turnAngle = index, places[index].Angle
					s.lead(index)
				}
			case pointer.Drag:
				if s.turning != index {
					continue
				}
				place := places[index]
				if d := e.Position.Sub(geo.toPx(place.X, place.Z)); d != (f32.Point{}) {
					s.turnAngle = snapAngle(math.Atan2(float64(d.X), float64(-d.Y)) * 180 / math.Pi)
				}
			case pointer.Release:
				if s.turning == index {
					s.turning = noToken
					if s.turnAngle != places[index].Angle {
						s.turnSlots(ash, s.picked(), s.turnAngle-places[index].Angle)
					}
				}
			case pointer.Cancel:
				if s.turning == index {
					s.turning = noToken
				}
			}
		}
	}
}

// readKeys is the grid's keys: the left and right arrows turn the picked
// summons a step. Esc is not the grid's: it closes a window, or asks to close
// the editor, and a click on the ground lets the summons go. The arrows are not asked while a text field has the keys - it
// wants them for its caret - or while a window is open over the grid.
func (s *State) readKeys(gtx layout.Context, ash *ashView, places [maxSlots]application.Placement, editable bool) {
	if !editable || s.chosenSlot == noToken || s.creatureSlot != noToken || s.pendingOpen ||
		gtx.Focused(&s.search) || gtx.Focused(&s.fpField) || gtx.Focused(&s.hpField) {
		return
	}
	by := 0.0
	for {
		got, ok := gtx.Event(key.Filter{Name: key.NameLeftArrow}, key.Filter{Name: key.NameRightArrow})
		if !ok {
			break
		}
		e, isKey := got.(key.Event)
		if !isKey || e.State != key.Press {
			continue
		}
		switch e.Name {
		case key.NameLeftArrow:
			by -= turnStep
		case key.NameRightArrow:
			by += turnStep
		}
	}
	if snapAngle(by) != 0 {
		s.turnSlots(ash, s.picked(), by)
	}
}

// picked is the summons chosen together: the chosen one first, then those
// picked with it.
func (s *State) picked() []int {
	if s.chosenSlot == noToken {
		return nil
	}
	return append([]int{s.chosenSlot}, s.together...)
}

// isPicked says whether index is among the picked summons.
func (s *State) isPicked(index int) bool {
	return index != noToken && (index == s.chosenSlot || slices.Contains(s.together, index))
}

// movers is what a drag moves: the picked summons when the pressed one is
// among them, and otherwise the pressed one alone.
func (s *State) movers() []int {
	if s.isPicked(s.pressSlot) {
		return s.picked()
	}
	return []int{s.pressSlot}
}

// pick answers a click on a summon, as the system's own lists do: it alone is
// chosen; with Ctrl it is added to the summons picked - and chosen, so its
// sight shows - or taken out of them if it was one; with Shift it joins them,
// as passing over it with Shift did, so the click undoes nothing.
func (s *State) pick(index int, mods key.Modifiers) {
	switch {
	case mods.Contain(key.ModShift):
		s.join(index)
	case !mods.Contain(key.ModCtrl) || s.chosenSlot == noToken:
		s.chosenSlot, s.together = index, nil
	case index == s.chosenSlot:
		s.chosenSlot = noToken
		if last := len(s.together) - 1; last >= 0 {
			s.chosenSlot, s.together = s.together[last], s.together[:last]
		}
	case slices.Contains(s.together, index):
		s.together = slices.DeleteFunc(s.together, func(one int) bool { return one == index })
	default:
		s.together, s.chosenSlot = append(s.together, s.chosenSlot), index
	}
}

// join adds index to the summons picked and leaves the chosen one chosen, so
// the sight drawn holds still while Shift sweeps; with none picked, it is
// chosen.
func (s *State) join(index int) {
	switch {
	case s.chosenSlot == noToken:
		s.chosenSlot, s.together = index, nil
	case !s.isPicked(index):
		s.together = append(s.together, index)
	}
}

// lead chooses index, keeping the summons picked with it when it is one of
// them; one that is not is chosen alone.
func (s *State) lead(index int) {
	if !s.isPicked(index) {
		s.chosenSlot, s.together = index, nil
		return
	}
	if index != s.chosenSlot {
		s.together = slices.DeleteFunc(s.together, func(one int) bool { return one == index })
		s.together, s.chosenSlot = append(s.together, s.chosenSlot), index
	}
}

// settlePicked lets go of picked summons that are gone; the chosen one gone,
// the last one picked with it is chosen instead.
func (s *State) settlePicked(slots [maxSlots]slotView) {
	gone := func(index int) bool { return index >= maxSlots || !slots[index].Filled }
	s.together = slices.DeleteFunc(s.together, func(index int) bool { return gone(index) || index == s.chosenSlot })
	if s.chosenSlot != noToken && gone(s.chosenSlot) {
		s.chosenSlot = noToken
		if last := len(s.together) - 1; last >= 0 {
			s.chosenSlot, s.together = s.together[last], s.together[:last]
		}
	}
}

// readGround is the ground's own input: the wheel zooms about the pointer, a
// drag on the ground moves the view, as far as the ground goes, and a click on
// the ground lets the picked summons go. That click is the first tap of a
// touchpad's drag as often as not, so a drag pressed within doubleTap of it
// picks them again.
func (s *State) readGround(gtx layout.Context, geo gridGeometry) {
	slop := float32(gtx.Dp(unit.Dp(6)))
	for {
		got, ok := gtx.Event(pointer.Filter{Target: &s.gridTag,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: math.MinInt32, Max: math.MaxInt32}})
		if !ok {
			break
		}
		e, isPointer := got.(pointer.Event)
		if !isPointer {
			continue
		}
		switch e.Kind {
		case pointer.Scroll:
			// Away from you, or a pinch outwards, comes nearer.
			s.gridView = geo.zoomAt(e.Position, math.Pow(zoomStep, -float64(e.Scroll.Y)/wheelNotch))
			geo = newGridGeometry(geo.size, s.gridView)
		case pointer.Press:
			takeKeys(gtx)
			// Each input reads its own events in a batch, so whether a token
			// is still held says nothing here: the press itself is compared.
			if !e.Buttons.Contain(pointer.ButtonPrimary) || s.claimed && e.Time == s.claimedAt {
				s.letGo = nil
				continue
			}
			if e.Time-s.letGoAt > s.doubleTap {
				s.letGo = nil
			}
			s.panning, s.panMoved, s.panFrom, s.panView, s.panAt = true, false, e.Position, s.gridView, e.Time
		case pointer.Drag:
			if !s.panning || !s.panMoved && distance(e.Position, s.panFrom) <= slop {
				continue
			}
			if !s.panMoved && s.letGo != nil {
				s.chosenSlot, s.together = s.letGo[0], s.letGo[1:]
			}
			s.panMoved, s.letGo = true, nil
			d := e.Position.Sub(s.panFrom)
			view := s.panView
			view.x -= float64(d.X / geo.cell)
			view.z += float64(d.Y / geo.cell)
			s.gridView = view.clamped()
		case pointer.Release:
			if s.panning && !s.panMoved && s.chosenSlot != noToken {
				s.letGo, s.letGoAt = s.picked(), s.panAt
				s.chosenSlot, s.together = noToken, nil
			}
			s.panning = false
		case pointer.Cancel:
			s.panning = false
		}
	}
}

// readViewButtons is the corner's three buttons: nearer, farther, and home -
// the player in the centre at the opening zoom.
func (s *State) readViewButtons(gtx layout.Context, geo gridGeometry) {
	if s.zoomInBtn.Clicked(gtx) {
		s.gridView = geo.zoomAt(geo.centre(), zoomStep)
	}
	if s.zoomOutBtn.Clicked(gtx) {
		s.gridView = geo.zoomAt(geo.centre(), 1/zoomStep)
	}
	if s.homeBtn.Clicked(gtx) {
		s.gridView = homeView
	}
}

// settleKnob lets go of the knobs of a summon that is gone.
func (s *State) settleKnob(slots [maxSlots]slotView) {
	gone := func(index int) bool { return index != noToken && (index >= maxSlots || !slots[index].Filled) }
	if gone(s.turning) {
		s.turning = noToken
	}
	if gone(s.knobHover) {
		s.knobHover = noToken
	}
}

// knobsShown is whose knobs show: only the summon's under the pointer, kept
// while the pointer crosses onto its knob, and the one being turned (a knob shows only while the pointer is on it). None while a token is moved, on an Ash
// that cannot be edited, or with the knobs switched off among the Ash's
// options - the creature window's facing and the arrow keys still turn.
func (s *State) knobsShown(slots [maxSlots]slotView, editable bool) []int {
	if !editable || s.moving || !s.showKnobs.Value {
		return nil
	}
	var shown []int
	for _, index := range [...]int{s.hoverSlot, s.knobHover, s.turning} {
		if index != noToken && index < maxSlots && slots[index].Filled && !slices.Contains(shown, index) {
			shown = append(shown, index)
		}
	}
	return shown
}

// takeKeys lets go of the keyboard focus. A text field keeps the keys until
// something takes them, and the grid holds no focus of its own, so without
// this the arrows would go on moving a field's caret after the player has
// turned to the grid.
func takeKeys(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{})
}

func distance(a, b f32.Point) float32 {
	d := a.Sub(b)
	return float32(math.Hypot(float64(d.X), float64(d.Y)))
}

// notices is how a summon standing at place with sight notices the point
// x, z: by its eyes, inside its eye cone - Eye metres from an apex Behind
// metres back, within half the Cone either side of its facing - and by its
// nose, all round to Nose, which is what reaches behind it.
func notices(sight application.Sight, place application.Placement, x, z float64) (eye, nose bool) {
	dx, dz := x-place.X, z-place.Z
	nose = sight.Nose > 0 && math.Hypot(dx, dz) <= sight.Nose
	if sight.Eye <= 0 || sight.Cone <= 0 {
		return false, nose
	}
	radians := place.Angle * math.Pi / 180
	forwardX, forwardZ := math.Sin(radians), math.Cos(radians)
	ax, az := dx+forwardX*sight.Behind, dz+forwardZ*sight.Behind
	reach := math.Hypot(ax, az)
	if reach > sight.Eye {
		return false, nose
	}
	if reach == 0 {
		return true, nose
	}
	cos := (ax*forwardX + az*forwardZ) / reach
	// The edge belongs to the cone: a 180-degree cone sees straight to either
	// side, where the cosine rounds to a hair above nought.
	return cos >= math.Cos(sight.Cone/2*math.Pi/180)-1e-9, nose
}
