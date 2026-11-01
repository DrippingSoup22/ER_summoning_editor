package reference

import (
	"testing"
)

func TestEmbeddedVanillaSnapshotDescribesAWholeReset(t *testing.T) {
	snapshot, err := Vanilla()
	if err != nil {
		t.Fatal(err)
	}
	// 84 families of eleven upgrade rows. A reset that restored fewer would
	// leave some level of some Ash still converted.
	if len(snapshot.Goods) != 84*11 {
		t.Errorf("goods rows = %d, want %d", len(snapshot.Goods), 84*11)
	}
	// All 170 BuddyParam rows, not only the 20 that carry the monument flag:
	// an authored formation writes three more fields, on rows a flag list would
	// never mention.
	if len(snapshot.Buddy) != 170 {
		t.Errorf("buddy rows = %d, want 170", len(snapshot.Buddy))
	}
	if len(snapshot.MonumentOriginRows()) != 20 {
		t.Errorf("monument-origin rows = %d, want 20", len(snapshot.MonumentOriginRows()))
	}
	// 115 is the count research measured for the clone namespace, and it is
	// only recoverable with the upper bound: NpcParam holds 766 rows at or
	// above the offset, and 651 of them are in the unrelated 2xxxxxxxx and
	// higher bands.
	if len(snapshot.NpcCloneRows) != 115 {
		t.Errorf("shipped NpcParam clone rows = %d, want the measured 115", len(snapshot.NpcCloneRows))
	}
	if len(snapshot.NpcThinkCloneRows) == 0 {
		t.Error("no shipped NpcThinkParam clone rows")
	}
	if len(snapshot.Stone) != 304 {
		t.Errorf("stone rows = %d, want 304", len(snapshot.Stone))
	}
	for rowID := range snapshot.Goods {
		values, ok := snapshot.GoodsRow(rowID)
		if !ok {
			t.Fatalf("goods row %d is listed but unreadable", rowID)
		}
		if values["refId_default"] <= 0 {
			t.Fatalf("goods row %d has a non-positive shipped trigger %d", rowID, values["refId_default"])
		}
		// No shipped Ash spends both resources, and the free spelling is
		// consumeMP 0 with consumeHP -1. A row outside that shape would mean
		// the extraction read the wrong offsets.
		if values["consumeMP"] > 0 && values["consumeHP"] > 0 {
			t.Errorf("goods row %d spends %d FP and %d HP; no shipped family spends both",
				rowID, values["consumeMP"], values["consumeHP"])
		}
	}
}

func TestVanillaRejectsAnIncompleteSnapshot(t *testing.T) {
	// A whole snapshot, so each case below removes exactly one thing and the
	// name of the case is the reason it is refused.
	whole := func() VanillaSnapshot {
		return VanillaSnapshot{
			RegulationVersion: 1, SourceSHA256: hex64,
			GoodsFields: []string{"refId_default"}, Goods: oneGoods(),
			BuddyFields: []string{"appearOnAroundSekihi"}, Buddy: map[int32][]float64{0: {1}},
			StoneFields: []string{"a"}, Stone: map[int32][]int64{0: {1}},
			NpcCloneRows: []int32{100000000}, NpcThinkCloneRows: []int32{100000000},
			NpcFields: []string{"hp"}, Npc: map[int32][]int64{100000000: {500}},
		}
	}
	for name, remove := range map[string]func(*VanillaSnapshot){
		"no version":          func(s *VanillaSnapshot) { s.RegulationVersion = 0 },
		"no source hash":      func(s *VanillaSnapshot) { s.SourceSHA256 = "" },
		"no goods":            func(s *VanillaSnapshot) { s.Goods = nil },
		"no goods fields":     func(s *VanillaSnapshot) { s.GoodsFields = nil },
		"a short goods row":   func(s *VanillaSnapshot) { s.Goods[232000] = nil },
		"no buddy fields":     func(s *VanillaSnapshot) { s.BuddyFields = nil },
		"no buddy rows":       func(s *VanillaSnapshot) { s.Buddy = nil },
		"ragged buddy":        func(s *VanillaSnapshot) { s.BuddyFields = []string{"a", "b"} },
		"no stone":            func(s *VanillaSnapshot) { s.Stone = nil },
		"ragged stone":        func(s *VanillaSnapshot) { s.StoneFields = []string{"a", "b"} },
		"no stone fields":     func(s *VanillaSnapshot) { s.StoneFields = nil },
		"no npc clones":       func(s *VanillaSnapshot) { s.NpcCloneRows = nil },
		"no npc think clones": func(s *VanillaSnapshot) { s.NpcThinkCloneRows = nil },
		"no npc fields":       func(s *VanillaSnapshot) { s.NpcFields = nil },
		// A shipped summon row listed and not recorded is a row the strength
		// switch could unlock and never put back.
		"an unrecorded summon row": func(s *VanillaSnapshot) { s.Npc = nil },
		"a ragged summon row":      func(s *VanillaSnapshot) { s.NpcFields = []string{"hp", "resident_0"} },
		"a summon row nothing lists": func(s *VanillaSnapshot) {
			s.Npc[100000010] = []int64{500}
		},
	} {
		snapshot := whole()
		remove(&snapshot)
		if err := snapshot.validate(); err == nil {
			t.Errorf("%s: validate accepted a snapshot that cannot describe a whole reset", name)
		}
	}
	complete := whole()
	if err := complete.validate(); err != nil {
		t.Fatalf("the whole snapshot every case is derived from is itself invalid: %v", err)
	}
}

const hex64 = "0000000000000000000000000000000000000000000000000000000000000000"

func oneGoods() map[int32][]int64 { return map[int32][]int64{232000: {232000}} }
