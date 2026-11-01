package plan

import (
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// A person slot (a player puts whatever they want
// in one formation, people and creatures together) is a row on one of the
// game's own person shells with the chosen CharaInitParam row and the
// person's emergence animation; a creature slot on the same shell row is the
// no-person spelling and the ordinary animation. Nothing else in the row
// differs, so a reset that restores these fields restores both kinds.
func TestAPersonSlotNamesItsRecordAndArrivesAsAPerson(t *testing.T) {
	shell := formation.Actor{Npc: 100000020, Think: 100000020}
	creature := filledRow(25900000, 259000, shell, Stance{}, formation.Placement{}, [formation.DopingLevels]int32{}, false)
	if got := creature.Set["npcPlayerInitParamId"]; got != NoNpcPlayerInit {
		t.Errorf("a creature slot names person %v, want %v", got, NoNpcPlayerInit)
	}
	if got := creature.Set["generateAnimId"]; got != OrdinarySummonAnimation {
		t.Errorf("a creature slot arrives on animation %v, want %v", got, OrdinarySummonAnimation)
	}

	shell.Person = 23342
	person := filledRow(25900000, 259000, shell, Stance{}, formation.Placement{}, [formation.DopingLevels]int32{}, false)
	if got := person.Set["npcPlayerInitParamId"]; got != int64(23342) {
		t.Errorf("the person slot names person %v, want 23342", got)
	}
	if got := person.Set["generateAnimId"]; got != PersonSummonAnimation {
		t.Errorf("the person slot arrives on animation %v, want %v", got, PersonSummonAnimation)
	}
	for field, value := range creature.Set {
		if field == "npcPlayerInitParamId" || field == "generateAnimId" {
			continue
		}
		if person.Set[field] != value {
			t.Errorf("%s = %v on the person row against %v on the creature row; only the record and the animation may differ", field, person.Set[field], value)
		}
	}

	// The shell is summon-side already: a person slot inserts nothing.
	compiled, err := formation.Compile(formation.Formation{Slots: []formation.Slot{{Actor: shell}}},
		formation.Block{Trigger: 259000, Rows: []formation.BlockRow{{RowID: 25900000, Doping: shippedDoping()}}},
		formation.Save{BuddyRows: map[int32]bool{25900000: true}, NpcRows: map[int32]bool{100000020: true}, NpcThinkRows: map[int32]bool{100000020: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Clones) != 0 {
		t.Errorf("a person slot on a shipped shell asked for %d clone insertions, want none", len(compiled.Clones))
	}
}

// shippedDoping is the 290000..290010 ladder every shipped formation carries.
func shippedDoping() [formation.DopingLevels]int32 {
	var ladder [formation.DopingLevels]int32
	for level := range ladder {
		ladder[level] = int32(290000 + level)
	}
	return ladder
}
