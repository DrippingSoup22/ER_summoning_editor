package savecore_test

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// authoredSave builds a save whose BuddyParam rows have been edited the way an
// authored formation edits them: a row re-pointed at a different actor, and a
// row removed from its formation with triggerSpEffectId = -1.
//
// It is built rather than produced by the product on purpose. Nothing in the
// CLI can author a formation yet, and a reset that could only undo edits this
// product knows how to make would be testing its own conversion recipe rather
// than restoring shipped behaviour "whatever edited the save".
func authoredSave(t *testing.T, rePointed, retired int32) []byte {
	t.Helper()
	options := savefixture.Defaults()
	for index, table := range options.Tables {
		if table.Entry != "BuddyParam.param" {
			continue
		}
		options.Tables[index].Init = func(rowID int32, row []byte) {
			// Start from a shipped row and change one thing, so that what this
			// save differs from a fresh fixture in is exactly the edit below.
			savefixture.SeedBuddyRow(rowID, row)
			switch rowID {
			case rePointed:
				writeFixtureBuddyField(row, "npcParamId", 145000000)
				writeFixtureBuddyField(row, "npcThinkParamId", 145000000)
			case retired:
				writeFixtureBuddyField(row, "triggerSpEffectId", -1)
			}
		}
	}
	save, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	return save
}

func writeFixtureBuddyField(row []byte, name string, value int32) {
	field := savefixture.BuddyFieldOffsets[name]
	row[field.Offset] = byte(value)
	row[field.Offset+1] = byte(value >> 8)
	row[field.Offset+2] = byte(value >> 16)
	row[field.Offset+3] = byte(value >> 24)
}

// The test F3 exists to pass. A save carrying an authored formation resets to
// the shipped one, including the two fields a v1 reset had no record of.
//
// Before F3 the snapshot stored only which rows carry appearOnAroundSekihi = 1.
// Every assertion below about npcParamId and triggerSpEffectId would have
// passed vacuously on a manifest that restored the flag and nothing else, and
// the save would have kept summoning whatever the formation was re-pointed at.
func TestResetRestoresAnAuthoredFormation(t *testing.T) {
	shippedRows := plan.MonumentOriginRows1_17
	rePointed, retired := shippedRows[0], shippedRows[1]

	original := writeSave(t, fixtureBytes(t))
	edited := writeSave(t, authoredSave(t, rePointed, retired))
	rows := allGoodsRows()

	shipped := vanillaOf(t, original, rows)
	current := vanillaOf(t, edited, rows)

	// Prove the edit is really in the save before asking a reset to undo it.
	if current.Buddy[rePointed]["npcParamId"] != 145000000 {
		t.Fatalf("row %d summons %v, the fixture did not re-point it",
			rePointed, current.Buddy[rePointed]["npcParamId"])
	}
	if current.Buddy[retired]["triggerSpEffectId"] != -1 {
		t.Fatalf("row %d trigger is %v, the fixture did not retire it",
			retired, current.Buddy[retired]["triggerSpEffectId"])
	}

	editedInfo, err := savecore.Inspect(context.Background(), edited)
	if err != nil {
		t.Fatal(err)
	}
	manifest := resetManifestFor(t, editedInfo.SHA256, shipped, current)
	restored := newOutputPath(t)
	if _, err := savecore.Apply(context.Background(), edited, restored, bytes.NewReader(manifest)); err != nil {
		t.Fatal(err)
	}

	after := vanillaOf(t, restored, rows)
	for _, rowID := range []int32{rePointed, retired} {
		if !maps.Equal(after.Buddy[rowID], shipped.Buddy[rowID]) {
			t.Errorf("buddy row %d = %v, want the shipped %v", rowID, after.Buddy[rowID], shipped.Buddy[rowID])
		}
	}
	if len(after.Buddy) != len(shipped.Buddy) {
		t.Fatalf("buddy rows = %d, want %d", len(after.Buddy), len(shipped.Buddy))
	}
	for rowID, want := range shipped.Buddy {
		if !maps.Equal(after.Buddy[rowID], want) {
			t.Errorf("buddy row %d = %v, want the shipped %v", rowID, after.Buddy[rowID], want)
		}
	}
}

// The four field names live in three places: savecore owns the offsets, plan
// decides which rows carry which values, and the embedded snapshot stores them
// as ordered columns. A rename in one is a silent no-op restore in the others,
// so they are checked against each other rather than kept in step by hand.
func TestTheRestorableBuddyFieldsAgreeAcrossTheProduct(t *testing.T) {
	fromSaveCore := savecore.BuddyFieldNames()
	fromPlan := slices.Clone(plan.RestorableBuddyFields)
	slices.Sort(fromPlan)
	if !slices.Equal(fromSaveCore, fromPlan) {
		t.Errorf("savecore writes %v, plan restores %v", fromSaveCore, fromPlan)
	}
}
