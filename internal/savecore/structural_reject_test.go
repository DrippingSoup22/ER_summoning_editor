package savecore_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// Every rejection the structural schema owns, one case each.
//
// These run against a save path that does not exist, which is the point: all of
// them must fail while reading the manifest, before anything is opened. A rule
// that only fires once the regulation is in memory is a rule that has already
// let a bad manifest choose what to do.
func TestManifestValidationRejectsEveryStructuralMistake(t *testing.T) {
	const sha = "116e8ecd89a4671df2264807b47d58dfe23dc13613081732c1f671cb39458ec0"
	// A minimal well-formed authored formation: one shipped row filled with an
	// actor that is already summon-side, so it needs no clone.
	authored := `"buddy_patches":[{"row_id":21500000,"set":{"triggerSpEffectId":215000,` +
		`"npcParamId":140700000,"npcThinkParamId":140700000,"appearOnAroundSekihi":0}}]`
	// A minimal well-formed reset: one row restored.
	restored := `"buddy_patches":[{"row_id":21500000,"set":{"triggerSpEffectId":215000,` +
		`"npcParamId":140700000,"npcThinkParamId":140700000,"appearOnAroundSekihi":1}}]`
	goods := `"goods_patches":[{"row_id":232000,"set":{"refId_default":20201000}}]`
	convertBuddy := `"buddy_patches":[{"row_id":21500000,"set":{"appearOnAroundSekihi":0}}]`
	stone := `"buddy_stone_patches":[{"set":{"eliminateTargetEntityId":0,"summonedEventFlagId":0,` +
		`"activateRange":65535,"overwriteReturnRange":-1,"overwriteActivateRegionEntityId":0,"warnRegionEntityId":0}}]`

	author := func(extra string) string {
		return `{"source_sha256":"` + sha + `","operation":"author",` + authored + `,` + extra + `}`
	}
	reset := func(extra string) string {
		return `{"source_sha256":"` + sha + `","operation":"reset",` + restored + `,` + extra + `}`
	}
	prune := func(body string) string {
		if body == "" {
			return `{"source_sha256":"` + sha + `","operation":"prune"}`
		}
		return `{"source_sha256":"` + sha + `","operation":"prune",` + body + `}`
	}

	for _, testCase := range []struct{ name, body, want string }{
		// The operations are disjoint, and each half of that is stated.
		{
			"a conversion may not insert",
			`{"source_sha256":"` + sha + `",` + goods + `,` + convertBuddy + `,` + stone +
				`,"npc_clone_sources":[36610500]}`,
			"may not add or remove",
		},
		{
			"a conversion may not remove",
			`{"source_sha256":"` + sha + `",` + goods + `,` + convertBuddy + `,` + stone +
				`,"buddy_row_removals":[21500004]}`,
			"may not add or remove",
		},
		{"authoring may not remove", author(`"buddy_row_removals":[21500004]`), "only a reset removes"},
		{"a reset may not insert", reset(`"npc_clone_sources":[36610500]`), "may not make any"},
		{
			"authoring writes no goods row",
			`{"source_sha256":"` + sha + `","operation":"author",` + authored + `,` + goods + `}`,
			"writes no goods row",
		},
		{
			"authoring writes no stone row",
			`{"source_sha256":"` + sha + `","operation":"author",` + authored + `,` + stone + `}`,
			"never BuddyStoneParam",
		},
		{
			"an authored formation fills at least one row",
			`{"source_sha256":"` + sha + `","operation":"author","npc_clone_sources":[36610500]}`,
			"at least one BuddyParam row",
		},

		// A clone is named by its source, so the only way to ask for a row
		// outside a model's namespace is to name a source that is not a world
		// actor. Both ends of that are refused.
		{"a clone key must be in a clone band", author(`"npc_clone_sources":[436610500]`), "not a clone key"},
		{"a clone key may not be zero", author(`"npc_clone_sources":[0]`), "not a clone key"},
		{"a clone source may not repeat", author(`"npc_clone_sources":[36610500,36610500]`), "twice"},
		{"a think clone key is checked too", author(`"npc_think_clone_sources":[2036610500]`), "not a clone key"},

		// Rule 1 as a bound: a world actor is not removable under any
		// circumstances, and neither is a row outside the namespace above.
		{"a world actor may not be removed", reset(`"npc_clone_removals":[36610500]`), "not removable"},
		{"a quest-band actor may not be removed", reset(`"npc_clone_removals":[536610500]`), "not removable"},
		{"a think clone removal is bounded too", reset(`"npc_think_clone_removals":[36610500]`), "not removable"},
		{"a clone removal may not repeat", reset(`"npc_clone_removals":[136610500,136610500]`), "twice"},

		// A prune removes creature rows nothing names, and does
		// nothing else: every other list empty, and bounded like a reset.
		{"a prune removes at least one row", prune(``), "at least one creature row"},
		{"a prune writes no value", prune(`"npc_clone_removals":[136610500],` + restored), "writes no value"},
		{"a prune removes no formation row", prune(`"npc_clone_removals":[136610500],"buddy_row_removals":[21500004]`), "never a formation row"},
		{"a prune may not insert", prune(`"npc_clone_removals":[136610500],"npc_clone_sources":[36610500]`), "may not add any"},
		{"a prune may not remove a world actor", prune(`"npc_clone_removals":[36610500]`), "not removable"},
		{"a prune may not reach the game's band 5", prune(`"npc_think_clone_removals":[536610500]`), "not removable"},

		// Placement is part of an insertion, not an implementation detail.
		{
			"an insertion must say where it goes",
			author(`"buddy_row_insertions":[{"row_id":21500004,"copy_from":21500003}]`),
			"must name the row it follows",
		},
		{
			"an insertion must copy something",
			author(`"buddy_row_insertions":[{"row_id":21500004,"insert_after":21500003}]`),
			"must copy an existing row",
		},
		{
			"an insertion may not copy a row onto itself",
			author(`"buddy_row_insertions":[{"row_id":21500004,"copy_from":21500004,"insert_after":21500003}]`),
			"onto itself",
		},
		{
			"an insertion may not repeat",
			author(`"buddy_row_insertions":[{"row_id":21500004,"copy_from":21500003,"insert_after":21500003},` +
				`{"row_id":21500004,"copy_from":21500003,"insert_after":21500003}]`),
			"twice",
		},

		// One row, one intent. A manifest is the audit record of what was done,
		// and "restored and deleted" is not something a reader can act on.
		{
			"a row may not be both restored and removed",
			reset(`"buddy_row_removals":[21500000]`),
			"both restored and removed",
		},
		{"a removal may not repeat", reset(`"buddy_row_removals":[21500004,21500004]`), "twice"},

		// The decoder rejects a field it does not know, so a manifest written
		// for a future schema fails here rather than being silently half-applied.
		{
			"an unknown structural field is refused",
			author(`"npc_clone_destinations":[136610500]`),
			"parse manifest",
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
