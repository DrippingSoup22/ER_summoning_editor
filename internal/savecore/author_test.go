package savecore_test

import (
	"bytes"
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// The fixture's Putrid Corpse block, and a world actor it carries with no clone
// beside it. Authoring into this block is the smallest complete exercise of the
// slice: it fills rows the game shipped, adds one it did not, and needs an
// actor the save cannot yet summon.
const (
	authoredTrigger   = int32(215000)
	authoredBlockHead = int32(21500000)
	// The row after the block's last shipped row. 21500004 is free: the fixture
	// carries 21500000..21500003 and then jumps to the 233xxxxx families.
	authoredNewRow = int32(21500004)
)

func worldActor() formation.Actor {
	source := savefixture.CloneableWorldActors[0]
	return formation.Actor{Npc: source, Think: source}
}

func summonedActor() formation.Actor {
	source := savefixture.CloneableWorldActors[0]
	return formation.Actor{Npc: source + formation.CloneOffset, Think: source + formation.CloneOffset}
}

// fiveOfOneWorldActor is a formation the game could not deliver before this
// slice: five of an actor that is not a summon at all, in a block the game
// shipped four rows for.
func fiveOfOneWorldActor() formation.Compiled {
	actor := summonedActor()
	source := worldActor()
	compiled := formation.Compiled{
		Trigger: authoredTrigger,
		Clones: []formation.Clone{{
			Source: source,
			Npc:    &formation.CloneRow{RowID: actor.Npc, CopyFrom: source.Npc},
			Think:  &formation.CloneRow{RowID: actor.Think, CopyFrom: source.Think},
		}},
		Inserts: []formation.RowInsert{{
			Slot: 5, RowID: authoredNewRow, CopyFrom: authoredBlockHead + 3,
			InsertAfter: authoredBlockHead + 3, Actor: actor,
		}},
	}
	for offset := int32(0); offset < 4; offset++ {
		compiled.Edits = append(compiled.Edits, formation.RowEdit{
			Slot: int(offset) + 1, RowID: authoredBlockHead + offset, Actor: actor,
		})
	}
	return compiled
}

func authorManifestFor(t *testing.T, sha string, compiled formation.Compiled, current plan.Vanilla) []byte {
	t.Helper()
	built, err := plan.BuildAuthor(sha, compiled, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := built.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func snapshotOf(t *testing.T, path string) *savecore.VanillaSnapshot {
	t.Helper()
	snapshot, err := savecore.ReadVanillaSnapshot(context.Background(), path, allGoodsRows())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// The test the slice exists to pass.
//
// Every capability F1, F2 and G0 built was unreachable from a manifest, which
// is the same as saying it was unreachable at all: nothing a user can ask for
// touched any of it. This asks for one authored formation and gets all four -
// an NpcParam clone, an NpcThinkParam clone, a BuddyParam row that did not
// exist, and the actor written onto every row of the block.
func TestAuthoringFillsABlockAndAddsTheRowsItNeeds(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	before := snapshotOf(t, input)
	output := filepath.Join(t.TempDir(), "authored.dat")

	encoded := authorManifestFor(t, before.SourceSHA256, fiveOfOneWorldActor(), vanillaOf(t, input, allGoodsRows()))
	if _, err := savecore.Apply(context.Background(), input, output, bytes.NewReader(encoded)); err != nil {
		t.Fatal(err)
	}

	after := snapshotOf(t, output)
	actor := summonedActor()

	// The block is one row longer than the game shipped, and every row of it
	// names the actor and the block's own trigger.
	if _, present := after.Buddy[authoredNewRow]; !present {
		t.Fatalf("BuddyParam row %d was not inserted", authoredNewRow)
	}
	if len(after.Buddy) != len(before.Buddy)+1 {
		t.Errorf("BuddyParam went from %d rows to %d, want exactly one more", len(before.Buddy), len(after.Buddy))
	}
	for offset := int32(0); offset < 5; offset++ {
		rowID := authoredBlockHead + offset
		row := after.Buddy[rowID]
		if row["triggerSpEffectId"] != float64(authoredTrigger) {
			t.Errorf("row %d trigger = %g, want %d", rowID, row["triggerSpEffectId"], authoredTrigger)
		}
		if row["npcParamId"] != float64(actor.Npc) || row["npcThinkParamId"] != float64(actor.Think) {
			t.Errorf("row %d names %g/%g, want %d/%d",
				rowID, row["npcParamId"], row["npcThinkParamId"], actor.Npc, actor.Think)
		}
		// The fixture ships this block with the placement flag set, so a row
		// still carrying 1 would mean authoring left a formation that spawns at
		// a distant monument.
		if row["appearOnAroundSekihi"] != float64(plan.AuthoredAppearOnAroundSekihi) {
			t.Errorf("row %d appearOnAroundSekihi = %g, want %d",
				rowID, row["appearOnAroundSekihi"], plan.AuthoredAppearOnAroundSekihi)
		}
	}

	// Both halves of the actor pair were inserted, and nothing else was.
	for _, table := range []struct {
		name          string
		before, after []int32
	}{
		{"NpcParam", before.NpcCloneRows, after.NpcCloneRows},
		{"NpcThinkParam", before.NpcThinkCloneRows, after.NpcThinkCloneRows},
	} {
		added := 0
		for _, rowID := range table.after {
			if !slices.Contains(table.before, rowID) {
				added++
				if rowID != actor.Npc {
					t.Errorf("%s gained undeclared clone %d", table.name, rowID)
				}
			}
		}
		if added != 1 {
			t.Errorf("%s gained %d clone rows, want 1", table.name, added)
		}
	}
}

// Authoring writes no goods row at all, which is the reason it cannot reach
// the multiplayer gate even by accident. A conversion needs a rule to keep
// away from enable_multi; authoring is the wrong shape to touch it.
func TestAuthoringLeavesEveryGoodsRowAlone(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	before := snapshotOf(t, input)
	output := filepath.Join(t.TempDir(), "authored.dat")

	encoded := authorManifestFor(t, before.SourceSHA256, fiveOfOneWorldActor(), vanillaOf(t, input, allGoodsRows()))
	if _, err := savecore.Apply(context.Background(), input, output, bytes.NewReader(encoded)); err != nil {
		t.Fatal(err)
	}
	if !equalRowTables(before.Goods, snapshotOf(t, output).Goods) {
		t.Error("authoring changed a goods row")
	}
	if strings.Contains(string(encoded), "goods_patches") {
		t.Error("an authored manifest carries a goods patch")
	}
}

// The other half of the slice, and the reason both halves had to land together:
// a reset that could not remove rows would restore the four shipped rows of a
// grown block and leave the fifth summoning.
func TestResettingAnAuthoredSaveRemovesTheRowsItAdded(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	// With the clone row sets, because they are what tells the reset which
	// actor rows are the editor's own. Without them it would restore values and
	// leave the inserted clones in the save.
	shipped := vanillaWithCloneRows(t, input)
	before := snapshotOf(t, input)

	authored := filepath.Join(t.TempDir(), "authored.dat")
	encoded := authorManifestFor(t, before.SourceSHA256, fiveOfOneWorldActor(), shipped)
	if _, err := savecore.Apply(context.Background(), input, authored, bytes.NewReader(encoded)); err != nil {
		t.Fatal(err)
	}

	restored := filepath.Join(t.TempDir(), "restored.dat")
	authoredState := vanillaWithCloneRows(t, authored)
	resetPlan, err := plan.BuildReset(snapshotOf(t, authored).SourceSHA256, shipped, authoredState)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(resetPlan.BuddyRowRemovals, authoredNewRow) {
		t.Fatalf("reset removals %v do not include the inserted row %d", resetPlan.BuddyRowRemovals, authoredNewRow)
	}
	var resetEncoded bytes.Buffer
	if err := resetPlan.Encode(&resetEncoded); err != nil {
		t.Fatal(err)
	}
	if _, err := savecore.Apply(context.Background(), authored, restored, bytes.NewReader(resetEncoded.Bytes())); err != nil {
		t.Fatal(err)
	}

	after := snapshotOf(t, restored)
	if len(after.Buddy) != len(before.Buddy) {
		t.Errorf("BuddyParam has %d rows after the reset, want the shipped %d", len(after.Buddy), len(before.Buddy))
	}
	for rowID, shippedValues := range before.Buddy {
		current, present := after.Buddy[rowID]
		if !present {
			t.Errorf("BuddyParam row %d is missing after the reset", rowID)
			continue
		}
		if !maps.Equal(map[string]float64(shippedValues), map[string]float64(current)) {
			t.Errorf("BuddyParam row %d = %v after the reset, want the shipped %v", rowID, current, shippedValues)
		}
	}
	if !slices.Equal(before.NpcCloneRows, after.NpcCloneRows) {
		t.Errorf("NpcParam clone rows = %v after the reset, want the shipped %v", after.NpcCloneRows, before.NpcCloneRows)
	}
	if !slices.Equal(before.NpcThinkCloneRows, after.NpcThinkCloneRows) {
		t.Errorf("NpcThinkParam clone rows = %v after the reset, want the shipped %v",
			after.NpcThinkCloneRows, before.NpcThinkCloneRows)
	}
}

// vanillaOf reads a save into the shape the builders want. The authoring path
// needs it for exactly one thing - a retired row keeps its own actor - and the
// reset path needs its key sets to decide which rows are the editor's.
func vanillaWithCloneRows(t *testing.T, path string) plan.Vanilla {
	t.Helper()
	snapshot := snapshotOf(t, path)
	values := vanillaOf(t, path, allGoodsRows())
	values.NpcCloneRows = snapshot.NpcCloneRows
	values.NpcThinkCloneRows = snapshot.NpcThinkCloneRows
	return values
}

// A reset whose shipped record cannot say which clone rows the game came with
// must refuse rather than guess. Guessing means deleting all 115 of them.
func TestAResetRefusesWhenTheShippedRecordNamesNoCloneRows(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	shipped := vanillaOf(t, input, allGoodsRows())
	current := vanillaWithCloneRows(t, input)
	if len(current.NpcCloneRows) == 0 {
		t.Fatal("the fixture carries no clone rows, so this test proves nothing")
	}
	if _, err := plan.BuildReset(snapshotOf(t, input).SourceSHA256, shipped, current); err == nil {
		t.Fatal("a reset was built from a shipped record with no clone rows")
	}
}
