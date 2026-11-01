package plan

import (
	"bytes"
	"encoding/json"
	"testing"
)

const fixtureSHA = "116e8ecd89a4671df2264807b47d58dfe23dc13613081732c1f671cb39458ec0"

func loneWolfToBloodfiend() Conversion {
	return Conversion{
		HostGoodsFamily: 232000,
		HostName:        "Lone Wolf Ashes",
		UpgradeLevels:   11,
		LevelTriggers:   sameTrigger(20201000, 11),
		DestinationName: "Bloodfiend Hexer",
	}
}

// sameTrigger is the ordinary case: 83 of the 84 families deliver one formation
// at every upgrade level.
func sameTrigger(trigger int32, levels int) []int32 {
	triggers := make([]int32, levels)
	for level := range triggers {
		triggers[level] = trigger
	}
	return triggers
}

// The safety property the whole v1 design rests on: a conversion re-points a
// goods row and touches nothing that could alter a world actor. If NpcParam ever
// appears here, the model-namespace hazard is back and the tool is
// no longer honest about what it does.
func TestV1NeverWritesActorTables(t *testing.T) {
	manifest, err := Build(fixtureSHA, []Conversion{loneWolfToBloodfiend()},
		Recipe{MonumentOriginRows: MonumentOriginRows1_17, Area: Anywhere})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range manifest.TouchedTables() {
		switch table {
		case "NpcParam", "NpcThinkParam":
			t.Fatalf("v1 manifest writes %s", table)
		}
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"npcParamId", "npcThinkParamId", "param_row_insertions", "NpcParam"} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Errorf("manifest contains %q", forbidden)
		}
	}
}

// Every upgrade level must be re-pointed. A goods
// family is eleven separate rows and each carries its own trigger, so patching
// only +0 leaves an upgraded Ash summoning its original formation.
func TestAllUpgradeLevelsArePatched(t *testing.T) {
	manifest, err := Build(fixtureSHA, []Conversion{loneWolfToBloodfiend()}, Recipe{Area: Anywhere})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int32]bool{}
	for level := int32(0); level < 11; level++ {
		want[232000+level] = true
	}
	for _, patch := range manifest.GoodsPatches {
		if !want[patch.RowID] {
			t.Errorf("unexpected goods row %d", patch.RowID)
		}
		delete(want, patch.RowID)
		if patch.Set["refId_default"] != int32(20201000) {
			t.Errorf("row %d points at %v, want 20201000", patch.RowID, patch.Set["refId_default"])
		}
	}
	if len(want) != 0 {
		t.Errorf("%d upgrade rows were not patched: %v", len(want), want)
	}
}

// Part one of the shipping recipe without part two is a bug: it makes every
// monument eligible, so a monument-origin formation resolves to an arbitrary
// distant stone and appears to vanish even where summoning is normal.
func TestExpandedAreaRecipeAlwaysShipsWithItsMonumentFix(t *testing.T) {
	manifest, err := Build(fixtureSHA, []Conversion{loneWolfToBloodfiend()},
		Recipe{MonumentOriginRows: MonumentOriginRows1_17, Area: Anywhere})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.BuddyStonePatches) != 1 {
		t.Fatalf("got %d stone patches, want 1", len(manifest.BuddyStonePatches))
	}
	for _, field := range []string{
		"activateRange", "overwriteReturnRange", "overwriteActivateRegionEntityId",
		"warnRegionEntityId", "summonedEventFlagId", "eliminateTargetEntityId",
	} {
		if _, present := manifest.BuddyStonePatches[0].Set[field]; !present {
			t.Errorf("stone recipe is missing %s", field)
		}
	}
	if len(manifest.BuddyPatches) != len(MonumentOriginRows1_17) {
		t.Fatalf("got %d monument fixes, want %d", len(manifest.BuddyPatches), len(MonumentOriginRows1_17))
	}
	for _, patch := range manifest.BuddyPatches {
		if patch.Set["appearOnAroundSekihi"] != 0 {
			t.Errorf("row %d does not clear appearOnAroundSekihi", patch.RowID)
		}
		if len(patch.Set) != 1 {
			t.Errorf("row %d writes %d fields; the fix is one byte", patch.RowID, len(patch.Set))
		}
	}
}

// The monument switch has two positions and no default.
//
// Off has to undo exactly what on did, both halves. Part one without part two
// is the bug the recipe's own comment describes - a formation resolving its
// spawn to a distant monument and appearing to vanish - so a switch that put
// back the stone table and left appearOnAroundSekihi cleared would leave the
// save in a state neither position describes.
func TestTheMonumentSwitchUndoesBothHalves(t *testing.T) {
	shipped := Vanilla{
		Stone: map[int32]map[string]int64{
			1: {"activateRange": 40, "overwriteReturnRange": 30, "overwriteActivateRegionEntityId": 0,
				"warnRegionEntityId": 0, "summonedEventFlagId": 62010, "eliminateTargetEntityId": 0},
		},
		Buddy: map[int32]map[string]float64{30400000: {"appearOnAroundSekihi": 1}},
	}

	on, err := Build(fixtureSHA, []Conversion{loneWolfToBloodfiend()},
		Recipe{MonumentOriginRows: []int32{30400000}, Area: Anywhere})
	if err != nil {
		t.Fatal(err)
	}
	if len(on.BuddyStonePatches) != 1 || len(on.BuddyStoneRowPatches) != 0 {
		t.Errorf("anywhere wrote %d all-row recipes and %d rows, want 1 and 0",
			len(on.BuddyStonePatches), len(on.BuddyStoneRowPatches))
	}
	if got := on.BuddyPatches[0].Set["appearOnAroundSekihi"]; got != 0 {
		t.Errorf("anywhere left appearOnAroundSekihi at %v, want it cleared", got)
	}

	off, err := Build(fixtureSHA, []Conversion{loneWolfToBloodfiend()},
		Recipe{MonumentOriginRows: []int32{30400000}, Area: Monuments, Shipped: shipped})
	if err != nil {
		t.Fatal(err)
	}
	if len(off.BuddyStonePatches) != 0 || len(off.BuddyStoneRowPatches) != 1 {
		t.Fatalf("monuments wrote %d all-row recipes and %d rows, want 0 and 1",
			len(off.BuddyStonePatches), len(off.BuddyStoneRowPatches))
	}
	// Every field the recipe writes goes back, not a subset: a row restored
	// halfway holds a combination the game never shipped.
	restored := off.BuddyStoneRowPatches[0].Set
	if len(restored) != len(expandedAreaRecipe) {
		t.Errorf("restored %d fields, want all %d the recipe writes",
			len(restored), len(expandedAreaRecipe))
	}
	if got := restored["summonedEventFlagId"]; got != int64(62010) {
		t.Errorf("summonedEventFlagId came back as %v, want the shipped 62010", got)
	}
	// And the other half.
	if got := off.BuddyPatches[0].Set["appearOnAroundSekihi"]; got != float64(1) {
		t.Errorf("monuments left appearOnAroundSekihi at %v, want the shipped 1", got)
	}
}
