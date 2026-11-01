package gio

// The editor's retained state.
//
// Two goroutines touch this and the division between them is the one rule the
// whole package hangs off. Everything not guarded by mu belongs to the frame
// goroutine and nothing else may read it. Everything guarded by mu is written
// by a worker and read by the frame.
//
// application.Session is owned by whoever is working on it. While busy is
// true, a worker holds the session and the frame must not call it at all -
// which is why the frame renders `loaded` (an immutable snapshot the worker
// produced) rather than asking the session what it holds. That is stricter
// than the sibling merchant editor's rule, and it is stricter because our
// session is a single-owner object rather than a set of maps.

import (
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/platform/clicks"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/platform/dialogs"
)

// State is the whole editor.
type State struct {
	session *application.Session
	dialogs dialogs.Service
	th      *material.Theme
	win     *app.Window
	// windowScale is the scale the frames are drawn at beyond the system's
	// (1.25 under WSL), which the window's own size must carry too.
	windowScale float32

	// Retained view and widget state. Frame goroutine only.
	settings   Settings
	view       viewID
	tabBtns    [viewCount]widget.Clickable
	openBtn    widget.Clickable
	loadBtn    widget.Clickable
	pathField  widget.Editor
	saveBtn    widget.Clickable
	fontCombo  combo
	themeCombo combo
	// settingsList scrolls the Settings view in a window too short for its
	// cards.
	settingsList widget.List
	// The three icon-size sliders, and whether a drag has left a value the
	// config file has not been told about yet. See sizeControl.
	iconSlider        widget.Float
	iconUnsaved       bool
	editorIconSlider  widget.Float
	spiritIconSlider  widget.Float
	editorIconUnsaved bool
	spiritIconUnsaved bool

	// The Ash rail. selected indexes loaded.Ashes and is -1 when no Ash is
	// open; ashBtns is grown to match the loaded save in the frame that first
	// draws it, since a different save may hold a different number.
	ashList  widget.List
	ashBtns  []widget.Clickable
	selected int
	// art and entityArt decode and hold the two icon sets. Both are keyed by
	// something that belongs to the game rather than to the save - an Ash's
	// name, an entity's model - so both survive a load.
	art       icons
	entityArt icons

	// The spirits panel. query and filtered cache the search, so 249 cards
	// are matched on a keystroke rather than on a frame.
	search    widget.Editor
	query     string
	shownKind int
	filtered  []int
	// kind indexes loaded.Categories, the hierarchy's own words in its order.
	// An int and not a vocabulary, because the words are research's and
	// change without this package.
	kind      int
	kindCombo combo
	// kindChosen is set once the player picks a category. Until then every
	// load opens the panel on the game's own Ashes (openingKind); after it,
	// their choice stays.
	kindChosen bool
	// sub indexes the chosen category's subcategories (savedView.
	// Subcategories): 0 is "All subcategories". Choosing a category puts it
	// back to 0, since the index means nothing under another category.
	// shownSub is what filtered was computed for.
	sub      int
	subCombo combo
	shownSub int
	// family is the open card and entry the open model inside it. Both are
	// -1 when nothing is open, and entry means nothing at all for a card that
	// holds one model - chosenEntry answers that case from the card itself
	// rather than from a number nobody set.
	family      int
	familyList  widget.List
	familyBtns  []widget.Clickable
	familyDrags []widget.Draggable
	// The open card's entries, framed under its row. Their buttons are
	// dropped when the open card changes, because two cards holding the same
	// number of models would otherwise carry each other's press state
	// across. reveal asks the grid to scroll a card just opened into view.
	entry      int
	entryBtns  []widget.Clickable
	entryDrags []widget.Draggable
	reveal     bool
	// openColumns is the catalog's column count this frame, which what an
	// open card holds keeps (cardRows).
	openColumns int
	// cardOpened is the card a click just opened and cardOpenedAt when. A
	// touchpad's tap-and-drag arrives as a click and then a press, so for
	// doubleTap the card stays under the pointer (keepInPlace; the reveal
	// waits), a second click on it - a double click - leaves it open, and a
	// drag from it closes it again. builderWait is a click on a custom
	// character, which waits doubleTap before the builder takes the panel,
	// since the builder would take the card from under a drag.
	cardOpened, builderWait     cardRef
	cardOpenedAt, builderWaitAt time.Time
	// familyFrame and entryFrame are the open card's and the open entry's
	// frames as the last frame drew them: what keepInPlace scrolls by when
	// one above the clicked card closes.
	familyFrame, entryFrame int
	// drawnFamilies and drawnEntries are the cells the LAST frame actually
	// laid out, which is exactly the set that can be carrying a press.
	//
	// They exist because the press has to be read before the panel decides
	// which depth to draw - see stepDepth - and asking all 249 cards costs a
	// quarter of a millisecond a frame, most of it spent on cards that were
	// never on screen to be clicked. A widget that registered no area receives
	// nothing, so this is the whole answer and not an approximation of one.
	drawnFamilies []int
	drawnEntries  []int
	variantBtns   []widget.Draggable

	// The character builder (builder.go): open over the Editor while builder
	// is not nil, from the custom characters' card, whose tile that adds one
	// is addCharacterBtn. items is what it offers, read once, with each
	// one's folded search text and an index by key; itemArt their icons.
	builder         *builder
	addCharacterBtn widget.Clickable
	// flaskBtn only carries the fixed flask cell's hover, for its tooltip.
	flaskBtn   widget.Clickable
	items      []application.BuildItem
	itemSearch []string
	itemIndex  map[string]int
	itemArt    icons

	// dragging is whether a catalog drag is in flight, which is when the grid's
	// cells are drop targets.
	dragging bool
	// allAshesBtn leaves an open Ash for the list of all of them.
	allAshesBtn widget.Clickable

	// The placement grid (placement.go). cellTags are the cells' drop targets
	// and tokenTags the summons' input areas - the address is the tag. dropCell
	// is the cell a catalog drag is over, and dropFrames how many more frames
	// the cells are asked for a drop once the drag has ended. chosenSlot is the
	// token the arrow keys turn and whose sight shows, and together the others
	// shift-clicked with it, which move and turn with it (picked); hoverSlot
	// the token under the pointer; pressSlot the token the pointer went down
	// on, which moving says is being dragged to moveAt.
	cellTags   []int
	dropCell   int
	dropFrames int
	tokenTags  [maxSlots]int
	chosenSlot int
	together   []int
	hoverSlot  int
	pressSlot  int
	pressAt    f32.Point
	moving     bool
	moveAt     f32.Point
	// doubleTap is the system's double-click time (platform/clicks): a
	// catalog card's grace after a click (cardOpened).
	doubleTap time.Duration
	// knobTags are the turn knobs' inputs, one a slot; knobHover is the slot
	// whose knob the pointer is on, and turning the one whose knob is being
	// dragged round to turnAngle.
	knobTags  [maxSlots]int
	knobHover int
	turning   int
	turnAngle float64
	// gridView is where the grid looks. gridTag is the ground's own input,
	// under everything on it: the wheel zooms, and a drag on the ground that
	// started at panFrom, pressed at panAt, moves the view from panView. A
	// press reaches both a token and the ground under it; claimed says a
	// token or the knob took the press made at claimedAt, so the ground lets
	// that one go. letGo is the summons a click on the ground pressed at
	// letGoAt let go, the chosen one first, which a drag pressed within
	// doubleTap picks again: a touchpad's drag starts with that click.
	gridView   gridView
	gridTag    int
	claimed    bool
	claimedAt  time.Duration
	panning    bool
	panMoved   bool
	panFrom    f32.Point
	panView    gridView
	panAt      time.Duration
	letGo      []int
	letGoAt    time.Duration
	zoomInBtn  widget.Clickable
	zoomOutBtn widget.Clickable
	homeBtn    widget.Clickable
	// showSight and showKnobs are the grid's two switches, stored in
	// settings: what each summon notices by itself, and the turning knobs.
	showSight, showKnobs widget.Bool
	// The divider between the Editor's two halves (views.go): dragging it
	// sizes the catalog, stored in settings.CatalogWidth when let go.
	splitTag   int
	splitHover bool
	splitting  bool
	splitMoved bool

	// The creature window (creature.go): which slot it is open on, or noToken,
	// its controls, and the refusal its last change met.
	creatureSlot    int
	creatureClose   widget.Clickable
	creatureScrim   widget.Clickable
	creatureRemove  widget.Clickable
	roleBtns        [len(roles)]widget.Clickable
	strengthBtns    [len(strengths)]widget.Clickable
	shareBox        widget.Bool
	enemyBox        widget.Bool
	lookBox         widget.Bool
	creatureRefusal string
	// The explanations (help.go): each topic's "?", the one a click keeps open
	// or noHelp, the open bubble's own input, and whether this frame drew it.
	helpBtns   [helpTopics]widget.Clickable
	helpPinned helpTopic
	helpTag    int
	helpShown  bool
	// noteOver is the catalog's note mark under the pointer (marks.go).
	noteOver noteTag
	// The creature window's facing (facing.go).
	facingButtons

	// The price fields hold one Ash at a time. priceFor says which, so moving
	// to another Ash reloads them instead of carrying a number across; priceErr
	// is what the fields say when what was typed cannot be staged; priceKeys
	// is the one that had the keys last frame (selectOnFocus).
	fpField   widget.Editor
	hpField   widget.Editor
	priceFor  int32
	priceErr  string
	priceKeys *widget.Editor
	// freeBox is the open Ash's Free box, and allFreeBox Settings' All free;
	// each is set from what is staged every frame before it is drawn.
	freeBox, allFreeBox widget.Bool
	// wideAreaBox and noRestLimitBox are Settings' two monument switches.
	wideAreaBox, noRestLimitBox widget.Bool

	// The Characters view, which is also where a save is opened. person indexes
	// loaded.Characters and is -1 when none is chosen.
	person      int
	personList  widget.List
	personBtns  []widget.Clickable
	grantList   widget.List
	grantBtns   []widget.Clickable
	grantAllBtn widget.Clickable
	// The upgrade-level fields: one per Ash, plus the one beside "Add all"
	// that sets every row at once.
	//
	// Only an Ash the character HOLDS has a field: on one they do not, the
	// level is the header's, because that is the level Add will use.
	//
	// grantLevelSliders is one upgrade-level slider per row and grantAllSlider
	// is the one over the whole list. Sliders rather than fields because a
	// level is one of eleven values and nothing else: a typed field can hold
	// "23" or "" and then has to explain itself. Sliders rather than lists
	// because a list opened under a row near the window's foot ran off the
	// screen.
	//
	// Nothing has to be pushed into them either: a slider is told what to show
	// every frame, so there is no last-written value to remember.
	grantLevelSliders []levelSlider
	grantAllSlider    levelSlider
	// grantAll is the level the header's chooser is on, which is what Add uses
	// and what setting the whole list applies.
	grantAll int
	levelFor int

	// The footer's list of staged changes, and the reset.
	pendingBtn   widget.Clickable
	pendingOpen  bool
	pendingList  widget.List
	pendingClose widget.Clickable
	pendingScrim widget.Clickable
	dropBtns     []widget.Clickable
	// foldBtns are the dialog's arrows, one per line like dropBtns, and
	// pendingFolded what the player folded, by section name or group key. It
	// outlives the dialog, so a section folded stays folded the next time it
	// opens.
	foldBtns      []widget.Clickable
	pendingFolded map[string]bool
	resetBtn      widget.Clickable

	// The dialog Esc opens to close the editor (quit.go), and quit, which
	// closes it: the window's close, set with the window. openAfter is the
	// same dialog asked before another save replaces unsaved work: what to
	// open once the player says yes.
	quitOpen                         bool
	quitScrim, quitCancel, quitClose widget.Clickable
	quit                             func()
	openAfter                        func()
	// fullScreen is the Settings switch for full screen; settings.FullScreen
	// is the stored answer.
	fullScreen widget.Bool

	// edits is what the player has composed, by Ash. It is drawn; the session
	// holds the same formations as what will be written. See staging.go.
	edits map[int32]*formationEdit

	// The message line (notice.go): which message it last drew and since
	// when, the one a press settled, and the press catcher's tag.
	noticeShown, noticeSettled uint64
	noticeSince                time.Time
	pressTag                   int

	mu         sync.Mutex
	busy       bool
	busyMsg    string
	loaded     *savedView
	justLoaded bool
	// justSaved marks the load a save made: the same file's Ashes and
	// characters, so the frame keeps the player's place.
	justSaved bool
	notice    notice
	// noticeSeq counts the messages set, so the frame can tell a new one
	// from the same one said again.
	noticeSeq uint64
}

