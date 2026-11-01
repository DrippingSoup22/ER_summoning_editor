package savecore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

func applyOwnEffectRows(t *testing.T, from, to string, insert, remove []int32) error {
	t.Helper()
	body, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(manifest{SourceSHA256: regulation.Sha256Hex(body), Operation: operationOwnEffects,
		OwnEffectInsertions: insert, OwnEffectRemovals: remove})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Apply(context.Background(), from, to, bytes.NewReader(encoded))
	return err
}

func effectRow(t *testing.T, bnd4 []byte, rowID int32) ([]byte, bool) {
	t.Helper()
	table, err := regulation.OpenTable(bnd4, effectLayout)
	if err != nil {
		t.Fatal(err)
	}
	row, present := table.ByID[rowID]
	if !present {
		return nil, false
	}
	return bytes.Clone(table.Data[row.DataOffset : row.DataOffset+effectLayout.RowSize]), true
}

func savedRegulation(t *testing.T, path string) []byte {
	t.Helper()
	loaded, err := regulation.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded.BND4
}

// sameApartFromVfx compares two effect rows on every byte but vfxId.
func sameApartFromVfx(a, b []byte) bool {
	offset := ownEffectFields["vfxId"].offset
	return bytes.Equal(a[:offset], b[:offset]) && bytes.Equal(a[offset+4:], b[offset+4:])
}

// Each row of our own is inserted as a copy of its shipped row with its own
// fields written over it, and removed again; the save written and reopened
// both times, every other row proved untouched by the structural stage.
func TestAnOwnEffectRowIsInsertedAsItsCopyAndRemoved(t *testing.T) {
	dir := t.TempDir()
	input, first, second := filepath.Join(dir, "in.dat"), filepath.Join(dir, "one.dat"), filepath.Join(dir, "two.dat")
	if err := os.WriteFile(input, fixtureSave(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := applyOwnEffectRows(t, input, first, []int32{OwnColoursPresence, QuietMarker}, nil); err != nil {
		t.Fatal(err)
	}
	written := savedRegulation(t, first)
	presence, _ := effectRow(t, written, spiritPresenceEffect)
	ownColours, held := effectRow(t, written, OwnColoursPresence)
	if !held || !sameApartFromVfx(presence, ownColours) || readS32(ownColours, ownEffectFields["vfxId"].offset) != -1 {
		t.Errorf("%d is not 295000 with no overwrite", OwnColoursPresence)
	}
	marker, _ := effectRow(t, written, spiritFollowEffect)
	quiet, held := effectRow(t, written, QuietMarker)
	if !held || !bytes.Equal(marker, quiet) {
		t.Errorf("%d is not a copy of 297000", QuietMarker)
	}

	if err := applyOwnEffectRows(t, first, second, nil, []int32{OwnColoursPresence, QuietMarker}); err != nil {
		t.Fatal(err)
	}
	removed := savedRegulation(t, second)
	for _, rowID := range []int32{OwnColoursPresence, QuietMarker} {
		if _, held := effectRow(t, removed, rowID); held {
			t.Errorf("%d is still in the save", rowID)
		}
	}
}

// The surface and the operation are the whole fence: another row, a row named
// twice, an own row on another operation and another write beside one are all
// refused before the save is opened.
func TestAnOwnEffectWriteStaysOnItsSurface(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.dat")
	if err := os.WriteFile(input, fixtureSave(t), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(input)
	source := regulation.Sha256Hex(body)
	for name, plan := range map[string]manifest{
		"another row":      {Operation: operationOwnEffects, OwnEffectInsertions: []int32{295002}},
		"a shipped row":    {Operation: operationOwnEffects, OwnEffectRemovals: []int32{spiritPresenceEffect}},
		"named twice":      {Operation: operationOwnEffects, OwnEffectInsertions: []int32{QuietMarker}, OwnEffectRemovals: []int32{QuietMarker}},
		"on a prune":       {Operation: operationPrune, NpcCloneRemovals: []int32{140000000}, OwnEffectRemovals: []int32{QuietMarker}},
		"beside a removal": {Operation: operationOwnEffects, OwnEffectInsertions: []int32{QuietMarker}, NpcCloneRemovals: []int32{140000000}},
	} {
		plan.SourceSHA256 = source
		encoded, _ := json.Marshal(plan)
		_, err := Apply(context.Background(), input, filepath.Join(dir, "out.dat"), bytes.NewReader(encoded))
		if !errors.Is(err, ErrManifestInvalid) {
			t.Errorf("%s: %v, want a refused manifest", name, err)
		}
	}
}

// A clone naming a row of our own that the save does not hold is refused: the
// summon would arrive with no spirit coat, or neither following nor fighting.
func TestAnActorNamingAMissingOwnRowIsRefused(t *testing.T) {
	strength := map[int32]npcCloneStrength{worldActor: {Source: worldActor, Presence: OwnColoursPresence, KeepBalance: true}}
	grown, err := insertNpcClones(cloneFixture(t), []int32{worldActor}, strength, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyOwnEffectsNamed(grown); err == nil {
		t.Error("a clone naming a missing presence passed")
	}
	held, err := applyOwnEffects(cloneFixture(t), &manifest{OwnEffectInsertions: []int32{OwnColoursPresence}})
	if err != nil {
		t.Fatal(err)
	}
	if grown, err = insertNpcClones(held, []int32{worldActor}, strength, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := verifyOwnEffectsNamed(grown); err != nil {
		t.Error(err)
	}
}

// On the real regulation: vfxId is where the surface writes it (295000 names
// vfx 57000 there), and both ids are free.
func TestTheOwnRowsFitTheRealRegulation(t *testing.T) {
	save := realSave(t)
	presence, _ := effectRow(t, save.BND4, spiritPresenceEffect)
	if got := readS32(presence, ownEffectFields["vfxId"].offset); got != spiritVfxRow {
		t.Fatalf("295000's vfxId reads %d, want %d", got, spiritVfxRow)
	}
	grown, err := applyOwnEffects(save.BND4, &manifest{OwnEffectInsertions: []int32{OwnColoursPresence, QuietMarker}})
	if err != nil {
		t.Fatal(err)
	}
	ownColours, _ := effectRow(t, grown, OwnColoursPresence)
	if !sameApartFromVfx(presence, ownColours) || readS32(ownColours, ownEffectFields["vfxId"].offset) != -1 {
		t.Errorf("%d is not 295000 with no overwrite", OwnColoursPresence)
	}
}
