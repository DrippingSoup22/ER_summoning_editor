package gio

// The character builder: a custom character's whole loadout on one screen,
// opened from the Custom characters card and drawn over the Editor's two
// halves while it is open.
//
// The left half is the character. What the character creator chooses - body
// type, base template, voice - and the attributes are one column; beside it,
// the equipment as the game's own equipment screen lays it out, slots of a kind
// side by side: the three armaments of the right hand over the left's, the
// arrows over the bolts, the spells, the armour beside the talismans, and the
// quick items. The first quick slot is the one drink every custom character
// carries, the game's own NPC flask (application.NPCFlask), shown and fixed;
// there are no flask counts to set. The pouch is not offered: the AI uses the
// quick items. The right half is the items the chosen slot takes
// (internal/items, through application.BuildItems): choose a slot, then an
// item. Only what the game's AI can use is offered, so no runes or keys.
//
// Nothing is staged until Done. The draft is checked against the save then
// (Session.StageBuild) and a refusal is said on the bar, where the player is
// looking, rather than staging half a character; Cancel drops the draft.
//
// A weapon's Ash of War is chosen from the weapon: its button turns the
// catalog to the Ashes that go on that weapon, and the affinities offered are
// then the Ash's - a lightning Ash, the physical four and Lightning and
// Sacred (application.BuildChoices). A weapon with no Ash fights with its own
// skill, at that skill's affinities. The name the player types is the
// character's in the editor only: the game never shows it.

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// noBuild opens the builder on a new character.
const noBuild = -1

// slotKind is what a slot holds.
type slotKind int

const (
	handSlot slotKind = iota
	ammoSlot
	armourSlot
	talismanSlot
	spellSlot
	quickSlot
	// ashSlot is a hand's Ash of War, chosen from the hand: no cell of its own.
	ashSlot
	slotKinds
)

// slotCounts is how many slots of each kind a character has, in the order of
// application.Build's arrays: hands L1 R1 L2 R2 L3 R3, ammunition arrow 1,
// bolt 1, arrow 2, bolt 2, armour head, chest, arms, legs, and an Ash of War
// per hand.
var slotCounts = [slotKinds]int{6, 4, 4, 4, 7, application.BuildQuickSlots, 6}

// builderSlot is one slot: its kind and its index in that kind's array.
type builderSlot struct {
	kind  slotKind
	index int
}

// flat numbers every slot from 0, for the slot buttons.
func (slot builderSlot) flat() int {
	at := slot.index
	for kind := range slot.kind {
		at += slotCounts[kind]
	}
	return at
}

const allSlots = 6 + 4 + 4 + 4 + 7 + application.BuildQuickSlots + 6

// rowCells is how many cells the widest row holds - the quick items, the
// flask's among them - and so every row's width.
const rowCells = application.BuildQuickSlots + 1

// flaskCell stands in a group's line for the flask every custom character
// carries, which is not a slot of the build's.
const flaskCell = -1

// slotGroup is one labelled grid of slots: a line of cells per row, the
// cells naming themselves (R1, A2), so the grid needs one label only.
type slotGroup struct {
	label string
	kind  slotKind
	lines [][]int
}

// equipmentRows are the slots as the equipment screen lays them out. A row
// holds groups side by side, the first at the left edge and the last at the
// right, as wide as the quick items' ten. The armaments are the right hand's
// three over the left's, the ammunition the arrows over the bolts, as the
// game stacks them.
var equipmentRows = [][]slotGroup{
	{
		{"Armaments", handSlot, [][]int{{1, 3, 5}, {0, 2, 4}}},
		{"Ammunition", ammoSlot, [][]int{{0, 2}, {1, 3}}},
	},
	{{"Spells", spellSlot, [][]int{{0, 1, 2, 3, 4, 5, 6}}}},
	{
		{"Armour", armourSlot, [][]int{{0, 1, 2, 3}}},
		{"Talismans", talismanSlot, [][]int{{0, 1, 2, 3}}},
	},
	{{"Quick items", quickSlot, [][]int{{flaskCell, 0, 1, 2, 3, 4, 5, 6, 7, 8}}}},
}

var armourNames = [4]string{"Head", "Chest", "Arms", "Legs"}

// short is what an empty slot shows in its square: where it is.
func (slot builderSlot) short() string {
	switch slot.kind {
	case handSlot:
		return [2]string{"L", "R"}[slot.index%2] + strconv.Itoa(slot.index/2+1)
	case ammoSlot:
		return [2]string{"A", "B"}[slot.index%2] + strconv.Itoa(slot.index/2+1)
	case armourSlot:
		return armourNames[slot.index]
	case quickSlot:
		return strconv.Itoa(slot.index + 2) // after the flask's
	}
	return strconv.Itoa(slot.index + 1)
}

// name is the slot's name in full, over the chosen slot's options.
func (slot builderSlot) name() string {
	switch slot.kind {
	case handSlot:
		return fmt.Sprintf("%s armament %d", [2]string{"Left", "Right"}[slot.index%2], slot.index/2+1)
	case ammoSlot:
		return fmt.Sprintf("%s %d", [2]string{"Arrows", "Bolts"}[slot.index%2], slot.index/2+1)
	case armourSlot:
		return armourNames[slot.index]
	case talismanSlot:
		return fmt.Sprintf("Talisman %d", slot.index+1)
	case spellSlot:
		return fmt.Sprintf("Spell %d", slot.index+1)
	case ashSlot:
		return "Ash of War for the " + strings.ToLower(builderSlot{handSlot, slot.index}.name())
	}
	return fmt.Sprintf("Quick item %d", slot.index+2)
}

// hand is the hand slot a slot belongs to: itself, or the hand an Ash of War
// is chosen for.
func (slot builderSlot) hand() builderSlot {
	if slot.kind == ashSlot {
		return builderSlot{handSlot, slot.index}
	}
	return slot
}

// takes is what the catalog offers for a slot: the item categories, and of
// the ammunition, the subcategories that are the slot's kind.
func (slot builderSlot) takes() (categories []string, subcategories []string) {
	switch slot.kind {
	case handSlot:
		return []string{"melee_armaments", "ranged_and_catalysts", "shields"}, nil
	case ammoSlot:
		if slot.index%2 == 0 {
			return []string{"arrows_and_bolts"}, []string{"Arrows", "Greatarrows"}
		}
		return []string{"arrows_and_bolts"}, []string{"Bolts", "Greatbolts"}
	case armourSlot:
		return []string{[4]string{"head", "chest", "arms", "legs"}[slot.index]}, nil
	case talismanSlot:
		return []string{"talismans"}, nil
	case spellSlot:
		return []string{"sorceries", "incantations"}, nil
	case ashSlot:
		return []string{"ashes_of_war"}, nil
	}
	return []string{"tools"}, nil
}

