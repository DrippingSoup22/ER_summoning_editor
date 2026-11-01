package application_test

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
)

// The two families seen hurting their own summoner warn, and nothing else
// does. An actor NOT on the list is not thereby safe: nobody has seen it.
func TestTheActorsThatHurtTheirOwnSummonerAreNamed(t *testing.T) {
	t.Parallel()
	families, err := catalog.LoadFamilies()
	if err != nil {
		t.Fatal(err)
	}
	warned := map[string]string{}
	for _, family := range families {
		why, hurts := application.HurtsSummoner(family.Model)
		if !hurts {
			continue
		}
		if why == "" {
			t.Errorf("%s (%s) hurts its summoner with nothing to say about it",
				family.Model, family.Name)
		}
		warned[family.Model] = family.Name
	}
	// The Furnace Golem and the Merciless Chariot. Pinned by count so that a
	// third arriving without a witness is a failing test, not a quiet addition.
	if len(warned) != 2 {
		t.Errorf("%d families warn about hurting their summoner, want the 2 that "+
			"were seen: %v", len(warned), warned)
	}
	for _, model := range []string{"c5170", "c8120"} {
		if _, found := warned[model]; !found {
			t.Errorf("%s was seen hurting its summoner and no longer warns", model)
		}
	}
	if _, hurts := application.HurtsSummoner("c4460"); hurts {
		t.Error("the Flame Chariot warns, and it was tested as an ordinary enemy")
	}
}
