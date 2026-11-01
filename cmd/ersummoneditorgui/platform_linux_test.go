//go:build linux

package main

import (
	"os"
	"testing"
)

func TestPlatformUIScale(t *testing.T) {
	t.Setenv("WSL_DISTRO_NAME", "")
	t.Setenv("WSL_INTEROP", "")
	if got := platformUIScale(); got != 1 {
		t.Fatalf("native Linux scale = %v, want 1", got)
	}

	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
	if got := platformUIScale(); got != 1.25 {
		t.Fatalf("WSL scale = %v, want 1.25", got)
	}
}

func TestOnlyWSLLeavesWayland(t *testing.T) {
	t.Setenv("WSL_DISTRO_NAME", "")
	t.Setenv("WSL_INTEROP", "")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	preparePlatform()
	if os.Getenv("WAYLAND_DISPLAY") != "wayland-0" {
		t.Fatal("native Linux lost its Wayland display")
	}

	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
	preparePlatform()
	if display, set := os.LookupEnv("WAYLAND_DISPLAY"); !set || display != "" {
		t.Fatalf("under WSL WAYLAND_DISPLAY is %q (set %v), want set and empty", display, set)
	}
}