// stacked reports whether the slot holds a count of what is in it.
func (slot builderSlot) stacked() bool {
	return slot.kind == ammoSlot || slot.kind == quickSlot
}

// builder is the open builder: the character being made and the controls
// that make it. Frame goroutine only, like everything outside State.mu.
type builder struct {
	n int
	// draft is the character as it stands, original as it was opened, and
	// held whether there is one to delete.
	draft, original application.Build
	held            bool
	slot            builderSlot
	refusal         string
	// short is what the draft's weapons and spells require that it lacks,
	// read each frame: warned of, never refused.
	short application.BuildShortfalls

	cancelBtn, doneBtn, deleteBtn, removeBtn widget.Clickable
	ashBtn                                   widget.Clickable
	bodyBtns                                 [2]widget.Clickable
	templateCombo, voiceCombo, classCombo    combo
	affinityCombo                            combo
	levelSlider                              levelSlider
	name                                     widget.Editor
	attributes                               [8]widget.Editor
	count                                    widget.Editor
	countFor                                 builderSlot
	// numberKeys is the number field that had the keys last frame
	// (selectOnFocus).
	numberKeys *widget.Editor
	// keysTag is the builder's whole area, on top and passing presses on:
	// any press takes the keys from the field holding them, and a press on a
	// field gives them back to it (readKeys).
	keysTag  bool
	slotBtns [allSlots]widget.Clickable
	sheet    widget.List

	// The item catalog: its filters, and the items they leave, cached for
	// what they were computed for.
	search              widget.Editor
	kind, sub           int
	kindCombo, subCombo combo
	filters             itemFilters
	list                widget.List
	itemBtns            []widget.Clickable
	offered             []int
	offeredFor          string
}

// The attributes, in the order of application.Build's.
var attributeNames = [8]string{"Vigor", "Mind", "Endurance", "Strength", "Dexterity", "Intelligence", "Faith", "Arcane"}

// voices are the character creator's six voices, CharaInitParam.voiceType 0
// to 5.
var voices = []string{"Voice 1", "Voice 2", "Voice 3", "Voice 4", "Voice 5", "Voice 6"}

// The counts a new item starts with in a slot that holds a count.
const (
	newAmmo  = 30
	newGoods = 5
)

// openBuilder opens the builder on custom character n, or on a new one.
func (s *State) openBuilder(n int) {
	if s.Busy() || s.LoadedSave() == nil {
		return
	}
	if err := s.loadItems(); err != nil {
		s.setNotice(notice{text: "Could not read the item list: " + err.Error(), bad: true})
		return
	}
	if loaded := s.LoadedSave(); loaded.choices == nil {
		choices, err := s.session.BuildChoices()
		if err != nil {
			s.setNotice(notice{text: "Could not read the save's weapons: " + err.Error(), bad: true})
			return
		}
		loaded.choices = &choices
	}
	open := &builder{n: n, slot: builderSlot{handSlot, application.RightHand}, kind: openingCategory}
	if n == noBuild {
		free, ok := s.session.FreeBuild()
		if !ok {
			s.setNotice(notice{text: "Ten custom characters at most: delete one first.", bad: true})
			return
		}
		open.n, open.draft = free, application.PlainBuild(true)
		open.draft.Class = 0 // the Vagabond
		if classes := s.LoadedSave().choices.Classes; len(classes) > 0 {
			open.draft.Attributes = classes[0]
		}
	} else {
		for _, one := range s.session.CustomCharacters() {
			if one.N == n {
				open.draft, open.held = one.Build, true
			}
		}
		if !open.held {
			return
		}
	}
	open.original = open.draft
	// A character built before the class was chosen is a Vagabond, and an
	// attribute under its class's is raised to it: shown as a change to save.
	floors := classFloors(s.LoadedSave(), open.draft.Class)
	for index, floor := range floors {
		open.draft.Attributes[index] = max(open.draft.Attributes[index], floor)
	}
	open.sheet.Axis, open.list.Axis = layout.Vertical, layout.Vertical
	open.search.SingleLine = true
	open.name.SingleLine, open.name.MaxLen = true, application.MaxNameLength
	open.name.SetText(open.draft.Name)
	for index := range open.attributes {
		numberField(&open.attributes[index], strconv.Itoa(int(open.draft.Attributes[index])))
	}
	numberField(&open.count, "")
	open.countFor = builderSlot{kind: -1}
	s.builder = open
}

// classFloors is the least each attribute of a class's character may be: the
// class's own, or 1 where the save's classes are not known.
func classFloors(loaded *savedView, class int) [8]uint8 {
	if loaded == nil || loaded.choices == nil || class < 0 || class >= len(loaded.choices.Classes) {
		return [8]uint8{1, 1, 1, 1, 1, 1, 1, 1}
	}
	return loaded.choices.Classes[class]
}

// chooseClass puts the draft on another starting class. Attributes still at
// the old class's are the new class's, as the game's character creator
// does; edited ones stay, raised where they fall under the new class's.
func (open *builder) chooseClass(loaded *savedView, class int) {
	old, next := classFloors(loaded, open.draft.Class), classFloors(loaded, class)
	untouched := open.draft.Attributes == old
	for index := range open.draft.Attributes {
		if untouched || open.draft.Attributes[index] < next[index] {
			open.draft.Attributes[index] = next[index]
		}
		open.attributes[index].SetText(strconv.Itoa(int(open.draft.Attributes[index])))
	}
	open.draft.Class = class
}

// numberField makes an editor a field of one small whole number.
func numberField(editor *widget.Editor, value string) {
	editor.SingleLine, editor.Filter, editor.MaxLen = true, "0123456789", 2
	editor.SetText(value)
}

// loadItems reads the catalog the builder offers, once.
func (s *State) loadItems() error {
	if s.items != nil {
		return nil
	}
	all, err := application.BuildItems()
	if err != nil {
		return err
	}
	s.items, s.itemSearch = all, make([]string, len(all))
	for index, item := range all {
		s.itemSearch[index] = strings.ToLower(item.Name + " " + item.Subcategory)
	}
	return nil
}

