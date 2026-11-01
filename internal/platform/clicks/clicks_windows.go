package clicks

import (
	"syscall"
	"time"
)

// DoubleClickTime is the player's double-click speed, as Windows holds it
// (GetDoubleClickTime, user32 - a known DLL, always loaded from the system).
func DoubleClickTime() time.Duration {
	ms, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetDoubleClickTime").Call()
	if ms == 0 || ms > 5000 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}
