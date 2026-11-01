package gio

// The placement grid.

import (
	"image"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// A spirit dropped on the grid is staged standing on the cell it was dropped
// on, and moving or turning it re-stages it there. What the formation writes is
// the only thing the grid is for, so the staged plan is what is read.
func TestTheGridStagesWhereASummonStandsAndFaces(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	state.openAsh(index)
	payload, _ := aSpirit(t, state)

	placed := func(want application.Placement) {
		t.Helper()
		if text, bad := state.Notice(); bad {
			t.Fatalf("refused: %s", text)
		}
		staged := state.session.Staged().Formations
		if len(staged) != 1 {
			t.Fatalf("%d formations are staged, want 1", len(staged))
		}
		last := staged[0].Formation.Slots[len(staged[0].Formation.Slots)-1]
		if last.Place != want {
			t.Errorf("the summon is staged at %+v, want %+v", last.Place, want)
		}
	}

	state.dropAt(ash, application.Placement{X: 3, Z: -2}, payload)
	placed(application.Placement{X: 3, Z: -2})

	slot := ash.Ships // the first free slot
	state.moveSlot(ash, slot, -4, 5)
	placed(application.Placement{X: -4, Z: 5})

	state.turnSlot(ash, slot, snapAngle(97))
	placed(application.Placement{X: -4, Z: 5, Angle: 90})
}

// Every summon stands in one cell. Ten slots with no place of their own - the
// writer's ring, every one between cells - are drawn in ten different cells,
// none of them the player's; and once the formation is edited, every summon
// is written in the cell it is drawn in.
func TestEverySummonStandsInOneCell(t *testing.T) {
	var ring [maxSlots]slotView
	for index := range ring {
		ring[index] = slotView{Filled: true}
	}
	cells := map[[2]float64]bool{{0, 0}: true}
	for index, place := range cellPlaces(ring) {
		at := [2]float64{place.X, place.Z}
		if place.X != math.Round(place.X) || place.Z != math.Round(place.Z) || cells[at] {
			t.Errorf("slot %d stands at %v: between cells, on the player, or on another summon", index, at)
		}
		cells[at] = true
	}

	state := loadedState(t)
	index, ash := editable(t, state)
	state.openAsh(index)
	payload, _ := aSpirit(t, state)
	state.dropAt(ash, application.Placement{X: 3, Z: -2}, payload)
	for _, slot := range state.session.Staged().Formations[0].Formation.Slots {
		if slot.Place.X != math.Round(slot.Place.X) || slot.Place.Z != math.Round(slot.Place.Z) {
			t.Errorf("a summon is written at %+v, between cells", slot.Place)
		}
	}
}

// A body covers each cell it reaches into, turned with its facing, and two
// bodies warn only when they stand too near each other. Soldiers fit their
// cells and stay quiet a cell apart; a wolf is wider than half a cell and
// warns beside the player but not on the diagonal, which its round body does
// not reach; two wolves two cells apart reach a tenth of the cell between
// them, which neither covers, and stay quiet, a metre of ground apart; a
// horse behind the player reaches them with its head when it faces them, and
// covers the cell beside it only when turned side on, not with the 20 cm its
// flank reaches in facing ahead; a horse covers the cell its head fills, not
// the next one its nose reaches 40 cm into; Gaius's boar, 2.2 m wide, covers
// the cells beside it, which it fills six tenths of; and two dragons placed
// across each other warn though their ends are far apart.
func TestABodyCoversEveryCellItReachesAndCrowdsOnlyWhereItTouches(t *testing.T) {
	bodies := map[string]application.Body{
		"soldier": {Front: 0.4, Back: 0.4, Side: 0.4},
		"wolf":    {Front: 0.6, Back: 0.6, Side: 0.6},
		"horse":   {Front: 1.9, Back: 2.5, Side: 0.7},
		"boar":    {Front: 4.7, Back: 5, Side: 1.1},
		"dragon":  {Front: 10, Back: 10, Side: 1},
	}
	type seat struct {
		model string
		place application.Placement
	}
	held := func(seats ...seat) []int {
		var slots [maxSlots]slotView
		var places [maxSlots]application.Placement
		for index, one := range seats {
			slots[index], places[index] = slotView{Filled: true, Model: one.model}, one.place
		}
		return footprint(bodies, slots, places)
	}
	quiet := func(what string, cells []int) {
		for cell, use := range cells {
			if use == cellCrowded {
				x, z := groundOf(cell)
				t.Errorf("%s warn at %g,%g", what, x, z)
			}
		}
	}
	player := cellOf(0, 0)

	quiet("soldiers a cell apart", held(seat{"soldier", application.Placement{X: 1}},
		seat{"soldier", application.Placement{X: 1, Z: 1}}, seat{"soldier", application.Placement{X: 2}}))
	if held(seat{"wolf", application.Placement{X: 1}})[player] != cellCrowded {
		t.Error("a wolf beside the player does not warn on the player's cell")
	}
	if diagonal := held(seat{"wolf", application.Placement{X: 2, Z: 2}}); diagonal[cellOf(1, 1)] != cellEmpty {
		t.Error("a wolf covers the cell on its diagonal, which its round body does not reach")
	}
	apart := held(seat{"wolf", application.Placement{X: -1, Z: 2}}, seat{"wolf", application.Placement{X: 1, Z: 2}})
	if apart[cellOf(0, 2)] != cellEmpty {
		t.Error("two wolves two cells apart cover the cell between them, of which each fills a tenth")
	}
	quiet("two wolves two cells apart", apart)

	facing := held(seat{"horse", application.Placement{Z: -2}})
	sideOn := held(seat{"horse", application.Placement{Z: -2, Angle: 90}})
	if facing[player] != cellCrowded || sideOn[player] == cellCrowded {
		t.Error("a horse two metres behind does not warn facing the player, or warns side on")
	}
	if beside := cellOf(1, -2); facing[beside] != cellEmpty || sideOn[beside] != cellCovered {
		t.Error("the cell beside the horse is covered facing the player, or not side on")
	}
	if ahead := held(seat{"horse", application.Placement{Z: -4}}); ahead[cellOf(0, -3)] != cellCovered || ahead[cellOf(0, -2)] != cellEmpty {
		t.Error("a horse does not cover the cell its head fills, or covers the one its nose only reaches into")
	}
	if wide := held(seat{"boar", application.Placement{Z: 9}}); wide[cellOf(1, 9)] != cellCovered || wide[cellOf(-1, 9)] != cellCovered {
		t.Error("a boar 2.2 m wide does not cover the cells beside it")
	}

	crossed := held(seat{"dragon", application.Placement{Z: 12}}, seat{"dragon", application.Placement{X: 1, Z: 12, Angle: 90}})
	if crossed[cellOf(0, 12)] != cellCrowded {
		t.Error("two dragons placed across each other do not warn where they cross")
	}
}

// Sight is drawn one summon at a time: the chosen one's, and none while
// nobody is chosen. Its eyes see ahead in full colour, red for an enemy; its
// nose, all round and so behind too, is faint.
func TestOnlyTheChosenSummonsSightIsDrawn(t *testing.T) {
	state := loadedState(t)
	_, ash := editable(t, state)
	slots := state.slotsFor(ash)
	places := cellPlaces(slots)
	state.chosenSlot = noToken
	if _, shown := state.chosenViewer(slots, places); shown {
		t.Error("with nobody chosen a summon's sight is drawn")
	}
	state.chosenSlot = slices.IndexFunc(slots[:], func(slot slotView) bool { return slot.Filled && !slot.NoActor })
	if one, shown := state.chosenViewer(slots, places); !shown || one.place != places[state.chosenSlot] {
		t.Errorf("with slot %d chosen the sight drawn is %+v, %v", state.chosenSlot, one, shown)
	}

	all := application.Sight{Eye: 6, Cone: 180, Nose: 10} // notices everything near
	wolf := viewer{sight: all, place: application.Placement{X: -3, Z: 1}}
	if ink, _ := sightInk(wolf, -3, 0); ink.A >= colorAllyChosen.A || alpha(ink, colorAllyChosen.A) != colorAllyChosen {
		t.Errorf("the cell behind the wolf is %v, want its blue, faint", ink)
	}
	if ink, _ := sightInk(wolf, -3, 4); ink != colorAllyChosen {
		t.Errorf("the cell ahead of the wolf is %v, want its blue in full", ink)
	}
	wolf.enemy = true
	if ink, _ := sightInk(wolf, -3, 4); ink != colorEnemyChosen {
		t.Errorf("the cell ahead of an enemy wolf is %v, want red", ink)
	}
}

// The grid answers the pointer through Gio's own router: the wheel zooms about
// the pointer, a drag on the ground looks around as far as the ground goes, a
// summon's knob turns it, and the arrow keys turn the chosen one. The press on
// the knob also reaches the ground under it, and must not pan. A summon turned
// is chosen, so its knob stays on it when the pointer leaves, rather than
// going back to the summon chosen before.
func TestTheGridZoomsPansAndTurns(t *testing.T) {
	state := loadedState(t)
	draw(t, state) // the frame that takes the load in, which closes any Ash
	index, ash := editable(t, state)
	state.openAsh(index)
	payload, _ := aSpirit(t, state)
	state.dropAt(ash, application.Placement{X: 3, Z: 0}, payload)
	slot := ash.Ships // the first free slot
	state.dropAt(ash, application.Placement{X: -2, Z: 2}, payload)
	other := state.chosenSlot // a drop chooses the summon it adds
	if other == slot || other == noToken {
		t.Fatalf("the second drop chose slot %d; the first is %d", other, slot)
	}

	router := new(input.Router)
	size := image.Pt(900, 700)
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		gtx := layout.Context{Ops: new(op.Ops), Metric: metric, Constraints: layout.Exact(size), Source: router.Source()}
		state.layoutPlacement(gtx, state.Theme(), state.LoadedSave(), ash, true)
		router.Frame(gtx.Ops)
	}
	geo := func() gridGeometry { return newGridGeometry(size, state.gridView) }
	mouse := func(kind pointer.Kind, at f32.Point, buttons pointer.Buttons) pointer.Event {
		return pointer.Event{Kind: kind, Source: pointer.Mouse, Position: at, Buttons: buttons}
	}
	staged := func() application.Placement {
		t.Helper()
		return state.placeOf(state.slotsFor(ash), slot)
	}
	frame()

	// A notch away from you comes nearer, and the ground under the pointer
	// stays under it.
	at := f32.Pt(250, 520)
	x, z := geo().toGround(at)
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: at, Scroll: f32.Pt(0, -wheelNotch)})
	frame()
	if state.gridView.reach >= homeReach {
		t.Fatalf("the wheel left the view at %.2f m", state.gridView.reach)
	}
	if nx, nz := geo().toGround(at); math.Abs(nx-x) > 1e-3 || math.Abs(nz-z) > 1e-3 {
		t.Errorf("zooming moved the ground under the pointer from %.2f,%.2f to %.2f,%.2f", x, z, nx, nz)
	}

	// A long drag on empty ground, to the left: the view goes right, and stops
	// where the ground does.
	state.gridView = homeView
	frame()
	from := geo().toPx(-4, -4)
	router.Queue(mouse(pointer.Press, from, pointer.ButtonPrimary),
		mouse(pointer.Move, from.Sub(f32.Pt(3000, 0)), pointer.ButtonPrimary),
		mouse(pointer.Release, from.Sub(f32.Pt(3000, 0)), 0))
	frame()
	if state.gridView.x != groundReach || state.gridView.z != 0 {
		t.Errorf("the drag left the view at %.2f,%.2f, want %d,0", state.gridView.x, state.gridView.z, groundReach)
	}

	// Over a summon that is not the chosen one its knob shows, and only its:
	// the chosen one's shows only under the pointer too. Dragged round to its
	// right, the summon faces right, and the view stays where it was.
	state.gridView = homeView
	frame()
	place := staged()
	centre := geo().toPx(place.X, place.Z)
	router.Queue(mouse(pointer.Move, centre, 0))
	frame()
	if shown := state.knobsShown(state.slotsFor(ash), true); !slices.Equal(shown, []int{slot}) {
		t.Fatalf("over slot %d with %d chosen, the knobs show on %v", slot, other, shown)
	}
	knob := geo().knobAt(layout.Context{Metric: metric}, place)
	ground := geo().toPx(-4, -4)
	router.Queue(mouse(pointer.Move, knob, 0),
		mouse(pointer.Press, knob, pointer.ButtonPrimary),
		mouse(pointer.Move, centre.Add(f32.Pt(120, 0)), pointer.ButtonPrimary),
		mouse(pointer.Release, centre.Add(f32.Pt(120, 0)), 0),
		mouse(pointer.Move, ground, 0))
	frame()
	if got := staged().Angle; got != 90 {
		t.Errorf("the summon faces %g after the knob was dragged to its right, want 90", got)
	}
	if state.gridView != homeView {
		t.Errorf("turning the summon moved the view to %+v", state.gridView)
	}
	if shown := state.knobsShown(state.slotsFor(ash), true); len(shown) != 0 {
		t.Errorf("once the pointer left the summon it turned, the knobs show on %v, want none", shown)
	}
	state.showKnobs.Value = false
	if shown := state.knobsShown(state.slotsFor(ash), true); len(shown) != 0 {
		t.Errorf("with the knobs switched off, they show on %v", shown)
	}
	state.showKnobs.Value = true

	// Chosen, it turns back a step on the left arrow.
	frame()
	router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	frame()
	if got := staged().Angle; got != 90-turnStep {
		t.Errorf("the left arrow left the summon facing %g, want %g", got, 90-turnStep)
	}
}