// layoutBuilder is the builder in the Editor's place: the character, and the
// items beside it, split as the Editor's two halves are.
func (s *State) layoutBuilder(gtx layout.Context, th *material.Theme, loaded *savedView) layout.Dimensions {
	open := s.builder
	if open.cancelBtn.Clicked(gtx) {
		s.builder = nil
		return s.layoutEditor(gtx, th)
	}
	if open.doneBtn.Clicked(gtx) {
		s.finishBuilder(open)
	}
	if open.deleteBtn.Clicked(gtx) {
		s.deleteBuilt(open)
	}
	if s.builder == nil {
		return s.layoutEditor(gtx, th)
	}
	open.readEquipment(gtx, loaded)
	open.readKeys(gtx)
	total, gap := gtx.Constraints.Max.X, gtx.Dp(splitGap)
	s.readSplit(gtx, total, gap)
	width := s.catalogPx(gtx, total)
	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutSheet(gtx, th, open, loaded)
			})
		}),
		layout.Rigid(layout.Spacer{Width: splitGap}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutItems(gtx, th, open)
			})
		}),
	)
	s.drawSplit(gtx, image.Rect(total-width-gap, 0, total-width, dims.Size.Y))
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &open.keysTag)
	area.Pop()
	pass.Pop()
	return dims
}

// readKeys takes the keys from the field holding them at any press in the
// builder, read before the fields so that a press on one gives them back to
// it. Gio moves the keys only between fields, so without this a field kept
// them through a click on a button, showing a value it had not taken.
func (open *builder) readKeys(gtx layout.Context) {
	for {
		got, ok := gtx.Event(pointer.Filter{Target: &open.keysTag, Kinds: pointer.Press})
		if !ok {
			return
		}
		if press, isPointer := got.(pointer.Event); isPointer && press.Kind == pointer.Press {
			takeKeys(gtx)
		}
	}
}

// readEquipment reads the clicks that choose what the catalog offers: a slot,
// its Ash of War, or Remove. Before either half is laid out, since a Flex lays
// its rigid child - the catalog - out first: read in the sheet, a click turned
// the catalog only at the next frame, which waits for the pointer to move.
func (open *builder) readEquipment(gtx layout.Context, loaded *savedView) {
	for kind := range slotKinds {
		for index := range slotCounts[kind] {
			slot := builderSlot{kind, index}
			if open.slotBtns[slot.flat()].Clicked(gtx) && open.slot != slot {
				open.slot = slot
				open.kind, open.sub = openingCategory, 0 // another slot's filters
			}
		}
	}
	if open.removeBtn.Clicked(gtx) {
		open.clear(loaded.choices)
	}
	if open.ashBtn.Clicked(gtx) {
		open.slot = builderSlot{ashSlot, open.slot.index}
		open.kind, open.sub = openingCategory, 0
	}
}

// finishBuilder stages the draft, or says why it cannot be.
func (s *State) finishBuilder(open *builder) {
	if s.Busy() {
		return
	}
	name := application.BuildName(open.n, open.draft)
	staged, err := s.session.StageBuild(open.n, open.draft)
	switch {
	case errors.Is(err, application.ErrBadBuild):
		open.refusal = "The game cannot build this character: " + err.Error()
		return
	case err != nil:
		open.refusal = "Could not stage the character: " + err.Error()
		return
	case staged == nil:
		s.setNotice(notice{text: "No change to " + name + "."})
	default:
		s.setNotice(notice{text: name + " staged: drag it into a formation."})
	}
	s.builder = nil
	s.refreshCustom()
}

// deleteBuilt stages the character's deletion, or says why it cannot be.
func (s *State) deleteBuilt(open *builder) {
	if s.Busy() || !open.held {
		return
	}
	name := application.BuildName(open.n, open.original)
	if err := s.session.StageBuildDeletion(open.n); errors.Is(err, application.ErrBuildNamed) {
		open.refusal = name + " is in a formation. Take it out of every formation first."
		return
	} else if err != nil {
		open.refusal = "Could not delete the character: " + err.Error()
		return
	}
	s.builder = nil
	s.refreshCustom()
	s.setNotice(notice{text: name + " will be deleted."})
}

// refreshCustom puts the custom characters' card back as the session now
// describes it: after a build or a deletion is staged or dropped.
func (s *State) refreshCustom() {
	loaded := s.LoadedSave()
	if loaded == nil || s.Busy() {
		return
	}
	custom, err := customFamily(s.session.CustomCharacterCard(), loaded.pictures)
	if err != nil {
		s.setNotice(notice{text: err.Error(), bad: true})
		return
	}
	loaded.all = withCustom(loaded.all, custom)
	// Re-pruned on the next frame, and the grid's cached filter with it.
	loaded.prunedFor = pruning{category: "\x00"}
	s.filtered = nil
	s.entryBtns, s.entryDrags = nil, nil
}

// layoutSheet is the left half: the bar, then the character and its
// equipment, scrolled together where the window is short.
func (s *State) layoutSheet(gtx layout.Context, th *material.Theme, open *builder, loaded *savedView) layout.Dimensions {
	s.readBuilderFields(gtx, open)
	open.short = loaded.choices.Shortfalls(open.draft)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutBuilderBar(gtx, th, open)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(horizontalDivider),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(th, &open.sheet).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutCreation(gtx, th, open)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(28)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return s.layoutEquipment(gtx, th, open, loaded)
					}),
				)
			})
		}),
	)
}

// layoutBuilderBar is the line over the sheet: which character, what it
// comes to, and the three ways out.
func (s *State) layoutBuilderBar(gtx layout.Context, th *material.Theme, open *builder) layout.Dimensions {
	draft := open.draft
	summary := fmt.Sprintf("%s · Level %d · %d HP · body type %s", application.ClassNames[draft.Class], draft.Level(), draft.HP(),
		map[bool]string{true: "A", false: "B"}[draft.Template < application.BuildTemplateB])
	if !open.held {
		summary = "New · " + summary
	} else if draft != open.original {
		summary += " · changed"
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					name := title(th, unit.Sp(17), application.BuildName(open.n, open.draft))
					name.MaxLines = 1
					return name.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					style := muted(th, summary)
					switch count := open.short.Count(); {
					case open.refusal != "":
						style = muted(th, open.refusal)
						style.Color = colorError
					case count == 1:
						style = muted(th, summary+" · 1 requirement not met")
						style.Color = colorWarn
					case count > 1:
						style = muted(th, fmt.Sprintf("%s · %d requirements not met", summary, count))
						style.Color = colorWarn
					}
					style.MaxLines = 2
					return style.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !open.held {
				return layout.Dimensions{}
			}
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, rowButton(th, &open.deleteBtn, "Delete").Layout)
		}),
		layout.Rigid(rowButton(th, &open.cancelBtn, "Cancel").Layout),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			done := button(th, &open.doneBtn, "Done")
			done.TextSize = th.TextSize * 14 / 16
			done.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(18), Right: unit.Dp(18)}
			return done.Layout(gtx)
		}),
	)
}

// creationWidth is the character column: a label and a chooser.
const creationWidth = unit.Dp(270)