// savedView is what the frame is allowed to know about the loaded save.
//
// A worker builds one and hands it over; nothing mutates it afterwards. Every
// later phase adds fields here rather than reaching into the session mid-frame,
// and the counts below are the first and smallest example of the shape.
type savedView struct {
	Name              string
	Path              string
	RegulationVersion uint32

	// Ashes is the rail: every Spirit Ash in catalog order, with the ten
	// slots each one currently delivers.
	Ashes []ashView
	// Containers are the Ashes the Editor lists, as indexes into Ashes: all
	// but those whose summon the game binds itself (application.FixedHost),
	// which hold nothing to edit (the Kristoff Ash).
	Containers []int
	// Families is the spirit catalog as this save can deliver it - the cards,
	// each holding its entries, each holding their variants - and Spirits the
	// variants across all of them.
	Families []familyView
	// all is every family the save can deliver, broken rows included, and
	// Families the same pruned for the switch and the subcategory
	// (spiritmodel.prune), prunedFor saying what for. The worker builds both
	// with nothing chosen; the frame re-prunes without a load.
	all       []familyView
	prunedFor pruning
	// Categories is the filter's vocabulary: the hierarchy's categories in
	// its order, the Ashes leading.
	Categories []string
	// Subcategories is the second filter's, per category: "All
	// subcategories" first (spiritmodel.subcategoryNames).
	Subcategories map[string][]string
	// CategoryLabels and SubcategoryLabels are the same words as the
	// dropdowns show them (categoryLabel), index for index. The filter
	// compares the words, never the labels.
	CategoryLabels    []string
	SubcategoryLabels map[string][]string
	Spirits           int
	// Characters is the ten slots. Read once when the save loads: it costs a
	// 26 MB read of its own and nothing about it changes until a commit. What
	// each one owns is measured against the Ashes rail above, which already
	// carries every family and its name.
	Characters []characterView
	// labels names an actor row, under both the world row an authored slot
	// carries and the summon-side row a shipped formation names. It comes from
	// Session.ActorLabels and is ungrouped - see buildAshes for why that
	// matters - so it can label any row a drop composes, not only the rows the
	// browsable Families list represents.
	labels map[int32]application.ActorLabel
	// pictures is which picture a slot's actor shows (pictures.go).
	pictures pictures
	// bodies is each family's footprint on the ground, by model, for the
	// grid's overlap tint.
	bodies map[string]application.Body
	// choices are what each weapon can be built as in this save's
	// regulation - its affinities, its top level, the Ashes of War it takes -
	// and what each Ash allows. Read when the builder first opens
	// (builder.go), since nothing else needs them.
	choices *application.BuildChoices
}

