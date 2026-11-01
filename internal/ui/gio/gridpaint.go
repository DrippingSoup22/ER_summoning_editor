package gio

// How the placement grid looks; placement.go is what it does.
//
// Bottom to top: the ground, each cell a tile in a calm frame; the inside of
// the cells the bodies cover, in yellow where two do; the frames of the cells
// the summons notice - blue for an ally, red for an enemy, the chosen summon's
// over every other, faint where only a nose reaches; the distance rings; the player; the summons, each a square
// with its portrait and an arrow for its facing; and over them the turn knob,
// a label, and the view buttons in the corner.

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// A cell is a tile: a dark gap from its neighbours on its left and top, its
// own frame inside that, and its inside. cellLines is the gap and the frame
// in pixels. The frame thins as the grid zooms out and goes where a cell is
// too small for one, leaving a single blue line between cells, so a far view
// is still cells and not a mesh.
func cellLines(gtx layout.Context, geo gridGeometry) (gap, frame int) {
	return max(gtx.Dp(unit.Dp(1)), 1), min(int(geo.cell/30), 2)
}

// cellTile is a cell without the gap before it: its frame and its inside.
func cellTile(geo gridGeometry, cell, gap int) image.Rectangle {
	rect := geo.cellRect(cell)
	rect.Min = rect.Min.Add(image.Pt(gap, gap))
	return rect
}

// cellInside is a cell's inside, within its frame.
func cellInside(geo gridGeometry, cell, gap, frame int) image.Rectangle {
	return cellTile(geo, cell, gap).Inset(frame)
}

// drawGround paints the ground the panel shows, every cell a framed tile. It
// is drawn as lines rather than tile by tile - each boundary carries the frame
// of the cell either side and the gap between them - so the far view costs a
// hundred lines rather than thousands of tiles.
func drawGround(gtx layout.Context, geo gridGeometry) {
	left, right, bottom, top := geo.span()
	if left > right || bottom > top {
		return
	}
	ground := geo.rectOf(float64(left)-0.5, float64(top)+0.5, float64(right)+0.5, float64(bottom)-0.5)
	defer clip.Rect(ground).Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, colorCellFill, clip.Rect(ground).Op())
	gap, frame := cellLines(gtx, geo)
	lines := func(from, to int, ink color.NRGBA) {
		for x := left; x <= right+1; x++ {
			at := geo.pxX(float64(x) - 0.5)
			paint.FillShape(gtx.Ops, ink,
				clip.Rect{Min: image.Pt(at+from, ground.Min.Y), Max: image.Pt(at+to, ground.Max.Y)}.Op())
		}
		for z := top + 1; z >= bottom; z-- {
			at := geo.pxZ(float64(z) - 0.5)
			paint.FillShape(gtx.Ops, ink,
				clip.Rect{Min: image.Pt(ground.Min.X, at+from), Max: image.Pt(ground.Max.X, at+to)}.Op())
		}
	}
	if frame == 0 {
		lines(0, gap, colorCellEdge)
		return
	}
	lines(-frame, gap+frame, colorCellEdge)
	lines(0, gap, colorCellGap)
}

// drawFootprint fills the inside of the cells the bodies cover: softly where
// a body stands, in yellow where two stand too near each other - the size
// warning.
func drawFootprint(gtx layout.Context, geo gridGeometry, held []int) {
	gap, frame := cellLines(gtx, geo)
	geo.eachCell(func(cell int, _, _ float64) {
		fill := colorBodyFill
		switch held[cell] {
		case cellCrowded:
			fill = colorCrowdFill
		case cellEmpty:
			return
		}
		paint.FillShape(gtx.Ops, fill, clip.Rect(cellInside(geo, cell, gap, frame)).Op())
	})
}

// viewer is the chosen summon as the sight frames see it: what it notices,
// where it stands, and whether it is an enemy.
type viewer struct {
	sight application.Sight
	place application.Placement
	enemy bool
}

// sightInk is the colour of a cell's frame, and whether the summon notices
// the cell at all: blue for an ally and red for an enemy. Where only its nose
// reaches - all round, so behind it too - the colour is faint: the eyes look
// ahead, the nose only notices what comes close.
func sightInk(one viewer, x, z float64) (color.NRGBA, bool) {
	eye, nose := notices(one.sight, one.place, x, z)
	switch {
	case !eye && !nose:
		return color.NRGBA{}, false
	case one.enemy:
		return faintUnless(colorEnemyChosen, eye), true
	}
	return faintUnless(colorAllyChosen, eye), true
}