// layoutCreation is the character column: what the character creator
// chooses, then the attributes.
func (s *State) layoutCreation(gtx layout.Context, th *material.Theme, open *builder) layout.Dimensions {
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(creationWidth), gtx.Dp(creationWidth)
	draft := &open.draft
	typeA := draft.Template < application.BuildTemplateB
	base := int32(application.BuildTemplateB)
	if typeA {
		base = application.BuildTemplateA
	}
	for index := range open.bodyBtns {
		if open.bodyBtns[index].Clicked(gtx) {
			first := int32(application.BuildTemplateA)
			if index == 1 {
				first = application.BuildTemplateB
			}
			draft.Template, base = first+(draft.Template-base), first
		}
	}
	children := []layout.FlexChild{
		layout.Rigid(s.helpSection(th, "NAME", helpName)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(fieldHeight)
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			return box(gtx, colorInputBg, colorDivider, unit.Dp(4), func(gtx layout.Context) layout.Dimensions {
				editor := material.Editor(th, &open.name, application.BuildName(open.n, application.Build{}))
				editor.Color, editor.HintColor = colorFg, colorMuted
				return editor.Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),
		layout.Rigid(sectionLabel(th, "BODY")),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return toggle(gtx, th, &open.bodyBtns[0], "Body type A", base == application.BuildTemplateA)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return toggle(gtx, th, &open.bodyBtns[1], "Body type B", base == application.BuildTemplateB)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return labelled(gtx, th, "Base template", func(gtx layout.Context) layout.Dimensions {
				names := application.TemplateNames[:]
				dims, chose := open.templateCombo.Layout(gtx, th, choiceWidth, names, int(draft.Template-base))
				if chose >= 0 {
					draft.Template = base + int32(chose)
				}
				return dims
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return labelled(gtx, th, "Voice", func(gtx layout.Context) layout.Dimensions {
				dims, chose := open.voiceCombo.Layout(gtx, th, choiceWidth, voices, int(draft.Voice))
				if chose >= 0 {
					draft.Voice = uint8(chose)
				}
				return dims
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),
		layout.Rigid(s.helpSection(th, "ATTRIBUTES", helpAttributes)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return labelled(gtx, th, "Starting class", func(gtx layout.Context) layout.Dimensions {
				dims, chose := open.classCombo.Layout(gtx, th, choiceWidth, application.ClassNames, draft.Class)
				if chose >= 0 {
					open.chooseClass(s.LoadedSave(), chose)
				}
				return dims
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
	}
	for index, name := range attributeNames {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return labelled(gtx, th, name, func(gtx layout.Context) layout.Dimensions {
					return numberBox(gtx, th, &open.attributes[index])
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout))
	}
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return labelled(gtx, th, "Level", muted(th, strconv.Itoa(int(draft.Level()))).Layout)
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// choiceWidth is a chooser in the character column.
const choiceWidth = unit.Dp(150)

// readBuilderFields takes what was typed into the number fields: a value in
// range is the draft's, and anything else - an emptied field being retyped -
// leaves the draft as it was until it is.
func (s *State) readBuilderFields(gtx layout.Context, open *builder) {
	typed := func(editor *widget.Editor, low, high int, set func(int)) {
		for {
			event, ok := editor.Update(gtx)
			if !ok {
				return
			}
			if _, changed := event.(widget.ChangeEvent); !changed {
				continue
			}
			if value, err := strconv.Atoi(editor.Text()); err == nil && value >= low && value <= high {
				set(value)
			}
		}
	}
	// An attribute takes a value from its class's own to 99. One typed below
	// it is not taken, and the field shows the attribute again once the keys
	// leave it, so typing 15 through its 1 still works.
	floors := classFloors(s.LoadedSave(), open.draft.Class)
	for index := range open.attributes {
		field := &open.attributes[index]
		typed(field, int(floors[index]), 99, func(value int) { open.draft.Attributes[index] = uint8(value) })
		if shown := strconv.Itoa(int(open.draft.Attributes[index])); !gtx.Focused(field) && field.Text() != shown {
			field.SetText(shown)
		}
	}
	for {
		event, ok := open.name.Update(gtx)
		if !ok {
			break
		}
		if _, changed := event.(widget.ChangeEvent); changed {
			open.draft.Name = strings.TrimSpace(open.name.Text())
		}
	}
	if open.countFor != open.slot {
		open.countFor = open.slot
		if stack := open.stack(open.slot); stack != nil && stack.Row != 0 {
			open.count.SetText(strconv.Itoa(stack.Count))
		}
	}
	typed(&open.count, 1, application.MaxCarried, func(value int) {
		if stack := open.stack(open.slot); stack != nil && stack.Row != 0 {
			stack.Count = value
		}
	})
	numbers := []*widget.Editor{&open.count}
	for index := range open.attributes {
		numbers = append(numbers, &open.attributes[index])
	}
	selectOnFocus(gtx, &open.numberKeys, numbers...)
}

// stack is the draft's count-holding slot, or nil for a slot without one.
func (open *builder) stack(slot builderSlot) *application.BuildStack {
	switch slot.kind {
	case ammoSlot:
		return &open.draft.Ammo[slot.index]
	case quickSlot:
		return &open.draft.Quick[slot.index]
	}
	return nil
}

// holds is what a slot holds: the item's table and row, and zero for an
// empty slot.
func (open *builder) holds(slot builderSlot) (string, int32) {
	draft := &open.draft
	switch slot.kind {
	case handSlot:
		return application.ItemWeapon, draft.Hands[slot.index].Base
	case armourSlot:
		return application.ItemProtector, draft.Armour[slot.index]
	case talismanSlot:
		return application.ItemAccessory, draft.Talismans[slot.index]
	case spellSlot:
		return application.ItemMagic, draft.Spells[slot.index]
	case ammoSlot:
		return application.ItemWeapon, open.stack(slot).Row
	case ashSlot:
		return application.ItemGem, draft.Hands[slot.index].Ash
	}
	return application.ItemGoods, open.stack(slot).Row
}

// put sets an item in the chosen slot. A weapon keeps the level it was at,
// as far as the new one goes, and its Ash of War and affinity where they go on
// it; an Ash of War goes on its weapon, which turns back to the weapon, at the
// affinity it had if the Ash allows it and else the Ash's own; a spell goes to
// the first free slot, since the row holds them packed; a slot that was empty
// gets a count to start from.
func (s *State) put(open *builder, item application.BuildItem, loaded *savedView) {
	draft, slot, choices := &open.draft, open.slot, loaded.choices
	switch slot.kind {
	case handSlot:
		hand := &draft.Hands[slot.index]
		ash := hand.Ash
		if !choices.Fits(item.ID, ash) {
			ash = 0
		}
		*hand = application.BuildWeapon{Base: item.ID, Level: min(hand.Level, choices.Weapons[item.ID].MaxLevel),
			Affinity: hand.Affinity, Ash: ash}
		keepAffinity(hand, choices, 0)
	case ashSlot:
		hand := &draft.Hands[slot.index]
		hand.Ash = item.ID
		keepAffinity(hand, choices, choices.Ashes[item.ID].Default)
		open.slot = slot.hand()
	case armourSlot:
		draft.Armour[slot.index] = item.ID
	case talismanSlot:
		draft.Talismans[slot.index] = item.ID
	case spellSlot:
		at := slot.index
		for free := range at {
			if draft.Spells[free] == 0 {
				at = free
				break
			}
		}
		draft.Spells[at] = item.ID
		open.slot.index = at
	default:
		stack := open.stack(slot)
		if stack.Row == 0 {
			stack.Count = newGoods
			if slot.kind == ammoSlot {
				stack.Count = newAmmo
			}
		}
		stack.Row = item.ID
		open.countFor = builderSlot{kind: -1}
	}
	open.refusal = ""
}

// keepAffinity keeps a weapon's affinity where its skill allows it, and puts
// it at fallback where not - or at Standard, the weapon as it ships, where the
// skill does not allow that either.
func keepAffinity(hand *application.BuildWeapon, choices *application.BuildChoices, fallback int) {
	offered := choices.Offers(hand.Base, hand.Ash)
	switch {
	case offered[hand.Affinity]:
	case offered[fallback]:
		hand.Affinity = fallback
	default:
		hand.Affinity = 0
	}
}

// clear empties the chosen slot; the spells close up behind it. A weapon
// without its Ash of War fights with its own skill, at an affinity that skill
// allows.
func (open *builder) clear(choices *application.BuildChoices) {
	draft, slot := &open.draft, open.slot
	switch slot.kind {
	case handSlot:
		draft.Hands[slot.index] = application.BuildWeapon{}
	case ashSlot:
		hand := &draft.Hands[slot.index]
		hand.Ash = 0
		keepAffinity(hand, choices, 0)
	case armourSlot:
		draft.Armour[slot.index] = 0
	case talismanSlot:
		draft.Talismans[slot.index] = 0
	case spellSlot:
		copy(draft.Spells[slot.index:], draft.Spells[slot.index+1:])
		draft.Spells[len(draft.Spells)-1] = 0
	default:
		*open.stack(slot) = application.BuildStack{}
	}
	open.refusal = ""
}

// equipmentCell is the side of an equipment slot.
const equipmentCell = unit.Dp(56)

// layoutEquipment is the equipment column: the slots in their groups, then
// what the chosen one holds.
func (s *State) layoutEquipment(gtx layout.Context, th *material.Theme, open *builder, loaded *savedView) layout.Dimensions {
	// Every row is as wide as the widest, the ten quick items, so a group
	// that closes a row lines up with the others' right edge.
	width := min(gtx.Dp(equipmentCell)*rowCells, gtx.Constraints.Max.X)
	children := []layout.FlexChild{layout.Rigid(sectionLabel(th, "EQUIPMENT"))}
	for _, row := range equipmentRows {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			groups := make([]layout.FlexChild, 0, 2*len(row))
			for index, group := range row {
				if index > 0 {
					groups = append(groups, layout.Flexed(1, flexSpacer))
				}
				groups = append(groups, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutSlotGroup(gtx, th, open, group)
				}))
			}
			return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, groups...)
			})
		}))
	}
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = width
			return horizontalDivider(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout))
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = width
		return s.layoutChosenSlot(gtx, th, open, loaded)
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutSlotGroup is one labelled grid of slots, its lines one above the
// next.
func (s *State) layoutSlotGroup(gtx layout.Context, th *material.Theme, open *builder, group slotGroup) layout.Dimensions {
	lines := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, muted(th, group.label).Layout)
	})}
	for _, line := range group.lines {
		cells := make([]layout.FlexChild, 0, len(line))
		for _, index := range line {
			slot := builderSlot{group.kind, index}
			cells = append(cells, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if index == flaskCell {
					return s.layoutFlaskCell(gtx, th)
				}
				return s.slotCell(gtx, th, open, slot)
			}))
		}
		lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, cells...)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lines...)
}

