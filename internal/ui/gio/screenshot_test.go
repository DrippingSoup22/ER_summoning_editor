//go:build screenshot

package gio

// A picture of one frame, for the developer who cannot see the window.
//
// Every other test here proves that a layout does not panic and that it claims
// the space it should. None of them can see it, and "the GUI is all over the
// place" is not a failure any of them can express. This renders a real frame
// through Gio's headless GPU path and writes a PNG, so a layout can be looked
// at without a display.
//
// Behind a build tag because it needs a GL or Vulkan context, which a machine
// running the test suite may not have. It is a developer tool, not a check:
//
//	go test -tags screenshot -run TestScreenshots ./internal/ui/gio/
//
// Pictures land in ER_SHOT_DIR, or the test's own temporary directory.

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

func shoot(t *testing.T, state *State, name string, size image.Point, scale float32) {
	t.Helper()
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("no headless GPU here: %v", err)
	}
	defer window.Release()

	gtx := headlessGtx()
	gtx.Metric = unit.Metric{PxPerDp: scale, PxPerSp: scale}
	gtx.Constraints = layout.Constraints{Max: size}
	ops := gtx.Ops
	state.Layout(gtx, state.Theme())
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(shot); err != nil {
		t.Fatal(err)
	}

	into := os.Getenv("ER_SHOT_DIR")
	if into == "" {
		into = t.TempDir()
	}
	path := filepath.Join(into, name+".png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, shot); err != nil {
		t.Fatal(err)
	}
	t.Log(path)
}

// shootScale is pixels per dp, and getting it wrong is how a layout that fits
// here overflows there.
//
// Gio takes the desktop's own scaling. Windows at 125% - which is what this
// ships to - means a maximised 1920x1080 screen is 1536x826 DP, not 1920x1080:
// a quarter less room in both directions than a Linux desktop at 100% reports
// for the same monitor. Every panel here is sized in dp, so a formation column
// that fit a 1010dp-tall window with room to spare ran off the bottom of an
// 826dp one, and no headless check at PxPerDp 1 could see it.
const shootScale = 1.25

// widestCard is the card holding the most entries, which is the one worth
// photographing: the drill is what these pictures are of, and a card with two
// entries shows less of it than a card with fourteen.
func widestCard(state *State) int {
	at := noFamily
	for index, family := range state.LoadedSave().Families {
		if len(family.Entries) < 2 {
			continue
		}
		if at == noFamily || len(family.Entries) > len(state.LoadedSave().Families[at].Entries) {
			at = index
		}
	}
	return at
}

// deepCard is a card with all three depths: several entries, and one of
// them several movesets, so both frames of an open card are in the picture.
func deepCard(state *State) int {
	for index, family := range state.LoadedSave().Families {
		if len(family.Entries) < 2 || len(family.Entries) > 6 {
			continue
		}
		for _, entry := range family.Entries {
			if len(entry.Variants) > 1 {
				return index
			}
		}
	}
	return widestCard(state)
}

// anEntryWithAChoice is the entry on a card with the most movesets behind it,
// so the third depth in the picture has something in it. Entry 0 usually does
// not: a card that holds fourteen models mostly holds one moveset each.
func anEntryWithAChoice(state *State, family int) int {
	entries := state.LoadedSave().Families[family].Entries
	at := 0
	for index := range entries {
		if len(entries[index].Variants) > len(entries[at].Variants) {
			at = index
		}
	}
	return at
}

// aLargeBody is a catalog variant whose body is several metres long, for a
// picture of the overlap tint.
func aLargeBody(state *State) (variantView, bool) {
	loaded := state.LoadedSave()
	for _, family := range loaded.all {
		for _, entry := range family.Entries {
			for _, variant := range entry.Variants {
				if body, known := loaded.bodies[variant.Model]; known && variant.Person == 0 && body.Front > 4 {
					return variant, true
				}
			}
		}
	}
	return variantView{}, false
}

