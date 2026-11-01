package gio

// The game's own Ashes as a category: the panel opens on it,
// every card is one creature delivered as the game ships it, and a slot the
// host already holds is re-staged as it stands.

import (
	"strconv"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

func mustAtoi(t testing.TB, raw string) int {
	t.Helper()
	number, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("%q is not a number: %v", raw, err)
	}
	return number
}

// ashesVariant is the Ashes card that ships the given row.
func ashesVariant(t *testing.T, state *State, key string) variantView {
	t.Helper()
	for _, family := range state.LoadedSave().all {
		for _, variant := range everyVariant(family) {
			if variant.Key == key {
				return variant
			}
		}
	}
	t.Fatalf("the fixture offers no %s", key)
	return variantView{}
}

// ashNamed is the rail's Ash of that name.
func ashNamed(t *testing.T, state *State, name string) *ashView {
	t.Helper()
	for index := range state.LoadedSave().Ashes {
		if state.LoadedSave().Ashes[index].Name == name {
			return &state.LoadedSave().Ashes[index]
		}
	}
	t.Fatalf("the fixture holds no %s", name)
	return nil
}

// An Ashes card dropped in a slot is staged on the Follower - which
// FromSoftware's row is - on the Ash's own stance,
// its share agreeing with it, a person on the Ash's own shell, and inserts no
// clone, because FromSoftware's rows are summon-side already. Latenna holds,
// the Putrid Corpse wanders with share off, Dolores is a person.
func TestAnAshesCardIsStagedOnTheFollower(t *testing.T) {
	state := loadedState(t)
	_, host := editable(t, state)
	for _, key := range []string{"ash:131700000", "ash:136610000", "ash:100000050"} {
		card := ashesVariant(t, state, key)
		state.drop(host, maxSlots-1, state.slotsFor(host), payload(card))
		staged := state.session.Staged().Formations
		if len(staged) != 1 {
			t.Fatalf("%s: %d formations staged, want one", key, len(staged))
		}
		last := staged[0].Requested[len(staged[0].Requested)-1]
		if last.Npc != card.Npc || last.Think != card.Think || last.Person != card.Person {
			t.Errorf("%s staged rows %d/%d/%d, want %d/%d/%d", key, last.Npc, last.Think, last.Person, card.Npc, card.Think, card.Person)
		}
		if last.Perception != application.PerceptionFollower || last.Stance != card.Stance || last.Share != application.AshShare(card.Stance) {
			t.Errorf("%s staged level %q share %d on %+v, want the Follower share %d on %+v",
				key, last.Perception.Name(), last.Share, last.Stance, application.AshShare(card.Stance), card.Stance)
		}
		for _, clone := range staged[0].Formation.Clones {
			if clone.SourceNpc == card.Npc {
				t.Errorf("%s: FromSoftware's row %d was cloned", key, card.Npc)
			}
		}
	}
	if puppet := ashesVariant(t, state, "ash:100000050"); puppet.Npc != 100000050 || puppet.Think != 100000050 {
		t.Errorf("Dolores rides %d/%d, want her own shell 100000050", puppet.Npc, puppet.Think)
	}
}

// A slot the host already holds is re-staged as it stands when the player
// drops something beside it: a Puppet keeps its person and every slot keeps
// the stance the file holds. Otherwise the Puppet would lose its person
// (authoring writes NoNpcPlayerInit for a slot naming none) and every slot
// would take the product's follow stance.
func TestAShippedSlotIsRestagedAsItStands(t *testing.T) {
	state := loadedState(t)
	for _, name := range []string{"Nepheli Loux Puppet", "Latenna the Albinauric"} {
		host := ashNamed(t, state, name)
		before := state.slotsFor(host)[0]
		if !before.Shipped {
			t.Fatalf("%s's own slot is not marked as shipped", name)
		}
		payload, _ := aSpirit(t, state)
		state.drop(host, maxSlots-1, state.slotsFor(host), payload)
		var found bool
		for _, formation := range state.session.Staged().Formations {
			if formation.HostGoodsFamily != host.GoodsFamily {
				continue
			}
			found = true
			first := formation.Requested[0]
			if first.Person != before.Person || first.Stance != before.Stance || first.Perception != application.PerceptionAsHeld {
				t.Errorf("%s's own slot re-staged as person %d on %+v level %q, want person %d on %+v as shipped",
					name, first.Person, first.Stance, first.Perception.Name(), before.Person, before.Stance)
			}
		}
		if !found {
			t.Fatalf("nothing was staged for %s", name)
		}
	}
	if puppet := ashNamed(t, state, "Nepheli Loux Puppet"); state.slotsFor(puppet)[0].Person != 23342 {
		t.Errorf("Nepheli's own slot reads person %d, want 23342", state.slotsFor(puppet)[0].Person)
	}
	if latenna := ashNamed(t, state, "Latenna the Albinauric"); state.slotsFor(latenna)[0].Stance.PcFollowType != 2 {
		t.Errorf("Latenna's own slot reads follow type %d, want her shipped 2", state.slotsFor(latenna)[0].Stance.PcFollowType)
	}
}

// Every Ashes card is pictured by its Ash's item icon and no other card is, so
// a creature the game ships as a summon is not mistaken for the enemy it is
// drawn from - an Ash summoning several creatures included.
func TestAnAshesCardShowsItsAshsIcon(t *testing.T) {
	state := loadedState(t)
	ashes := 0
	for _, family := range state.LoadedSave().all {
		if family.Category != application.AshesCategory {
			if family.Ash != "" {
				t.Errorf("%s, a %s card, is pictured as the Ash %q", family.Name, family.Category, family.Ash)
			}
			continue
		}
		ashes++
		if _, has := state.art.op(family.Ash, 34); !has {
			t.Errorf("the Ashes card %s has no icon for its Ash %q", family.Name, family.Ash)
		}
	}
	if ashes == 0 {
		t.Fatal("the fixture offers no Ashes card")
	}
}

// The catalog opens on the game's own Ashes, and every choice its category
// filter offers is one category: there is no choice showing the whole catalog.
func TestTheCatalogOpensOnTheAshesAndEveryChoiceIsACategory(t *testing.T) {
	state := loadedState(t)
	draw(t, state) // the frame that takes the load in
	loaded := state.LoadedSave()
	if got := loaded.Categories[state.kind]; got != application.AshesCategory {
		t.Errorf("the catalog opened on %q, want the Ashes", got)
	}
	for kind, category := range loaded.Categories {
		state.chooseKind(kind)
		visible := state.visibleFamilies(loaded)
		if len(visible) == 0 {
			t.Errorf("the choice %q shows no card", category)
		}
		for _, index := range visible {
			if family := loaded.Families[index]; family.Category != category {
				t.Errorf("the choice %q shows %s, a %s card", category, family.Name, family.Category)
				break
			}
		}
	}
}
