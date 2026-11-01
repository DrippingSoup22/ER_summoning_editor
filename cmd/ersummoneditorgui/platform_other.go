//go:build !linux

package main

// Windows and macOS give Gio their own display scale.
func platformUIScale() float32 { return 1 }

// preparePlatform has nothing to prepare outside Linux.
func preparePlatform() {}
