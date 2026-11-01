package gio

// The Ash icons, and the derivation that finds them.

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/items"
)

// The icon file is derived from the Ash's name rather than recorded beside it,
// so the derivation covering all 84 is the thing holding that decision up. A
// renamed Ash or a renamed file breaks it here rather than by drawing an empty
// cell nobody notices.

// newAshCache is the cache the editor builds, not a zero value: the cache now
// carries where it reads from, so a bare icons{} reads from nothing at all.
func newAshCache() *icons {
	cache := ashArt()
	return &cache
}

func TestEveryAshHasAnIcon(t *testing.T) {
	ashes, err := catalog.LoadAshes()
	if err != nil {
		t.Fatal(err)
	}
	if len(ashes) != 84 {
		t.Fatalf("the catalog holds %d Ashes, want 84", len(ashes))
	}
	cache := newAshCache()
	for _, ash := range ashes {
		if _, ok := cache.source(iconName(ash.Name)); !ok {
			t.Errorf("%s has no icon at assets/ashes/%s", ash.Name, iconName(ash.Name))
		}
	}
}

// And nothing embedded is unreachable: an icon no Ash names is either a file
// that should not be in the binary or a name the derivation gets wrong, and
// both are worth knowing about.
func TestNoIconIsOrphaned(t *testing.T) {
	ashes, err := catalog.LoadAshes()
	if err != nil {
		t.Fatal(err)
	}
	named := make(map[string]bool, len(ashes))
	for _, ash := range ashes {
		named[iconName(ash.Name)] = true
	}
	files, err := ashIcons.ReadDir("assets/ashes")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if !named[file.Name()] {
			t.Errorf("assets/ashes/%s is embedded and no Ash names it", file.Name())
		}
	}
	if len(files) != len(ashes) {
		t.Errorf("%d icons are embedded for %d Ashes", len(files), len(ashes))
	}
}

// Every item the builder offers has its icon, and no icon is left that no item
// names: tools/itempack writes both, so a mismatch means one was edited alone.
func TestEveryOfferedItemHasItsIconAndNoOther(t *testing.T) {
	offered, err := items.All()
	if err != nil {
		t.Fatal(err)
	}
	named := make(map[string]bool, len(offered)+1)
	for _, item := range offered {
		named[item.Key()+".png"] = true
	}
	// And the flask every custom character carries, which is drawn, not offered.
	named[application.BuildItem{Table: application.ItemGoods, ID: application.NPCFlask}.Key()+".png"] = true
	files, err := itemIcons.ReadDir("assets/items")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if !named[file.Name()] {
			t.Errorf("assets/items/%s is embedded and no item names it", file.Name())
		}
		delete(named, file.Name())
	}
	for missing := range named {
		t.Errorf("%s has no icon", missing)
	}
}
