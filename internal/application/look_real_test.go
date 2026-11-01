package application_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// What a creature allows reaches the write, on the real catalog: the large
// Shadow Commoner takes the Statue and the Hunter only, on the quiet marker;
// the Elden Beast is drawn in its own colours and cannot be the ghost; Melina
// loses her gold whatever her look; and a creature nobody ruled takes the
// player's look.
func TestACreaturesRulesReachTheWrite(t *testing.T) {
	t.Parallel()
	commoner := application.AuthoredSlot{Npc: 52410088, Think: 52410000}
	beast := application.AuthoredSlot{Npc: 22000078, Think: 22000000}
	melina := application.AuthoredSlot{Npc: 21801034, Think: 21800000}
	session := loadedSession(t, realSavePath(t))

	follower := commoner
	if _, err := session.StageFormation("Lone Wolf Ashes", []application.AuthoredSlot{follower}); !errors.Is(err, application.ErrLevelNotOffered) {
		t.Errorf("the Commoner as the Follower: %v, want ErrLevelNotOffered", err)
	}
	ghost := beast
	ghost.Look = application.LookGhost
	if _, err := session.StageFormation("Lone Wolf Ashes", []application.AuthoredSlot{ghost}); !errors.Is(err, application.ErrLookFixed) {
		t.Errorf("the Elden Beast in the ghost: %v, want ErrLookFixed", err)
	}

	hunter := commoner
	hunter.Perception = application.PerceptionHunter
	hunter.Stance, _ = application.LevelStance(hunter)
	for _, slot := range []application.AuthoredSlot{hunter, beast, melina} {
		if slot.Stance == (application.Stance{}) {
			slot.Stance, _ = application.LevelStance(slot)
		}
		if _, err := session.StageFormation("Lone Wolf Ashes", []application.AuthoredSlot{slot}); err != nil {
			t.Fatalf("row %d: %v", slot.Npc, err)
		}
		manifest := mustPrepare(t, session).Manifest()
		for _, want := range map[int32][]string{
			52410088: {`"marker": 297001`, `297001`},
			22000078: {`"presence": 295001`},
			21801034: {`"removes": [`, `18745`},
		}[slot.Npc] {
			if !bytes.Contains(manifest, []byte(want)) {
				t.Errorf("row %d: the manifest does not carry %s", slot.Npc, want)
			}
		}
	}
}