// dragged names what a drag payload identifies, from the hierarchy the panel
// drew it from: the variant's label, its card and its portrait model. A person
// is found by its CharaInitParam row, a creature by its two rows.
func (loaded *savedView) dragged(npc, think, person int32) (application.ActorLabel, bool) {
	label, _, ok := loaded.draggedVariant(npc, think, person)
	return label, ok
}

// draggedVariant is dragged with the variant itself, which says whether the
// card is delivered as shipped and on which stance.
func (loaded *savedView) draggedVariant(npc, think, person int32) (application.ActorLabel, variantView, bool) {
	return loaded.findVariant(func(variant variantView) bool {
		return variant.Person == person && (person != 0 || (variant.Npc == npc && variant.Think == think))
	})
}

// findVariant is the first variant the hierarchy holds that match accepts,
// named as a drop of it is named.
func (loaded *savedView) findVariant(match func(variantView) bool) (application.ActorLabel, variantView, bool) {
	for _, family := range loaded.all {
		for _, entry := range family.Entries {
			for _, variant := range entry.Variants {
				if !match(variant) {
					continue
				}
				return application.ActorLabel{Name: variant.Label, Family: family.Name, Model: variant.Model}, variant, true
			}
		}
	}
	return application.ActorLabel{}, variantView{}, false
}

