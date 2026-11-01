package savecore

import (
	"strings"
	"testing"
)

// A team on a clone is one byte: teamType. Everything else the recipe writes
// is the recipe, so a hostile clone and a friendly clone of one row differ
// at that byte and nowhere else (Spirit Battler's route).
func TestATeamOnACloneMovesTheTeamByteAndNothingElse(t *testing.T) {
	friendly, err := insertNpcClones(cloneFixture(t), []int32{worldActor}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hostile, err := insertNpcClones(cloneFixture(t), []int32{worldActor}, nil, map[int32]uint8{worldActor: 6}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, b := clonedRow(t, friendly, npcLayout, worldActor), clonedRow(t, hostile, npcLayout, worldActor)
	if a[npcTeamTypeOffset] != spiritSummonTeam || b[npcTeamTypeOffset] != 6 {
		t.Fatalf("teams %d and %d, want %d and 6", a[npcTeamTypeOffset], b[npcTeamTypeOffset], spiritSummonTeam)
	}
	for i := range a {
		if i != npcTeamTypeOffset && a[i] != b[i] {
			t.Errorf("byte %d differs between the friendly and the hostile clone", i)
		}
	}
	if _, err := insertNpcClones(cloneFixture(t), []int32{worldActor}, nil, map[int32]uint8{worldActor: 7}, nil, nil, nil); err == nil {
		t.Error("team 7, which no summon has stood on, was written")
	}
}

// The manifest's rules for the two lists, without opening a save: a copy is
// of a shell and lands in the band, a team modifies an insertion the manifest
// makes, and a copy without a team would be a pointless twin.
func TestCopiesAndTeamsAreValidatedAsModifiersOfInsertions(t *testing.T) {
	base := func() *manifest {
		return &manifest{Operation: operationAuthor, SourceSHA256: strings.Repeat("0", 64),
			NpcCloneSources: []int32{worldActor}}
	}
	for _, refusal := range []struct {
		name string
		edit func(*manifest)
		want string
	}{
		{"a team on a row nothing inserts", func(m *manifest) { m.NpcCloneTeams = []npcCloneTeam{{Source: 1, Team: 6}} }, "neither"},
		{"a team twice", func(m *manifest) {
			m.NpcCloneTeams = []npcCloneTeam{{Source: worldActor, Team: 6}, {Source: worldActor, Team: 27}}
		}, "twice"},
		{"an unknown team", func(m *manifest) { m.NpcCloneTeams = []npcCloneTeam{{Source: worldActor, Team: 47}} }, "not one of"},
		{"a copy of a world row", func(m *manifest) { m.NpcSummonCopies = []int32{worldActor} }, "copy band"},
		{"a copy of a shipped creature clone", func(m *manifest) { m.NpcSummonCopies = []int32{150400000} }, "copy band"},
		{"a copy without a team", func(m *manifest) { m.NpcSummonCopies = []int32{100000020} }, "no team"},
		{"a copy twice", func(m *manifest) {
			m.NpcSummonCopies = []int32{100000020, 100000020}
			m.NpcCloneTeams = []npcCloneTeam{{Source: 100000020, Team: 6}}
		}, "twice"},
	} {
		plan := base()
		refusal.edit(plan)
		err := validateStructuralWork(plan)
		if err == nil || !strings.Contains(err.Error(), refusal.want) {
			t.Errorf("%s: err = %v, want one saying %q", refusal.name, err, refusal.want)
		}
	}
	plan := base()
	plan.NpcSummonCopies = []int32{100000020}
	plan.NpcCloneTeams = []npcCloneTeam{{Source: 100000020, Team: 6}, {Source: worldActor, Team: 27}}
	if err := validateStructuralWork(plan); err != nil {
		t.Errorf("a copy with a team and a clone with a team were refused: %v", err)
	}
}