// The click rules, the same with a mouse and a touchpad. A left click on a
// summon chooses it and opens nothing, however long the player waits; a tap
// and then a tap held and dragged - a touchpad's drag - moves it; a right
// click opens its window at once; and a click on empty ground lets the chosen
// summon go, unless a press held and dragged follows it - a touchpad's way to
// move the view. (Esc does not let it go: TestEscAsksBeforeClosingTheEditor.)
func TestTheClickRules(t *testing.T) {
	state := loadedState(t)
	draw(t, state) // the frame that takes the load in, which closes any Ash
	index, ash := editable(t, state)
	state.openAsh(index)
	payload, _ := aSpirit(t, state)
	state.dropAt(ash, application.Placement{X: 3, Z: 0}, payload)
	slot := ash.Ships // the first free slot
	state.chosenSlot = noToken

	router := new(input.Router)
	size := image.Pt(900, 700)
	now := time.Unix(1000, 0)
	frame := func() {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size), Source: router.Source(), Now: now}
		state.layoutPlacement(gtx, state.Theme(), state.LoadedSave(), ash, true)
		router.Frame(gtx.Ops)
	}
	geo := newGridGeometry(size, homeView)
	press := func(at f32.Point, buttons pointer.Buttons) pointer.Event {
		return pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: at, Buttons: buttons,
			Time: time.Duration(now.UnixNano())}
	}
	let := func(kind pointer.Kind, at f32.Point) pointer.Event {
		return pointer.Event{Kind: kind, Source: pointer.Mouse, Position: at, Buttons: pointer.ButtonPrimary}
	}
	at := geo.toPx(3, 0)
	frame()

	router.Queue(press(at, pointer.ButtonPrimary))
	frame()
	router.Queue(let(pointer.Release, at))
	frame()
	now = now.Add(2 * state.doubleTap)
	frame()
	if state.chosenSlot != slot || state.creatureSlot != noToken {
		t.Fatalf("after a left click and a wait the chosen slot is %d and the window is on %d, want %d and none",
			state.chosenSlot, state.creatureSlot, slot)
	}

	router.Queue(press(at, pointer.ButtonPrimary), let(pointer.Release, at))
	frame()
	now = now.Add(state.doubleTap / 3)
	to := geo.toPx(-2, 2)
	router.Queue(press(at, pointer.ButtonPrimary), let(pointer.Move, to), let(pointer.Release, to))
	frame()
	if place := state.placeOf(state.slotsFor(ash), slot); place.X != -2 || place.Z != 2 {
		t.Errorf("a tap and a tap dragged left the summon at %g,%g, want -2,2", place.X, place.Z)
	}

	// The frame that took the drag drew the summon where it was let go, so a
	// click there at once finds it (a stale frame drew it where it had been).
	router.Queue(press(to, pointer.ButtonSecondary), let(pointer.Release, to))
	frame()
	if state.creatureSlot != slot {
		t.Errorf("a right click left the window on %d, want %d at once", state.creatureSlot, slot)
	}
	state.creatureSlot = noToken

	ground, aside := geo.toPx(-4, -4), geo.toPx(-1, -4)
	now = now.Add(2 * state.doubleTap)
	router.Queue(press(ground, pointer.ButtonPrimary), let(pointer.Release, ground))
	frame()
	now = now.Add(state.doubleTap / 3)
	router.Queue(press(ground, pointer.ButtonPrimary), let(pointer.Move, aside), let(pointer.Release, aside))
	frame()
	if state.chosenSlot != slot {
		t.Errorf("a tap on the ground and a tap dragged left the chosen slot %d, want %d", state.chosenSlot, slot)
	}
	now = now.Add(2 * state.doubleTap)
	router.Queue(press(ground, pointer.ButtonPrimary), let(pointer.Release, ground))
	frame()
	if state.chosenSlot != noToken || state.creatureSlot != noToken {
		t.Errorf("a click on the ground left the chosen slot %d and the window on %d, want none",
			state.chosenSlot, state.creatureSlot)
	}
}

