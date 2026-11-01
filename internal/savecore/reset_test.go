package savecore_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// vanillaOf reads the shipped values out of a save and puts them in the shape
// plan.BuildReset wants.
func vanillaOf(t *testing.T, path string, goodsRows []int32) plan.Vanilla {
	t.Helper()
	snapshot, err := savecore.ReadVanillaSnapshot(context.Background(), path, goodsRows)
	if err != nil {
		t.Fatal(err)
	}
	stone := make(map[int32]map[string]int64, len(snapshot.Stone))
	for rowID, values := range snapshot.Stone {
		stone[rowID] = maps.Clone(values)
	}
	buddy := make(map[int32]map[string]float64, len(snapshot.Buddy))
	for rowID, values := range snapshot.Buddy {
		buddy[rowID] = maps.Clone(values)
	}
	goods := make(map[int32]map[string]int64, len(snapshot.Goods))
	for rowID, values := range snapshot.Goods {
		goods[rowID] = maps.Clone(values)
	}
	return plan.Vanilla{Goods: goods, Buddy: buddy, Stone: stone}
}

// equalRowTables compares two per-row, per-field records. maps.Equal cannot:
// its values must be comparable, and a row here is itself a map.
func equalRowTables[V ~map[string]int64](left, right map[int32]V) bool {
	if len(left) != len(right) {
		return false
	}
	for rowID, values := range left {
		other, present := right[rowID]
		if !present || !maps.Equal(values, other) {
			return false
		}
	}
	return true
}

