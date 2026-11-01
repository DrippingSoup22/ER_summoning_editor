package savecore_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

func costManifestFor(t *testing.T, sha string, price plan.Price) []byte {
	t.Helper()
	built, err := plan.BuildCost(sha, []plan.Cost{{
		HostGoodsFamily: loneWolfFamily, HostName: "Lone Wolf Ashes",
		UpgradeLevels: savefixture.LevelRows, Price: price,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := built.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

// The bytes actually move, read back by the fixture's own reader rather than by
// the writer that put them there.
func TestACostWritesThePriceAndNothingElse(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	output := newOutputPath(t)
	before, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	// Both halves positive, which the game reads as AND logic and which no
	// shipped family uses, so nothing here can pass by resembling vanilla.
	summary, err := savecore.Apply(context.Background(), input, output,
		bytes.NewReader(costManifestFor(t, before.SHA256, plan.Price{FP: 40, HP: 25})))
	if err != nil {
		t.Fatal(err)
	}
	if summary.GoodsRows != savefixture.LevelRows {
		t.Errorf("wrote %d goods rows, want the family's %d", summary.GoodsRows, savefixture.LevelRows)
	}

	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := savefixture.Open(written)
	if err != nil {
		t.Fatal(err)
	}
	for level := int32(0); level < savefixture.LevelRows; level++ {
		rowID := int32(loneWolfFamily) + level
		fp, hp, err := reader.GoodsPrice(rowID)
		if err != nil {
			t.Fatal(err)
		}
		if fp != 40 || hp != 25 {
			t.Errorf("row %d costs %d FP and %d HP, want 40 and 25", rowID, fp, hp)
		}
		// The formation this Ash delivers is not a price's business, and
		// neither is the multiplayer bit.
		trigger, enableMulti, err := reader.GoodsTrigger(rowID)
		if err != nil {
			t.Fatal(err)
		}
		if want := savefixture.GoodsFamilies[loneWolfFamily][level]; trigger != want {
			t.Errorf("row %d now delivers %d, want its shipped %d", rowID, trigger, want)
		}
		if !enableMulti {
			t.Errorf("row %d lost its multiplayer bit to a price change", rowID)
		}
	}
	if before.SHA256 == summary.OutputSHA256 {
		t.Error("the save did not change")
	}
}

// Each operation owns its fields, and the validator says so before the save is
// opened. These are the crossings that would let one operation do another's
// work while claiming to be itself.
func TestGoodsFieldsBelongToOneOperationEach(t *testing.T) {
	const sha = "0000000000000000000000000000000000000000000000000000000000000000"
	const buddy = `"buddy_patches":[{"row_id":21500000,"set":{"appearOnAroundSekihi":0}}]`
	const stone = `"buddy_stone_patches":[{"set":{"eliminateTargetEntityId":0,"summonedEventFlagId":0,` +
		`"activateRange":65535,"overwriteReturnRange":-1,"overwriteActivateRegionEntityId":0,"warnRegionEntityId":0}}]`

	for _, test := range []struct {
		name     string
		manifest string
		want     string
	}{
		{"a cost that re-points a formation",
			`{"source_sha256":"` + sha + `","operation":"cost","goods_patches":` +
				`[{"row_id":232000,"set":{"consumeMP":1,"consumeHP":-1,"refId_default":20201000}}]}`,
			"must set exactly"},
		{"a conversion that sets a price",
			`{"source_sha256":"` + sha + `","operation":"convert","goods_patches":` +
				`[{"row_id":232000,"set":{"refId_default":20201000,"consumeMP":1}}],` + buddy + `,` + stone + `}`,
			"must set exactly"},
		{"a cost that clears the monument flag",
			`{"source_sha256":"` + sha + `","operation":"cost","goods_patches":` +
				`[{"row_id":232000,"set":{"consumeMP":1,"consumeHP":-1}}],` + buddy + `}`,
			"nothing else"},
		{"a cost that inserts a row",
			`{"source_sha256":"` + sha + `","operation":"cost","goods_patches":` +
				`[{"row_id":232000,"set":{"consumeMP":1,"consumeHP":-1}}],"npc_clone_sources":[36610500]}`,
			"may not add or remove"},
		{"a cost with no price to set",
			`{"source_sha256":"` + sha + `","operation":"cost"}`,
			"at least one goods patch"},
		{"a price the field cannot hold",
			`{"source_sha256":"` + sha + `","operation":"cost","goods_patches":` +
				`[{"row_id":232000,"set":{"consumeMP":100000,"consumeHP":-1}}]}`,
			"outside"},
		{"a fractional price",
			`{"source_sha256":"` + sha + `","operation":"cost","goods_patches":` +
				`[{"row_id":232000,"set":{"consumeMP":1.5,"consumeHP":-1}}]}`,
			"outside"},
		{"an authored formation that sets a price",
			`{"source_sha256":"` + sha + `","operation":"author","goods_patches":` +
				`[{"row_id":232000,"set":{"consumeMP":1,"consumeHP":-1}}],` + buddy + `}`,
			"writes no goods row"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := savecore.Apply(context.Background(), writeSave(t, fixtureBytes(t)), newOutputPath(t),
				strings.NewReader(test.manifest))
			if err == nil {
				t.Fatal("a manifest that should have been refused was applied")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refused with %q, which does not mention %q", err, test.want)
			}
		})
	}
}