// dressPupil dresses a new character in the builder - a heavy longsword with an
// Ash of War, a catalyst, armour, a talisman, a spell, items and arrows - and
// leaves the longsword's slot chosen.
func dressPupil(state *State, open *builder) {
	dress := func(slot builderSlot, key string) {
		open.slot = slot
		if item, ok := state.itemNamed(key); ok {
			state.put(open, item, state.LoadedSave())
		}
	}
	dress(builderSlot{handSlot, 1}, "weapon-2000000")
	open.draft.Hands[1].Affinity, open.draft.Hands[1].Level = 1, 25
	dress(builderSlot{handSlot, 0}, "weapon-30060000")
	dress(builderSlot{armourSlot, 0}, "protector-40000")
	dress(builderSlot{armourSlot, 1}, "protector-40100")
	dress(builderSlot{talismanSlot, 0}, "accessory-1000")
	dress(builderSlot{spellSlot, 0}, "magic-6000")
	dress(builderSlot{quickSlot, 0}, "goods-300")
	dress(builderSlot{quickSlot, 1}, "goods-820")
	dress(builderSlot{ammoSlot, 0}, "weapon-50000000")
	open.draft.Attributes = [8]uint8{40, 12, 25, 50, 18, 9, 15, 10}
	open.draft.Name = "Rennala's Pupil"
	open.name.SetText(open.draft.Name)
	dress(builderSlot{ashSlot, 1}, "gem-10000")
	open.slot = builderSlot{handSlot, 1}
}

// inThemes shoots the frame again in each theme but Spirit, then puts Spirit
// back.
func inThemes(t *testing.T, state *State, name string, size image.Point) {
	for _, theme := range themeKeys[1:] {
		state.settings.Theme = theme
		state.applyPalette()
		draw(t, state)
		shoot(t, state, name+"-"+theme, size, shootScale)
	}
	state.settings.Theme = "spirit"
	state.applyPalette()
	draw(t, state)
}

