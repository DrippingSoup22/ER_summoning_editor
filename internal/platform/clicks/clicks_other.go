//go:build !windows

package clicks

import "time"

// DoubleClickTime is Windows' default where the system is not Windows.
func DoubleClickTime() time.Duration { return fallback }
