package plan

// The colour switch: whether a summon renders in the white spirit look or in
// the colours the creature has in the world.
//
// # What it writes, and why it is one field
//
// A summon carries resident effect 295000, which delivers vfx 57000, which sets
// phantomParamOverwriteId to PhantomParam 200 - the profile whose shipped name
// is "Spirit Summon". Moving that pointer to 0, the game's own spelling for no
// overwrite, takes the profile off and the creature renders as itself.
//
// The pointer is written rather than PhantomParam 200's colours because 200 has
// a second caller, vfx 51220, reached by unrelated effect 15032. Editing the
// palette would change that as well; editing the pointer cannot reach it.
//
// # What it covers, exactly
//
// 108 of the game's 115 summon-side actor rows carry resident effect 295000 and
// so pass through vfx 57000. The other seven - the five Puppet Ashes and the
// two Nightmaiden & Swordstress puppets - carry 295200, which delivers vfx
// 57100 and PhantomParam 201, a second profile this switch does not touch.
//
// Deliberate, on two counts. A Puppet's look is its identity rather than a coat
// over a creature, so a player asking to see creatures as they are is not asking
// about them. And vfx 57100 has TWO callers - 295200, and a bare effect 18672
// that nothing in the regulation references - so writing it would carry an
// unknown along, which is the very thing writing the pointer instead of the
// palette was chosen to avoid.
//
// An authored formation is unaffected either way: its actors are creature clones
// carrying 295000, whichever Ash hosts them. Only an untouched Puppet Ash
// summons through the second profile.
//
// # What it does NOT do
//
// PhantomParam 200 never repainted anything. Its diffMulColor is 255/255/255 at
// alpha 0.9 - a ninety percent multiplier on the creature's own diffuse - under
// a white rim at alpha 0.1 and a thin dark overlay at 0.25. So a summon looks
// white or not depending on how saturated the creature already is, which is why
// two identification runs found three models that kept their colours and no
// NpcParam field that explained them. This switch removes the coat; it does not
// restore a colour that was replaced, because none was.

import "fmt"

// OperationTint is the operation value a colour manifest carries.
const OperationTint = "tint"

// Tint is which way the colour switch is set.
//
// Like Area it has no zero value that means anything. A switch whose off
// position is also its unset position is not a switch: a caller that forgot to
// say would get one of the two positions silently, and which one it got would
// be an accident of which constant happened to be first.
type Tint int

const (
	// The zero value is refused, so the only Tints a caller can name are the
	// two real positions.
	_ Tint = iota
	// SpiritTint is the white look, and what the game ships: vfx 57000 points
	// at PhantomParam 200.
	SpiritTint
	// OriginalColours points the profile at a neutral one, so every summon
	// the file can deliver renders in the creature's own colours.
	OriginalColours
)

// String is what the pending list and the manifest call each position.
func (t Tint) String() string {
	switch t {
	case SpiritTint:
		return "the white spirit look, as the game ships"
	case OriginalColours:
		return "the creature's own colours"
	}
	return "unset"
}

// ErrTintUnset is returned when a caller did not say which way the switch is.
var ErrTintUnset = fmt.Errorf("the colour switch must say which way it is set")

// SpiritVfxRow is the SpEffectVfxParam row the switch writes: the one effect
// 295000 delivers, and the only row in the table this product ever touches.
//
// Exported because the application layer reads it back out of a loaded save to
// answer which way the switch already is - and reading and writing must name the
// same row or the control would report on one thing and change another.
const SpiritVfxRow = int32(57000)

// The two values the field takes. Both are shipped rows; neither position
// invents a number.
const (
	SpiritPhantomProfile = int64(200)
	// NeutralPhantomProfile is the off position, and it is a REDIRECT rather
	// than a removal. Clearing the field to 0 - the "no overwrite" value 1,457
	// of the 1,513 rows carry - does show the creature's own colours, and it
	// was what this switch once wrote. It also puts a red name plate
	// over every summon, because a summon has no RoleParam row of its own:
	// no row carries teamType 47, so it falls back to row 0, "Host of Fingers"
	// at roleNameColor 1, and the phantom overwrite had been suppressing that
	// fallback all along.
	//
	// PhantomParam 410 keeps an overwrite live while changing nothing:
	// diffMulColor 255/255/255 at alpha 1, with edgeColorA, frontColorA,
	// lightColorA and glowScale all zero. It multiplies nothing and adds
	// nothing, so the creature renders as itself and the role never speaks.
	// Seen on the two summons that had shown the plate.
	NeutralPhantomProfile = int64(410)
	// NoPhantomProfile is kept because TintOf still has to recognise a file
	// an older build of this switch wrote.
	NoPhantomProfile = int64(0)
)

// BuildTint is the colour switch as its own manifest.
func BuildTint(sourceSHA256 string, tint Tint) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationTint}
	if err := writeTint(manifest, tint); err != nil {
		return nil, err
	}
	return manifest, nil
}

// writeTint puts the switch into a manifest, in whichever position.
//
// Shared by BuildTint and by the session so the two cannot drift: what a
// session writes for a position is exactly what the switch alone writes for it.
func writeTint(manifest *Manifest, tint Tint) error {
	value := SpiritPhantomProfile
	switch tint {
	case OriginalColours:
		value = NeutralPhantomProfile
	case SpiritTint:
	default:
		return ErrTintUnset
	}
	manifest.VfxPatches = append(manifest.VfxPatches, RowPatch{
		RowID: SpiritVfxRow,
		Name:  "Colour switch: " + tint.String(),
		Set:   map[string]any{"phantomParamOverwriteId": value},
	})
	return nil
}

// TintOf reads a position back from the value a save holds.
//
// A file that holds neither shipped value is reported as SpiritTint rather than
// refused, and the caller is told it was not recognised. The switch has to
// answer for a file it did not write, and the honest answer for an unknown
// profile is that the summons are not showing their own colours - which is
// true of every value except 0.
func TintOf(phantomParamOverwriteID int64) (Tint, bool) {
	switch phantomParamOverwriteID {
	case NeutralPhantomProfile, NoPhantomProfile:
		// Both read as the creature's own colours: 410 is what this switch
		// writes and 0 is what an older build wrote, and a save
		// carrying either is showing colours rather than the spirit coat.
		return OriginalColours, true
	case SpiritPhantomProfile:
		return SpiritTint, true
	}
	return SpiritTint, false
}
