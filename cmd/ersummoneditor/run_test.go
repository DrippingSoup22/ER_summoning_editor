package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// fixtureSaveAt writes a save with every row the catalog names.
//
// Not savefixture.Defaults, which carries eight goods families where the
// regulation has eighty-four. That was enough while an operation read only the
// rows it was about to write; a session reads all of them, because the Ashes
// view describes all of them, and a file holding a tenth is not a 1.17 save.
func fixtureSaveAt(t *testing.T) string {
	t.Helper()
	options, err := savefixture.VanillaOptions()
	if err != nil {
		t.Fatal(err)
	}
	save, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "memory.dat")
	if err := os.WriteFile(path, save, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The dry run has to be trustworthy on both counts: it must describe the edit,
// and it must leave the directory as it found it.
func TestDryRunDescribesTheEditAndWritesNothing(t *testing.T) {
	input := fixtureSaveAt(t)
	directory := t.TempDir()
	var stdout, stderr strings.Builder

	err := run(context.Background(), []string{
		"-save", input,
		"-out", filepath.Join(directory, "out.dat"),
		"-manifest-out", filepath.Join(directory, "manifest.json"),
		"-convert", "Lone Wolf Ashes=Bloodfiend Hexer's Ashes",
		"-dry-run",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("%v (stderr: %s)", err, stderr.String())
	}

	report := stdout.String()
	for _, want := range []string{
		"nothing was written", "Lone Wolf Ashes", "Bloodfiend Hexer's Ashes",
		"EquipParamGoods", "BuddyStoneParam", "once-per-rest",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("dry run output is missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "already applied") {
		t.Errorf("an unedited save was reported as already converted:\n%s", report)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a dry run created %d files", len(entries))
	}
}

func TestRunConvertsAndReportsTheResult(t *testing.T) {
	input := fixtureSaveAt(t)
	directory := t.TempDir()
	output := filepath.Join(directory, "out.dat")
	var stdout, stderr strings.Builder

	err := run(context.Background(), []string{
		"-save", input, "-out", output,
		"-convert", "Lone Wolf Ashes=Bloodfiend Hexer's Ashes",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("%v (stderr: %s)", err, stderr.String())
	}
	report := stdout.String()
	if !strings.Contains(report, "created "+output) {
		t.Errorf("output does not report what it created:\n%s", report)
	}
	if !strings.Contains(report, "input SHA-256:") || !strings.Contains(report, "output SHA-256:") {
		t.Errorf("output does not report both hashes:\n%s", report)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("the converted save is missing: %v", err)
	}
}