// faintUnless is ink at full strength where the eyes see, and at two fifths
// where only the nose notices.
func faintUnless(ink color.NRGBA, eye bool) color.NRGBA {
	if eye {
		return ink
	}
	return alpha(ink, uint8(uint16(ink.A)*2/5))
}

// chosenViewer is the chosen summon as its sight frames see it, and whether
// a summon is chosen. Sight is drawn one summon at a time, so it reads
// clearly, and none while nobody is chosen.
func (s *State) chosenViewer(slots [maxSlots]slotView, places [maxSlots]application.Placement) (viewer, bool) {
	if s.chosenSlot == noToken || s.chosenSlot >= maxSlots {
		return viewer{}, false
	}
	slot := slots[s.chosenSlot]
	if !slot.Filled || slot.NoActor {
		return viewer{}, false
	}
	sight, _ := s.session.SightOf(slot.authored())
	return viewer{sight, places[s.chosenSlot], slot.Enemy}, true
}

// drawSight frames each cell the chosen summon notices by itself in the
// colour sightInk gives it.
func (s *State) drawSight(gtx layout.Context, geo gridGeometry, slots [maxSlots]slotView,
	places [maxSlots]application.Placement) {
	one, chosen := s.chosenViewer(slots, places)
	if s.Busy() || !chosen {
		return
	}
	// A sight frame is drawn over the cell's own and is never thinner.
	gap, frame := cellLines(gtx, geo)
	width := max(frame, 1, min(gtx.Dp(unit.Dp(2.5)), int(geo.cell/14)))
	geo.eachCell(func(cell int, x, z float64) {
		if ink, seen := sightInk(one, x, z); seen {
			edge(gtx, cellTile(geo, cell, gap), ink, width)
		}
	})
}

// drawDropTargets marks the cell a catalog drag is over, and registers each
// cell in view as a target while a drag is in flight: an area registered for
// nothing would still swallow the pointer.
func (s *State) drawDropTargets(gtx layout.Context, geo gridGeometry, targets bool) {
	if s.dropCell != noCell && s.dropCell < len(s.cellTags) {
		gap, _ := cellLines(gtx, geo)
		outline(gtx, cellTile(geo, s.dropCell, gap), colorAccent, unit.Dp(2))
	}
	if !targets {
		return
	}
	geo.eachCell(func(cell int, _, _ float64) {
		area := clip.Rect(geo.cellRect(cell)).Push(gtx.Ops)
		event.Op(gtx.Ops, &s.cellTags[cell])
		area.Pop()
	})
}

// drawRings draws the distance from the player every five metres, each ring
// labelled where it crosses the line straight ahead.
func drawRings(gtx layout.Context, th *material.Theme, geo gridGeometry) {
	centre := geo.toPx(0, 0)
	width := float32(gtx.Dp(unit.Dp(1)))
	for metres := 5; metres <= groundReach; metres += 5 {
		r := float32(metres) * geo.cell
		box := image.Rect(int(centre.X-r), int(centre.Y-r), int(centre.X+r), int(centre.Y+r))
		paint.FillShape(gtx.Ops, alpha(colorSlotEdge, 0x70),
			clip.Stroke{Path: clip.Ellipse(box).Path(gtx.Ops), Width: width}.Op())
		at := image.Pt(int(centre.X)+gtx.Dp(unit.Dp(4)), int(centre.Y-r)+gtx.Dp(unit.Dp(1)))
		if !geo.inside(layout.FPt(at)) {
			continue
		}
		label := material.Caption(th, fmt.Sprintf("%d m", metres))
		label.Color = colorMuted
		offset := op.Offset(at).Push(gtx.Ops)
		label.Layout(gtx)
		offset.Pop()
	}
}

// drawPlayer marks the player in the centre cell: an arrow pointing the way
// they face, which is up.
func drawPlayer(gtx layout.Context, geo gridGeometry) {
	half := min(max(geo.cell*0.32, float32(gtx.Dp(unit.Dp(6)))), float32(gtx.Dp(unit.Dp(16))))
	triangle(gtx, geo.toPx(0, 0), f32.Pt(0, -1), half, colorFg)
}

// triangle fills an arrowhead at centre pointing along dir.
func triangle(gtx layout.Context, centre, dir f32.Point, half float32, fill color.NRGBA) {
	side := f32.Pt(-dir.Y, dir.X)
	tip := centre.Add(dir.Mul(half))
	back := centre.Sub(dir.Mul(half * 0.7))
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(tip)
	path.LineTo(back.Add(side.Mul(half * 0.8)))
	path.LineTo(back.Sub(side.Mul(half * 0.8)))
	path.Close()
	paint.FillShape(gtx.Ops, fill, clip.Outline{Path: path.End()}.Op())
}