// notice is the footer's one line: what just happened, and whether it went
// wrong. A cancelled dialog produces no notice at all, because nothing
// happened.
//
// bad and warn are different things and at most one of them is ever true. bad
// is a refusal: the editor did not do what was asked. warn is the opposite -
// the edit was made, and there is something about it worth knowing before it
// is carried into a fight.
type notice struct {
	text string
	bad  bool
	warn bool
}

// NewState builds the editor over a service's catalog and a dialog adapter.
// The session starts with no save, which is the application's real initial
// state and not a placeholder for one.
func NewState(service *application.Service, picker dialogs.Service) *State {
	state := &State{
		session:      service.NewSession(),
		dialogs:      picker,
		art:          ashArt(),
		entityArt:    entityArt(),
		itemArt:      itemArt(),
		th:           newTheme(),
		view:         viewCharacters,
		selected:     noAsh,
		family:       noFamily,
		entry:        noEntry,
		dropCell:     noCell,
		chosenSlot:   noToken,
		hoverSlot:    noToken,
		pressSlot:    noToken,
		knobHover:    noToken,
		turning:      noToken,
		helpPinned:   noHelp,
		doubleTap:    clicks.DoubleClickTime(),
		cardOpened:   noCard,
		builderWait:  noCard,
		gridView:     homeView,
		creatureSlot: noToken,
		priceFor:     noPrice,
		person:       noPerson,
		levelFor:     noPerson,
	}
	state.ashList.Axis = layout.Vertical
	state.familyList.Axis = layout.Vertical
	state.pendingList.Axis = layout.Vertical
	state.grantList.Axis = layout.Vertical
	state.personList.Axis = layout.Vertical
	state.settingsList.Axis = layout.Vertical
	state.search.SingleLine = true
	// Submit so Enter loads the typed path, which is what a player who just
	// pasted one expects rather than a reach for the button beside it.
	state.pathField.SingleLine, state.pathField.Submit = true, true
	// The level fields hold one small number each and Enter means "that is what
	// I typed", the same as it does for a price.

	// Submit so Enter commits a price the way a player expects, even though
	// every keystroke has already staged it.
	state.fpField.SingleLine, state.fpField.Submit = true, true
	state.hpField.SingleLine, state.hpField.Submit = true, true
	// Read rather than applied: applySettings writes the file, and starting
	// the editor is not a change the player made.
	state.settings = loadSettings().normalize()
	state.iconSlider.Value = sliderFromIconSize(state.settings.IconSize, ashSizes)
	state.editorIconSlider.Value = sliderFromIconSize(state.settings.EditorIconSize, ashSizes)
	state.spiritIconSlider.Value = sliderFromIconSize(state.settings.SpiritIconSize, spiritSizes)
	state.fullScreen.Value = state.settings.FullScreen
	state.showSight.Value, state.showKnobs.Value = state.settings.ShowSight, !state.settings.HideKnobs
	state.th.Face = typefaceFor(state.settings.Font)
	state.applyPalette()
	return state
}

