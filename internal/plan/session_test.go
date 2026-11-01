package plan_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
)

const composeSHA = "116e8ecd89a4671df2264807b47d58dfe23dc13613081732c1f671cb39458ec0"

func conversionPart(t *testing.T) *plan.Manifest {
	t.Helper()
	triggers := make([]int32, 11)
	for level := range triggers {
		triggers[level] = 20201000
	}
	built, err := plan.Build(composeSHA, []plan.Conversion{{
		HostGoodsFamily: 232000, HostName: "Host", UpgradeLevels: 11,
		LevelTriggers: triggers, DestinationName: "Destination",
	}}, plan.Recipe{MonumentOriginRows: []int32{30400000}, Area: plan.Anywhere})
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func authorPart(rowID int32) *plan.Manifest {
	return &plan.Manifest{
		SourceSHA256: composeSHA,
		Operation:    plan.OperationAuthor,
		BuddyPatches: []plan.RowPatch{{
			RowID: rowID, Name: "Authored: formation row",
			Set: map[string]any{
				"triggerSpEffectId": int64(215000), "npcParamId": int64(136610500),
				"npcThinkParamId": int64(136610500), "appearOnAroundSekihi": int64(0),
			},
		}},
	}
}

// Several operations become one manifest that describes nothing itself.
func TestASessionCarriesItsOperationsAndNothingOfItsOwn(t *testing.T) {
	composed, err := plan.Compose(composeSHA, []*plan.Manifest{
		conversionPart(t), authorPart(21500000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if composed.Operation != plan.OperationSession {
		t.Fatalf("operation = %q, want %q", composed.Operation, plan.OperationSession)
	}
	if len(composed.Parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(composed.Parts))
	}
	if len(composed.GoodsPatches)+len(composed.BuddyPatches)+len(composed.BuddyStonePatches) != 0 {
		t.Fatal("the session carries edits of its own")
	}
	// Inside a session there is no single operation for the file to imply, so
	// the conversion has to start saying which one it is.
	if composed.Parts[0].Operation != plan.OperationConvert {
		t.Fatalf("first part operation = %q, want %q", composed.Parts[0].Operation, plan.OperationConvert)
	}
	for index, part := range composed.Parts {
		if part.SourceSHA256 != composeSHA {
			t.Fatalf("part %d was planned against %q", index+1, part.SourceSHA256)
		}
	}

	var encoded bytes.Buffer
	if err := composed.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["goods_patches"]; present {
		t.Fatal("the encoded session names goods patches of its own")
	}
	if parts, ok := decoded["parts"].([]any); !ok || len(parts) != 2 {
		t.Fatalf("encoded parts = %v", decoded["parts"])
	}
}
