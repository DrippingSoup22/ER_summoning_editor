// Package paths resolves the writable per-user locations the desktop editor
// uses. Ported from the sibling ER Merchant Editor.
//
// Settings live in the OS user-config directory and never beside the
// executable: the release archive is meant to be portable, and a build that
// writes into its own folder stops being something you can drop on a USB stick
// or extract into Program Files.
package paths

import (
	"os"
	"path/filepath"
)

const directoryName = "er_summoning_editor"

// ConfigFile returns the platform-appropriate settings path.
func ConfigFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, directoryName, "config.json"), nil
}