func TestScreenshots(t *testing.T) {
	// The window as it really is on the machine this ships to: maximised on a
	// 1920x1080 desktop at Windows' 125% scaling. The scaling is the whole
	// point of the constant - see shootScale.
	size := image.Pt(1920, 1032)

	empty := newTestState(t, picker{})
	shoot(t, empty, "characters-empty", size, shootScale)

	state := loadedState(t)
	state.view = viewCharacters
	draw(t, state)
	shoot(t, state, "characters", size, shootScale)

	// With somebody chosen, which is the half of the view the fixture cannot
	// reach on its own - it is built from the regulation and holds no
	// characters, so a slot is described here to draw the populated case.
	loaded := state.LoadedSave()
	loaded.Characters[0] = characterView{
		Slot: 0, Used: true, Name: "DrippingSoup", Level: 150, Runes: 421337,
		KnowsStorage: true, Owns: map[int32]bool{}, Levels: map[int32]int{},
	}
	for index, ash := range loaded.Ashes {
		if index%3 == 0 {
			loaded.Characters[0].Owns[ash.GoodsFamily] = true
			// Spread across the eleven so the picture shows the field doing
			// its job rather than a column of the same number.
			loaded.Characters[0].Levels[ash.GoodsFamily] = index % 11
		}
	}
	loaded.Characters[3] = characterView{
		Slot: 3, Used: true, Name: "Granny", Level: 62,
		KnowsStorage: false, Owns: map[int32]bool{},
	}
	state.person = 0
	draw(t, state)
	shoot(t, state, "characters-chosen", size, shootScale)

	// The real save, when ER_SAVE names the author's: eight
	// characters, six of them with one name, which is what the chooser has to
	// tell apart and the fixture cannot show. The second picture is scrolled to
	// the Kristoff Ash, the one Ash the Editor cannot change.
	if path := os.Getenv("ER_SAVE"); path != "" {
		real := newTestState(t, picker{path: path})
		real.begin("test")
		real.openWorker()
		if real.LoadedSave() != nil {
			real.view = viewCharacters
			draw(t, real)
			shoot(t, real, "characters-real", size, shootScale)
			inThemes(t, real, "characters-real", size)
			for index, ash := range real.LoadedSave().Ashes {
				if ash.GoodsFamily == 256000 {
					real.grantList.Position.First = index / 2
				}
			}
			draw(t, real)
			shoot(t, real, "characters-real-kristoff", size, shootScale)

			// Every missing Ash staged: the lit rows, their Undo, and the
			// toolbar button turned into the one that drops them.
			person := real.LoadedSave().Characters[real.person]
			real.grantEveryMissing(person, real.LoadedSave())
			real.grantList.Position.First = 0
			draw(t, real)
			shoot(t, real, "characters-real-staged", size, shootScale)
			real.dropGrants(person)

			// The Editor on the real save: the list of Ashes, then one open on
			// its grid, then the same with a large body set down beside the
			// summons so the overlap tint and the turn handle show.
			real.view = viewEditor
			draw(t, real)
			shoot(t, real, "editor-real-list", size, shootScale)
			// A card open in place, with an entry open inside it.
			if deep := deepCard(real); deep != noFamily {
				loaded := real.LoadedSave()
				real.chooseKind(slices.Index(loaded.Categories, loaded.Families[deep].Category))
				draw(t, real)
				real.family, real.reveal = deep, true
				real.entry = anEntryWithAChoice(real, deep)
				draw(t, real)
				shoot(t, real, "editor-real-open", size, shootScale)
				real.closeFamily()
				real.chooseKind(openingKind(loaded.Categories))
				draw(t, real)
			}
			// The marks' legend, then a card's note under the pointer, on the
			// enemies, where most marks are.
			loaded := real.LoadedSave()
			real.chooseKind(slices.Index(loaded.Categories, "enemy"))
			real.helpPinned = helpMarks
			draw(t, real)
			shoot(t, real, "editor-real-labels", size, shootScale)
			real.helpPinned = noHelp
			// The enemies' kinds, in the order the author ruled them.
			real.subCombo.open = true
			draw(t, real)
			shoot(t, real, "editor-real-kinds", size, shootScale)
			real.subCombo.open = false
			for _, index := range real.drawnFamilies {
				if family := loaded.Families[index]; len(family.Notes) > 0 {
					real.noteOver = noteTag{"card", family.Key}
					break
				}
			}
			draw(t, real)
			shoot(t, real, "editor-real-note", size, shootScale)
			real.noteOver = noteTag{}
			real.chooseKind(openingKind(loaded.Categories))
			draw(t, real)
			for index, ash := range real.LoadedSave().Ashes {
				if ash.Name == "Lone Wolf Ashes" {
					real.openAsh(index)
				}
			}
			draw(t, real)
			shoot(t, real, "editor-real-grid", size, shootScale)
			inThemes(t, real, "editor-real-grid", size)
			// A whole card dropped ahead of the wolves: its seven knights fill
			// the seven free slots, clear of each other. Then put back.
			if ash := real.selectedAsh(real.LoadedSave()); ash != nil {
				for _, family := range real.LoadedSave().Families {
					if family.Name == "Crucible Knight" {
						real.dropAt(ash, application.Placement{X: 0, Z: 3}, payloadOf(family.firstOfEach()))
						draw(t, real)
						shoot(t, real, "editor-real-card", size, shootScale)
						real.unstageFormation(ash.GoodsFamily)
						draw(t, real)
					}
				}
			}
			if ash := real.selectedAsh(real.LoadedSave()); ash != nil {
				if big, ok := aLargeBody(real); ok {
					real.dropAt(ash, application.Placement{X: 2, Z: 2, Angle: -45}, payload(big))
					real.chosenSlot = 3
					real.hoverSlot = 0
					draw(t, real)
					shoot(t, real, "editor-real-grid-big", size, shootScale)
					// The container's sight switch on: what the chosen summon
					// notices by itself.
					real.showSight.Value = true
					draw(t, real)
					shoot(t, real, "editor-real-sight", size, shootScale)
					// The large body as an enemy turned towards the wolves,
					// still chosen: its sight in red.
					real.hoverSlot = noToken
					real.setEnemy(ash, 3, true)
					real.turnSlot(ash, 3, -135)
					real.turnSlot(ash, 0, 180)
					draw(t, real)
					shoot(t, real, "editor-real-sight-enemy", size, shootScale)
					real.setEnemy(ash, 3, false)
					real.showSight.Value = false
					// Zoomed out and looked round, then home.
					real.gridView = gridView{x: 6, z: 4, reach: 14}
					draw(t, real)
					shoot(t, real, "editor-real-grid-far", size, shootScale)
					real.gridView = homeView
					// The divider dragged left: the catalog takes three cards
					// a row.
					real.settings.CatalogWidth = 760
					draw(t, real)
					shoot(t, real, "editor-real-wide-catalog", size, shootScale)
					real.settings.CatalogWidth = 0
					// The creature window: on the large body, then the same as
					// an enemy, then on a wolf kept as the save holds it, whose
					// role is locked.
					real.openCreature(3)
					draw(t, real)
					shoot(t, real, "editor-real-creature", size, shootScale)
					inThemes(t, real, "editor-real-creature", size)
					// An explanation kept open: Share aggro's, then the roles'.
					real.helpPinned = helpShare
					draw(t, real)
					shoot(t, real, "editor-real-creature-help", size, shootScale)
					real.helpPinned = helpRole
					draw(t, real)
					shoot(t, real, "editor-real-creature-help-role", size, shootScale)
					// A Fighter, with what the strengths mean kept open.
					real.setFighter(ash, 3, true)
					real.helpPinned = helpStrength
					draw(t, real)
					shoot(t, real, "editor-real-creature-help-strength", size, shootScale)
					real.setFighter(ash, 3, false)
					real.helpPinned = noHelp
					real.setEnemy(ash, 3, true)
					draw(t, real)
					shoot(t, real, "editor-real-creature-enemy", size, shootScale)
					real.openCreature(0)
					draw(t, real)
					shoot(t, real, "editor-real-creature-held", size, shootScale)
					// The large Shadow Commoner: two roles only, and a tint
					// of its own instead of a look to choose.
					real.dropAt(ash, application.Placement{X: -3, Z: 3}, "52410088,52410000,0")
					real.openCreature(4)
					draw(t, real)
					shoot(t, real, "editor-real-creature-limited", size, shootScale)
					// A creature of the game's own Ashes: only its strength
					// and its removal are offered, the rest locked.
					if variant, found := aShippedCreature(real); found {
						real.dropAt(ash, application.Placement{X: 3, Z: 3}, payload(variant))
						real.openCreature(real.chosenSlot)
						draw(t, real)
						shoot(t, real, "editor-real-creature-shipped", size, shootScale)
					}
					real.creatureSlot = noToken
					real.unstageFormation(ash.GoodsFamily)
				}
			}

			// The player's own characters: their category, the custom
			// characters' card open on the tile that adds one, the builder
			// on a new character as it opens and once dressed, and the card
			// again with the character staged on it.
			real.selected = noAsh
			real.chooseKind(slices.Index(real.LoadedSave().Categories, application.CustomCategory))
			draw(t, real)
			shoot(t, real, "editor-real-characters", size, shootScale)
			for index, family := range real.LoadedSave().Families {
				if family.custom {
					real.family, real.entry = index, noEntry
				}
			}
			draw(t, real)
			shoot(t, real, "editor-real-custom-empty", size, shootScale)
			real.openBuilder(noBuild)
			draw(t, real)
			shoot(t, real, "builder-new", size, shootScale)
			if open := real.builder; open != nil {
				dressPupil(real, open)
				draw(t, real)
				shoot(t, real, "builder-weapon", size, shootScale)
				inThemes(t, real, "builder-weapon", size)
				open.slot = builderSlot{ashSlot, 1}
				draw(t, real)
				shoot(t, real, "builder-ash", size, shootScale)
				open.slot = builderSlot{quickSlot, 0}
				draw(t, real)
				shoot(t, real, "builder-quick", size, shootScale)
				// Too weak for the Heavy Longsword and the incantation: warned of,
				// not refused.
				strong := open.draft.Attributes
				open.draft.Attributes[3], open.draft.Attributes[6] = 8, 5
				open.slot = builderSlot{handSlot, 1}
				draw(t, real)
				shoot(t, real, "builder-short", size, shootScale)
				open.draft.Attributes = strong
				real.finishBuilder(open)
				draw(t, real)
				shoot(t, real, "editor-real-custom-one", size, shootScale)
			}

			// Something of every kind a player can stage, and the dialog that
			// lists it: Ashes given to a character, one levelled and one taken;
			// a container recomposed with a new price; the game's own wolves
			// made Fighters; the custom character just built; and both
			// monument switches.
			if real.person >= 0 {
				person := real.LoadedSave().Characters[real.person]
				var owned, missing []int32
				for _, ash := range real.LoadedSave().Ashes {
					if person.Owns[ash.GoodsFamily] {
						owned = append(owned, ash.GoodsFamily)
					} else {
						missing = append(missing, ash.GoodsFamily)
					}
				}
				for _, family := range missing[:min(3, len(missing))] {
					real.session.StageGrant(person.Slot, family, 10)
				}
				if len(owned) > 1 {
					real.session.StageGrant(person.Slot, owned[0], 10)
					real.session.StageRemoval(person.Slot, owned[1])
				}
			}
			for index, ash := range real.LoadedSave().Ashes {
				if ash.Name == "Lone Wolf Ashes" {
					real.openAsh(index)
				}
			}
			if ash := real.selectedAsh(real.LoadedSave()); ash != nil {
				if big, ok := aLargeBody(real); ok {
					real.dropAt(ash, application.Placement{X: 2, Z: 2}, payload(big))
					real.setEnemy(ash, 3, true)
				}
				real.session.StageCost(ash.Name, application.Price{FP: 70})
				real.setFighter(ash, 0, true)
			}
			real.session.SetWideArea(true)
			real.session.SetNoRestLimit(true)
			real.pendingOpen = true
			draw(t, real)
			shoot(t, real, "pending", size, shootScale)
			// The characters folded away, which is how a player after one
			// price gets past every Ash a character was given.
			real.pendingFolded = map[string]bool{"Characters": true}
			draw(t, real)
			shoot(t, real, "pending-folded", size, shootScale)
			real.pendingFolded = nil
			real.pendingOpen = false
			// Esc with changes staged: the dialog that asks to close.
			real.quitOpen = true
			draw(t, real)
			shoot(t, real, "quit", size, shootScale)
			real.quitOpen = false
		}
	}

	// A character whose storage box could not be read, which is the one
	// caveat the view keeps on screen.
	state.person = 3
	draw(t, state)
	shoot(t, state, "characters-storage", size, shootScale)
	state.person = 0

	state.view = viewEditor
	// An Ash chosen, because the formation panel is what changed and an empty
	// one shows none of it.
	state.selected = 0
	draw(t, state)
	shoot(t, state, "editor", size, shootScale)

	// A slot holding something that has been seen to hurt the player who
	// summoned it. There is no way to check an ember border from a layout
	// assertion, and this is the picture that says whether the warning reads as
	// a caution or as a refusal. The fixture delivers neither of the two
	// families, so the label is pointed at one - the same trick and the same
	// reason as TestASummonThatCanHurtYouWarnsAndIsStillSummoned.
	if ash := state.selectedAsh(loaded); ash != nil {
		dangerous, _ := warnsAbout(t, state, "c5170")
		state.drop(ash, 0, state.slotsFor(ash), payloadFor(dangerous))
		draw(t, state)
		shoot(t, state, "editor-warned", size, shootScale)
		state.unstageFormation(ash.GoodsFamily)
		draw(t, state)
	}

	// The filter in force, which is the state the panel spends most of its
	// life in and the one the dropdown has to read correctly in.
	state.chooseKind(slices.Index(loaded.Categories, "boss"))
	draw(t, state)
	shoot(t, state, "editor-bosses", size, shootScale)

	// And the list open over the grid, which is the whole of what the deferred
	// overlay has to get right.
	state.kindCombo.open = true
	draw(t, state)
	shoot(t, state, "editor-categories", size, shootScale)
	state.kindCombo.open = false

	// The subcategory beside it, open, on the category with the most words
	// in the fixture - the npc's.
	if npc := slices.Index(loaded.Categories, "npc"); npc >= 0 {
		state.chooseKind(npc)
		draw(t, state)
		state.subCombo.open = true
		draw(t, state)
		shoot(t, state, "editor-subcategories", size, shootScale)
		state.subCombo.open = false
	}
	state.chooseKind(openingKind(loaded.Categories))

	// The three levels the catalog browses. This is the whole of the panel's
	// navigation, and it is also the only place the artwork gap can be looked
	// at: a card with no picture and an entry with no picture are the same
	// empty square one level apart, and no assertion can say which of them
	// reads as unfinished.
	//
	// A frame first, because restoring the filter above is a change to it and
	// the panel closes whatever is open whenever the catalog is narrowed or
	// widened. Setting the card before that frame would be setting it and
	// having it closed in the same breath.
	draw(t, state)
	if drilled := deepCard(state); drilled != noFamily {
		// In its own category, opened as a click opens it: scrolled to.
		state.chooseKind(slices.Index(loaded.Categories, loaded.Families[drilled].Category))
		draw(t, state)
		state.family, state.entry, state.reveal = drilled, noEntry, true
		draw(t, state)
		shoot(t, state, "editor-entries", size, shootScale)

		state.entry = anEntryWithAChoice(state, drilled)
		draw(t, state)
		shoot(t, state, "editor-variants", size, shootScale)

		state.closeFamily()
		state.chooseKind(openingKind(loaded.Categories))
		draw(t, state)
	}

	state.view = viewSettings
	draw(t, state)
	shoot(t, state, "settings", size, shootScale)
	inThemes(t, state, "settings", size)
	// An explanation open, which is all a row says about itself now: the
	// longest of the view's.
	state.helpPinned = helpWideArea
	draw(t, state)
	shoot(t, state, "settings-help", size, shootScale)
	state.helpPinned = noHelp

	// The font list open, which is the same deferred overlay as the catalog's
	// filter in a different place: it opens over the sliders rather than over a
	// grid, and the row it hangs off is the one the reset shares.
	state.fontCombo.open = true
	draw(t, state)
	shoot(t, state, "settings-font", size, shootScale)
	state.fontCombo.open = false

	// Both sizes are settings, so both grids are worth a picture away from the
	// default rather than only at it. The rail's width follows its own size, so
	// what these show is whether the panel beside it grows to match.
	state.view = viewCharacters
	for _, icon := range []float32{40, 140} {
		state.settings.IconSize = icon
		draw(t, state)
		shoot(t, state, fmt.Sprintf("characters-%ddp", int(icon)), size, shootScale)
	}
	state.settings.IconSize = float32(defaultIconSize)
	state.view = viewEditor
	for _, icon := range []float32{40, 120} {
		state.settings.EditorIconSize = icon
		draw(t, state)
		shoot(t, state, fmt.Sprintf("editor-%ddp", int(icon)), size, shootScale)
	}
	state.settings.EditorIconSize = float32(defaultIconSize)
}