// Ctrl-click and passing over with Shift pick several summons, and they go
// together: dragging one moves them all as far, the arrow keys turn them all,
// and dragging one off the grid removes them all. A group never stands on a
// summon it left behind, a plain click picks one alone again, and a Shift
// sweep leaves the chosen one chosen. (A click on the ground lets one go as it
// lets all go: TestTheClickRules.)
func TestCtrlAndShiftPickSummonsThatMoveTurnAndGoTogether(t *testing.T) {
	state := loadedState(t)
	draw(t, state) // the frame that takes the load in, which closes any Ash
	index, ash := editable(t, state)
	state.openAsh(index)
	payload, _ := aSpirit(t, state)
	var a, b, c int
	for slot, at := range map[*int]application.Placement{&a: {X: 4, Z: 3}, &b: {X: -4, Z: 3}, &c: {X: -3, Z: 5}} {
		state.dropAt(ash, at, payload)
		*slot = state.chosenSlot
	}

	router := new(input.Router)
	size := image.Pt(900, 700)
	frame := func() {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size), Source: router.Source()}
		state.layoutPlacement(gtx, state.Theme(), state.LoadedSave(), ash, true)
		router.Frame(gtx.Ops)
	}
	geo := newGridGeometry(size, homeView)
	mouse := func(kind pointer.Kind, at f32.Point, buttons pointer.Buttons, mods key.Modifiers) pointer.Event {
		return pointer.Event{Kind: kind, Source: pointer.Mouse, Position: at, Buttons: buttons, Modifiers: mods}
	}
	click := func(x, z float64, mods key.Modifiers) {
		at := geo.toPx(x, z)
		router.Queue(mouse(pointer.Press, at, pointer.ButtonPrimary, mods), mouse(pointer.Release, at, 0, mods))
		frame()
	}
	drag := func(x, z float64, to f32.Point) {
		from := geo.toPx(x, z)
		router.Queue(mouse(pointer.Press, from, pointer.ButtonPrimary, 0),
			mouse(pointer.Move, to, pointer.ButtonPrimary, 0), mouse(pointer.Release, to, 0, 0))
		frame()
	}
	at := func(slot int) (float64, float64, float64) {
		place := state.placeOf(state.slotsFor(ash), slot)
		return place.X, place.Z, place.Angle
	}
	frame()

	click(4, 3, 0)
	click(-4, 3, key.ModCtrl)
	drag(4, 3, geo.toPx(4, 4))
	if ax, az, _ := at(a); ax != 4 || az != 4 {
		t.Errorf("the dragged summon stands at %g,%g, want 4,4", ax, az)
	}
	if bx, bz, _ := at(b); bx != -4 || bz != 4 {
		t.Errorf("the summon picked with it stands at %g,%g, want -4,4", bx, bz)
	}
	if cx, cz, _ := at(c); cx != -3 || cz != 5 {
		t.Errorf("the summon not picked moved to %g,%g", cx, cz)
	}

	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	frame()
	if _, _, turned := at(a); turned != turnStep {
		t.Errorf("the right arrow turned the chosen summon to %g, want %g", turned, turnStep)
	}
	if _, _, turned := at(b); turned != turnStep {
		t.Errorf("the right arrow turned the summon picked with it to %g, want %g", turned, turnStep)
	}
	if _, _, turned := at(c); turned != 0 {
		t.Errorf("the right arrow turned the summon not picked to %g", turned)
	}

	// One step ahead and to the right would put the second on the third.
	drag(4, 4, geo.toPx(5, 5))
	if ax, az, _ := at(a); ax != 4 || az != 4 {
		t.Errorf("a group moved onto a summon it left behind: the dragged one stands at %g,%g", ax, az)
	}
	if _, bad := state.Notice(); !bad {
		t.Error("a group refused its move without saying why")
	}

	click(-3, 5, 0)
	if state.isPicked(a) || state.isPicked(b) || !state.isPicked(c) {
		t.Errorf("a plain click left %v picked, want the third alone", state.picked())
	}
	router.Queue(mouse(pointer.Move, geo.toPx(0, 1), 0, key.ModShift),
		mouse(pointer.Move, geo.toPx(4, 4), 0, key.ModShift))
	frame()
	if state.chosenSlot != c || !state.isPicked(a) {
		t.Errorf("a Shift sweep over the first left %v picked and %d chosen, want both, the third chosen",
			state.picked(), state.chosenSlot)
	}
	drag(4, 4, f32.Pt(-40, 350))
	slots := state.slotsFor(ash)
	if slots[a].Filled || slots[c].Filled || !slots[b].Filled {
		t.Errorf("dragging one of two picked off the grid left them %v and %v, and the third %v",
			slots[a].Filled, slots[c].Filled, slots[b].Filled)
	}
}

