//go:build linux

package main

import "os"

// underWSL is whether the editor runs in WSL, whose window system needs two
// corrections that native Linux must not get.
func underWSL() bool {
	return os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != ""
}

// platformUIScale: WSLg reports a scale of 1 even when its Windows host runs at
// the usual 125%, so under WSL the window would be a fifth smaller than on
// Windows. Native Linux compositors report their own scale.
func platformUIScale() float32 {
	if underWSL() {
		return 1.25
	}
	return 1
}

// preparePlatform sends Gio to X11 under WSL. Through WSLg's Wayland, Mesa
// finds no driver and the window stays black; through X11 it falls back to
// software rendering and the window draws. Native Linux keeps Wayland.
//
// Gio tries Wayland first and X11 when that fails, and only build tags choose
// between them. An empty WAYLAND_DISPLAY makes the Wayland connection fail;
// an unset one would not, because libwayland then tries wayland-0, which WSLg
// provides.
func preparePlatform() {
	if underWSL() {
		os.Setenv("WAYLAND_DISPLAY", "")
	}
}