func resetManifestFor(t *testing.T, sha string, shipped, current plan.Vanilla) []byte {
	t.Helper()
	built, err := plan.BuildReset(sha, shipped, current)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := built.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

// The test the whole feature exists to pass.
//
// Convert a save, then reset it, and every field the product can write is back
// to what the game shipped — including the 304 distinct stone rows the
// conversion recipe flattened into one value set, which is the part no
// replay-the-manifest-backwards scheme could restore.
func TestResetReturnsAConvertedSaveToShippedValues(t *testing.T) {
	original := writeSave(t, fixtureBytes(t))
	rows := allGoodsRows()
	shipped := vanillaOf(t, original, rows)

	converted := newOutputPath(t)
	convertedInfo := applyConversion(t, original, converted)

	// Prove the conversion really did move all three tables first, so a reset
	// that appeared to work could not be a reset of nothing.
	afterConversion, err := savecore.ReadVanillaSnapshot(context.Background(), converted, rows)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(afterConversion.MonumentOriginRows(), monumentRowsOf(shipped)) {
		t.Fatal("the conversion did not clear any monument-origin row")
	}
	if maps.Equal(afterConversion.Goods[loneWolfFamily], shipped.Goods[loneWolfFamily]) {
		t.Fatal("the conversion did not change the host's +0 trigger")
	}
	distinctAfter := map[string]bool{}
	for _, values := range afterConversion.Stone {
		distinctAfter[valueKey(values)] = true
	}
	if len(distinctAfter) != 1 {
		t.Fatalf("the conversion left %d distinct stone value sets, want the recipe's single one", len(distinctAfter))
	}

	restored := newOutputPath(t)
	manifest := resetManifestFor(t, convertedInfo.SHA256, shipped, vanillaOf(t, converted, rows))
	summary, err := savecore.Apply(context.Background(), converted, restored, bytes.NewReader(manifest))
	if err != nil {
		t.Fatal(err)
	}
	// Only the rows the conversion actually moved: eleven goods rows for the
	// one converted family, not all of them.
	if summary.GoodsRows != savefixture.LevelRows {
		t.Errorf("reset wrote %d goods rows, want the %d the conversion changed",
			summary.GoodsRows, savefixture.LevelRows)
	}

	after := vanillaOf(t, restored, rows)
	if !equalRowTables(after.Goods, shipped.Goods) {
		t.Errorf("goods triggers were not restored")
	}
	if !slices.Equal(monumentRowsOf(after), monumentRowsOf(shipped)) {
		t.Errorf("monument-origin rows = %v, want the shipped %v", monumentRowsOf(after), monumentRowsOf(shipped))
	}
	if len(after.Buddy) != len(shipped.Buddy) {
		t.Fatalf("buddy rows = %d, want %d", len(after.Buddy), len(shipped.Buddy))
	}
	for rowID, want := range shipped.Buddy {
		if !maps.Equal(after.Buddy[rowID], want) {
			t.Errorf("buddy row %d = %v, want the shipped %v", rowID, after.Buddy[rowID], want)
		}
	}
	if len(after.Stone) != len(shipped.Stone) {
		t.Fatalf("stone rows = %d, want %d", len(after.Stone), len(shipped.Stone))
	}
	for rowID, want := range shipped.Stone {
		if !maps.Equal(after.Stone[rowID], want) {
			t.Errorf("stone row %d = %v, want the shipped %v", rowID, after.Stone[rowID], want)
		}
	}
}

// A reset writes the same three tables a conversion does, so the multiplayer
// rule applies to it identically: the shipped gate stays, whatever it is.
func TestResetPreservesEveryMultiplayerGateValue(t *testing.T) {
	original := writeSave(t, fixtureBytes(t))
	rows := allGoodsRows()
	shipped := vanillaOf(t, original, rows)

	converted := newOutputPath(t)
	convertedInfo := applyConversion(t, original, converted)

	restored := newOutputPath(t)
	if _, err := savecore.Apply(context.Background(), converted, restored,
		bytes.NewReader(resetManifestFor(t, convertedInfo.SHA256, shipped,
			vanillaOf(t, converted, rows)))); err != nil {
		t.Fatal(err)
	}
	state, err := savecore.ReadState(context.Background(), restored, rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, rowID := range rows {
		if !state.Goods[rowID].EnableMulti {
			t.Errorf("goods row %d lost its shipped multiplayer gate through a reset", rowID)
		}
	}
}

// The two operations are inverses, so a manifest that mixes them is incoherent
// and must be refused rather than resolved in some order.
func TestManifestRejectsMixedOperations(t *testing.T) {
	original := writeSave(t, fixtureBytes(t))
	info, err := savecore.Inspect(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	shipped := vanillaOf(t, original, allGoodsRows())

	reset, err := plan.BuildReset(info.SHA256, shipped, plan.Vanilla{})
	if err != nil {
		t.Fatal(err)
	}
	// A reset carrying the conversion's all-row recipe as well.
	reset.BuddyStonePatches = []plan.TablePatch{{Name: "recipe", Set: map[string]any{
		"activateRange": 65535, "overwriteReturnRange": -1,
		"overwriteActivateRegionEntityId": 0, "warnRegionEntityId": 0,
		"summonedEventFlagId": 0, "eliminateTargetEntityId": 0,
	}}}
	var encoded bytes.Buffer
	if err := reset.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	_, err = savecore.Apply(context.Background(), original, newOutputPath(t), bytes.NewReader(encoded.Bytes()))
	if !errors.Is(err, savecore.ErrManifestInvalid) {
		t.Fatalf("error = %v, want ErrManifestInvalid", err)
	}
}

// A conversion must not be able to reach the per-row writer, which is the only
// path in the product that writes a value not fixed at compile time.
func TestAConversionCannotUseThePerRowStoneWriter(t *testing.T) {
	original := writeSave(t, fixtureBytes(t))
	info, err := savecore.Inspect(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	built, err := plan.Build(info.SHA256, []plan.Conversion{loneWolfToGiantRat()},
		plan.Recipe{MonumentOriginRows: plan.MonumentOriginRows1_17, Area: plan.Anywhere})
	if err != nil {
		t.Fatal(err)
	}
	built.BuddyStoneRowPatches = []plan.RowPatch{{RowID: 0, Name: "smuggled", Set: map[string]any{
		"activateRange": 1, "overwriteReturnRange": 1,
		"overwriteActivateRegionEntityId": 1, "warnRegionEntityId": 1,
		"summonedEventFlagId": 1, "eliminateTargetEntityId": 1,
	}}}
	var encoded bytes.Buffer
	if err := built.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	_, err = savecore.Apply(context.Background(), original, newOutputPath(t), bytes.NewReader(encoded.Bytes()))
	if !errors.Is(err, savecore.ErrManifestInvalid) {
		t.Fatalf("error = %v, want ErrManifestInvalid", err)
	}
}

// applyConversion runs one ordinary conversion and returns what the result is.
func applyConversion(t *testing.T, input, output string) *savecore.Info {
	t.Helper()
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := savecore.Apply(context.Background(), input, output,
		bytes.NewReader(manifestFor(t, info.SHA256, loneWolfToGiantRat()))); err != nil {
		t.Fatal(err)
	}
	converted, err := savecore.Inspect(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	return converted
}

func loneWolfToGiantRat() plan.Conversion {
	return plan.Conversion{
		HostGoodsFamily: loneWolfFamily,
		HostName:        "Lone Wolf Ashes",
		UpgradeLevels:   savefixture.LevelRows,
		LevelTriggers:   append([]int32(nil), savefixture.GoodsFamilies[233000]...),
		DestinationName: "Giant Rat Ashes",
	}
}

func allGoodsRows() []int32 {
	var rows []int32
	for family := range savefixture.GoodsFamilies {
		for level := int32(0); level < savefixture.LevelRows; level++ {
			rows = append(rows, family+level)
		}
	}
	slices.Sort(rows)
	return rows
}

func valueKey(values savecore.StoneRow) string {
	ordered := make([]int64, 0, len(values))
	for _, name := range savecore.StoneFieldNames() {
		ordered = append(ordered, values[name])
	}
	return fmt.Sprint(ordered)
}

// A path Apply may create. writeSave would occupy it, and the product refuses
// to overwrite an existing output.
func newOutputPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "written.dat")
}

// monumentRowsOf projects the rows carrying the monument flag out of a
// plan.Vanilla, which records all four restorable fields per row.
func monumentRowsOf(vanilla plan.Vanilla) []int32 {
	var rows []int32
	for rowID, values := range vanilla.Buddy {
		if values["appearOnAroundSekihi"] != 0 {
			rows = append(rows, rowID)
		}
	}
	slices.Sort(rows)
	return rows
}