// squareAt is the pixel square of side 2*half around a point.
func squareAt(centre f32.Point, half float32) image.Rectangle {
	round := func(v float32) int { return int(math.Round(float64(v))) }
	return image.Rect(round(centre.X-half), round(centre.Y-half), round(centre.X+half), round(centre.Y+half))
}

// drawToken is one summon: its portrait in a square, turned a quarter at a time
// with its facing; the frame, which says what it is to the player; an arrow on
// the frame for the facing itself, past its knob when it shows one; and its
// input area.
func (s *State) drawToken(gtx layout.Context, geo gridGeometry, index int, slot slotView,
	place application.Placement, editable, knob bool) {
	moving := s.moving && s.pressSlot == index
	centre := geo.toPx(place.X, place.Z)
	if moving {
		centre = s.moveAt
	}
	half := geo.tokenSide(gtx) / 2
	box := squareAt(centre, half)
	corner := gtx.Dp(unit.Dp(3))
	square := clip.RRect{Rect: box, SE: corner, SW: corner, NE: corner, NW: corner}

	paint.FillShape(gtx.Ops, colorPanelBg, square.Op(gtx.Ops))
	if picture, has := s.entityArt.op(s.slotPicture(slot), box.Dx()); has && !slot.NoActor {
		clipped := square.Push(gtx.Ops)
		turn := float32(portraitTurn(place.Angle) * math.Pi / 180)
		turned := op.Affine(f32.Affine2D{}.Rotate(layout.FPt(box.Min.Add(box.Max)).Mul(0.5), turn)).Push(gtx.Ops)
		offset := op.Offset(box.Min).Push(gtx.Ops)
		picture.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		offset.Pop()
		turned.Pop()
		clipped.Pop()
	}
	ink, width := s.tokenInk(gtx, index, slot, moving)
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: square.Path(gtx.Ops), Width: width}.Op())

	// The arrow sits on the frame, or just past the knob when the knob shows.
	// A plain frame is quiet; its arrow is not, since the facing is what a
	// glance at the grid is for.
	dir := facingVector(place.Angle)
	at := edgePoint(centre, half, dir)
	if knob {
		at = at.Add(dir.Mul(float32(gtx.Dp(knobRadius))))
	}
	if ink == colorSlotEdge {
		ink = colorGlow
	}
	arrow := float32(gtx.Dp(unit.Dp(6)))
	triangle(gtx, at.Add(dir.Mul(arrow*0.7)), dir, arrow, ink)

	// The input area is the token's own, so a press lands on one token even
	// where two overlap - except while a catalog drag is in flight, when it
	// passes through so a drop still reaches the cell under it.
	if s.dragging {
		pass := pointer.PassOp{}.Push(gtx.Ops)
		defer pass.Pop()
	}
	area := clip.Rect(box).Push(gtx.Ops)
	if editable {
		pointer.CursorGrab.Add(gtx.Ops)
	}
	event.Op(gtx.Ops, &s.tokenTags[index])
	area.Pop()
}

// tokenInk is a token's frame: red for an enemy, ember for a summon seen to
// hurt the player, accent for the chosen one - an enemy's chosen in its
// brighter red - and thicker when chosen.
func (s *State) tokenInk(gtx layout.Context, index int, slot slotView, moving bool) (color.NRGBA, float32) {
	chosen := moving || s.isPicked(index)
	width := float32(gtx.Dp(unit.Dp(1.5)))
	if chosen {
		width = float32(gtx.Dp(unit.Dp(2.5)))
	}
	switch {
	case slot.Enemy && chosen:
		return colorEnemyChosen, width
	case slot.Enemy:
		return colorEnemySight, width
	case chosen:
		return colorAccent, width
	case slot.warning() != "":
		return colorWarn, width
	}
	return colorSlotEdge, width
}

// drawMoveTarget marks where a token being moved will land: its cell, or the
// grid's whole edge in ember when letting go there removes it.
func (s *State) drawMoveTarget(gtx layout.Context, geo gridGeometry, slots [maxSlots]slotView,
	places [maxSlots]application.Placement, movers []int) {
	x, z := geo.toGround(s.moveAt)
	if !geo.inside(s.moveAt) || !onGround(x, z) {
		outline(gtx, image.Rectangle{Max: geo.size}, colorWarn, unit.Dp(2))
		return
	}
	gap, _ := cellLines(gtx, geo)
	// A group's cells are drawn where each would stand, in warning where one
	// would leave the ground or stand on a summon left behind (moveSlots).
	unmoved := cellPlaces(slots)
	for _, index := range movers {
		at := places[index]
		ink := colorAccent
		if len(movers) > 1 {
			if blocked, _ := blockedAt(slots, unmoved, movers, at.X, at.Z); blocked {
				ink = colorWarn
			}
		}
		if onGround(at.X, at.Z) {
			outline(gtx, cellTile(geo, cellOf(at.X, at.Z), gap), ink, unit.Dp(2))
		}
	}
}

