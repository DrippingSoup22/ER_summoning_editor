package application

import (
	"errors"
	"fmt"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// A frontend depends only on this package, so every failure it needs to tell
// apart is reachable from here. Matching on message text is the thing these
// exist to replace: messages are written for people and change freely.
//
// The categories below are re-exported from the save layer unchanged, so
// errors.Is works across the boundary without this package having to recognise
// a failure by its spelling.
var (
	// ErrNotDecryptedPS4Save means the chosen file is not a decrypted PS4
	// save. The fix is a different file — offer the picker again.
	ErrNotDecryptedPS4Save = savecore.ErrNotDecryptedPS4Save

	// ErrUnsupportedRegulation means the save was read but its layout is not
	// one this build can write. There is no option to change that makes it
	// work.
	ErrUnsupportedRegulation = savecore.ErrUnsupportedRegulation

	// ErrOutputExists means the output path is taken. Offer a different name;
	// the product never overwrites.
	ErrOutputExists = savecore.ErrOutputExists

	// ErrRowNotFound means this save lacks a row the conversion needs.
	ErrRowNotFound = savecore.ErrRowNotFound

	// ErrNoChange means the save already says what the request asks for.
	ErrNoChange = savecore.ErrNoChange

	// ErrSourceMismatch means the file on disk is no longer the one a plan was
	// made against. A session pins the save it loaded, so this is what a
	// frontend gets when the save was replaced while the player was editing.
	ErrSourceMismatch = savecore.ErrSourceMismatch
)

// Failures that belong to this layer rather than to the save format.
var (
	// ErrPathRequired means an input or output path was left empty.
	ErrPathRequired = errors.New("input and output paths are required")

	// ErrPathCollision means two of the three paths name the same file, which
	// would destroy an input or overwrite the result.
	ErrPathCollision = errors.New("paths must name different files")

	// ErrNoConversions means the request asked for no work.
	ErrNoConversions = errors.New("at least one conversion is required")

	// ErrAuthoringTakesNoConversions means one request described both a
	// conversion and an authored formation. A conversion re-points the host at
	// another family's formation; authoring rewrites the host's own. Together
	// they name two different formations for one item.
	ErrAuthoringTakesNoConversions = errors.New("a conversion and an authored formation cannot be asked for together")

	// ErrNoSlots means an authoring request named no actors. An empty formation
	// is not a smaller formation; it is a request to summon nothing, which a
	// reset expresses and this does not.
	ErrNoSlots = errors.New("an authored formation needs at least one actor")

	// ErrAshLevelOwed means a slot asked for a level other than the Follower on
	// a creature row FromSoftware ships as an Ash. That row IS the Follower
	// (the Follower level was built as a copy of what FromSoftware ships) and
	// is never written, so another level needs a second row of the creature -
	// a copy of FromSoftware's row in a clone band above 1, owed until
	// research shows the game summons one.
	ErrAshLevelOwed = errors.New("a creature of the game's own Ashes takes the Follower, its shipped row; another level is not built yet")

	// ErrLookFixed means a slot chose a look its creature does not offer: a
	// body the ghost loses a part of is drawn in its own colours, a creature
	// with a tint of its own keeps it, and a person or a row the game ships
	// arrives as shipped (CreatureRules).
	ErrLookFixed = errors.New("this creature's look is not the player's to choose")

	// ErrLevelNotOffered means a slot asked a creature for a role the author
	// ruled it does not take (the large Shadow Commoner: the Statue and the
	// Hunter only; CreatureRules).
	ErrLevelNotOffered = errors.New("this creature does not take that role")

	// ErrOneCreatureTwoStrengths means two slots of one formation name the same
	// creature and disagree about a knob only a research seat sets - its extra
	// residents or the bare row. They are recorded per creature, so the two
	// slots would silently share one answer. A player never reaches it: the
	// GUI offers neither.
	ErrOneCreatureTwoStrengths = errors.New("two slots of one creature ask for different strength knobs, and one creature row carries one")

	// ErrHeldRowStrength means a role was asked for a row that cannot take it:
	// a Fighter slot on a row the save already holds, which is never cloned and
	// so carries the row's own strength, or a role for a row that is not one
	// of the game's own summon rows (Session.SetRowFighter) - a clone this
	// editor wrote keeps the role it was written with.
	ErrHeldRowStrength = errors.New("a row the save holds keeps its strength; only the game's own summon rows take another, row-wide")

	// ErrResetTakesNoConversions means a reset was asked for alongside a
	// conversion. A reset restores every row the product can write, so the two
	// requests contradict each other and there is no order that satisfies both.
	//
	// It is a rule about these methods, not about what a save write may carry.
	// Each of the three performs one operation and refuses the other two, which
	// is what a command line wants: one invocation is one intent. A Session
	// carries all three at once and writes them together; see session.go.
	ErrResetTakesNoConversions = errors.New("a reset restores the whole save and cannot be combined with a conversion")

	// ErrNoSaveLoaded means a session was asked to stage or commit before a
	// save was opened. Loading one is the first thing a frontend does and the
	// only thing available until it succeeds.
	ErrNoSaveLoaded = errors.New("no save is loaded")
	// ErrStagedChangesCannotCombine means the staged set is individually valid
	// and cannot be written as one save. It is separated from the stage-time
	// refusals because there is nothing wrong with any one change: what a
	// frontend should offer is dropping one of them, not correcting any.
	ErrStagedChangesCannotCombine = errors.New("these staged changes cannot be written together")
	// ErrBadBuild means a custom character asks for something the game cannot
	// equip or its rows cannot hold: a weapon past its upgrade ladder, a bolt
	// in an arrow slot, an attribute outside 1 to 99.
	ErrBadBuild = character.ErrBadBuild
	// ErrBuildNamed means a custom character was asked to be deleted while a
	// formation the save will hold still summons it. Taking it out of that
	// formation first is what a frontend should offer.
	ErrBuildNamed = errors.New("a formation summons this character")

	// ErrNothingStaged means a commit was asked for with no pending changes.
	// It is not a failure of the request so much as the absence of one, and a
	// frontend should disable Save rather than report it.
	ErrNothingStaged = plan.ErrNothingStaged

	// ErrAlreadyVanilla means a reset was asked for on a save that already
	// holds every shipped value. Like ErrNothingStaged it is an answer rather
	// than a fault: it is what the button asked, and a frontend reporting it
	// as an error tells the player something went wrong when nothing did.
	ErrAlreadyVanilla = plan.ErrAlreadyVanilla
)

// HostStagedTwiceError reports one Ash with both an authored formation and a
// conversion staged against it.
//
// A conversion re-points the host at another family's formation; authoring
// rewrites the host's own. Together they name two different formations for one
// item, and there is no order in which both survive - so this is refused while
// the player is still staging rather than resolved silently at Save.
type HostStagedTwiceError struct {
	Name        string
	GoodsFamily int32
	// Destination is the family the staged conversion points at.
	Destination string
}

func (e *HostStagedTwiceError) Error() string {
	return fmt.Sprintf("%s has both an authored formation and a conversion to %s staged; drop one of them",
		e.Name, e.Destination)
}

// UnsupportedRegulationError reports a save built for a different game version.
//
// It carries both numbers because the frontend has to tell the user which
// version they have and which one is needed, and because the failure is not the
// user's mistake: a save whose regulation predates the installed game has that
// regulation silently replaced on load, so the edit would appear to do nothing.
type UnsupportedRegulationError struct {
	Found uint32
}

func (e *UnsupportedRegulationError) Error() string {
	return fmt.Sprintf("unsupported regulation version %d; this build supports %s", e.Found, SupportedRegulations())
}

// Unwrap makes the re-exported sentinel true for this error as well. Without
// it a frontend would have to know that a version mismatch found by this layer
// is a different Go value from one found by the save layer, even though both
// mean "this build cannot write that file" and both lead to the same dialog.
func (e *UnsupportedRegulationError) Unwrap() error { return ErrUnsupportedRegulation }

// UnknownAshError reports a selector that matched no Spirit Ash. Role is
// "host" or "destination", and Selector is what the user actually typed, so a
// frontend can highlight the right field and offer near matches.
type UnknownAshError struct {
	Role     string
	Selector string
	cause    error
}

func (e *UnknownAshError) Error() string {
	return fmt.Sprintf("%s %q: %v", e.Role, e.Selector, e.cause)
}

func (e *UnknownAshError) Unwrap() error { return e.cause }

// DestinationNotShippedError reports a destination family whose own goods rows
// no longer point at any formation the catalog says it ships.
//
// It means the save was edited before, and that family was the host of that
// edit. Copying what it says now would spread the earlier edit into a second
// family without saying so, and the catalog would stop describing the save.
type DestinationNotShippedError struct {
	Name        string
	GoodsFamily int32
	// Level is the upgrade level whose row is wrong, zero-based.
	Level int
	// Found is the trigger that row points at now.
	Found int32
}

func (e *DestinationNotShippedError) Error() string {
	return fmt.Sprintf("%s +%d points at formation %d, which is not one it ships; that family was converted by an earlier edit",
		e.Name, e.Level, e.Found)
}

// LevelCountMismatchError reports a host and destination with different numbers
// of upgrade levels, which leaves no correct way to line their levels up.
type LevelCountMismatchError struct {
	HostName          string
	HostLevels        int
	DestinationName   string
	DestinationLevels int
}

func (e *LevelCountMismatchError) Error() string {
	return fmt.Sprintf("%s has %d upgrade levels and %s has %d; there is no correct mapping between them",
		e.HostName, e.HostLevels, e.DestinationName, e.DestinationLevels)
}

// DuplicateHostError reports one Ash family requested as the host of two
// conversions. The second would silently overwrite the first.
type DuplicateHostError struct {
	GoodsFamily int32
	First       string
	Second      string
}

func (e *DuplicateHostError) Error() string {
	return fmt.Sprintf("goods family %d is converted twice, by %s and %s", e.GoodsFamily, e.First, e.Second)
}

// HostFixedError reports an Ash whose summon the game resolves without the
// params, so a formation written for it is never delivered.
//
// Ancient Dragon Knight Kristoff (256000) is the one: an authored block under
// its key still delivers the shipped Kristoff, though the block is read and
// its row count honoured.
// Converting it, or authoring into it, would stage a change the game ignores.
type HostFixedError struct {
	Name        string
	GoodsFamily int32
}

func (e *HostFixedError) Error() string {
	return fmt.Sprintf("%s summons what the game itself binds to the item, so its formation cannot be changed", e.Name)
}

// HostRePointedError reports an Ash that no longer delivers its own formation,
// which is what an earlier conversion leaves behind.
//
// Authoring into it would rewrite a formation belonging to a different item -
// changing that item for the player, and leaving the one they named unchanged.
type HostRePointedError struct {
	Name string
	// Shipped is the formation this Ash ships with; Found is what its rows
	// point at now.
	Shipped int32
	Found   int32
}

func (e *HostRePointedError) Error() string {
	return fmt.Sprintf("%s was converted by an earlier edit and now delivers formation %d instead of its own %d; reset the save before authoring into it",
		e.Name, e.Found, e.Shipped)
}

// UnknownActorError reports a slot naming a row this save does not hold.
//
// It is not the same as an actor that needs cloning: a clone is copied from the
// world row, so a world row that is not there leaves nothing to copy from.
type UnknownActorError struct {
	Slot  int
	Table string
	RowID int32
}

func (e *UnknownActorError) Error() string {
	return fmt.Sprintf("slot %d names %s row %d, which this save does not hold", e.Slot, e.Table, e.RowID)
}

func (e *UnknownActorError) Unwrap() error { return ErrRowNotFound }

// SlotRuleError is a slot that asked its creature for something the creature
// does not allow (CreatureRules): ErrLookFixed or ErrLevelNotOffered, with why
// in a few words.
type SlotRuleError struct {
	Slot  int
	Npc   int32
	Why   string
	cause error
}

func (e *SlotRuleError) Error() string {
	return fmt.Sprintf("slot %d, row %d: %v: %s", e.Slot, e.Npc, e.cause, e.Why)
}

func (e *SlotRuleError) Unwrap() error { return e.cause }
