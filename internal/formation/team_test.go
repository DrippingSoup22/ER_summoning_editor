package formation_test

import (
	"errors"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// nepheli is a person on the game's own shell 100000020: summon-side already,
// shipped, and never written.
var nepheli = formation.Actor{Npc: 100000020, Think: 100000020, Person: 23342}

// A team on a shipped shell cannot be written on the shell, so the slot moves
// to a copy of it in the copy band, and the copy is what the row summons.
func TestATeamOnAShellSummonsACopyOfIt(t *testing.T) {
	block, save := loneWolfBlock(), saveHolding(loneWolfBlock())
	hostile := nepheli
	hostile.Team = formation.TeamEnemy

	compiled := mustCompile(t, formation.Formation{Slots: slots(hostile)}, block, save)

	if len(compiled.Clones) != 0 {
		t.Fatalf("a shell asked for %d clones", len(compiled.Clones))
	}
	if len(compiled.Copies) != 1 {
		t.Fatalf("got %d copies, want 1", len(compiled.Copies))
	}
	copied := compiled.Copies[0]
	if copied.CopyFrom != nepheli.Npc || copied.RowID != 199000020 || copied.Source.Team != formation.TeamEnemy {
		t.Errorf("copy = %+v, want 199000020 from %d on team %d", copied, nepheli.Npc, formation.TeamEnemy)
	}
	got := compiled.Edits[0].Actor
	if got.Npc != 199000020 || got.Think != nepheli.Think || got.Person != nepheli.Person {
		t.Errorf("the row summons %+v: want the copy, the shell's own think row and the person", got)
	}
	// Without a team the shell is summoned as shipped, and nothing is copied.
	plain := mustCompile(t, formation.Formation{Slots: slots(nepheli)}, block, save)
	if len(plain.Copies) != 0 || plain.Edits[0].Actor.Npc != nepheli.Npc {
		t.Errorf("a shell on the summon team was copied: %+v", plain.Copies)
	}
}

// A slot may only ask for a team a summon has been seen standing on, and a
// shipped creature row outside the shell windows has nowhere to carry one.
func TestAnUnknownTeamAndAnUncopyableRowAreRefused(t *testing.T) {
	block, save := loneWolfBlock(), saveHolding(loneWolfBlock())
	odd := wolf
	odd.Team = 7
	if _, err := formation.Compile(formation.Formation{Slots: slots(odd)}, block, save); !errors.Is(err, formation.ErrUnknownTeam) {
		t.Errorf("team 7 err = %v, want ErrUnknownTeam", err)
	}
	hostileTiche := tiche
	hostileTiche.Team = formation.TeamEnemy
	if _, err := formation.Compile(formation.Formation{Slots: slots(hostileTiche)}, block, save); !errors.Is(err, formation.ErrNotSummonable) {
		t.Errorf("a team on shipped %d err = %v, want ErrNotSummonable", tiche.Npc, err)
	}
}