// drawKnob is the turn knob on a token's edge, out along its facing: a round
// button with a turning arrow in it, lit under the pointer and while held.
func (s *State) drawKnob(gtx layout.Context, geo gridGeometry, index int, place application.Placement) {
	at := geo.knobAt(gtx, place)
	r := float32(gtx.Dp(knobRadius))
	box := squareAt(at, r)
	fill := colorControl
	if s.turning == index || s.knobHover == index {
		fill = colorAccentFill
	}
	paint.FillShape(gtx.Ops, fill, clip.Ellipse(box).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, colorAccent,
		clip.Stroke{Path: clip.Ellipse(box).Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1.5)))}.Op())
	drawTurnSign(gtx, at, r*0.48, colorFg)
	area := clip.Ellipse(box.Inset(-gtx.Dp(unit.Dp(2)))).Push(gtx.Ops)
	pointer.CursorPointer.Add(gtx.Ops)
	event.Op(gtx.Ops, &s.knobTags[index])
	area.Pop()
}

// drawTurnSign is a turning arrow: three quarters of a ring, open at the top,
// its head pointing on round.
func drawTurnSign(gtx layout.Context, centre f32.Point, radius float32, ink color.NRGBA) {
	const from, sweep, steps = -60.0, 270.0, 18
	// On screen y runs down, so a growing angle goes round clockwise.
	point := func(degrees float64) f32.Point {
		r := degrees * math.Pi / 180
		return centre.Add(f32.Pt(float32(math.Cos(r)), float32(math.Sin(r))).Mul(radius))
	}
	var ring clip.Path
	ring.Begin(gtx.Ops)
	ring.MoveTo(point(from))
	for step := 1; step <= steps; step++ {
		ring.LineTo(point(from + sweep*float64(step)/steps))
	}
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: ring.End(), Width: max(radius*0.3, 1)}.Op())
	end := (from + sweep) * math.Pi / 180
	on := f32.Pt(float32(-math.Sin(end)), float32(math.Cos(end)))
	head := radius * 0.6
	triangle(gtx, point(from+sweep).Add(on.Mul(head*0.7)), on, head, ink)
}

// drawGridLabel is a few words beside a token - who it is, or while it turns
// which way it faces - on the side away from its arrow and knob.
func drawGridLabel(gtx layout.Context, th *material.Theme, geo gridGeometry,
	place application.Placement, words string) {
	centre := geo.toPx(place.X, place.Z)
	half := geo.tokenSide(gtx) / 2
	macro := op.Record(gtx.Ops)
	label := gtx
	label.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(unit.Dp(340)), gtx.Dp(unit.Dp(120)))}
	dims := layout.UniformInset(unit.Dp(5)).Layout(label, func(gtx layout.Context) layout.Dimensions {
		style := material.Body2(th, words)
		style.Color, style.MaxLines = colorFg, 4
		return style.Layout(gtx)
	})
	call := macro.Stop()

	gap := gtx.Dp(unit.Dp(6))
	above := int(centre.Y-half) - dims.Size.Y - gap
	below := int(centre.Y+half) + gap
	at := image.Pt(int(centre.X)-dims.Size.X/2, above)
	if facingVector(place.Angle).Y < 0 || above < 0 {
		at.Y = below
	}
	if at.Y+dims.Size.Y > geo.size.Y {
		at.Y = max(above, 0)
	}
	at.X = min(max(at.X, 0), max(geo.size.X-dims.Size.X, 0))
	defer op.Offset(at).Push(gtx.Ops).Pop()
	corner := gtx.Dp(unit.Dp(3))
	paint.FillShape(gtx.Ops, colorControl,
		clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: corner, SW: corner, NE: corner, NW: corner}.Op(gtx.Ops))
	call.Add(gtx.Ops)
}

