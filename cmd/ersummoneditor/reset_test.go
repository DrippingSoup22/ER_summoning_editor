package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

func writeVanillaSave(t *testing.T) string {
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

func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestResetRestoresThroughTheCLI(t *testing.T) {
	input := writeVanillaSave(t)
	dir := t.TempDir()
	converted := filepath.Join(dir, "converted.dat")
	if _, _, err := runCLI(t, "-save", input, "-out", converted,
		"-convert", "Lone Wolf Ashes=Giant Rat Ashes"); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(dir, "restored.dat")
	stdout, _, err := runCLI(t, "-save", converted, "-out", restored, "-reset")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "restored rows: goods=11 buddy=20 buddy-stone=304") {
		t.Errorf("output does not report the restore:\n%s", stdout)
	}
	if _, err := os.Stat(restored); err != nil {
		t.Fatalf("reset did not create the output: %v", err)
	}
	// And the result reads back as unedited.
	after, _, err := runCLI(t, "-save", restored, "-reset", "-dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, "already unedited") {
		t.Errorf("the restored save is still reported as edited:\n%s", after)
	}
}
