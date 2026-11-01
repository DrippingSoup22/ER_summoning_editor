package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

const (
	loneWolfFamily = 232000
	giantRatFamily = 233000
)

// The point of a preview is that it costs nothing. A preview that left a file
// behind would be an edit with a different name.
func TestPreviewWritesNothing(t *testing.T) {
	t.Parallel()
	input := defaultSaveAt(t)
	directory := t.TempDir()
	preview, err := newService(t).Preview(context.Background(), loneWolfRequest(
		input, filepath.Join(directory, "out.dat"), filepath.Join(directory, "manifest.json")))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("preview created %d files", len(entries))
	}
	if len(preview.Manifest) == 0 {
		t.Fatal("preview returned no manifest to show")
	}
}

// A preview a frontend cannot trust is worse than none: the user approves one
// thing and gets another. Apply and Preview share one preparation step, and
// this is what holds them to it.
func TestPreviewPromisesExactlyWhatApplyWrites(t *testing.T) {
	t.Parallel()
	input := defaultSaveAt(t)
	service := newService(t)

	previewDirectory := t.TempDir()
	preview, err := service.Preview(context.Background(), loneWolfRequest(
		input, filepath.Join(previewDirectory, "out.dat"), filepath.Join(previewDirectory, "manifest.json")))
	if err != nil {
		t.Fatal(err)
	}

	applyDirectory := t.TempDir()
	manifestPath := filepath.Join(applyDirectory, "manifest.json")
	result, err := service.Apply(context.Background(), loneWolfRequest(
		input, filepath.Join(applyDirectory, "out.dat"), manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preview.Manifest, written) {
		t.Fatal("the manifest Apply wrote differs from the one Preview showed")
	}
	if preview.GoodsRows != result.GoodsRows || preview.BuddyRows != result.BuddyRows {
		t.Fatalf("preview promised goods=%d buddy=%d, apply did goods=%d buddy=%d",
			preview.GoodsRows, preview.BuddyRows, result.GoodsRows, result.BuddyRows)
	}
	if preview.StoneRows != result.BuddyStoneRows {
		t.Fatalf("preview promised %d stone rows, apply wrote %d", preview.StoneRows, result.BuddyStoneRows)
	}
	if len(preview.Conversions) != 1 || preview.Conversions[0].ResolvedConversion != result.Conversions[0] {
		t.Fatal("preview and apply resolved the conversion differently")
	}
}

func TestApplicationFailuresCarryTheirType(t *testing.T) {
	t.Parallel()
	input := defaultSaveAt(t)
	service := newService(t)
	output := filepath.Join(t.TempDir(), "out.dat")

	t.Run("unknown ash names the field and the selector", func(t *testing.T) {
		_, err := service.Preview(context.Background(), Request{
			InputPath: input, OutputPath: output,
			Conversions: []ConversionRequest{{Host: "Lone Wolf Ashes", Destination: "Wolf of Radagon"}},
		})
		var unknown *UnknownAshError
		if !errors.As(err, &unknown) {
			t.Fatalf("err = %v, want *UnknownAshError", err)
		}
		if unknown.Role != "destination" || unknown.Selector != "Wolf of Radagon" {
			t.Fatalf("error = %+v, want the destination field and the typed name", unknown)
		}
		// The catalog's own reason stays reachable, so a sentinel added there
		// later is matchable through this error without changing it.
		if errors.Unwrap(unknown) == nil {
			t.Fatal("the typed error hid the catalog reason instead of wrapping it")
		}
	})

	t.Run("unsupported regulation carries both versions", func(t *testing.T) {
		options := savefixture.Defaults()
		options.RegulationVersion = 11611000
		_, err := service.Preview(context.Background(), loneWolfRequest(saveAt(t, options), output, ""))
		var unsupported *UnsupportedRegulationError
		if !errors.As(err, &unsupported) {
			t.Fatalf("err = %v, want *UnsupportedRegulationError", err)
		}
		if unsupported.Found != 11611000 {
			t.Fatalf("error = %+v", unsupported)
		}
	})

	t.Run("duplicate host names both requests", func(t *testing.T) {
		_, err := service.Preview(context.Background(), Request{
			InputPath: input, OutputPath: output,
			Conversions: []ConversionRequest{
				{Host: "Lone Wolf Ashes", Destination: "Bloodfiend Hexer's Ashes"},
				{Host: "232000", Destination: "Giant Rat Ashes"},
			},
		})
		var duplicate *DuplicateHostError
		if !errors.As(err, &duplicate) {
			t.Fatalf("err = %v, want *DuplicateHostError", err)
		}
		if duplicate.GoodsFamily != loneWolfFamily {
			t.Fatalf("error = %+v", duplicate)
		}
	})

	for _, testCase := range []struct {
		name    string
		request Request
		want    error
	}{
		{"no paths", Request{Conversions: []ConversionRequest{{Host: "232000", Destination: "233000"}}}, ErrPathRequired},
		{"output over input", loneWolfRequest(input, input, ""), ErrPathCollision},
		{"manifest over output", loneWolfRequest(input, output, output), ErrPathCollision},
		{"nothing to do", Request{InputPath: input, OutputPath: output}, ErrNoConversions},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := service.Preview(context.Background(), testCase.request)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
		})
	}
}

