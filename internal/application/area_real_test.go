package application_test

import (
	"context"
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// The two monument switches move apart: each save below moves one, reopens
// reading both as written, and the last puts the baseline back byte for byte,
// so off undoes exactly what on did. The wide area wakes the sleeping
// monuments, each keyed to its own entity, and putting it back takes them out.
func TestTheMonumentSwitchesMoveApartAndPutTheBaselineBack(t *testing.T) {
	t.Parallel()
	input := realSavePath(t)
	directory := t.TempDir()
	ctx := context.Background()

	steps := []struct {
		name string
		move func(*application.Session) error
		want application.MonumentSwitches
	}{
		{"wide.dat", func(s *application.Session) error { return s.SetWideArea(true) }, application.MonumentSwitches{WideArea: true}},
		{"both.dat", func(s *application.Session) error { return s.SetNoRestLimit(true) }, application.MonumentSwitches{WideArea: true, NoRestLimit: true}},
		{"rest.dat", func(s *application.Session) error { return s.SetWideArea(false) }, application.MonumentSwitches{NoRestLimit: true}},
		{"back.dat", func(s *application.Session) error { return s.SetNoRestLimit(false) }, application.MonumentSwitches{}},
	}
	path := input
	if got := loadedSession(t, path).Monuments(); got != (application.MonumentSwitches{}) {
		t.Fatalf("the baseline reads as %+v", got)
	}
	for _, step := range steps {
		session := loadedSession(t, path)
		if err := step.move(session); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if session.PendingCount() != 1 {
			t.Fatalf("%s: %d changes staged, want the one switch", step.name, session.PendingCount())
		}
		path = filepath.Join(directory, step.name)
		if _, err := session.Commit(ctx, mustPrepare(t, session), path, ""); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		reopened := loadedSession(t, path)
		if got := reopened.Monuments(); got != step.want {
			t.Errorf("%s reads back as %+v, want %+v", step.name, got, step.want)
		}
		if reopened.PendingCount() != 0 {
			t.Errorf("%s: reopening staged %d changes", step.name, reopened.PendingCount())
		}
		woken := wokenRows(t, path)
		if !step.want.WideArea {
			if len(woken) != 0 {
				t.Errorf("%s holds %d woken monuments, want none", step.name, len(woken))
			}
			continue
		}
		sleeping := savecore.SleepingMonuments()
		if len(woken) != len(sleeping) {
			t.Fatalf("%s holds %d woken monuments, want %d", step.name, len(woken), len(sleeping))
		}
		for _, entity := range sleeping {
			row := woken[entity]
			wantFlag := uint32(entity + 2000)
			if step.want.NoRestLimit {
				wantFlag = 0
			}
			if talk, flag := binary.LittleEndian.Uint32(row[4:]), binary.LittleEndian.Uint32(row[12:]); talk != uint32(entity) || flag != wantFlag {
				t.Fatalf("%s: monument %d is keyed to %d with flag %d, want itself and %d", step.name, entity, talk, flag, wantFlag)
			}
		}
	}
	for _, entry := range differingEntries(t, input, path) {
		t.Errorf("with both switches back %s differs from the baseline", entry)
	}
}

// Wider summoning area off puts the monument flag back only on formations the
// game ships: one authored into a monument-origin block keeps the 0 authoring
// wrote, so its summons stand where they were placed.
func TestWideAreaOffLeavesAnAuthoredOriginBlockAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	directory := t.TempDir()
	session := loadedSession(t, realSavePath(t))
	soldier := application.AuthoredSlot{Npc: 43110000} // Godrick Soldier, as the session test seats it
	if _, err := session.StageFormation(authoringHost, []application.AuthoredSlot{soldier, soldier}); err != nil {
		t.Fatal(err)
	}
	if err := session.SetWideArea(true); err != nil {
		t.Fatal(err)
	}
	wide := filepath.Join(directory, "wide.dat")
	if _, err := session.Commit(ctx, mustPrepare(t, session), wide, ""); err != nil {
		t.Fatal(err)
	}
	reopened := loadedSession(t, wide)
	if err := reopened.SetWideArea(false); err != nil {
		t.Fatal(err)
	}
	narrow := filepath.Join(directory, "narrow.dat")
	if _, err := reopened.Commit(ctx, mustPrepare(t, reopened), narrow, ""); err != nil {
		t.Fatal(err)
	}
	const landSquirt = int32(244000)
	formations, err := savecore.ReadFormations(ctx, narrow, []int32{authoringTrigger, landSquirt})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range formations.Blocks[authoringTrigger].Rows {
		if row.AppearOnAroundSekihi != 0 {
			t.Errorf("authored row %d was put back on the monument", row.RowID)
		}
	}
	for _, row := range formations.Blocks[landSquirt].Rows {
		if row.AppearOnAroundSekihi != 1 {
			t.Errorf("shipped row %d was not put back on the monument", row.RowID)
		}
	}
}

// wokenRows is each sleeping monument's BuddyStoneParam row the save holds.
func wokenRows(t *testing.T, path string) map[int32][]byte {
	t.Helper()
	loaded, err := regulation.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	layout := regulation.MustLayout("BuddyStoneParam")
	table, err := regulation.OpenTable(loaded.BND4, layout)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[int32][]byte{}
	for _, entity := range savecore.SleepingMonuments() {
		if row, present := table.ByID[entity]; present {
			rows[entity] = table.Data[row.DataOffset : row.DataOffset+layout.RowSize]
		}
	}
	return rows
}