// slotCell is one equipment slot: the icon of what it holds, or where it is
// when it holds nothing, a weapon's level or a stack's count in its corner,
// and a weapon's Ash of War small in the other. The chosen slot is ringed, and
// a hand while its Ash of War is being chosen.
func (s *State) slotCell(gtx layout.Context, th *material.Theme, open *builder, slot builderSlot) layout.Dimensions {
	table, id := open.holds(slot)
	key := application.BuildItem{Table: table, ID: id}.Key()
	cell := iconCell{Size: equipmentCell, Empty: id == 0, Tooltip: slot.name(), Label: slot.name()}
	if id != 0 {
		cell.Img, _ = s.itemArt.op(key, gtx.Dp(equipmentCell))
		if item, ok := s.itemNamed(key); ok {
			cell.Tooltip = slot.name() + ": " + item.Name
		}
	}
	ashKey := ""
	if slot.kind == handSlot && id != 0 && open.draft.Hands[slot.index].Ash != 0 {
		ashKey = application.BuildItem{Table: application.ItemGem, ID: open.draft.Hands[slot.index].Ash}.Key()
		if ash, ok := s.itemNamed(ashKey); ok {
			cell.Tooltip += ", with " + ash.Name
		}
	}
	if needs := open.needs(slot); needs != "" {
		warn := colorWarn
		cell.Border = &warn
		cell.Tooltip += " - needs " + needs
	}
	if open.slot.hand() == slot {
		ring := colorAccent
		cell.Border = &ring
	}
	corner := ""
	switch {
	case id == 0:
	case slot.kind == handSlot && open.draft.Hands[slot.index].Level > 0:
		corner = fmt.Sprintf("+%d", open.draft.Hands[slot.index].Level)
	case slot.stacked():
		corner = strconv.Itoa(open.stack(slot).Count)
	}
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return cell.Layout(gtx, th, &open.slotBtns[slot.flat()])
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if ashKey == "" {
				return layout.Dimensions{}
			}
			return layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					side := equipmentCell / 3
					icon, has := s.itemArt.op(ashKey, gtx.Dp(side))
					return imageCell(gtx, side, icon, has)
				})
			})
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			label, at := corner, layout.SE
			if id == 0 {
				label, at = slot.short(), layout.Center
			}
			if label == "" {
				return layout.Dimensions{}
			}
			return at.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Label(th, th.TextSize*11/16, label)
					style.Color = colorMuted
					if id != 0 {
						style.Color = colorFg
					}
					return style.Layout(gtx)
				})
			})
		}),
	)
}

