package savecore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// Two well-formed source hashes, one standing for another save. These tests
// never reach a save.
const (
	baselineSHA = "116e8ecd89a4671df2264807b47d58dfe23dc13613081732c1f671cb39458ec0"
	otherSHA    = "3b51e58a6eccadc037e76775187bf9fb40ba480fa643f228e4f08ffccecca0f6"
)

// A session manifest is not trusted because the layer above built it. These all
// fail before a save is opened, which is the same contract every other manifest
// refusal in this package has: a plan that cannot be right is rejected without
// reading 26 MB to find out.

// authoredSet is every field an authored formation row must state, as a
// manifest spells them. It is one function because it was three copies, and
// three copies of a field list is how the list stops being the same list.
func authoredSet() string {
	return `{"triggerSpEffectId":215000,"npcParamId":136610500,` +
		`"npcThinkParamId":136610500,"appearOnAroundSekihi":0,` +
		`"disablePCTargetShare":0,"pcFollowType":0,` +
		`"npcPlayerInitParamId":-1,"generateAnimId":1830,` +
		`"x_offset":0,"z_offset":1.3,"y_angle":0}`
}

func authoringPatch(rowID string) string {
	return `{"row_id":` + rowID + `,"set":` + authoredSet() + `}`
}

func authoringPart(rowID string) string {
	return authoringPartFor(baselineSHA, rowID)
}

func authoringPartFor(sha, rowID string) string {
	return `{"source_sha256":"` + sha + `","operation":"author","buddy_patches":[` +
		authoringPatch(rowID) + `]}`
}

func session(parts ...string) string {
	return `{"source_sha256":"` + baselineSHA + `","operation":"session","parts":[` +
		strings.Join(parts, ",") + `]}`
}

func TestSessionManifestRefusals(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		manifest string
		want     string
	}{
		{
			// A commit carrying one operation is written as that operation, so
			// a file claiming to be a session while holding one part is not
			// something this product produces. Accepting it would mean two
			// spellings of the same manifest.
			name:     "a session holding a single operation",
			manifest: session(authoringPart("21500000")),
			want:     "holds at least 2 operations",
		},
		{
			// The container's own lists are the ones no operation's rules were
			// ever applied to, so they have to be empty rather than ignored.
			name: "a session that also describes an edit of its own",
			manifest: `{"source_sha256":"` + baselineSHA + `","operation":"session",` +
				`"goods_patches":[{"row_id":232000,"set":{"refId_default":233000}}],` +
				`"parts":[` + authoringPart("21500000") + `,` + authoringPart("21500001") + `]}`,
			want: "also carries",
		},
		{
			name: "a part planned against another save",
			manifest: session(authoringPart("21500000"),
				authoringPartFor(otherSHA, "21500001")),
			want: "was planned against",
		},
		{
			// Outside a session an empty operation means a conversion. Inside
			// one there is no single operation for the file to imply, so every
			// part has to say what it is.
			name: "a part that does not say which operation it is",
			manifest: session(authoringPart("21500000"),
				`{"source_sha256":"`+baselineSHA+`","goods_patches":[{"row_id":232000,"set":{"refId_default":233000}}]}`),
			want: "does not say which operation",
		},
		{
			// The point of the arrangement: a part faces the validator it would
			// face alone, and a conversion without its monument-origin fix is
			// still refused inside a session.
			name: "a part that would be refused on its own",
			manifest: session(authoringPart("21500000"),
				`{"source_sha256":"`+baselineSHA+`","operation":"convert",`+
					`"goods_patches":[{"row_id":232000,"set":{"refId_default":233000}}]}`),
			want: "monument-origin",
		},
		{
			name:     "two parts writing the same row",
			manifest: session(authoringPart("21500000"), authoringPart("21500000")),
			want:     "both describe BuddyParam row 21500000",
		},
		{
			name: "a part that removes a row another part writes into",
			manifest: session(authoringPart("21500000"),
				`{"source_sha256":"`+baselineSHA+`","operation":"reset","buddy_row_removals":[21500000]}`),
			want: "writes a value into it",
		},
		{
			// Only a creature row may be replaced; a formation
			// row added by one part and removed by another is still two
			// answers to one question.
			name: "a formation row one part adds and another removes",
			manifest: session(
				`{"source_sha256":"`+baselineSHA+`","operation":"author","buddy_patches":[`+authoringPatch("21500000")+`],`+
					`"buddy_row_insertions":[{"row_id":21500009,"copy_from":21500000,"insert_after":21500003}]}`,
				`{"source_sha256":"`+baselineSHA+`","operation":"reset","buddy_row_removals":[21500009]}`),
			want: "removes it",
		},
		{
			// A creature row on its way out, or replaced by a clone written
			// whole, is not a row to write a value into.
			name: "a value written into a creature row another part removes",
			manifest: session(
				`{"source_sha256":"`+baselineSHA+`","operation":"prune","npc_clone_removals":[136610500]}`,
				`{"source_sha256":"`+baselineSHA+`","operation":"strength","npc_patches":[{"row_id":136610500,"set":{"hp":100}}]}`),
			want: "writes a value into it",
		},
		{
			name: "a session nested inside a session",
			manifest: session(authoringPart("21500000"),
				session(authoringPart("21500001"), authoringPart("21500002"))),
			want: "one level deep",
		},
		{
			name: "a part with an unknown operation",
			manifest: session(authoringPart("21500000"),
				`{"source_sha256":"`+baselineSHA+`","operation":"delete"}`),
			want: "unknown operation",
		},
		{
			// Only a session has parts. An unread field in an audit artifact is
			// a field that can say something untrue.
			name: "parts smuggled into a single-operation manifest",
			manifest: `{"source_sha256":"` + baselineSHA + `","operation":"author",` +
				`"buddy_patches":[` + authoringPatch("21500000") + `],` +
				`"parts":[` + authoringPart("21500001") + `]}`,
			want: "only a session manifest holds parts",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := savecore.Apply(context.Background(), "missing.dat", "output.dat",
				strings.NewReader(testCase.manifest))
			if err == nil {
				t.Fatal("an invalid session manifest was accepted")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to mention %q", err, testCase.want)
			}
		})
	}
}

// A well-formed session gets past the manifest checks and fails on the save
// instead, which is how the refusals above are known to be about their own
// subject rather than about the shape of the file.
func TestAWellFormedSessionReachesTheSave(t *testing.T) {
	for name, manifest := range map[string]string{
		"two authoring parts": session(authoringPart("21500000"), authoringPart("21500001")),
		// A creature row the prune removes and an authoring part inserts again
		// on another recipe is REPLACED, not a contradiction.
		"a creature row replaced": session(
			`{"source_sha256":"`+baselineSHA+`","operation":"author","buddy_patches":[`+authoringPatch("21500000")+`],`+
				`"npc_clone_sources":[36610500],"npc_think_clone_sources":[36610500]}`,
			`{"source_sha256":"`+baselineSHA+`","operation":"prune",`+
				`"npc_clone_removals":[136610500],"npc_think_clone_removals":[136610500]}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := savecore.Apply(context.Background(), "missing.dat", "output.dat", strings.NewReader(manifest))
			if err == nil {
				t.Fatal("a missing save was accepted")
			}
			if strings.Contains(err.Error(), "session operation") {
				t.Fatalf("a valid session was refused as a manifest: %v", err)
			}
		})
	}
}
