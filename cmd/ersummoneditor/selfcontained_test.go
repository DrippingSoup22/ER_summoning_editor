package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// What the released executable is allowed to be, pinned so it stays that way.
//
// The product is one binary that takes a save and writes a save. It is not a
// pipeline stage: it does not consume a manifest, a catalog, a snapshot or any
// other side file, and a user never has to produce one to use it. The manifest
// is internal plumbing between plan and savecore, and the optional
// -manifest-out artifact is an audit record the tool *writes* — never one it
// reads. The GUI in phase I inherits this from application.Service, so the
// boundary has to hold here first.
//
// Every check below would pass today by accident. They exist so that adding an
// input file to the tool is a deliberate act that turns a test red, rather than
// a convenience that arrives during some later slice and is noticed after
// release.

// declaredFlags is the whole command-line surface. It is written out rather
// than derived so that a new flag has to be added here too, which is the point:
// the reviewer of that change is asked, in this file, whether the new flag
// makes the tool read something.
var declaredFlags = map[string]struct {
	// readsAFile is true only for a path the tool opens for reading. Exactly
	// one flag may set it, and it must be the save.
	readsAFile bool
}{
	"save": {readsAFile: true},
	// A switch and nothing more: it changes which values the recipe writes,
	// and reads nothing.
	"fighter": {},
	// A name from a five-word vocabulary, choosing which perception profile
	// the recipe writes over a think clone; the profiles are compiled in
	// beside the recipe. It reads nothing.
	"perception":   {},
	"out":          {},
	"manifest-out": {},
	"dry-run":      {},
	"list":         {},
	"reset":        {},
	"convert":      {},
	// Authoring names an Ash and a list of actor row IDs. Neither is a path,
	// and neither may become one: an actor catalog belongs inside the binary
	// beside the Ash catalog, not in a file the user has to be handed.
	"author": {},
	"slots":  {},
}

func flagNames(t *testing.T) []string {
	t.Helper()
	// run builds its flag set privately, so parse a help request and collect
	// what it registered rather than duplicating the construction here.
	var stderr strings.Builder
	set := flag.NewFlagSet("ersummoneditor", flag.ContinueOnError)
	set.SetOutput(&stderr)
	_ = run(context.Background(), []string{"-h"}, &strings.Builder{}, &stderr)
	var names []string
	for _, line := range strings.Split(stderr.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(line, "-"), " ")
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// A flag that exists but is not declared above has skipped the question this
// file asks. A declared flag that no longer exists means the table went stale.
func TestTheCommandLineSurfaceIsTheDeclaredOne(t *testing.T) {
	present := map[string]bool{}
	for _, name := range flagNames(t) {
		present[name] = true
		if _, declared := declaredFlags[name]; !declared {
			t.Errorf("flag -%s is not declared in declaredFlags; say whether it reads a file", name)
		}
	}
	for name := range declaredFlags {
		if !present[name] {
			t.Errorf("declaredFlags names -%s, which the tool no longer has", name)
		}
	}
}

// The real check, and the one that would catch a regression the flag table
// cannot see: run a complete conversion from a directory holding nothing but
// the save, with the repository's own reference files nowhere in reach. The
// catalog and the vanilla snapshot are compiled in, so this passes; if either
// ever became an os.ReadFile, it would stop passing here rather than on a
// user's machine.
//
// Chdir rather than a path argument because a relative read is exactly the
// mistake being guarded against, and only a changed working directory can
// witness one. These tests are not parallel.
func TestAConversionRunsFromADirectoryHoldingOnlyTheSave(t *testing.T) {
	save := fixtureSaveAt(t)
	elsewhere := t.TempDir()
	if err := os.Rename(save, filepath.Join(elsewhere, "memory.dat")); err != nil {
		t.Fatal(err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	}()

	var stdout, stderr strings.Builder
	if err := run(context.Background(), []string{
		"-save", "memory.dat",
		"-out", "edited.dat",
		"-convert", "Lone Wolf Ashes=Giant Rat Ashes",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("%v (stderr: %s)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "created edited.dat") {
		t.Fatalf("run output = %q", stdout.String())
	}

	// And it produced one file. A tool that quietly dropped a manifest, a
	// cache or a log beside the output would be a different product from the
	// one documented, even if the save were correct.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "edited.dat" || names[1] != "memory.dat" {
		t.Errorf("working directory holds %v, want exactly the input and the output", names)
	}
}
