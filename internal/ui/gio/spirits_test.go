package gio

// Browsing the hierarchy, and the chooser behind the cards that hold a choice.
//
// The panel builds from research's browse hierarchy, so these
// tests hold the panel's BEHAVIOUR - the depths, the click, the drag, the
// filter, the switch - and not the vocabulary: which categories, cards, types
// and words the hierarchy holds is research's, changes without this package,
// and is pinned there (check_record.py), not here.

import (
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// A click on a catalog card opens it at once, and a double click leaves it
// open; a tap and then a tap held and dragged - a touchpad's drag - leaves it
// closed, so the drag is only a drag of the card.
func TestACardOpensAtOnceAndNotUnderADrag(t *testing.T) {
	state := loadedState(t)
	state.view = viewEditor
	router := new(input.Router)
	size := image.Pt(1400, 800)
	now := time.Unix(1000, 0)
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		gtx := layout.Context{Ops: new(op.Ops), Metric: metric, Constraints: layout.Exact(size),
			Source: router.Source(), Now: now}
		state.layoutEditor(gtx, state.Theme())
		router.Frame(gtx.Ops)
	}
	mouse := func(kind pointer.Kind, at f32.Point, buttons pointer.Buttons) pointer.Event {
		return pointer.Event{Kind: kind, Source: pointer.Mouse, Position: at, Buttons: buttons}
	}
	frame()

	// The first card is found where the pointer first rests on it, down the
	// catalog's first column.
	card := state.visibleFamilies(state.LoadedSave())[0]
	x := float32(size.X - state.catalogPx(layout.Context{Metric: metric}, size.X) + 60)
	var at f32.Point
	for y := float32(0); y < float32(size.Y) && at == (f32.Point{}); y += 6 {
		router.Queue(mouse(pointer.Move, f32.Pt(x, y), 0))
		frame()
		if state.familyBtns[card].Hovered() {
			at = f32.Pt(x, y+12)
		}
	}
	if at == (f32.Point{}) {
		t.Fatal("the first card is nowhere in the catalog")
	}

	// A hand's click: the press and the release come in two frames.
	router.Queue(mouse(pointer.Press, at, pointer.ButtonPrimary))
	frame()
	router.Queue(mouse(pointer.Release, at, 0))
	frame()
	if state.family != card {
		t.Fatal("a click did not open the card at once")
	}
	now = now.Add(state.doubleTap / 3)
	router.Queue(mouse(pointer.Press, at, pointer.ButtonPrimary), mouse(pointer.Release, at, 0))
	frame()
	if state.family != card {
		t.Fatal("the second click of a double click closed the card")
	}

	now = now.Add(2 * state.doubleTap)
	frame()
	state.closeFamily()
	frame()
	router.Queue(mouse(pointer.Press, at, pointer.ButtonPrimary), mouse(pointer.Release, at, 0))
	frame()
	now = now.Add(state.doubleTap / 3)
	router.Queue(mouse(pointer.Press, at, pointer.ButtonPrimary))
	frame()
	away := f32.Pt(300, 400)
	router.Queue(mouse(pointer.Move, away, pointer.ButtonPrimary))
	frame()
	frame()
	router.Queue(mouse(pointer.Release, away, 0))
	frame()
	if state.family != noFamily {
		t.Error("a tap and a tap dragged away left the card open")
	}
}

// everyVariant is every actor a card can deliver, across all of its entries.
func everyVariant(family familyView) []variantView {
	all := make([]variantView, 0, family.spirits())
	for _, entry := range family.Entries {
		all = append(all, entry.Variants...)
	}
	return all
}

// aDeepFamily is a card with all three depths under it: several entries, one
// of which has several variants. It returns the card and that entry.
func aDeepFamily(t *testing.T, state *State) (int, int) {
	t.Helper()
	for index, family := range state.LoadedSave().Families {
		if len(family.Entries) < 2 {
			continue
		}
		for at, entry := range family.Entries {
			if len(entry.Variants) > 1 {
				return index, at
			}
		}
	}
	t.Fatal("the fixture holds no card with entries and variants under it - see browseRows")
	return noFamily, noEntry
}

