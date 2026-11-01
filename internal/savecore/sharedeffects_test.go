package savecore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

const sharedTestSource = "0000000000000000000000000000000000000000000000000000000000000000"

func sharedPatches(value func(SharedEffectWrite) float64) []rowPatch {
	byRow := map[int32]map[string]float64{}
	var order []int32
	for _, write := range sharedEffectWrites {
		if byRow[write.Row] == nil {
			byRow[write.Row] = map[string]float64{}
			order = append(order, write.Row)
		}
		byRow[write.Row][write.Field] = value(write)
	}
	patches := make([]rowPatch, 0, len(order))
	for _, row := range order {
		patches = append(patches, rowPatch{RowID: row, Set: byRow[row]})
	}
	return patches
}

func sameF32(a, b float64) bool { return math.Float32bits(float32(a)) == math.Float32bits(float32(b)) }

// Written and put back on a real save: the fields move, and putting them back
// returns the regulation byte for byte.
func TestASharedWriteMovesItsFieldsAndPuttingThemBackRestoresTheSave(t *testing.T) {
	source := realSavePath(t)
	original, err := regulation.Load(source)
	if err != nil {
		t.Fatal(err)
	}
	apply := func(from, to string, value func(SharedEffectWrite) float64) {
		t.Helper()
		body, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := json.Marshal(manifest{SourceSHA256: regulation.Sha256Hex(body), Operation: operationSharedEffects,
			EffectPatches: sharedPatches(value)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Apply(context.Background(), from, to, bytes.NewReader(manifest)); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	written, restored := filepath.Join(dir, "written.dat"), filepath.Join(dir, "restored.dat")
	apply(source, written, func(w SharedEffectWrite) float64 { return w.Written })
	loaded, err := regulation.Load(written)
	if err != nil {
		t.Fatal(err)
	}
	held, err := readSharedEffects(loaded.BND4)
	if err != nil {
		t.Fatal(err)
	}
	for _, write := range sharedEffectWrites {
		if value := held[SharedEffectKey{write.Row, write.Field}]; !sameF32(value, write.Written) {
			t.Errorf("%d.%s reads %g after the write, want %g", write.Row, write.Field, value, write.Written)
		}
	}
	state, err := ReadState(context.Background(), written, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.SharedEffects) != len(sharedEffectWrites) {
		t.Errorf("ReadState reports %d shared fields, want %d", len(state.SharedEffects), len(sharedEffectWrites))
	}
	apply(written, restored, func(w SharedEffectWrite) float64 { return w.Shipped })
	back, err := regulation.Load(restored)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back.BND4, original.BND4) {
		t.Error("putting every shared field back does not return the shipped regulation")
	}
}

// The exception cannot widen: every way out of the surface is refused.
func TestASharedWriteOutsideItsSurfaceIsRefused(t *testing.T) {
	good := func() *manifest {
		return &manifest{SourceSHA256: sharedTestSource, Operation: operationSharedEffects,
			EffectPatches: []rowPatch{{RowID: 13615, Set: map[string]float64{"categoryPriority": 42}}}}
	}
	if err := validateManifest(good()); err != nil {
		t.Fatalf("the Ancestor's own write is refused: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*manifest)
		want   string
	}{
		{"a row not on the list", func(m *manifest) { m.EffectPatches[0].RowID = 13614 }, "not a field"},
		{"a field the row does not move", func(m *manifest) {
			m.EffectPatches[0].Set = map[string]float64{"effectEndurance": 5}
		}, "not a field"},
		{"a third value", func(m *manifest) { m.EffectPatches[0].Set["categoryPriority"] = 44 }, "may hold"},
		{"the same row twice", func(m *manifest) { m.EffectPatches = append(m.EffectPatches, m.EffectPatches[0]) }, "twice"},
		{"a patch that sets nothing", func(m *manifest) { m.EffectPatches[0].Set = map[string]float64{} }, "sets nothing"},
		{"another table beside it", func(m *manifest) {
			m.BuddyPatches = []rowPatch{{RowID: 1, Set: map[string]float64{"npcParamId": 1}}}
		}, "nothing else"},
		{"another operation carrying it", func(m *manifest) {
			m.Operation = operationTint
			m.VfxPatches = []rowPatch{{RowID: spiritVfxRow, Set: map[string]float64{"phantomParamOverwriteId": 200}}}
		}, "may not write SpEffectParam"},
		{"an operation with nothing to write", func(m *manifest) { m.EffectPatches = nil }, "needs the rows"},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := good()
			test.mutate(plan)
			err := validateManifest(plan)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refused with %q, want %q", err, test.want)
			}
		})
	}
	// Two parts of one session may not both describe a row.
	session := &manifest{SourceSHA256: sharedTestSource, Operation: operationSession, Parts: []*manifest{good(), good()}}
	if err := validateManifest(session); err == nil || !strings.Contains(err.Error(), "SpEffectParam row 13615") {
		t.Fatalf("two parts writing one row: %v", err)
	}
	// And a session may not carry one of its own.
	own := &manifest{SourceSHA256: sharedTestSource, Operation: operationSession, Parts: []*manifest{good(), good()},
		EffectPatches: good().EffectPatches}
	if err := validateManifest(own); err == nil || !strings.Contains(err.Error(), "only through parts") {
		t.Fatalf("a session's own effect patch: %v", err)
	}
	var invalid error = ErrManifestInvalid
	if err := validateManifest(func() *manifest { m := good(); m.EffectPatches[0].RowID = 1; return m }()); !errors.Is(err, invalid) {
		t.Fatalf("a refusal is not ErrManifestInvalid: %v", err)
	}
}