// SetWindow wires the window a worker invalidates when it has something new to
// show, and the scale its frames are drawn at beyond the system's.
func (s *State) SetWindow(win *app.Window, scale float32) {
	s.win, s.quit, s.windowScale = win, closer(win), scale
}

// Theme is the material theme every widget style is built from.
func (s *State) Theme() *material.Theme { return s.th }

// invalidate asks for a redraw. Safe from any goroutine; a nil window (tests)
// is not an error, because a test that never draws still exercises the state
// machine.
func (s *State) invalidate() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

// --- snapshot accessors, safe from the frame at any time ---

// Busy reports whether a worker currently holds the session.
func (s *State) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

// BusyMsg describes the in-flight work, or "" when idle.
func (s *State) BusyMsg() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.busy {
		return ""
	}
	return s.busyMsg
}

// LoadedSave is the open save as the frame may describe it, or nil when there
// is none.
func (s *State) LoadedSave() *savedView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loaded
}

// setNotice replaces the footer's message from either goroutine.
func (s *State) setNotice(message notice) {
	s.mu.Lock()
	s.notice = message
	s.noticeSeq++
	s.mu.Unlock()
}

// Notice is the footer's last message.
func (s *State) Notice() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notice.text, s.notice.bad
}

// Warned is whether that message is a caution rather than news or a refusal.
// Separate from Notice rather than a third return value, because every caller
// of Notice wants the text and almost none of them want this.
func (s *State) Warned() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notice.warn
}