// drawViewButtons is the grid's corner: nearer, farther and home. The signs
// are drawn rather than typed, since the editor ships its own faces and a sign
// missing from one would be a box.
func (s *State) drawViewButtons(gtx layout.Context, geo gridGeometry) {
	side, gap, margin := gtx.Dp(unit.Dp(30)), gtx.Dp(unit.Dp(6)), gtx.Dp(unit.Dp(10))
	buttons := [...]struct {
		click   *widget.Clickable
		sign    func(gtx layout.Context, centre f32.Point, r float32, ink color.NRGBA)
		enabled bool
	}{
		{&s.zoomInBtn, drawPlus, s.gridView.reach > nearestReach},
		{&s.zoomOutBtn, drawMinus, s.gridView.reach < farthestReach},
		{&s.homeBtn, drawHome, s.gridView != homeView},
	}
	at := image.Pt(geo.size.X-margin-side, geo.size.Y-margin-len(buttons)*side-(len(buttons)-1)*gap)
	for _, one := range buttons {
		offset := op.Offset(at).Push(gtx.Ops)
		button := gtx
		button.Constraints = layout.Exact(image.Pt(side, side))
		if !one.enabled {
			button = button.Disabled()
		}
		one.click.Layout(button, func(gtx layout.Context) layout.Dimensions {
			fill, ink := alpha(colorControl, 0xE8), colorFg
			switch {
			case !one.enabled:
				ink = colorMuted
			case one.click.Hovered():
				fill = colorAccentFill
			}
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			corner := gtx.Dp(unit.Dp(4))
			shape := clip.RRect{Rect: rect, SE: corner, SW: corner, NE: corner, NW: corner}
			paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colorDivider,
				clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			one.sign(gtx, layout.FPt(rect.Max).Mul(0.5), float32(side)*0.26, ink)
			return layout.Dimensions{Size: rect.Max}
		})
		offset.Pop()
		at.Y += side + gap
	}
}

// bar is a filled rectangle from its corners, rounded to pixels.
func bar(gtx layout.Context, from, to f32.Point, ink color.NRGBA) {
	round := func(v float32) int { return int(math.Round(float64(v))) }
	paint.FillShape(gtx.Ops, ink,
		clip.Rect{Min: image.Pt(round(from.X), round(from.Y)), Max: image.Pt(round(to.X), round(to.Y))}.Op())
}

func drawPlus(gtx layout.Context, c f32.Point, r float32, ink color.NRGBA) {
	t := max(r*0.12, 1)
	bar(gtx, c.Sub(f32.Pt(r, t)), c.Add(f32.Pt(r, t)), ink)
	bar(gtx, c.Sub(f32.Pt(t, r)), c.Add(f32.Pt(t, r)), ink)
}

func drawMinus(gtx layout.Context, c f32.Point, r float32, ink color.NRGBA) {
	t := max(r*0.12, 1)
	bar(gtx, c.Sub(f32.Pt(r, t)), c.Add(f32.Pt(r, t)), ink)
}

// drawHome is a sight: a ring with four ticks and a dot, where the player is.
func drawHome(gtx layout.Context, c f32.Point, r float32, ink color.NRGBA) {
	t := max(r*0.12, 1)
	ring := r * 0.62
	paint.FillShape(gtx.Ops, ink,
		clip.Stroke{Path: clip.Ellipse(squareAt(c, ring)).Path(gtx.Ops), Width: 2 * t}.Op())
	bar(gtx, c.Sub(f32.Pt(t, r*1.1)), c.Add(f32.Pt(t, -ring)), ink)
	bar(gtx, c.Add(f32.Pt(-t, ring)), c.Add(f32.Pt(t, r*1.1)), ink)
	bar(gtx, c.Sub(f32.Pt(r*1.1, t)), c.Add(f32.Pt(-ring, t)), ink)
	bar(gtx, c.Add(f32.Pt(ring, -t)), c.Add(f32.Pt(r*1.1, t)), ink)
	paint.FillShape(gtx.Ops, ink, clip.Ellipse(squareAt(c, r*0.2)).Op(gtx.Ops))
}

// edge strokes a rectangle's edge inside it, width pixels wide.
func edge(gtx layout.Context, rect image.Rectangle, ink color.NRGBA, width int) {
	for _, side := range [...]image.Rectangle{
		{Min: rect.Min, Max: image.Pt(rect.Max.X, rect.Min.Y+width)},
		{Min: image.Pt(rect.Min.X, rect.Max.Y-width), Max: rect.Max},
		{Min: rect.Min, Max: image.Pt(rect.Min.X+width, rect.Max.Y)},
		{Min: image.Pt(rect.Max.X-width, rect.Min.Y), Max: rect.Max},
	} {
		paint.FillShape(gtx.Ops, ink, clip.Rect(side).Op())
	}
}

// outline is edge in dp.
func outline(gtx layout.Context, rect image.Rectangle, ink color.NRGBA, width unit.Dp) {
	edge(gtx, rect, ink, gtx.Dp(width))
}
