// Package clicks reads how far apart two clicks may come and still be one
// double click, from the system the editor runs on.
//
// The editor needs it because a touchpad's tap-and-drag arrives as a click
// and then a press: a single click that acts at once would open a window, or
// replace a catalog card, under the press that was meant to drag. So a click
// on something that can also be dragged waits this long first, and the
// system's own double-click time is the time its tap-and-drag keeps to.
package clicks

import "time"

// fallback is Windows' own default, for a system that does not say.
const fallback = 500 * time.Millisecond
