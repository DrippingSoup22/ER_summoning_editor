// BuildReset decides which rows a reset writes, and "only the ones that
// differ" is a safety property rather than an optimisation: the manifest is the
// declared mutation range savecore fences every changed byte against. These
// tests hold that decision directly, with three rows instead of the game's 924,
// so a failure names the rule that broke rather than a row count in a save.
package plan

import (
	"testing"
)

// shippedFor is a small stand-in for the embedded snapshot: three goods rows,
// three formation rows and two stone rows instead of the game's 924, 170 and
// 304, so a failure names the rule that broke rather than a row count.
func shippedFor() Vanilla {
	return Vanilla{
		Goods: map[int32]map[string]int64{
			232000: shippedGoodsRow(232000, 88),
			232001: shippedGoodsRow(232001, 88),
			240000: shippedGoodsRow(240000, 55),
		},
		Buddy: map[int32]map[string]float64{
			20100: shippedBuddyRow(140700000, 1),
			20200: shippedBuddyRow(140700000, 1),
			20300: shippedBuddyRow(143111300, 0),
		},
		Stone: map[int32]map[string]int64{7000: shippedStoneRow(), 7001: shippedStoneRow()},
	}
}

// shippedGoodsRow is one goods row's shipped values: which formation it
// delivers, and its FP price. Every family the game ships spends one resource
// or neither, so the HP half is the unset spelling here.
func shippedGoodsRow(trigger, fp int64) map[string]int64 {
	return map[string]int64{"refId_default": trigger, "consumeMP": fp, "consumeHP": -1}
}

func shippedBuddyRow(actor int64, onMonument int64) map[string]float64 {
	row := map[string]float64{
		"triggerSpEffectId":    232000,
		"npcParamId":           float64(actor),
		"npcThinkParamId":      float64(actor),
		"appearOnAroundSekihi": float64(onMonument),
		// A creature that follows, arriving on the ordinary spirit animation:
		// what 140 of the 170 shipped rows say, and what a reset has to be able
		// to put back now that authoring writes all four.
		"disablePCTargetShare": 0,
		"pcFollowType":         0,
		"npcPlayerInitParamId": -1,
		"generateAnimId":       1830,
		// Where the actor stands. Authoring writes it, so a reset puts it back.
		"x_offset": 0,
		"z_offset": 0,
		"y_angle":  0,
	}
	// The reinforcement ladder as every shipped formation carries it, which a
	// reset restores along with the rest of the row.
	row["dopingSpEffect_lv0"] = float64(290000 + 0)
	row["dopingSpEffect_lv1"] = float64(290000 + 1)
	row["dopingSpEffect_lv2"] = float64(290000 + 2)
	row["dopingSpEffect_lv3"] = float64(290000 + 3)
	row["dopingSpEffect_lv4"] = float64(290000 + 4)
	row["dopingSpEffect_lv5"] = float64(290000 + 5)
	row["dopingSpEffect_lv6"] = float64(290000 + 6)
	row["dopingSpEffect_lv7"] = float64(290000 + 7)
	row["dopingSpEffect_lv8"] = float64(290000 + 8)
	row["dopingSpEffect_lv9"] = float64(290000 + 9)
	row["dopingSpEffect_lv10"] = float64(290000 + 10)
	return row
}

func shippedStoneRow() map[string]int64 {
	return map[string]int64{
		"activateRange":                   10,
		"overwriteReturnRange":            20,
		"overwriteActivateRegionEntityId": 30,
		"warnRegionEntityId":              40,
		"summonedEventFlagId":             50,
		"eliminateTargetEntityId":         60,
	}
}

const testSHA = "0000000000000000000000000000000000000000000000000000000000000000"