// needs says what the weapon or spell in a slot requires that the draft lacks
// - "Strength 16 (has 10), Intelligence 52 (has 20)" - or nothing. An Ash of
// War's slot is its weapon's.
func (open *builder) needs(slot builderSlot) string {
	var unmet []application.BuildUnmet
	switch slot = slot.hand(); slot.kind {
	case handSlot:
		unmet = open.short.Hands[slot.index]
	case spellSlot:
		unmet = open.short.Spells[slot.index]
	}
	parts := make([]string, len(unmet))
	for index, one := range unmet {
		parts[index] = fmt.Sprintf("%s %d (has %d)", attributeNames[one.Attribute], one.Need,
			open.draft.Attributes[one.Attribute])
	}
	return strings.Join(parts, ", ")
}

// layoutFlaskCell is the first quick slot: the one drink every custom
// character carries, which is shown and cannot be changed.
func (s *State) layoutFlaskCell(gtx layout.Context, th *material.Theme) layout.Dimensions {
	icon, has := s.itemArt.op(application.BuildItem{Table: application.ItemGoods, ID: application.NPCFlask}.Key(),
		gtx.Dp(equipmentCell))
	cell := iconCell{Img: icon, Empty: !has, Size: equipmentCell,
		Tooltip: "Quick item 1: the NPC Flask of Crimson Tears. One drink that heals half the health, " +
			"the game's own people's flask; every custom character carries it."}
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return cell.Layout(gtx, th, &s.flaskBtn)
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Label(th, th.TextSize*11/16, "1")
					style.Color = colorFg
					return style.Layout(gtx)
				})
			})
		}),
	)
}

// layoutChosenSlot is what the chosen slot holds and its options: a
// weapon's affinity, level and Ash of War, a stack's count, and taking it out.
// While a weapon's Ash of War is being chosen it is the Ash, with the
// weapon's options under it.
func (s *State) layoutChosenSlot(gtx layout.Context, th *material.Theme, open *builder, loaded *savedView) layout.Dimensions {
	table, id := open.holds(open.slot)
	key := application.BuildItem{Table: table, ID: id}.Key()
	name := "Empty - choose an item on the right"
	if open.slot.kind == ashSlot {
		name = "Its own skill - choose an Ash of War on the right"
	}
	if item, ok := s.itemNamed(key); ok && id != 0 {
		name = item.Name
	}
	options := []layout.FlexChild{}
	if hand := open.slot.hand(); hand.kind == handSlot && open.draft.Hands[hand.index].Base != 0 {
		options = append(options,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutWeaponOptions(gtx, th, open, loaded)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutAshLine(gtx, th, open, loaded)
				})
			}))
	}
	if open.slot.stacked() && id != 0 {
		options = append(options, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(muted(th, "How many").Layout),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return numberBox(gtx, th, &open.count) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(muted(th, fmt.Sprintf("1 to %d", application.MaxCarried)).Layout),
			)
		}))
	}
	return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}
		return macro.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				icon, has := s.itemArt.op(key, gtx.Dp(equipmentCell))
				return iconCell{Img: icon, Empty: id == 0 || !has, Size: equipmentCell}.Layout(gtx, th, nil)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				rows := []layout.FlexChild{
					layout.Rigid(muted(th, open.slot.name()).Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						style := title(th, unit.Sp(16), name)
						style.MaxLines = 1
						return style.Layout(gtx)
					}),
				}
				if needs := open.needs(open.slot); needs != "" {
					rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.withHelp(gtx, th, helpNeeds, func(gtx layout.Context) layout.Dimensions {
							style := muted(th, "Needs "+needs+".")
							style.Color = colorWarn
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(style.Layout),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.helpBadge(gtx, th, helpNeeds) }),
								layout.Flexed(1, flexSpacer))
						})
					}))
				}
				rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout))
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, append(rows, options...)...)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if id == 0 {
					return layout.Dimensions{}
				}
				return rowButton(th, &open.removeBtn, "Remove").Layout(gtx)
			}),
		)
	})
}

// layoutAshLine is a weapon's Ash of War: what it carries, and the button that
// turns the catalog to the Ashes that go on it - or that it takes none.
func (s *State) layoutAshLine(gtx layout.Context, th *material.Theme, open *builder, loaded *savedView) layout.Dimensions {
	hand := open.draft.Hands[open.slot.index]
	if loaded.choices.Weapons[hand.Base].Mount < 0 {
		return muted(th, "Takes no Ash of War.").Layout(gtx)
	}
	carried := "its own skill"
	if hand.Ash != 0 {
		carried = fmt.Sprintf("Ash of War %d", hand.Ash)
		if ash, ok := s.itemNamed(application.BuildItem{Table: application.ItemGem, ID: hand.Ash}.Key()); ok {
			carried = ash.Name
		}
	}
	return s.withHelp(gtx, th, helpAshOfWar, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(muted(th, "Ash of War").Layout),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.helpBadge(gtx, th, helpAshOfWar) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(th, carried)
				style.Color = colorFg
				return style.Layout(gtx)
			}),
		}
		// Choosing already, the catalog is the list Choose would open.
		if open.slot.kind != ashSlot {
			children = append(children,
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(rowButton(th, &open.ashBtn, "Choose").Layout))
		}
		children = append(children, layout.Flexed(1, flexSpacer))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

// layoutWeaponOptions is a weapon's affinity and level, from what the save's
// regulation has for it: the affinities its skill allows - its Ash of War's,
// or its own.
func (s *State) layoutWeaponOptions(gtx layout.Context, th *material.Theme, open *builder, loaded *savedView) layout.Dimensions {
	hand := &open.draft.Hands[open.slot.index]
	choice := loaded.choices.Weapons[hand.Base]
	affinities, numbers := []string{}, []int{}
	for affinity, offered := range loaded.choices.Offers(hand.Base, hand.Ash) {
		if offered {
			affinities, numbers = append(affinities, application.Affinities[affinity]), append(numbers, affinity)
		}
	}
	current := 0
	for index, affinity := range numbers {
		if affinity == hand.Affinity {
			current = index
		}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(muted(th, "Affinity").Layout),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(affinities) < 2 {
				return muted(th, application.Affinities[hand.Affinity]+" only").Layout(gtx)
			}
			// The list opens upward: the row sits at the sheet's foot.
			open.affinityCombo.up = true
			dims, chose := open.affinityCombo.Layout(gtx, th, unit.Dp(140), affinities, current)
			if chose >= 0 {
				hand.Affinity = numbers[chose]
			}
			return dims
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(18)}.Layout),
		layout.Rigid(muted(th, "Level").Layout),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims, chose := open.levelSlider.Layout(gtx, th, min(hand.Level, choice.MaxLevel), choice.MaxLevel)
			if chose >= 0 {
				hand.Level = chose
			}
			return dims
		}),
	)
}

