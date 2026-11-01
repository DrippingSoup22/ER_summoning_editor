package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// A save an earlier build switched to own colours as a whole gets the game's
// spirit look back with the next change that writes the regulation: the look
// is each creature's now, and the switch would draw every summon without its
// coat whatever the creature asked.
func TestAnEarlierColourSwitchIsPutBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	input := writeVanilla(t)
	directory := t.TempDir()
	switched := filepath.Join(directory, "switched.dat")
	session := loadedSession(t, input)
	manifest, err := plan.BuildTint(session.Save().SHA256, plan.OriginalColours)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := savecore.Apply(ctx, input, switched, &encoded); err != nil {
		t.Fatal(err)
	}

	edited := loadedSession(t, switched)
	if _, err := edited.StageFormation(authoringHost, []application.AuthoredSlot{{Npc: worldActorSlot().Npc}}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(directory, "edited.dat")
	if _, err := edited.Commit(ctx, mustPrepare(t, edited), out, ""); err != nil {
		t.Fatal(err)
	}
	state, err := savecore.ReadState(ctx, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.PhantomProfile != plan.SpiritPhantomProfile {
		t.Errorf("the summons are drawn through profile %d, want the spirit look %d", state.PhantomProfile, plan.SpiritPhantomProfile)
	}
}
