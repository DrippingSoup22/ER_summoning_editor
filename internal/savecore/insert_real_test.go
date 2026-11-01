// Row insertion rests on a set of claims about how the shipped regulation is
// laid out — contiguous binder allocations, stored entries, names ahead of
// data, row bodies in header order. Those were measured rather than assumed,
// and this is where the measurement lives, so a regulation that stops matching
// fails a test instead of producing a save nobody can load.
//
// It needs the private research checkout and skips without it, like the golden
// conversion test. The synthetic fixture proves the same behavior in a clean
// public clone; this proves the fixture is describing the real thing.
package savecore

import (
	"os"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

func realSave(t *testing.T) *regulation.Regulation {
	t.Helper()
	source, err := regulation.Load(realSavePath(t))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

// realSavePath is the same file for callers that need to go through the
// product's own entry points rather than through load.
func realSavePath(t *testing.T) string {
	t.Helper()
	path := os.Getenv("ER_SAVE")
	if path == "" {
		t.Skip("ER_SAVE not set")
	}
	return path
}