// trippingContext reports cancellation the nth time Err is consulted, which
// places the abort *between* two stages instead of before all of them.
//
// A context cancelled up front, as above, never reaches the later stages at
// all: the first check refuses and everything after it is untested. Counting
// calls is only a way to move the abort; the test below sweeps n rather than
// asserting any particular stage, because the guarantee is not about one
// boundary. Wherever a cancellation lands, the operation publishes nothing.
type trippingContext struct {
	context.Context
	calls  *int
	tripOn int
}

func (c trippingContext) Err() error {
	*c.calls++
	if *c.calls >= c.tripOn {
		return context.Canceled
	}
	return c.Context.Err()
}

func TestCancellationAtEveryStageBoundaryPublishesNothing(t *testing.T) {
	t.Parallel()
	input := defaultSaveAt(t)
	service := newService(t)

	aborted, completed := 0, 0
	// The sweep runs until a cancellation no longer lands: that run takes the
	// ordinary success path and proves the sweep is not passing by never
	// getting there, and every later check would land nowhere too.
	for tripOn := 1; completed == 0; tripOn++ {
		if tripOn > 100 {
			t.Fatal("no run completed in 100 checks")
		}
		t.Run(fmt.Sprintf("abort on check %d", tripOn), func(t *testing.T) {
			calls := 0
			ctx := trippingContext{Context: context.Background(), calls: &calls, tripOn: tripOn}

			directory := t.TempDir()
			output := filepath.Join(directory, "out.dat")
			manifest := filepath.Join(directory, "manifest.json")
			_, err := service.Apply(ctx, loneWolfRequest(input, output, manifest))

			entries, readErr := os.ReadDir(directory)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err == nil {
				// The cancellation never landed. Then this is a normal run and
				// both artifacts must exist.
				completed++
				if len(entries) != 2 {
					t.Fatalf("a successful apply left %d files, want the save and its manifest", len(entries))
				}
				return
			}
			aborted++
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
			if len(entries) != 0 {
				names := make([]string, 0, len(entries))
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				t.Fatalf("a cancelled operation left %v behind", names)
			}
		})
	}

	// Without this the sweep could pass while proving nothing, which is exactly
	// how the test it replaces went unnoticed: every case must not land in the
	// same branch. There has to be a real abort and a real completion.
	if aborted == 0 || completed == 0 {
		t.Fatalf("sweep aborted %d and completed %d runs; it must do both to mean anything", aborted, completed)
	}
}

// Giant Rat Ashes delivers three rats at +0..+4, four at +5..+9 and five at
// +10. Its formations split the reinforcement table between those ranges, so
// it is the explicit exception to the fixed ordinary-family rule.
func TestConvertingToALevelVaryingFamilyKeepsItsProgression(t *testing.T) {
	t.Parallel()
	input := defaultSaveAt(t)
	preview, err := newService(t).Preview(context.Background(), Request{
		InputPath:   input,
		Conversions: []ConversionRequest{{Host: "Lone Wolf Ashes", Destination: "Giant Rat Ashes"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	conversion := preview.Conversions[0]
	if !conversion.VariesByLevel {
		t.Fatal("a level-varying destination was reported as delivering one formation")
	}
	want := savefixture.GoodsFamilies[giantRatFamily]
	if !slices.Equal(conversion.LevelTriggers, want) {
		t.Fatalf("level triggers = %v, want %v", conversion.LevelTriggers, want)
	}
	// The three formations, with the upgrade level taken back out: three rats
	// through +4, four through +9, five at +10.
	wantFormations := []int32{
		233000, 233000, 233000, 233000, 233000,
		233100, 233100, 233100, 233100, 233100,
		233200,
	}
	if !slices.Equal(conversion.LevelFormations, wantFormations) {
		t.Fatalf("level formations = %v, want %v", conversion.LevelFormations, wantFormations)
	}

	// And the manifest has to carry them through to the individual rows, since
	// that is the artifact the save layer actually applies.
	var manifest struct {
		GoodsPatches []struct {
			RowID int32          `json:"row_id"`
			Set   map[string]any `json:"set"`
		} `json:"goods_patches"`
	}
	if err := json.Unmarshal(preview.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.GoodsPatches) != len(want) {
		t.Fatalf("got %d goods patches, want %d", len(manifest.GoodsPatches), len(want))
	}
	for level, patch := range manifest.GoodsPatches {
		if patch.RowID != loneWolfFamily+int32(level) {
			t.Fatalf("patch %d targets row %d", level, patch.RowID)
		}
		if got := int32(patch.Set["refId_default"].(float64)); got != want[level] {
			t.Fatalf("row %d points at %d, want %d", patch.RowID, got, want[level])
		}
	}
}
