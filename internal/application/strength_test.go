package application_test

// Summoning a creature at the strength the game gives it, from the catalog end.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/strength"
)

// authoredRows is every formation-row patch in a manifest, by row ID.
//
// The manifest is walked rather than typed, because a composed session manifest
// nests its parts and a test that hard-coded the nesting would break for a
// reason that has nothing to do with what it is checking.
func authoredRows(t *testing.T, manifest []byte) map[float64]map[string]any {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatal(err)
	}
	rows := map[float64]map[string]any{}
	var walk func(node any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if key != "buddy_patches" {
					walk(child)
					continue
				}
				patches, _ := child.([]any)
				for _, entry := range patches {
					patch, _ := entry.(map[string]any)
					name, _ := patch["name"].(string)
					set, _ := patch["set"].(map[string]any)
					rowID, _ := patch["row_id"].(float64)
					if strings.Contains(name, "formation row") {
						rows[rowID] = set
					}
				}
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(decoded)
	return rows
}

// strengthEntries is every npc_clone_strength entry in a manifest, at any
// depth: a session that composes several parts nests them.
func strengthEntries(t *testing.T, manifest []byte) []map[string]any {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatalf("decoding the manifest: %v", err)
	}
	var entries []map[string]any
	var walk func(node any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if key != "npc_clone_strength" {
					walk(child)
					continue
				}
				listed, _ := child.([]any)
				for _, entry := range listed {
					if fields, ok := entry.(map[string]any); ok {
						entries = append(entries, fields)
					}
				}
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(decoded)
	return entries
}

// Each creature carries its own role. Two Helpers and two Fighters of one
// creature are two clones: the Helper keeps the balance package and 2.7x the
// world row's health (Malenia's row, 2,489, in the fixture), the Fighter goes
// without it at the row's own. Every authored row carries the eleven effects
// its Ash ships with whichever the role.
func TestEachCreatureCarriesItsOwnRoleAndTheLadderStaysTheAshs(t *testing.T) {
	t.Parallel()
	session := loadedSession(t, writeVanilla(t))
	// Two more slots than the host's rows, so two rows are inserted copies.
	slots := slotsOf(authoringRows + 2)
	for index := range slots[:3] {
		slots[index].Fighter = true
	}
	if _, err := session.StageFormation(authoringHost, slots); err != nil {
		t.Fatal(err)
	}
	manifest := mustPrepare(t, session).Manifest()
	type role struct {
		keep bool
		hp   float64
	}
	found := map[role]int{}
	for _, entry := range strengthEntries(t, manifest) {
		keep, _ := entry["keep_balance"].(bool)
		hp, _ := entry["hp"].(float64)
		found[role{keep, hp}]++
	}
	helper, fighter := role{true, float64(strength.Helper(2489))}, role{false, 2489}
	if len(found) != 2 || found[helper] != 1 || found[fighter] != 1 {
		t.Errorf("the clones carry %v, want one Helper %v and one Fighter %v", found, helper, fighter)
	}
	rows := authoredRows(t, manifest)
	if len(rows) == 0 {
		t.Fatal("the manifest holds no formation rows at all")
	}
	for row, set := range rows {
		for level := 0; level < strength.Levels; level++ {
			name := fmt.Sprintf("dopingSpEffect_lv%d", level)
			if set[name] != float64(290000+level) {
				t.Errorf("row %.0f writes %s = %v, want the Ash's %d", row, name, set[name], 290000+level)
			}
		}
	}
}

// A clone the save holds is reused only for its own role. The role is part of
// the recipe, so the clone of a Fighter written earlier is that Fighter again,
// and a Helper of the same creature is a clone of its own.
func TestAHeldCloneIsReusedOnlyForItsOwnRole(t *testing.T) {
	t.Parallel()
	fighter := worldActorSlot()
	fighter.Fighter = true
	session := loadedSession(t, writeVanilla(t))
	if _, err := session.StageFormation(authoringHost, []application.AuthoredSlot{fighter}); err != nil {
		t.Fatal(err)
	}
	written := filepath.Join(t.TempDir(), "fighter.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), written, ""); err != nil {
		t.Fatalf("writing the Fighter: %v", err)
	}

	for _, want := range []struct {
		name    string
		slot    application.AuthoredSlot
		inserts int
	}{
		{"the Fighter again", fighter, 0},
		{"a Helper", worldActorSlot(), 1},
	} {
		reopened := loadedSession(t, written)
		if _, err := reopened.StageFormation(authoringHost, []application.AuthoredSlot{want.slot}); err != nil {
			t.Fatal(err)
		}
		if got := len(strengthEntries(t, mustPrepare(t, reopened).Manifest())); got != want.inserts {
			t.Errorf("%s writes %d clones, want %d", want.name, got, want.inserts)
		}
	}
}
