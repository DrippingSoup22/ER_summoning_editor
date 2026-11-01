package plan

import (
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// A team on a slot reaches the manifest as a modifier of the row the slot
// inserts: a creature's clone carries the team in npc_clone_teams; a person's
// shell is copied (npc_summon_copies) and the copy carries it, and the
// formation row names the copy. A slot on the summon team writes neither
// list, so every archived manifest replays unchanged.
func TestATeamReachesTheManifestOnTheCloneOrOnAShellCopy(t *testing.T) {
	block := formation.Block{Trigger: 259000, Rows: []formation.BlockRow{
		{RowID: 25900000, Doping: shippedDoping()}, {RowID: 25900001, Doping: shippedDoping()}}}
	save := formation.Save{BuddyRows: map[int32]bool{25900000: true, 25900001: true},
		NpcRows: map[int32]bool{100000020: true}, NpcThinkRows: map[int32]bool{100000020: true}}
	person := formation.Actor{Npc: 100000020, Think: 100000020, Person: 23342, Team: formation.TeamEnemy}
	wolf := formation.Actor{Npc: 40700000, Think: 40700000, Team: formation.TeamHostileNPC}

	compiled, err := formation.Compile(formation.Formation{Slots: []formation.Slot{{Actor: person}, {Actor: wolf}}}, block, save)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildAuthor(strings.Repeat("0", 64), *compiled, Vanilla{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.NpcSummonCopies) != 1 || manifest.NpcSummonCopies[0] != 100000020 {
		t.Errorf("copies = %v, want the shell 100000020", manifest.NpcSummonCopies)
	}
	if len(manifest.NpcCloneSources) != 1 || manifest.NpcCloneSources[0] != 40700000 {
		t.Errorf("clone sources = %v, want the wolf", manifest.NpcCloneSources)
	}
	teams := map[int32]uint8{}
	for _, entry := range manifest.NpcCloneTeams {
		teams[entry.Source] = entry.Team
	}
	if teams[100000020] != 6 || teams[40700000] != 27 || len(teams) != 2 {
		t.Errorf("teams = %v, want the shell on 6 and the wolf on 27", teams)
	}
	if got := manifest.BuddyPatches[0].Set["npcParamId"]; got != int64(199000020) {
		t.Errorf("the person's row names %v, want the copy 199000020", got)
	}
	if got := manifest.BuddyPatches[0].Set["npcPlayerInitParamId"]; got != int64(23342) {
		t.Errorf("the copy's row lost the person: %v", got)
	}
	if got := manifest.BuddyPatches[1].Set["npcParamId"]; got != int64(140700000) {
		t.Errorf("the wolf's row names %v, want its clone", got)
	}

	// On the summon team, neither list is written.
	person.Team, wolf.Team = formation.TeamSummon, formation.TeamSummon
	compiled, err = formation.Compile(formation.Formation{Slots: []formation.Slot{{Actor: person}, {Actor: wolf}}}, block, save)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := BuildAuthor(strings.Repeat("0", 64), *compiled, Vanilla{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.NpcSummonCopies) != 0 || len(plain.NpcCloneTeams) != 0 {
		t.Errorf("a formation on the summon team wrote copies %v and teams %v", plain.NpcSummonCopies, plain.NpcCloneTeams)
	}
}