func mustBuildReset(t *testing.T, current Vanilla) *Manifest {
	t.Helper()
	manifest, err := BuildReset(testSHA, shippedFor(), current)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

// The rule the audit was about: a row already holding its shipped value is not
// written, and therefore not declared as writable either.
func TestResetDeclaresOnlyTheRowsThatDiffer(t *testing.T) {
	current := shippedFor()
	current.Goods[232001] = shippedGoodsRow(999999, 88) // the one edited row

	manifest := mustBuildReset(t, current)

	if len(manifest.GoodsPatches) != 1 {
		t.Fatalf("reset declares %d goods rows, want only the one that differs", len(manifest.GoodsPatches))
	}
	patch := manifest.GoodsPatches[0]
	if patch.RowID != 232001 {
		t.Errorf("reset declares row %d, want 232001", patch.RowID)
	}
	// All three fields go back, not only the one that differed: a row restored
	// halfway holds a combination the game never shipped.
	for name, want := range map[string]int64{"refId_default": 232001, "consumeMP": 88, "consumeHP": -1} {
		if patch.Set[name] != want {
			t.Errorf("reset writes %s = %v, want the shipped %d", name, patch.Set[name], want)
		}
	}
	if len(manifest.BuddyPatches) != 0 || len(manifest.BuddyStoneRowPatches) != 0 {
		t.Errorf("reset touched other tables: buddy=%d stone=%d",
			len(manifest.BuddyPatches), len(manifest.BuddyStoneRowPatches))
	}
	if manifest.Operation != OperationReset {
		t.Errorf("operation = %q, want %q", manifest.Operation, OperationReset)
	}
}

// The reason the record became per row and per field. An authored formation
// re-points a row at a different actor and can retire it from the formation
// outright; a reset that only knew the monument flag would put the flag back,
// report success, and leave the formation in place.
func TestResetRestoresAnAuthoredFormationRow(t *testing.T) {
	for name, edit := range map[string]map[string]float64{
		"re-pointed at another actor": {"npcParamId": 145000000, "npcThinkParamId": 145000000},
		"retired from the formation":  {"triggerSpEffectId": -1},
	} {
		t.Run(name, func(t *testing.T) {
			current := shippedFor()
			for field, value := range edit {
				current.Buddy[20100][field] = value
			}

			manifest := mustBuildReset(t, current)

			if len(manifest.BuddyPatches) != 1 || manifest.BuddyPatches[0].RowID != 20100 {
				t.Fatalf("reset declares %+v, want row 20100 restored", manifest.BuddyPatches)
			}
			// The whole row comes back, not only the field that differed: one
			// changed field is reason enough to restore all four.
			patch := manifest.BuddyPatches[0]
			if len(patch.Set) != len(RestorableBuddyFields) {
				t.Fatalf("reset writes %d fields, want all %d", len(patch.Set), len(RestorableBuddyFields))
			}
			for field, want := range shippedFor().Buddy[20100] {
				if patch.Set[field] != want {
					t.Errorf("%s = %v, want the shipped %g", field, patch.Set[field], want)
				}
			}
		})
	}
}

// The stone table is restored per row, because all 304 shipped rows are
// distinct even though the recipe that overwrites them is uniform.
func TestResetRestoresAStoneRowFieldByField(t *testing.T) {
	current := shippedFor()
	current.Stone[7001] = map[string]int64{
		"activateRange": 65535, "overwriteReturnRange": -1,
		"overwriteActivateRegionEntityId": 0, "warnRegionEntityId": 0,
		"summonedEventFlagId": 0, "eliminateTargetEntityId": 0,
	}

	manifest := mustBuildReset(t, current)

	if len(manifest.BuddyStoneRowPatches) != 1 {
		t.Fatalf("reset declares %d stone rows, want only the one carrying the recipe",
			len(manifest.BuddyStoneRowPatches))
	}
	patch := manifest.BuddyStoneRowPatches[0]
	if patch.RowID != 7001 {
		t.Fatalf("reset declares stone row %d, want 7001", patch.RowID)
	}
	for name, want := range shippedStoneRow() {
		if patch.Set[name] != want {
			t.Errorf("%s = %v, want the shipped %d", name, patch.Set[name], want)
		}
	}
}

// A snapshot that disagrees with the table it describes is a bug in the
// extraction, not a value to write.
func TestResetRejectsASnapshotItCannotTrust(t *testing.T) {
	t.Run("non-positive shipped trigger", func(t *testing.T) {
		shipped := shippedFor()
		shipped.Goods[232000] = shippedGoodsRow(0, 88)
		if _, err := BuildReset(testSHA, shipped, Vanilla{}); err == nil {
			t.Fatal("a zero shipped trigger was accepted")
		}
	})
	t.Run("stone field the recipe never writes", func(t *testing.T) {
		shipped := shippedFor()
		row := shippedStoneRow()
		delete(row, "warnRegionEntityId")
		row["enable_multi"] = 1
		shipped.Stone[7000] = row
		if _, err := BuildReset(testSHA, shipped, Vanilla{}); err == nil {
			t.Fatal("a snapshot naming an unwritten field was accepted")
		}
	})
	t.Run("stone row missing a field", func(t *testing.T) {
		shipped := shippedFor()
		row := shippedStoneRow()
		delete(row, "warnRegionEntityId")
		shipped.Stone[7000] = row
		if _, err := BuildReset(testSHA, shipped, Vanilla{}); err == nil {
			t.Fatal("a short stone row was accepted")
		}
	})
}