// Two summons never share a cell. One dropped on another's cell takes its
// slot, so an Ash of ten takes it too; one moved onto another's cell takes
// the cell and the other leaves; and a cell the save gives twice is drawn -
// and, once edited, written - for the first summon only.
func TestTwoSummonsNeverShareACell(t *testing.T) {
	state := loadedState(t)
	index, ash := editable(t, state)
	state.openAsh(index)
	payload, _ := aSpirit(t, state)
	state.dropAt(ash, application.Placement{X: 3, Z: 0}, payload)
	first := state.chosenSlot
	state.dropAt(ash, application.Placement{X: -3, Z: 0}, payload)
	second := state.chosenSlot
	count := len(filledSlots(state, ash))

	state.dropAt(ash, application.Placement{X: 3, Z: 0}, payload)
	if state.chosenSlot != first || len(filledSlots(state, ash)) != count {
		t.Errorf("a drop on slot %d's cell went to slot %d, %d summons, want slot %d and %d",
			first, state.chosenSlot, len(filledSlots(state, ash)), first, count)
	}

	state.moveSlot(ash, second, 3, 0)
	slots := state.slotsFor(ash)
	if slots[first].Filled || !slots[second].Filled || len(filledSlots(state, ash)) != count-1 {
		t.Errorf("after a move onto its cell slot %d is filled %t and slot %d %t, %d summons",
			first, slots[first].Filled, second, slots[second].Filled, len(filledSlots(state, ash)))
	}

	var twice [maxSlots]slotView
	twice[0] = slotView{Filled: true, Placed: true, Place: application.Placement{X: 2, Z: 2}}
	twice[1] = slotView{Filled: true, Placed: true, Place: application.Placement{X: 2, Z: 2}}
	places := cellPlaces(twice)
	if places[0].X != 2 || places[0].Z != 2 || places[1].X == 2 && places[1].Z == 2 {
		t.Errorf("a cell given twice is drawn at %+v and %+v", places[0], places[1])
	}
}

