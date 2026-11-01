package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

var (
	vanillaSaveOnce sync.Once
	vanillaSave     []byte
	vanillaSaveErr  error
)

// A save carrying every row the shipped snapshot records. Reset restores all of
// them, so the small default fixture cannot exercise it at all.
func vanillaBytes(t *testing.T) []byte {
	t.Helper()
	vanillaSaveOnce.Do(func() {
		options, err := savefixture.VanillaOptions()
		if err != nil {
			vanillaSaveErr = err
			return
		}
		vanillaSave, vanillaSaveErr = savefixture.Build(options)
	})
	if vanillaSaveErr != nil {
		t.Fatal(vanillaSaveErr)
	}
	return vanillaSave
}

func writeVanilla(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.dat")
	if err := os.WriteFile(path, vanillaBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newService(t *testing.T) *application.Service {
	t.Helper()
	service, err := application.New()
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// The workflow a user runs: convert a save, then put it back.
func TestResetUndoesAConversionThroughTheApplication(t *testing.T) {
	t.Parallel()
	service := newService(t)
	input := writeVanilla(t)
	dir := t.TempDir()
	converted := filepath.Join(dir, "converted.dat")

	if _, err := service.Apply(context.Background(), application.Request{
		InputPath:   input,
		OutputPath:  converted,
		Conversions: []application.ConversionRequest{{Host: "Lone Wolf Ashes", Destination: "Giant Rat Ashes"}},
	}); err != nil {
		t.Fatal(err)
	}

	// The preview must recognise the save as edited before the reset runs.
	preview, err := service.PreviewReset(context.Background(), application.Request{InputPath: converted})
	if err != nil {
		t.Fatal(err)
	}
	if preview.AlreadyVanilla {
		t.Fatal("a converted save was reported as already vanilla")
	}
	if !preview.StoneRecipeApplied {
		t.Error("the expanded-area recipe was not detected on a converted save")
	}
	// Only what the conversion actually changed: the host family's eleven
	// goods rows, not all 924.
	if preview.GoodsRows != 11 || preview.BuddyRows != 20 || preview.StoneRows != 304 {
		t.Errorf("reset would write goods=%d buddy=%d stone=%d, want 11/20/304",
			preview.GoodsRows, preview.BuddyRows, preview.StoneRows)
	}

	restored := filepath.Join(dir, "restored.dat")
	result, err := service.Reset(context.Background(), application.Request{
		InputPath: converted, OutputPath: restored,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GoodsRows != 11 || result.BuddyRows != 20 || result.BuddyStoneRows != 304 {
		t.Errorf("reset wrote goods=%d buddy=%d stone=%d, want 11/20/304",
			result.GoodsRows, result.BuddyRows, result.BuddyStoneRows)
	}

	// And the restored save is indistinguishable from an unedited one.
	after, err := service.PreviewReset(context.Background(), application.Request{InputPath: restored})
	if err != nil {
		t.Fatal(err)
	}
	if !after.AlreadyVanilla {
		t.Errorf("after a reset the save still reports goods=%d buddy=%d stone=%d to restore",
			after.GoodsRows, after.BuddyRows, after.StoneRows)
	}
}

// A preview is binding for reset too: it must not report a save as unedited and
// then have the write refuse it, or the reverse.
func TestResetPreviewAgreesWithWhatResetDoes(t *testing.T) {
	t.Parallel()
	service := newService(t)
	input := writeVanilla(t)

	preview, err := service.PreviewReset(context.Background(), application.Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.AlreadyVanilla {
		t.Fatal("an unedited save was not reported as already vanilla")
	}
	_, err = service.Reset(context.Background(), application.Request{
		InputPath: input, OutputPath: filepath.Join(t.TempDir(), "out.dat"),
	})
	if !errors.Is(err, application.ErrNoChange) {
		t.Fatalf("error = %v, want ErrNoChange, which is what AlreadyVanilla predicts", err)
	}
}
