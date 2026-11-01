// Package dialogs provides the desktop file-picker adapter.
//
// Ported from the sibling ER Merchant Editor, which is the same author's and
// under the same licence. Keeping this behind an interface lets Linux move to
// XDG portals, and macOS to a bundled native implementation, without touching
// the editor core or the views - and lets a test substitute a picker that
// returns a path without opening anything.
package dialogs

import "github.com/ncruces/zenity"

// ErrCanceled identifies a user-dismissed dialog. A cancel is not a failure
// and must not be reported as one.
var ErrCanceled = zenity.ErrCanceled

// Service selects input and output save paths.
type Service interface {
	OpenSave() (string, error)
	SaveAs(suggested string) (string, error)
}

// Native is the cross-platform desktop implementation.
type Native struct{}

// NewNative constructs the default desktop dialog service.
func NewNative() Native { return Native{} }

// OpenSave asks for a decrypted Elden Ring save.
func (Native) OpenSave() (string, error) {
	return zenity.SelectFile(
		zenity.Title("Open Elden Ring save file"),
		zenity.FileFilters{
			// .dat is the decrypted PlayStation export. The container is
			// detected from its magic and the regulation version from its
			// contents, never from the extension, so this is a convenience.
			{Name: "Save files", Patterns: []string{"*.dat"}, CaseFold: true},
			{Name: "All files", Patterns: []string{"*"}},
		},
	)
}

// SaveAs asks where to write the edited save. The editor never writes the file
// it loaded, so this is asked for every commit rather than remembered.
func (Native) SaveAs(suggested string) (string, error) {
	return zenity.SelectFileSave(
		zenity.Title("Save edited file as"),
		zenity.Filename(suggested),
		zenity.ConfirmOverwrite(),
		zenity.FileFilters{
			{Name: "Save files", Patterns: []string{"*.dat"}, CaseFold: true},
			{Name: "All files", Patterns: []string{"*"}},
		},
	)
}
