package application_test

import (
	"context"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// The categories come in the author's order: the game's own Ashes, which the
// panel opens on, the game's five, then the player's own characters and the
// custom ones last (2026-10-27).
func TestTheCategoriesComeInTheAuthorsOrder(t *testing.T) {
	t.Parallel()
	session := newService(t).NewSession()
	if err := session.Load(context.Background(), realSavePath(t)); err != nil {
		t.Fatal(err)
	}
	categories, err := session.Hierarchy()
	if err != nil {
		t.Fatalf("Hierarchy: %v", err)
	}
	var names []string
	for _, category := range categories {
		names = append(names, category.Name)
	}
	want := []string{"ashes", "boss", "enemy", "wildlife", "npc", "environment", application.CustomCategory}
	if !slices.Equal(names, want) {
		t.Errorf("the categories are %v, want %v", names, want)
	}
}

// Every creature leaf the hierarchy offers must be a row this save actually
// carries. Offering one it would not be able to summon puts a tile in the grid
// that cannot be used.
//
// The oracle is ActorLabels, which names every row the save can summon. An
// Ashes leaf ships FromSoftware's own summon-side row, so it must
// instead be a row one of the save's own Ashes summons (AshStates). A leaf
// research promoted by a console seat (its `seen` tag) was proved more directly:
// the seat summoned it from a real save and saw it fight.
func TestEveryCreatureLeafIsARowTheSaveCanSummon(t *testing.T) {
	t.Parallel()
	session := newService(t).NewSession()
	if err := session.Load(context.Background(), realSavePath(t)); err != nil {
		t.Fatal(err)
	}
	known, err := session.ActorLabels()
	if err != nil {
		t.Fatalf("ActorLabels: %v", err)
	}
	ashes, err := session.AshStates()
	if err != nil {
		t.Fatalf("AshStates: %v", err)
	}
	shipped := map[int32]bool{}
	for _, ash := range ashes {
		for _, slot := range ash.Slots {
			shipped[slot.Npc] = true
		}
	}
	categories, err := session.Hierarchy()
	if err != nil {
		t.Fatalf("Hierarchy: %v", err)
	}
	var stray, fromAshes int
	for _, category := range categories {
		for _, card := range category.Cards {
			for _, one := range card.Types {
				for _, entry := range one.Entries {
					if entry.Trunk != "creature" {
						continue
					}
					if application.AsShipped(entry) {
						fromAshes++
						if !shipped[entry.Ships] {
							t.Errorf("%s offers npc %d, which none of the save's own Ashes summons", entry.Key, entry.Ships)
						}
						continue
					}
					if _, summonable := known[entry.Ships]; !summonable && entry.Tags["seen"] == "" {
						stray++
						if stray <= 5 {
							t.Errorf("%s offers npc %d, which this save cannot summon",
								entry.Key, entry.Ships)
						}
					}
				}
			}
		}
	}
	if stray > 5 {
		t.Errorf("... and %d more", stray-5)
	}
	if fromAshes == 0 {
		t.Error("the hierarchy offers no Ashes creature; the check above read nothing")
	}
}