// The three "nothing is chosen" sentinels. Each is how the editor starts, and
// what it returns to when the thing that was chosen stops existing.
const (
	noAsh    = -1
	noFamily = -1
	noEntry  = -1
	noPerson = -1
)

// consumeJustLoaded reports, once, that a save finished loading, and whether
// it was the file the player just saved.
//
// The worker cannot reset the frame's state itself - which view is showing is
// frame state - so it leaves this behind and the frame acts on it.
func (s *State) consumeJustLoaded() (just, saved bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	just, saved = s.justLoaded, s.justSaved
	s.justLoaded, s.justSaved = false, false
	return just, saved
}

// PendingCount is how many staged changes a commit would write.
//
// It asks the session, so it answers 0 while a worker holds it - which is
// correct rather than approximate: during a load there is nothing staged, and
// after one the session has cleared what there was.
func (s *State) PendingCount() int {
	if s.Busy() || !s.session.Loaded() {
		return 0
	}
	return s.session.PendingCount()
}

// --- worker handshake ---

// begin claims the session for a worker. It reports false if one already has
// it, which is how a second Open while a load is in flight is ignored.
func (s *State) begin(message string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	s.busy = true
	s.busyMsg = message
	s.notice = notice{}
	s.noticeSeq++
	return true
}

// progress updates what the in-flight work says it is doing.
func (s *State) progress(message string) {
	s.mu.Lock()
	s.busyMsg = message
	s.mu.Unlock()
	s.invalidate()
}

// finish hands the session back and records what happened. A worker calls it
// exactly once, on every path out including a cancel.
func (s *State) finish(loaded *savedView, result notice) {
	s.finishLoad(loaded, false, result)
}

// finishLoad is finish, saying whether loaded is the file just saved.
func (s *State) finishLoad(loaded *savedView, saved bool, result notice) {
	s.mu.Lock()
	s.busy = false
	s.busyMsg = ""
	if loaded != nil {
		s.loaded = loaded
		s.justLoaded = true
		s.justSaved = saved
	}
	s.notice = result
	s.noticeSeq++
	s.mu.Unlock()
	s.invalidate()
}