// A card opens in place: its entries framed under its row with the rest of
// the catalog still drawn, an entry's movesets one frame deeper, each named by
// its own words alone, and a second click on either closes it.
func TestACardOpensInPlaceAndASecondClickClosesIt(t *testing.T) {
	state := browseState(t)
	loaded := state.LoadedSave()
	deep, entry := aDeepFamily(t, state)
	family := loaded.Families[deep]
	open := family.Entries[entry]
	saying := func(name, subtitle string) string {
		if subtitle == "" {
			return name
		}
		return name + " " + subtitle
	}
	card := saying(family.Name, family.subtitle())
	model := saying(open.Name, open.subtitle())
	// A moveset is dragged, not clicked, so it is found by its words: its own,
	// capitalised, without the entry's name its tile sits under.
	moveset := open.Variants[0].Name
	if moveset == "" {
		t.Fatalf("%s's first moveset has no words of its own - see browseRows", open.Name)
	}
	moveset = strings.ToUpper(moveset[:1]) + moveset[1:]

	h := newHands(t, state) // its first frame takes the load in
	state.chooseKind(slices.Index(loaded.Categories, family.Category))
	h.frame()
	shows := func(label string) bool {
		_, ok := h.find(label)
		return ok || slices.ContainsFunc(h.screen(), func(line string) bool { return strings.HasSuffix(line, " "+label) })
	}
	// Down the catalog until the card is on screen.
	for row := 0; !shows(card); row++ {
		if row > len(loaded.Families) {
			t.Fatalf("%q is nowhere in the catalog", card)
		}
		state.familyList.Position.First, state.familyList.Position.Offset = row, 0
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label)
		h.wait(state.doubleTap + time.Millisecond)
	}

	click(card)
	if state.family != deep || !shows(model) || !shows(card) {
		t.Fatalf("after a click %s is open %t, shows %q %t, and the card is still drawn %t",
			family.Name, state.family == deep, model, shows(model), shows(card))
	}
	click(model)
	if state.entry != entry || !shows(moveset) || !shows(card) {
		t.Fatalf("after a click %s is open %t and shows %q %t, the card still drawn %t",
			open.Name, state.entry == entry, moveset, shows(moveset), shows(card))
	}
	click(model)
	if state.entry != noEntry || shows(moveset) {
		t.Errorf("a second click left %s open %t, %q shown %t", open.Name, state.entry == entry, moveset, shows(moveset))
	}
	click(card)
	if state.family != noFamily || shows(model) {
		t.Errorf("a second click left %s open %t, %q shown %t", family.Name, state.family == deep, model, shows(model))
	}
}

// Every variant on screen is a row the panel can stage: a creature with its
// two rows, a person with the shell and its CharaInitParam row - and never a
// held one, which the session already dropped.
func TestEveryVariantIsStageableAndNoneIsHeld(t *testing.T) {
	state := loadedState(t)
	people, creatures := 0, 0
	for _, family := range state.LoadedSave().all {
		for _, variant := range everyVariant(family) {
			switch variant.Trunk {
			case "creature":
				creatures++
				if variant.Npc == 0 || variant.Think == 0 || variant.Person != 0 {
					t.Errorf("%s: a creature with rows %d/%d/%d", variant.Key, variant.Npc, variant.Think, variant.Person)
				}
			case "person":
				people++
				// The default shell, but for an Ashes person, which arrives on
				// exactly its own (the shell tag) and never on the default.
				shell := DefaultPersonShell
				if variant.Shipped {
					shell = int32(mustAtoi(t, variant.Tags["shell"]))
				}
				if variant.Npc != shell || variant.Person == 0 {
					t.Errorf("%s: a person with rows %d/%d/%d, want shell %d", variant.Key, variant.Npc, variant.Think, variant.Person, shell)
				}
			default:
				t.Errorf("%s: trunk %q", variant.Key, variant.Trunk)
			}
			if variant.Offer == "hold" || variant.Offer == "" {
				t.Errorf("%s reached the panel with offer %q", variant.Key, variant.Offer)
			}
		}
	}
	if people == 0 || creatures == 0 {
		t.Errorf("%d creatures and %d people on the panel; both trunks must be there", creatures, people)
	}
}