func filledSlots(state *State, ash *ashView) []int {
	var out []int
	for index, slot := range state.slotsFor(ash) {
		if slot.Filled {
			out = append(out, index)
		}
	}
	return out
}

// The portrait turns a quarter at a time: upright to 45 degrees either way, on
// its side to 135, and upside down beyond, looking back at the player.
func TestAPortraitTurnsInQuarters(t *testing.T) {
	for angle, want := range map[float64]float64{
		0: 0, 45: 0, -45: 0, 60: 90, 90: 90, 120: 90, -90: -90,
		135: 180, -135: 180, 180: 180, 225: 180, 270: -90,
	} {
		if got := portraitTurn(angle); got != want {
			t.Errorf("a summon facing %g has its portrait turned %g, want %g", angle, got, want)
		}
	}
}

// The divider between the Editor's halves sizes the catalog where it is
// dragged to, and the width is stored when it is let go.
func TestTheDividerSizesTheCatalog(t *testing.T) {
	state := loadedState(t)
	state.view = viewEditor
	router := new(input.Router)
	size := image.Pt(1400, 800)
	frame := func() {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size), Source: router.Source()}
		state.layoutEditor(gtx, state.Theme())
		router.Frame(gtx.Ops)
	}
	frame()
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	before := state.catalogPx(gtx, size.X)
	divider := f32.Pt(float32(size.X-before)-float32(splitGap)/2, 400)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: divider, Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: divider.Sub(f32.Pt(150, 0)), Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: divider.Sub(f32.Pt(150, 0))})
	frame()
	if got := state.catalogPx(gtx, size.X); got != before+150 {
		t.Errorf("the catalog is %d px after the divider moved 150 px left from %d", got, before)
	}
	if stored := loadSettings().CatalogWidth; stored != float32(before+150) {
		t.Errorf("the stored catalog width is %g, want %d", stored, before+150)
	}
}

