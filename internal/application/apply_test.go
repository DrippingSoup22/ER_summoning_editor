package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

var (
	fixtureOnce sync.Once
	fixtureSave []byte
	fixtureErr  error
)

// defaultSaveAt writes the shared fixture, built once because building is the
// expensive part and writing is not.
//
// It is built from the recorded shipped values rather than from
// savefixture.Defaults, which carries eight goods families where the
// regulation has eighty-four. That was enough while an operation read only the
// rows it was about to write; it is not a save this product could ever meet. A
// session reads every row the catalog names, because the Ashes view describes
// every one of them, and a file holding a tenth of them is not a 1.17 save.
// Tests that want a deliberately broken shape still build their own.
func defaultSaveAt(t *testing.T) string {
	t.Helper()
	fixtureOnce.Do(func() {
		options, err := savefixture.VanillaOptions()
		if err != nil {
			fixtureErr = err
			return
		}
		fixtureSave, fixtureErr = savefixture.Build(options)
	})
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
	return writeTo(t, fixtureSave)
}

// saveAt builds a one-off fixture for a test that needs a different save.
func saveAt(t *testing.T, options savefixture.Options) string {
	t.Helper()
	save, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	return writeTo(t, save)
}

func writeTo(t *testing.T, save []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.dat")
	if err := os.WriteFile(path, save, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loneWolfRequest(input, output, manifest string) Request {
	return Request{
		InputPath: input, OutputPath: output, ManifestPath: manifest,
		Conversions: []ConversionRequest{{Host: "Lone Wolf Ashes", Destination: "Bloodfiend Hexer's Ashes"}},
	}
}

func TestApplyRunsTheWholeWorkflowAndReportsWhatItDid(t *testing.T) {
	t.Parallel()
	input := defaultSaveAt(t)
	directory := t.TempDir()
	output := filepath.Join(directory, "converted.dat")
	manifestPath := filepath.Join(directory, "conversion.json")

	result, err := newService(t).Apply(context.Background(), loneWolfRequest(input, output, manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	if reader, err := savefixture.Open(fixtureSave); err != nil {
		t.Fatal(err)
	} else if result.RegulationVersion != reader.RegulationVersion() {
		t.Fatalf("reported regulation %d, the save holds %d", result.RegulationVersion, reader.RegulationVersion())
	}
	if result.GoodsRows != savefixture.LevelRows {
		t.Fatalf("goods rows = %d, want %d", result.GoodsRows, savefixture.LevelRows)
	}
	if len(result.Conversions) != 1 {
		t.Fatalf("reported %d conversions", len(result.Conversions))
	}
	if got := result.Conversions[0]; got.HostGoodsFamily != 232000 || got.TriggerSpEffectID != 20201000 {
		t.Fatalf("resolved conversion = %+v", got)
	}
	if result.InputSHA256 == result.OutputSHA256 {
		t.Fatal("input and output hashes match, so nothing was written")
	}

	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := savefixture.Open(written)
	if err != nil {
		t.Fatal(err)
	}
	trigger, enableMulti, err := reader.GoodsTrigger(232000)
	if err != nil {
		t.Fatal(err)
	}
	if trigger != 20201000 || !enableMulti {
		t.Fatalf("converted row: trigger %d, enable_multi %v", trigger, enableMulti)
	}

	// The audit manifest must describe the edit that was actually made.
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var audit struct {
		SourceSHA256 string `json:"source_sha256"`
		GoodsPatches []struct {
			RowID int32 `json:"row_id"`
		} `json:"goods_patches"`
	}
	if err := json.Unmarshal(body, &audit); err != nil {
		t.Fatal(err)
	}
	if audit.SourceSHA256 != result.InputSHA256 {
		t.Fatal("the manifest is not pinned to the save it was applied to")
	}
	if len(audit.GoodsPatches) != savefixture.LevelRows {
		t.Fatalf("manifest lists %d goods rows", len(audit.GoodsPatches))
	}
}

// A save whose regulation is older than the installed game has that regulation
// replaced wholesale the moment a character loads, offline and with no error.
// In game that is indistinguishable from an edit that did nothing, and it
// voided two console runs on the research project.
func TestApplyRefusesASaveFromAnotherRegulationVersion(t *testing.T) {
	t.Parallel()
	options := savefixture.Defaults()
	options.RegulationVersion = 11611000
	input := saveAt(t, options)
	directory := t.TempDir()

	_, err := newService(t).Apply(context.Background(), loneWolfRequest(input, filepath.Join(directory, "out.dat"), ""))
	if err == nil {
		t.Fatal("a save from another regulation version was accepted")
	}
	if !strings.Contains(err.Error(), "11611000") || !strings.Contains(err.Error(), "11701000") {
		t.Fatalf("err = %v, want it to name both the found and required versions", err)
	}
	if _, statErr := os.Stat(filepath.Join(directory, "out.dat")); statErr == nil {
		t.Fatal("a rejected conversion still produced an output file")
	}
}

func TestApplyRejectsPathsThatWouldDestroyAnInput(t *testing.T) {
	t.Parallel()
	input := defaultSaveAt(t)
	directory := t.TempDir()
	output := filepath.Join(directory, "out.dat")

	for _, testCase := range []struct {
		name    string
		request Request
		want    string
	}{
		{"manifest over the input", loneWolfRequest(input, output, input), "manifest path must differ"},
		{"manifest over the output", loneWolfRequest(input, output, output), "manifest path must differ"},
		{"no output path", loneWolfRequest(input, "", ""), "required"},
		{"no input path", loneWolfRequest("", output, ""), "required"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := newService(t).Apply(context.Background(), testCase.request)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to mention %q", err, testCase.want)
			}
		})
	}
}