// Every variant reads in the catalog's one label style: its entry's name, and
// its own words after it in parentheses ("Red Bear (brown fur)") - in the
// chooser and in a filled slot alike.
func TestAVariantIsNamedByItsEntryThenItsOwnWords(t *testing.T) {
	view := browseState(t).LoadedSave()
	words := 0
	for _, family := range view.all {
		for _, entry := range family.Entries {
			for _, variant := range entry.Variants {
				want := entry.Name
				if variant.Name != "" {
					want += " (" + variant.Name + ")"
					words++
				}
				if variant.Label != want {
					t.Errorf("%s shows %q, want %q", variant.Key, variant.Label, want)
				}
				held, _, ok := view.findVariant(func(one variantView) bool { return one.Key == variant.Key })
				if !ok || held.Name != want {
					t.Errorf("a slot holding %s says %q, want %q", variant.Key, held.Name, want)
				}
			}
		}
	}
	if words == 0 {
		t.Fatal("no variant on the panel has words of its own")
	}
}

// browseRows are the world actors the panel's navigation tests need. The
// shared fixture carries only Malenia's two, so no creature card holds a
// choice there. The Bear card (c6031)
// has two entries, Bear and Red Bear, and Red Bear two variants: all three
// depths. The Royal Revenant (c4020) is one entry with two variants.
var browseRows = struct{ npc, think []int32 }{
	npc:   []int32{60310010, 54210090, 54211093, 40200040, 40208022},
	think: []int32{60310000, 54210000, 40200000},
}

// browseState is loadedState on a fixture that also carries browseRows.
func browseState(t *testing.T) *State {
	t.Helper()
	return stateWithWorldRows(t, browseRows.npc, browseRows.think)
}

// stateWithWorldRows is loadedState on the vanilla fixture plus these
// NpcParam and NpcThinkParam rows.
func stateWithWorldRows(t *testing.T, npcs, thinks []int32) *State {
	t.Helper()
	options, err := savefixture.VanillaOptions()
	if err != nil {
		t.Fatal(err)
	}
	for index, table := range options.Tables {
		var add []int32
		switch table.Entry {
		case "NpcParam.param":
			add = npcs
		case "NpcThinkParam.param":
			add = thinks
		default:
			continue
		}
		rows := slices.Clone(table.RowIDs)
		for _, one := range add {
			if !slices.Contains(rows, one) {
				rows = append(rows, one)
			}
		}
		slices.Sort(rows)
		options.Tables[index].RowIDs = rows
	}
	save, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "memory.dat")
	if err := os.WriteFile(path, save, 0o600); err != nil {
		t.Fatal(err)
	}
	state := newTestState(t, picker{path: path})
	state.begin("test")
	state.openWorker()
	if state.LoadedSave() == nil {
		text, _ := state.Notice()
		t.Fatalf("the fixture did not load: %s", text)
	}
	state.view = viewEditor
	return state
}

// Every name on screen is a name you can type: the card, every entry and
// every variant answer a query for their own words.
func TestEveryNameOnScreenIsANameYouCanType(t *testing.T) {
	state := loadedState(t)
	for _, family := range state.LoadedSave().Families {
		for _, entry := range family.Entries {
			query := strings.ToLower(entry.Name)
			if !family.matches(query, family.Category) {
				t.Errorf("%s does not answer to its own entry %q", family.Name, entry.Name)
			}
			for _, variant := range entry.Variants {
				query := strings.ToLower(variant.Label)
				if !family.matches(query, family.Category) {
					t.Errorf("%s does not answer to its own variant %q", family.Name, variant.Label)
				}
			}
		}
	}
}
