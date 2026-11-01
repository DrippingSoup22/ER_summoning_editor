package gio

// The size of what the binary embeds.

import (
	"os"
	"path/filepath"
	"testing"
)

// iconCeiling is what one embedded entity icon may weigh.
//
// A packed 128px icon is about 32 KB and the largest is well under this; a
// full-resolution master is 3.5 MB, a hundred times over. The gap is wide
// enough that this needs no tuning and catches the only mistake it is for.
const iconCeiling = 300 * 1024

// No master ever reaches the embedded tree.
//
// The desktop binary embeds this directory whole, because embed variables are
// not dead-code eliminated, so one master left in it is 3.5 MB of executable
// for a picture drawn into a cell 34 dp across. 93 of them accumulated
// unnoticed once and five more landed within the hour of clearing them out,
// which is what makes this a test rather than a note in a script.
//
// The fix when this fails is one command:
//
//	go run ./tools/artpack
func TestNoUnpackedMasterReachesTheBinary(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("assets", "entities"))
	if err != nil {
		t.Fatal(err)
	}
	total, heavy := int64(0), 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
		if info.Size() > iconCeiling {
			heavy++
			t.Errorf("%s is %.1f MB; it is a master, not an icon - run go run ./tools/artpack",
				entry.Name(), float64(info.Size())/(1024*1024))
		}
	}
	if heavy == 0 && len(entries) > 0 {
		t.Logf("%d icons, %.1f MB, average %.0f KB",
			len(entries), float64(total)/(1024*1024), float64(total)/float64(len(entries))/1024)
	}
}
