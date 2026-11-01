package savecore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

func characterData(size int, fill byte) string {
	return hex.EncodeToString(bytes.Repeat([]byte{fill}, size))
}

func applyCharacters(t *testing.T, from, to string, rows []characterRow, removals []CharacterRowKey) error {
	t.Helper()
	body, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(manifest{SourceSHA256: regulation.Sha256Hex(body), Operation: operationCharacters,
		CharacterRows: rows, CharacterRowRemovals: removals})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Apply(context.Background(), from, to, bytes.NewReader(encoded))
	return err
}

func heldCharacterRows(t *testing.T, path string) map[CharacterRowKey][]byte {
	t.Helper()
	loaded, err := regulation.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	held, err := readCharacterRows(loaded.BND4)
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// A character's rows are inserted, rewritten when the character changes, and
// removed once nothing names them - each in the character band, and every
// other row of the save proved untouched by the structural and bounded stages.
func TestACharacterIsInsertedRewrittenAndRemoved(t *testing.T) {
	dir := t.TempDir()
	input, first, second := filepath.Join(dir, "in.dat"), filepath.Join(dir, "one.dat"), filepath.Join(dir, "two.dat")
	if err := os.WriteFile(input, fixtureSave(t), 0o600); err != nil {
		t.Fatal(err)
	}
	init := CharacterRowKey{"CharaInitParam", 2990010}
	face := CharacterRowKey{"FaceParam", 2990010}
	weapon := CharacterRowKey{"EquipParamCustomWeapon", 2990011}
	if err := applyCharacters(t, input, first, []characterRow{
		{Table: init.Table, RowID: init.RowID, Insert: true, Data: characterData(320, 0xA1)},
		{Table: face.Table, RowID: face.RowID, Insert: true, Data: characterData(240, 0xB2)},
		{Table: weapon.Table, RowID: weapon.RowID, Insert: true, Data: characterData(16, 0xC3)},
	}, nil); err != nil {
		t.Fatal(err)
	}
	held := heldCharacterRows(t, first)
	if len(held) != 3 || held[init][0] != 0xA1 || held[face][239] != 0xB2 || held[weapon][15] != 0xC3 {
		t.Fatalf("after the insertion the band holds %d rows", len(held))
	}

	if err := applyCharacters(t, first, second, []characterRow{
		{Table: init.Table, RowID: init.RowID, Data: characterData(320, 0xD4)},
	}, []CharacterRowKey{weapon}); err != nil {
		t.Fatal(err)
	}
	held = heldCharacterRows(t, second)
	if _, gone := held[weapon]; gone || len(held) != 2 || held[init][0] != 0xD4 || held[face][0] != 0xB2 {
		t.Fatalf("after the rewrite the band holds %v", held)
	}
}

// The band and the operation are the whole fence: a row outside the band, in
// another table or of the wrong size, a character row on another operation,
// and another write on the character operation are all refused.
func TestACharacterWriteStaysInItsBand(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.dat")
	if err := os.WriteFile(input, fixtureSave(t), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(input)
	source := regulation.Sha256Hex(body)
	row := func(table string, id int32, size int) characterRow {
		return characterRow{Table: table, RowID: id, Insert: true, Data: characterData(size, 1)}
	}
	for name, plan := range map[string]manifest{
		"below the band":      {Operation: operationCharacters, CharacterRows: []characterRow{row("FaceParam", 2989999, 240)}},
		"above the band":      {Operation: operationCharacters, CharacterRows: []characterRow{row("FaceParam", 2990200, 240)}},
		"another table":       {Operation: operationCharacters, CharacterRows: []characterRow{row("NpcParam", 2990000, 736)}},
		"the wrong size":      {Operation: operationCharacters, CharacterRows: []characterRow{row("FaceParam", 2990000, 239)}},
		"named twice":         {Operation: operationCharacters, CharacterRows: []characterRow{row("FaceParam", 2990000, 240), row("FaceParam", 2990000, 240)}},
		"on the tint":         {Operation: operationTint, VfxPatches: []rowPatch{{RowID: 57000, Set: map[string]float64{"phantomParamOverwriteId": 0}}}, CharacterRows: []characterRow{row("FaceParam", 2990000, 240)}},
		"beside a write":      {Operation: operationCharacters, CharacterRows: []characterRow{row("FaceParam", 2990000, 240)}, NpcPatches: []rowPatch{{RowID: 100000020, Set: map[string]float64{"hp": 1}}}},
		"beside a removal":    {Operation: operationCharacters, CharacterRows: []characterRow{row("FaceParam", 2990000, 240)}, NpcCloneRemovals: []int32{140000000}},
		"a body off its band": {Operation: operationCharacters, CharacterBodies: []characterBody{{RowID: 199900200, Shell: 100000020, HP: 1}}},
		"a body of no shell":  {Operation: operationCharacters, CharacterBodies: []characterBody{{RowID: 199900000, Shell: 140700000, HP: 1}}},
		"a body as a clone":   {Operation: operationPrune, NpcCloneRemovals: []int32{199900000}},
	} {
		plan.SourceSHA256 = source
		encoded, _ := json.Marshal(plan)
		_, err := Apply(context.Background(), input, filepath.Join(dir, "out.dat"), bytes.NewReader(encoded))
		if !errors.Is(err, ErrManifestInvalid) {
			t.Errorf("%s: %v, want a refused manifest", name, err)
		}
	}
}

// A body is the person shell with the character's health and active effects,
// and copies the shell as the commit leaves it - but its role is its own: with
// a strength write taking 296000 off the shell in the same commit (resident
// slot 27), the Helper's body carries it still and the Fighter's does not.
// Played on the real shell 100000020.
func TestABodyIsItsShellWithTheCharactersHealthAndEffects(t *testing.T) {
	const shell, lamenter = int32(100000020), int32(19982)
	source := realSavePath(t)
	original, err := regulation.Load(source)
	if err != nil {
		t.Fatal(err)
	}
	shells, err := readPersonShells(original.BND4)
	if err != nil {
		t.Fatal(err)
	}
	if residentEffectsOf(shells[shell])[0] != 5400 || !holdsEffect(residentEffectsOf(shells[shell]), spiritBalanceEffect) {
		t.Fatalf("shell %d is not the shell this test was written against", shell)
	}
	body0 := characterBody{RowID: 199900020, Shell: shell, Insert: true, HP: 1900, Effects: []int32{lamenter}}
	fighter := characterBody{RowID: 199900022, Shell: shell, Insert: true, HP: 700, Effects: []int32{lamenter}, Fighter: true}
	switched := rowPatch{RowID: shell, Set: map[string]float64{npcResidentFieldName(27): -1}}
	file, _ := os.ReadFile(source)
	digest := regulation.Sha256Hex(file)
	encoded, err := json.Marshal(manifest{SourceSHA256: digest, Operation: operationSession, Parts: []*manifest{
		{SourceSHA256: digest, Operation: operationStrength, NpcPatches: []rowPatch{switched}},
		{SourceSHA256: digest, Operation: operationCharacters, CharacterBodies: []characterBody{body0, fighter}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.dat")
	if _, err := Apply(context.Background(), source, out, bytes.NewReader(encoded)); err != nil {
		t.Fatal(err)
	}
	written, err := regulation.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	held, err := readCharacterRows(written.BND4)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []characterBody{body0, fighter} {
		got := held[CharacterRowKey{characterBodyTable, body.RowID}]
		if got == nil || readS32(got, npcHPOffset) != body.HP {
			t.Fatalf("body %d is missing or does not carry the character's health", body.RowID)
		}
		effects := residentEffectsOf(got)
		if !holdsEffect(effects, lamenter) || holdsEffect(effects, spiritBalanceEffect) == body.Fighter || !holdsEffect(effects, 5400) {
			t.Errorf("body %d (Fighter %t) carries %v: want the shell's, the package only on the Helper, and %d",
				body.RowID, body.Fighter, effects, lamenter)
		}
	}
}
