package savecore

import (
	"errors"
	"fmt"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// These sentinels exist so a caller can tell one failure from another without
// matching on message text. A frontend never sees them directly — it depends
// only on internal/application, which re-exports the ones it needs — but the
// application layer cannot translate a failure it can only recognise by
// spelling.
//
// Each is joined to a message that keeps the specifics: the row that was
// missing, the version that was found. Use errors.Is for the category and the
// message for the detail.
var (
	// ErrNotDecryptedPS4Save means the file is not a decrypted PS4 save at
	// all: too small, still carrying console encryption, or a PC save. The
	// user's fix is to supply a different file, not to change an option.
	ErrNotDecryptedPS4Save = regulation.ErrNotDecryptedSave

	// ErrUnsupportedRegulation means the container was recognised but its
	// contents are a shape this build cannot write: an unknown compression
	// format, or a PARAM whose schema or row stride is not what v1 expects.
	// Refusing is the safe outcome; patching at guessed offsets is not.
	ErrUnsupportedRegulation = regulation.ErrUnsupportedRegulation

	// ErrOutputExists means publishing would destroy a file that is already
	// there. The product never overwrites.
	ErrOutputExists = regulation.ErrOutputExists

	// ErrSourceMismatch means the manifest was planned against a different
	// save, so its row IDs describe someone else's regulation.
	ErrSourceMismatch = errors.New("manifest was planned against a different save")

	// ErrRowNotFound means the save does not contain a row the plan needs, or
	// a trigger points at an SpEffect row that is not there. Either would be a
	// silent no-op in game.
	ErrRowNotFound = errors.New("the save is missing a row the plan needs")

	// ErrManifestInvalid means the manifest itself is malformed or asks for
	// something outside what v1 is allowed to write.
	ErrManifestInvalid = errors.New("invalid manifest")

	// ErrNoChange means the manifest would produce a file identical to its
	// input. Handing that back as an edited save would be a lie.
	ErrNoChange = errors.New("the plan changes nothing in this save")
)

// loadSave opens a save for this product, which edits PS4 saves only: its
// character code knows the PS4 slot layout, and a PC save would also need every
// written region's MD5 refreshed.
func loadSave(path string) (*regulation.Regulation, error) {
	loaded, err := regulation.Load(path)
	if err == nil && loaded.Platform != regulation.PS4 {
		return nil, fmt.Errorf("%s is a %s save: %w", path, loaded.Platform, ErrNotDecryptedPS4Save)
	}
	return loaded, err
}
