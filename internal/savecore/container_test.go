// Malformed input is the ordinary case, not the exotic one: a user picks the
// wrong file, an unfinished download, or a save from a future patch. Every one
// of these must produce an error, and none may panic — a frontend has no way
// to recover from a panic in the layer that opens the file.
package savecore_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// The encrypted regulation begins after UserData11's 0x10 header and its
// 16-byte initialisation vector.
const ciphertextStart = savefixture.UserData11Offset + 0x10 + 16

func TestInspectRefusesMalformedContainers(t *testing.T) {
	valid := fixtureBytes(t)
	for _, testCase := range []struct {
		name    string
		corrupt func([]byte) []byte
		want    string
	}{
		{
			name:    "truncated before the regulation",
			corrupt: func(save []byte) []byte { return save[:1024] },
			want:    "too small",
		},
		{
			name: "UserData11 still encrypted by the console",
			corrupt: func(save []byte) []byte {
				copy(save[savefixture.UserData11Offset:], "XXXX")
				return save
			},
			want: "not decrypted",
		},
		{
			name:    "ciphertext not block aligned",
			corrupt: func(save []byte) []byte { return save[:len(save)-1] },
			want:    "AES-block-aligned",
		},
		{
			name: "regulation encrypted with a different key",
			corrupt: func(save []byte) []byte {
				// Flipping a byte of the first block ruins the DCX magic the
				// decrypt is supposed to reveal.
				save[ciphertextStart] ^= 0xFF
				return save
			},
			want: "not DCX",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			corrupted := testCase.corrupt(bytes.Clone(valid))
			_, err := savecore.Inspect(context.Background(), writeSave(t, corrupted))
			if err == nil {
				t.Fatal("a malformed save was accepted")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to mention %q", err, testCase.want)
			}
		})
	}
}

// A manifest is user-editable input, and the file it produces is a save. Every
// rejection below is the difference between refusing a hand-edited plan and
// writing whatever it asked for.
func TestManifestValidationRejectsEveryMalformedPlan(t *testing.T) {
	const sha = "116e8ecd89a4671df2264807b47d58dfe23dc13613081732c1f671cb39458ec0"
	goods := `"goods_patches":[{"row_id":232000,"set":{"refId_default":20201000}}]`
	buddy := `"buddy_patches":[{"row_id":21500000,"set":{"appearOnAroundSekihi":0}}]`
	stone := `"buddy_stone_patches":[{"set":{"eliminateTargetEntityId":0,"summonedEventFlagId":0,` +
		`"activateRange":65535,"overwriteReturnRange":-1,"overwriteActivateRegionEntityId":0,"warnRegionEntityId":0}}]`

	for _, testCase := range []struct{ name, body, want string }{
		{
			"digest is not hex",
			`{"source_sha256":"` + strings.Repeat("z", 64) + `",` + goods + `,` + buddy + `,` + stone + `}`,
			"source_sha256",
		},
		{"digest is too short", `{"source_sha256":"abcd",` + goods + `,` + buddy + `,` + stone + `}`, "source_sha256"},
		{"no goods patch", `{"source_sha256":"` + sha + `",` + buddy + `,` + stone + `}`, "at least one goods patch"},
		{"no monument fix", `{"source_sha256":"` + sha + `",` + goods + `,` + stone + `}`, "monument-origin"},
		{"no stone recipe", `{"source_sha256":"` + sha + `",` + goods + `,` + buddy + `}`, "at most one all-row BuddyStone patch and shipped rows"},
		{
			"duplicate goods row",
			`{"source_sha256":"` + sha + `","goods_patches":[` +
				`{"row_id":232000,"set":{"refId_default":20201000}},` +
				`{"row_id":232000,"set":{"refId_default":20201000}}],` + buddy + `,` + stone + `}`,
			"duplicate row ID",
		},
		{
			"multiplayer field supplied",
			`{"source_sha256":"` + sha + `","goods_patches":[{"row_id":232000,"set":{"refId_default":20201000,"enable_multi":1}}],` + buddy + `,` + stone + `}`,
			"must set exactly refId_default",
		},
		{
			"multiplayer clearing supplied",
			`{"source_sha256":"` + sha + `","goods_patches":[{"row_id":232000,"set":{"refId_default":20201000,"enable_multi":0}}],` + buddy + `,` + stone + `}`,
			"must set exactly refId_default",
		},
		{
			"trigger is not a whole number",
			`{"source_sha256":"` + sha + `","goods_patches":[{"row_id":232000,"set":{"refId_default":20201000.5}}],` + buddy + `,` + stone + `}`,
			"refId_default",
		},
		{
			"buddy patch reaches beyond its one field",
			`{"source_sha256":"` + sha + `",` + goods + `,"buddy_patches":[{"row_id":21500000,"set":{"appearOnAroundSekihi":0,"npcParamId":1}}],` + stone + `}`,
			"appearOnAroundSekihi to 0 or 1",
		},
		{
			"stone recipe is missing a field",
			`{"source_sha256":"` + sha + `",` + goods + `,` + buddy + `,"buddy_stone_patches":[{"set":{"activateRange":65535}}]}`,
			"a whole half of the monument switch",
		},
		{
			"stone recipe changes a required value",
			`{"source_sha256":"` + sha + `",` + goods + `,` + buddy + `,"buddy_stone_patches":[{"set":{` +
				`"eliminateTargetEntityId":0,"summonedEventFlagId":0,"activateRange":100,` +
				`"overwriteReturnRange":-1,"overwriteActivateRegionEntityId":0,"warnRegionEntityId":0}}]}`,
			"activateRange is not one of the six at its recipe value",
		},
		{
			"a second manifest is appended",
			`{"source_sha256":"` + sha + `",` + goods + `,` + buddy + `,` + stone + `} {"source_sha256":"` + sha + `"}`,
			"unexpected data after manifest",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := savecore.Apply(context.Background(),
				filepath.Join(t.TempDir(), "absent.dat"),
				filepath.Join(t.TempDir(), "out.dat"),
				strings.NewReader(testCase.body))
			if err == nil {
				t.Fatal("a malformed manifest was accepted")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to mention %q", err, testCase.want)
			}
		})
	}
}
