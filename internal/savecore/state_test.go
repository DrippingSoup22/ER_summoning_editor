package savecore_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

func loneWolfRows() []int32 {
	rows := make([]int32, 0, savefixture.LevelRows)
	for level := int32(0); level < savefixture.LevelRows; level++ {
		rows = append(rows, loneWolfFamily+level)
	}
	return rows
}

// Reading back what the writer wrote is the whole point: it is how a frontend
// can tell an already-converted save from a fresh one without guessing.
func TestReadStateReportsAnEditedSave(t *testing.T) {
	input := writeSave(t, fixtureBytes(t))
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "converted.dat")
	if _, err := savecore.Apply(context.Background(), input, output,
		bytes.NewReader(manifestFor(t, info.SHA256, loneWolfToBloodfiend()))); err != nil {
		t.Fatal(err)
	}

	state, err := savecore.ReadState(context.Background(), output, loneWolfRows())
	if err != nil {
		t.Fatal(err)
	}
	for _, rowID := range loneWolfRows() {
		row := state.Goods[rowID]
		if row.Trigger != bloodfiendTrigger {
			t.Errorf("row %d trigger = %d, want %d", rowID, row.Trigger, bloodfiendTrigger)
		}
		if !row.EnableMulti {
			t.Errorf("row %d had its multiplayer gate changed", rowID)
		}
	}
	if len(state.MonumentOriginRows) != 0 {
		t.Fatalf("%d rows still resolve their origin to a monument", len(state.MonumentOriginRows))
	}
	if !state.ExpandedAreaApplied() {
		t.Fatalf("recipe rows = %d of %d, want every row", state.ExpandedAreaRows, state.StoneRows)
	}
}

// A partially applied recipe is a state no version of this product produces.
// Reporting it as applied would let a reset skip rows that still need fixing.
func TestReadStateDoesNotCountAPartiallyRecipedRow(t *testing.T) {
	options := savefixture.Defaults()
	for index := range options.Tables {
		if options.Tables[index].Entry != "BuddyStoneParam.param" {
			continue
		}
		options.Tables[index].Init = func(rowID int32, row []byte) {
			// Every field at its target except one.
			row[28], row[29] = 0xFF, 0xFF
			row[30], row[31] = 0xFF, 0xFF
			if rowID == 0 {
				row[28], row[29] = 0x01, 0x00
			}
		}
	}
	save, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	state, err := savecore.ReadState(context.Background(), writeSave(t, save), nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.ExpandedAreaApplied() {
		t.Fatal("a table with one incomplete row must not report the recipe as applied")
	}
	if state.ExpandedAreaRows != state.StoneRows-1 {
		t.Fatalf("recipe rows = %d of %d, want all but the one incomplete row", state.ExpandedAreaRows, state.StoneRows)
	}
}

// Every failure a frontend has to tell apart must be reachable with errors.Is,
// because message text is written for people and changes freely.
func TestFailuresCarryTheirCategory(t *testing.T) {
	valid := fixtureBytes(t)
	input := writeSave(t, valid)
	info, err := savecore.Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	good := manifestFor(t, info.SHA256, loneWolfToBloodfiend())

	corrupt := bytes.Clone(valid)
	copy(corrupt[savefixture.UserData11Offset:], "XXXX")

	taken := filepath.Join(t.TempDir(), "taken.dat")
	if err := os.WriteFile(taken, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	danglingConversion := loneWolfToBloodfiend()
	danglingConversion.LevelTriggers = everyLevel(999999998)

	unsupported := savefixture.Defaults()
	unsupported.DCXFormat = "KRAK"
	unsupportedSave, err := savefixture.Build(unsupported)
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name string
		run  func() error
		want error
	}{
		{
			"a file that is not a decrypted PS4 save",
			func() error { _, err := savecore.Inspect(context.Background(), writeSave(t, corrupt)); return err },
			savecore.ErrNotDecryptedPS4Save,
		},
		{
			"a regulation this build cannot read",
			func() error {
				_, err := savecore.Inspect(context.Background(), writeSave(t, unsupportedSave))
				return err
			},
			savecore.ErrUnsupportedRegulation,
		},
		{
			"an output path that is taken",
			func() error {
				_, err := savecore.Apply(context.Background(), input, taken, bytes.NewReader(good))
				return err
			},
			savecore.ErrOutputExists,
		},
		{
			"a manifest planned against another save",
			func() error {
				_, err := savecore.Apply(context.Background(), input, filepath.Join(t.TempDir(), "o.dat"),
					bytes.NewReader(manifestFor(t, "0000000000000000000000000000000000000000000000000000000000000000", loneWolfToBloodfiend())))
				return err
			},
			savecore.ErrSourceMismatch,
		},
		{
			"a trigger with no SpEffect row",
			func() error {
				_, err := savecore.Apply(context.Background(), input, filepath.Join(t.TempDir(), "o.dat"),
					bytes.NewReader(manifestFor(t, info.SHA256, danglingConversion)))
				return err
			},
			savecore.ErrRowNotFound,
		},
		{
			"a malformed manifest",
			func() error {
				_, err := savecore.Apply(context.Background(), input, filepath.Join(t.TempDir(), "o.dat"),
					bytes.NewReader([]byte(`{"source_sha256":"short"}`)))
				return err
			},
			savecore.ErrManifestInvalid,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.run()
			if !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want it to carry %v", err, testCase.want)
			}
		})
	}
}
