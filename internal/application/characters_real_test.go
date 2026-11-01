package application_test

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// The player's own characters: the card offers every filled slot, Save builds
// a placed character's rows from its slot - its body casting for free - a save
// already holding them writes nothing again, and once no Ash names the
// character its rows go, and free casting and the own colours with the last
// body. Played on the baseline's slot 4 (index 3), a Confessor whose two
// weapons carry Ashes of War of their own: a CharaInitParam row, a FaceParam
// row, two custom weapons and four bodies, the Helper and the Fighter each in
// the Puppet coat and in its own colours.
func TestACharacterFromASlotIsWrittenAtSaveAndGoesWhenUnnamed(t *testing.T) {
	t.Parallel()
	const shell, godrickSoldier = int32(100000020), int32(43110000)
	input := restorableSavePath(t)
	directory := t.TempDir()
	session := newService(t).NewSession()
	if err := session.Load(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	categories, err := session.Hierarchy()
	if err != nil {
		t.Fatal(err)
	}
	var offered []application.HierarchyEntry
	for _, category := range categories {
		for _, card := range category.Cards {
			if card.Card == "characters" {
				offered = card.Types[0].Entries
			}
		}
	}
	if len(offered) != 8 {
		t.Fatalf("the characters' card offers %d characters, want the baseline's 8", len(offered))
	}
	confessor := offered[3]

	var state *savecore.State
	rows := func(path string) int {
		t.Helper()
		read, err := savecore.ReadState(context.Background(), path, nil)
		if err != nil {
			t.Fatal(err)
		}
		state = read
		return len(read.CharacterRows)
	}
	save := func(name string, slots ...application.AuthoredSlot) (string, *application.CommitPlan) {
		t.Helper()
		if _, err := session.StageFormation(authoringHost, slots); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, name)
		commit := mustPrepare(t, session)
		if _, err := session.Commit(context.Background(), commit, path, ""); err != nil {
			t.Fatal(err)
		}
		return path, commit
	}
	// What the panel stages: the character's own body, on the shell's think.
	body, think, own, err := application.PersonShell(confessor)
	if err != nil || !own || think != shell {
		t.Fatalf("the character rides %d / %d (own %v): %v", body, think, own, err)
	}
	person := application.AuthoredSlot{Npc: body, Think: think, Person: confessor.Ships}
	creature := application.AuthoredSlot{Npc: godrickSoldier}

	with, _ := save("with.dat", person)
	if got := rows(with); got != 4+character.Bodies {
		t.Errorf("the character is %d rows, want %d", got, 4+character.Bodies)
	}
	if !state.OwnEffects[savecore.FreeCasting] || !slices.Contains(state.OwnEffectNames[body], savecore.FreeCasting) {
		t.Errorf("the character does not cast for free: its body names %v", state.OwnEffectNames[body])
	}
	still, second := save("still.dat", person, creature)
	if rows(still) != 4+character.Bodies || bytes.Contains(second.Manifest(), []byte(`"characters"`)) {
		t.Errorf("a save already holding the character's rows wrote them again: %s", second.Manifest())
	}
	without, _ := save("without.dat", creature)
	if got := rows(without); got != 0 || state.OwnEffects[savecore.FreeCasting] || state.OwnEffects[savecore.OwnColoursPresence] {
		t.Errorf("no Ash names the character and the save still holds %d of its rows, free casting %t, own colours %t",
			got, state.OwnEffects[savecore.FreeCasting], state.OwnEffects[savecore.OwnColoursPresence])
	}
}