// itemNamed is the catalog's item under a key.
func (s *State) itemNamed(key string) (application.BuildItem, bool) {
	if s.itemIndex == nil {
		s.itemIndex = make(map[string]int, len(s.items))
		for index, item := range s.items {
			s.itemIndex[item.Key()] = index
		}
	}
	index, ok := s.itemIndex[key]
	if !ok {
		return application.BuildItem{}, false
	}
	return s.items[index], true
}

// sectionLabel is a column's small heading.
func sectionLabel(th *material.Theme, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			style := heading(th, label)
			style.TextSize = th.TextSize * 13 / 16
			return style.Layout(gtx)
		})
	}
}

// helpSection is a section's label with its "?", and the explanation under
// them while it is open.
func (s *State) helpSection(th *material.Theme, label string, topic helpTopic) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return s.withHelp(gtx, th, topic, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := heading(th, label)
				style.TextSize = th.TextSize * 13 / 16
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(style.Layout),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.helpBadge(gtx, th, topic) }),
					layout.Flexed(1, flexSpacer))
			})
		})
	}
}

// labelled is a control with its name before it, the control at the end of
// the line.
func labelled(gtx layout.Context, th *material.Theme, label string, control layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			style := material.Body2(th, label)
			style.Color = colorFg
			return style.Layout(gtx)
		}),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(control),
	)
}

// numberBox is a field for a small whole number.
func numberBox(gtx layout.Context, th *material.Theme, editor *widget.Editor) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(64)), gtx.Dp(unit.Dp(30))
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	return box(gtx, colorInputBg, colorDivider, unit.Dp(4), func(gtx layout.Context) layout.Dimensions {
		style := material.Editor(th, editor, "")
		style.Color = colorFg
		return style.Layout(gtx)
	})
}

// toggle is one of two buttons standing for a choice: the chosen one lit.
func toggle(gtx layout.Context, th *material.Theme, click *widget.Clickable, label string, on bool) layout.Dimensions {
	style := quietButton(th, click, label)
	if on {
		style = button(th, click, label)
	}
	style.TextSize = th.TextSize * 14 / 16
	style.Inset = layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(7), Left: unit.Dp(10), Right: unit.Dp(10)}
	return style.Layout(gtx)
}

// The item catalog, the builder's right half.

// layoutItems is the catalog of what the chosen slot takes: a search, the
// category and subcategory, and the items as cards.
func (s *State) layoutItems(gtx layout.Context, th *material.Theme, open *builder) layout.Dimensions {
	loaded := s.LoadedSave()
	takes := open.offer(loaded)
	filters := s.itemFiltersFor(open, takes)
	kinds, shown, subs := filters.kinds, filters.shown, filters.subs
	offered := s.offeredItems(open, takes, kinds[open.kind], subs[open.sub])
	if len(open.itemBtns) != len(offered) {
		open.itemBtns = make([]widget.Clickable, len(offered))
	}
	for index := range offered {
		if open.itemBtns[index].Clicked(gtx) {
			s.put(open, s.items[offered[index]], loaded)
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			header := "ITEMS FOR " + strings.ToUpper(open.slot.name())
			if open.slot.kind == ashSlot {
				header = "ASHES OF WAR FOR " + open.slot.hand().short()
			}
			return panelHeader(gtx, th, header, fmt.Sprintf("%d items", len(offered)))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var dropdowns []dropdown
			if filters.choiceOfKinds {
				kindWidth := open.kindCombo.fit(gtx, th, shown)
				dropdowns = append(dropdowns, dropdown{width: kindWidth, draw: func(gtx layout.Context) layout.Dimensions {
					dims, chose := open.kindCombo.Layout(gtx, th, kindWidth, shown, open.kind)
					if chose >= 0 {
						open.kind, open.sub = chose, 0
					}
					return dims
				}})
			}
			if filters.choiceOfSubs {
				subWidth := open.subCombo.fit(gtx, th, subs)
				dropdowns = append(dropdowns, dropdown{width: subWidth, draw: func(gtx layout.Context) layout.Dimensions {
					dims, chose := open.subCombo.Layout(gtx, th, subWidth, subs, open.sub)
					if chose >= 0 {
						open.sub = chose
					}
					return dims
				}})
			}
			return filterBar(gtx, dropdowns, func(gtx layout.Context) layout.Dimensions {
				return box(gtx, colorInputBg, colorDivider, unit.Dp(4), func(gtx layout.Context) layout.Dimensions {
					editor := material.Editor(th, &open.search, "Search items")
					editor.Color, editor.HintColor = colorFg, colorMuted
					return editor.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(offered) == 0 {
				return emptyState(gtx, th, "Nothing matches that.")
			}
			_, held := open.holds(open.slot)
			return s.grid(gtx, th, &open.list, len(offered), func(gtx layout.Context, at int) layout.Dimensions {
				item := s.items[offered[at]]
				return s.itemCard(gtx, th, &open.itemBtns[at], item, item.ID == held)
			})
		}),
	)
}

// itemCard is one item in the catalog: its icon, its name and its kind.
func (s *State) itemCard(gtx layout.Context, th *material.Theme, click *widget.Clickable,
	item application.BuildItem, chosen bool) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return selectableRow(gtx, chosen || click.Hovered(), func(gtx layout.Context) layout.Dimensions {
			return s.cardFaceOf(gtx, th, func(gtx layout.Context) layout.Dimensions {
				side := s.SpiritIconSize()
				icon, has := s.itemArt.op(item.Key(), gtx.Dp(side))
				return imageCell(gtx, side, icon, has)
			}, strings.TrimSpace(item.Name), cmpOr(item.Subcategory, categoryLabel(item.Category)), tileMarks{}, chosen)
		})
	})
}

// offer is what the catalog offers a slot: the categories and subcategories
// it takes, and for an Ash of War, the weapon it must go on.
type offer struct {
	categories, subcategories []string
	weapon                    int32
	fits                      func(application.BuildItem) bool
}

// takes reports whether the slot takes an item.
func (o offer) takes(item application.BuildItem) bool {
	return takesItem(item, o.categories, o.subcategories) && (o.fits == nil || o.fits(item))
}

// offer is what the catalog offers the chosen slot: an Ash of War only where
// it goes on the hand's weapon.
func (open *builder) offer(loaded *savedView) offer {
	categories, subcategories := open.slot.takes()
	out := offer{categories: categories, subcategories: subcategories}
	if open.slot.kind == ashSlot {
		base := open.draft.Hands[open.slot.index].Base
		out.weapon, out.fits = base, func(item application.BuildItem) bool { return loaded.choices.Fits(base, item.ID) }
	}
	return out
}

