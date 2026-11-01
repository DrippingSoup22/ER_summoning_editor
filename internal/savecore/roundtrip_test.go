// These tests exercise the whole save path — decrypt, parse, patch, rebuild,
// recompress, encrypt, publish, reopen — against a synthetic save, so a clean
// public clone covers the code that can corrupt a user's file. Before they
// existed the only test that reached this code skipped whenever the private
// research checkout was absent, and the package reported `ok` at 7.3% coverage.
package savecore_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

const (
	loneWolfFamily     = 232000
	bloodfiendTrigger  = 20201000
	wanderingNoble     = 240000
	messmerSoldierBase = 2207000
)

var (
	defaultSaveOnce sync.Once
	defaultSave     []byte
	defaultSaveErr  error
)

// Building the fixture is the expensive part, so do it once and pay only the
// file write per test.
func fixtureBytes(t *testing.T) []byte {
	t.Helper()
	defaultSaveOnce.Do(func() { defaultSave, defaultSaveErr = savefixture.Build(savefixture.Defaults()) })
	if defaultSaveErr != nil {
		t.Fatal(defaultSaveErr)
	}
	return defaultSave
}

func writeSave(t *testing.T, save []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.dat")
	if err := os.WriteFile(path, save, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func manifestFor(t *testing.T, sha string, conversions ...plan.Conversion) []byte {
	t.Helper()
	built, err := plan.Build(sha, conversions, plan.Recipe{MonumentOriginRows: plan.MonumentOriginRows1_17, Area: plan.Anywhere})
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := built.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func loneWolfToBloodfiend() plan.Conversion {
	return plan.Conversion{
		HostGoodsFamily: loneWolfFamily, HostName: "Lone Wolf Ashes",
		UpgradeLevels: savefixture.LevelRows, LevelTriggers: everyLevel(bloodfiendTrigger),
		DestinationName: "Bloodfiend Hexer's Ashes",
	}
}

// everyLevel is the ordinary case: one formation for all upgrade levels.
func everyLevel(trigger int32) []int32 {
	triggers := make([]int32, savefixture.LevelRows)
	for level := range triggers {
		triggers[level] = trigger
	}
	return triggers
}

// The manifest shape is declared twice, in plan and in savecore, and the two
// meet only as JSON. This proves the real encoder satisfies the real strict
// decoder: a renamed field fails here rather than in a user's hands.
func TestPlanManifestSatisfiesTheSaveCoreDecoder(t *testing.T) {
	encoded := manifestFor(t, strings.Repeat("a", 64), loneWolfToBloodfiend())
	_, err := savecore.Apply(context.Background(), filepath.Join(t.TempDir(), "absent.dat"), filepath.Join(t.TempDir(), "out.dat"), bytes.NewReader(encoded))
	if err == nil {
		t.Fatal("a manifest against a missing save must fail")
	}
	// Reaching the save means the manifest decoded and validated; any parse or
	// validation complaint means the two declarations have drifted apart.
	if strings.Contains(err.Error(), "manifest") || strings.Contains(err.Error(), "patch") || strings.Contains(err.Error(), "recipe") {
		t.Fatalf("plan and savecore disagree about the manifest: %v", err)
	}
}

func TestApplyConvertsASaveAndTouchesNothingElse(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if info.RegulationVersion != 11701000 {
		t.Fatalf("fixture regulation version = %d", info.RegulationVersion)
	}

	output := filepath.Join(t.TempDir(), "converted.dat")
	summary, err := savecore.Apply(context.Background(), input, output, bytes.NewReader(manifestFor(t, info.SHA256, loneWolfToBloodfiend())))
	if err != nil {
		t.Fatal(err)
	}
	if summary.GoodsRows != savefixture.LevelRows || summary.BuddyRows != len(plan.MonumentOriginRows1_17) {
		t.Fatalf("verified row counts = %+v", summary)
	}

	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := savefixture.Open(written)
	if err != nil {
		t.Fatalf("the product wrote a save its own format reader cannot open: %v", err)
	}

	// Every level points at the new formation while the multiplayer gate remains
	// byte-identical to the input.
	for level := int32(0); level < savefixture.LevelRows; level++ {
		trigger, enableMulti, err := reader.GoodsTrigger(loneWolfFamily + level)
		if err != nil {
			t.Fatal(err)
		}
		if trigger != bloodfiendTrigger {
			t.Errorf("Lone Wolf +%d trigger = %d, want %d", level, trigger, bloodfiendTrigger)
		}
		if !enableMulti {
			t.Errorf("Lone Wolf +%d had enable_multi changed", level)
		}
	}

	// Families nobody converted keep both their trigger and their multiplayer
	// bit. The goods edit must not leak across families.
	for _, family := range []int32{wanderingNoble, messmerSoldierBase} {
		for level := int32(0); level < savefixture.LevelRows; level++ {
			trigger, enableMulti, err := reader.GoodsTrigger(family + level)
			if err != nil {
				t.Fatal(err)
			}
			if want := savefixture.GoodsFamilies[family][level]; trigger != want {
				t.Errorf("untouched family %d +%d trigger = %d, want %d", family, level, trigger, want)
			}
			if !enableMulti {
				t.Errorf("untouched family %d +%d lost its multiplayer bit", family, level)
			}
		}
	}
}

func TestApplyPreservesEveryMultiplayerGateValue(t *testing.T) {
	options := savefixture.Defaults()
	initializeGoods := options.Tables[0].Init
	options.Tables[0].Init = func(rowID int32, row []byte) {
		initializeGoods(rowID, row)
		if rowID%2 == 0 {
			row[72] &^= 1 << 4
		}
	}
	inputBytes, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	input := writeSave(t, inputBytes)
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "converted.dat")
	if _, err := savecore.Apply(context.Background(), input, output,
		bytes.NewReader(manifestFor(t, info.SHA256, loneWolfToBloodfiend()))); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	before, err := savefixture.Open(inputBytes)
	if err != nil {
		t.Fatal(err)
	}
	after, err := savefixture.Open(written)
	if err != nil {
		t.Fatal(err)
	}
	for level := int32(0); level < savefixture.LevelRows; level++ {
		rowID := loneWolfFamily + level
		_, beforeGate, err := before.GoodsTrigger(rowID)
		if err != nil {
			t.Fatal(err)
		}
		_, afterGate, err := after.GoodsTrigger(rowID)
		if err != nil {
			t.Fatal(err)
		}
		if afterGate != beforeGate {
			t.Errorf("row %d enable_multi changed from %v to %v", rowID, beforeGate, afterGate)
		}
	}
}

// Part one of the shipping recipe without part two is a bug, not a partial fix:
// four vanilla families spawn at a distant monument and appear to break.
func TestApplyWritesBothHalvesOfTheShippingRecipe(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "converted.dat")
	summary, err := savecore.Apply(context.Background(), input, output, bytes.NewReader(manifestFor(t, info.SHA256, loneWolfToBloodfiend())))
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := savefixture.Open(written)
	if err != nil {
		t.Fatal(err)
	}

	for _, rowID := range plan.MonumentOriginRows1_17 {
		appears, err := reader.BuddyAppearsOnMonument(rowID)
		if err != nil {
			t.Fatal(err)
		}
		if appears {
			t.Errorf("buddy row %d still resolves its origin to a monument", rowID)
		}
	}

	stoneIDs, err := reader.RowIDs("BuddyStoneParam.param")
	if err != nil {
		t.Fatal(err)
	}
	if summary.BuddyStoneRows != len(stoneIDs) {
		t.Fatalf("recipe reported %d stone rows, the table has %d", summary.BuddyStoneRows, len(stoneIDs))
	}
	want := map[string]int64{
		"eliminateTargetEntityId": 0, "summonedEventFlagId": 0,
		"activateRange": 65535, "overwriteReturnRange": -1,
		"overwriteActivateRegionEntityId": 0, "warnRegionEntityId": 0,
	}
	for _, rowID := range stoneIDs {
		for name, expected := range want {
			got, err := reader.StoneField(rowID, name)
			if err != nil {
				t.Fatal(err)
			}
			if got != expected {
				t.Errorf("stone row %d field %s = %d, want %d", rowID, name, got, expected)
			}
		}
	}
}

func TestApplyLeavesTheInputSaveUntouched(t *testing.T) {
	original := fixtureBytes(t)
	input := writeSave(t, original)
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "converted.dat")
	if _, err := savecore.Apply(context.Background(), input, output, bytes.NewReader(manifestFor(t, info.SHA256, loneWolfToBloodfiend()))); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Fatal("the input save was modified")
	}
}