// TestReadmePictures takes the README's pictures, one per part of the editor,
// on a real save opened as memory.dat, with the footer quiet:
//
//	ER_SHOT_DIR="$PWD/docs/images" go test -tags screenshot -run TestReadmePictures ./internal/ui/gio/
func TestReadmePictures(t *testing.T) {
	source := os.Getenv("ER_SAVE")
	if source == "" {
		t.Skip("ER_SAVE not set")
	}
	bytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "memory.dat")
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		t.Fatal(err)
	}
	size := image.Pt(1920, 1032)
	state := newTestState(t, picker{path: path})
	state.begin("test")
	state.openWorker()
	if state.LoadedSave() == nil {
		t.Fatal("the save did not load")
	}
	picture := func(name string) {
		draw(t, state)
		state.mu.Lock()
		state.noticeSettled = state.noticeSeq
		state.mu.Unlock()
		draw(t, state)
		shoot(t, state, name, size, shootScale)
	}

	state.view = viewCharacters
	picture("characters")

	// The Lone Wolves with a whole card of Crucible Knights set down ahead of
	// them, and that card open in the catalog.
	state.view = viewEditor
	loaded := state.LoadedSave()
	for index, ash := range loaded.Ashes {
		if ash.Name == "Lone Wolf Ashes" {
			state.openAsh(index)
		}
	}
	draw(t, state)
	ash := state.selectedAsh(loaded)
	knights := slices.IndexFunc(loaded.Families, func(family familyView) bool { return family.Name == "Crucible Knight" })
	if ash == nil || knights < 0 {
		t.Fatal("no Lone Wolf Ashes or no Crucible Knight card")
	}
	state.dropAt(ash, application.Placement{X: 0, Z: 3}, payloadOf(loaded.Families[knights].firstOfEach()))
	state.chooseKind(slices.Index(loaded.Categories, loaded.Families[knights].Category))
	draw(t, state)
	state.family, state.entry, state.reveal = knights, noEntry, true
	picture("editor")

	state.openCreature(3)
	picture("creature")
	state.creatureSlot = noToken

	state.selected = noAsh
	state.chooseKind(slices.Index(loaded.Categories, application.CustomCategory))
	draw(t, state)
	state.openBuilder(noBuild)
	draw(t, state)
	open := state.builder
	if open == nil {
		t.Fatal("the builder did not open")
	}
	dressPupil(state, open)
	picture("builder")
	state.finishBuilder(open)

	// Two Ashes given, a price, and both monument switches, beside the
	// formation and the character staged above.
	person := state.LoadedSave().Characters[state.person]
	given := 0
	for _, each := range state.LoadedSave().Ashes {
		if !person.Owns[each.GoodsFamily] && given < 2 {
			state.session.StageGrant(person.Slot, each.GoodsFamily, 10)
			given++
		}
	}
	state.session.StageCost(ash.Name, application.Price{FP: 70})
	state.session.SetWideArea(true)
	state.session.SetNoRestLimit(true)
	state.pendingOpen = true
	picture("pending")
	state.pendingOpen = false

	state.view = viewSettings
	picture("settings")
}