// A text field keeps the keys only until the player turns to the grid: a
// press on the ground hands the arrows back to the chosen summon. And a click
// into a number field selects what it holds, so the number typed is the
// number staged rather than one joined to the old.
func TestTheFieldsHandTheKeysBackToTheGrid(t *testing.T) {
	state := loadedState(t)
	draw(t, state) // the frame that takes the load in, which closes any Ash
	index, ash := editable(t, state)
	state.view = viewEditor
	state.openAsh(index)
	payload, _ := aSpirit(t, state)
	state.dropAt(ash, application.Placement{X: 3, Z: 0}, payload)
	slot := state.chosenSlot // a drop chooses the summon it adds
	h := newHands(t, state)

	// The price fields are the two in the bar over the grid, FP first.
	var fields []image.Rectangle
	for bounds := range h.controls(true) {
		if bounds.Max.X < 770 {
			fields = append(fields, bounds)
		}
	}
	slices.SortFunc(fields, func(a, b image.Rectangle) int { return a.Min.X - b.Min.X })
	if len(fields) != 2 {
		t.Fatalf("the bar holds %d fields, want FP and HP", len(fields))
	}
	h.clickAt(layout.FPt(fields[0].Min.Add(fields[0].Max).Div(2)))
	h.type_("70")
	if costs := state.session.Staged().Costs; len(costs) != 1 || costs[0].Price.FP != 70 {
		t.Errorf("70 typed into the FP field holding %q staged %+v", state.fpField.Text(), costs)
	}
	// Free closes the two fields: they show the price and are fields no more.
	h.click("Free")
	if price, _ := state.session.StagedPrice(ash.GoodsFamily); !price.Free() || len(h.controls(true)) != 1 {
		t.Errorf("Free staged %+v and left %d fields open, want the search alone", price, len(h.controls(true)))
	}
	h.click("Free")

	// A drag from the ground's far corner, nowhere near a summon: a click
	// there would let the summon go.
	from, to := f32.Pt(60, 640), f32.Pt(100, 640)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: from},
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: from},
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: to},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: to})
	h.frame()
	angle := state.placeOf(state.slotsFor(ash), slot).Angle
	h.press(key.NameRightArrow)
	if got := state.placeOf(state.slotsFor(ash), slot).Angle; got != angle+turnStep {
		t.Errorf("after a price was typed and the ground dragged, the right arrow left the summon facing %g, want %g",
			got, angle+turnStep)
	}
}

// A summon's whole name shows while the pointer is on it, and stays when the
// pointer moves onto its turning knob, which sits on the token's edge.
func TestASummonsNameShowsOverItAndOverItsKnob(t *testing.T) {
	state := loadedState(t)
	draw(t, state)
	index, ash := editable(t, state)
	state.view = viewEditor
	state.openAsh(index)
	state.dropAt(ash, application.Placement{X: 2, Z: 3}, payload(aCreature(t, state)))
	slot := ash.Ships
	name := state.slotsFor(ash)[slot].Name
	h := newHands(t, state)
	shows := func() bool {
		return slices.ContainsFunc(h.screen(), func(line string) bool { return strings.HasSuffix(line, " "+name) })
	}

	state.hoverSlot, state.knobHover = slot, noToken
	h.frame()
	if !shows() {
		t.Fatalf("pointing at the summon does not show %q", name)
	}
	state.hoverSlot, state.knobHover = noToken, slot
	h.frame()
	if !shows() {
		t.Errorf("pointing at its knob hides %q", name)
	}
}