func TestApplyRefusesToOverwriteAnExistingOutput(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "taken.dat")
	if err := os.WriteFile(output, []byte("a file the user cares about"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = savecore.Apply(context.Background(), input, output, bytes.NewReader(manifestFor(t, info.SHA256, loneWolfToBloodfiend())))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want a refusal to overwrite", err)
	}
	surviving, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(surviving) != "a file the user cares about" {
		t.Fatal("the existing output file was damaged by the refused write")
	}
}

// A manifest is pinned to the save it was planned against. Applying it to a
// different save would write rows chosen from another regulation's row IDs.
func TestApplyRejectsAManifestPinnedToADifferentSave(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	encoded := manifestFor(t, strings.Repeat("0", 64), loneWolfToBloodfiend())
	_, err := savecore.Apply(context.Background(), input, filepath.Join(t.TempDir(), "out.dat"), bytes.NewReader(encoded))
	if err == nil || !strings.Contains(err.Error(), "manifest requires") {
		t.Fatalf("err = %v, want a source-hash mismatch", err)
	}
}

// Writing a save that is byte-identical to its input would hand the user a file
// they believe is edited and is not.
func TestApplyRejectsAManifestThatChangesNothing(t *testing.T) {
	options := savefixture.Defaults()
	// Seed the family already converted, so every write the manifest asks for is
	// a no-op. The multiplayer bit remains at its shipped value.
	options.Tables[0].Init = func(rowID int32, row []byte) {
		trigger := int32(0)
		for family, vanilla := range savefixture.GoodsFamilies {
			if rowID >= family && rowID < family+savefixture.LevelRows {
				trigger = vanilla[rowID-family]
			}
		}
		if rowID >= loneWolfFamily && rowID < loneWolfFamily+savefixture.LevelRows {
			trigger = bloodfiendTrigger
		}
		row[4] = byte(trigger)
		row[5] = byte(trigger >> 8)
		row[6] = byte(trigger >> 16)
		row[7] = byte(trigger >> 24)
	}
	options.Tables[1].Init = nil // monument flag already clear
	options.Tables[2].Init = func(_ int32, row []byte) {
		row[28], row[29] = 0xFF, 0xFF // activateRange already 65535
		row[30], row[31] = 0xFF, 0xFF // overwriteReturnRange already -1
	}
	save, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	input := writeSave(t, save)
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = savecore.Apply(context.Background(), input, filepath.Join(t.TempDir(), "out.dat"), bytes.NewReader(manifestFor(t, info.SHA256, loneWolfToBloodfiend())))
	if err == nil || !strings.Contains(err.Error(), "changed no save bytes") {
		t.Fatalf("err = %v, want a no-change rejection", err)
	}
}