// offeredItems is the catalog's items for the slot under the filters, as
// indexes into State.items, recomputed only when one of them moves.
func (s *State) offeredItems(open *builder, takes offer, kind, sub string) []int {
	query := strings.ToLower(strings.TrimSpace(open.search.Text()))
	slotKey := fmt.Sprint(takes.categories, takes.subcategories, takes.weapon)
	key := fmt.Sprint(slotKey, kind, sub, query)
	if key == open.offeredFor {
		return open.offered
	}
	if open.offeredFor != "" && !strings.HasPrefix(key, slotKey) {
		open.list.Position.First, open.list.Position.Offset = 0, 0
	}
	open.offeredFor, open.offered = key, open.offered[:0]
	for index, item := range s.items {
		if !takes.takes(item) || item.Category != kind ||
			(sub != allSubcategories && item.Subcategory != sub) ||
			(query != "" && !strings.Contains(s.itemSearch[index], query)) {
			continue
		}
		open.offered = append(open.offered, index)
	}
	return open.offered
}

// openingCategory is a slot's category before one is chosen: the one
// openingKind picks.
const openingCategory = -1

// itemFilters is the item catalog's two dropdowns for the chosen slot, as the
// spirits' catalog has them: the categories holding
// an item the slot takes, shown as labels, one always chosen - no "All
// categories" - and the chosen category's subcategories, "All subcategories"
// first. Each is offered only where it is a choice:
// a talisman slot's one category is none. An Ash of War has neither: its list
// is already only what goes on the weapon, and the groups it is filed under
// are not a category a player looks for.
type itemFilters struct {
	key                         string
	kinds, shown, subs          []string
	choiceOfKinds, choiceOfSubs bool
}

// itemFiltersFor is the slot's filters, computed again only when the slot or
// the category moves. A slot opens on its opening category, and a
// subcategory past the category's goes back to "All".
func (s *State) itemFiltersFor(open *builder, takes offer) itemFilters {
	key := fmt.Sprint(takes.categories, takes.subcategories, takes.weapon, open.kind)
	if key != open.filters.key {
		present := map[string]bool{}
		for _, item := range s.items {
			if takes.takes(item) {
				present[item.Category] = true
			}
		}
		var f itemFilters
		for _, category := range takes.categories {
			if present[category] {
				f.kinds = append(f.kinds, category)
			}
		}
		if len(f.kinds) == 0 {
			f.kinds = takes.categories[:1]
		}
		if open.kind < 0 || open.kind >= len(f.kinds) {
			open.kind = s.openingKind(open, f.kinds)
		}
		f.key = fmt.Sprint(takes.categories, takes.subcategories, takes.weapon, open.kind)
		f.shown = make([]string, len(f.kinds))
		for index, kind := range f.kinds {
			f.shown[index] = categoryLabel(kind)
		}
		kind := f.kinds[open.kind]
		f.subs = []string{allSubcategories}
		seen, bare := map[string]bool{}, false
		for _, item := range s.items {
			if !takes.takes(item) || item.Category != kind {
				continue
			}
			switch {
			case item.Subcategory == "":
				bare = true
			case !seen[item.Subcategory]:
				seen[item.Subcategory] = true
				f.subs = append(f.subs, item.Subcategory)
			}
		}
		// One subcategory is a choice only beside items that have none.
		f.choiceOfKinds = len(f.kinds) > 1
		f.choiceOfSubs = open.slot.kind != ashSlot && (len(f.subs) > 2 || len(f.subs) == 2 && bare)
		open.filters = f
	}
	if open.sub >= len(open.filters.subs) {
		open.sub = 0
	}
	return open.filters
}

// openingKind is the category a slot opens on: the one of what it holds, else
// a right hand's melee armaments, a left hand's shields, and otherwise the
// slot's first (the sorceries, for a spell).
func (s *State) openingKind(open *builder, kinds []string) int {
	want := ""
	table, held := open.holds(open.slot)
	for _, item := range s.items {
		if held != 0 && item.Table == table && item.ID == held {
			want = item.Category
			break
		}
	}
	if want == "" && open.slot.kind == handSlot {
		want = [2]string{"shields", "melee_armaments"}[open.slot.index%2]
	}
	return max(slices.Index(kinds, want), 0)
}

// takesItem reports whether a slot taking these categories and subcategories
// takes an item.
func takesItem(item application.BuildItem, categories, subcategories []string) bool {
	in := func(word string, words []string) bool {
		for _, one := range words {
			if one == word {
				return true
			}
		}
		return false
	}
	return in(item.Category, categories) && (subcategories == nil || in(item.Subcategory, subcategories))
}

// categoryLabel is an item category as a player reads it: melee_armaments
// is "Melee armaments".
func categoryLabel(category string) string {
	words := strings.ReplaceAll(category, "_", " ")
	if words == "" || words == "npc" {
		return strings.ToUpper(words)
	}
	return strings.ToUpper(words[:1]) + words[1:]
}

func cmpOr(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// addCharacterCard is the tile on the custom characters' card that opens the
// builder on a new character.
func (s *State) addCharacterCard(gtx layout.Context, th *material.Theme) layout.Dimensions {
	click := &s.addCharacterBtn
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return selectableRow(gtx, click.Hovered(), func(gtx layout.Context) layout.Dimensions {
			return s.cardFaceOf(gtx, th, func(gtx layout.Context) layout.Dimensions {
				side := gtx.Dp(s.SpiritIconSize())
				gtx.Constraints = layout.Exact(image.Pt(side, side))
				box(gtx, colorControl, colorSlotEdge, unit.Dp(3), func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{}
				})
				// A plus drawn rather than typed: the serif's own is a small
				// mark at the foot of the line.
				arm, thick := side*2/5, max(gtx.Dp(unit.Dp(2)), side/14)
				middle := image.Pt(side/2, side/2)
				for _, bar := range []image.Rectangle{
					image.Rect(middle.X-arm/2, middle.Y-thick/2, middle.X+arm/2, middle.Y+thick-thick/2),
					image.Rect(middle.X-thick/2, middle.Y-arm/2, middle.X+thick-thick/2, middle.Y+arm/2),
				} {
					paint.FillShape(gtx.Ops, colorAccent, clip.Rect(bar).Op())
				}
				return layout.Dimensions{Size: image.Pt(side, side)}
			}, "Add a character", "Opens the builder", tileMarks{}, false)
		})
	})
}

// build is the custom character a variant is, from its build tag.
func (variant variantView) build() int {
	if n, err := strconv.Atoi(variant.Tags["build"]); err == nil {
		return n
	}
	return noBuild
}
