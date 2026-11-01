package savecore_test

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// The reason a snapshot has to exist. The conversion recipe writes one uniform
// value set across every stone row, so applying it destroys 304 rows of
// distinct shipped data. If the snapshot collapsed those rows to a single
// representative — the shape the manifest's tablePatch already has — a reset
// would restore one row's values onto all of them and call it vanilla.
func TestVanillaSnapshotRecordsEveryStoneRowSeparately(t *testing.T) {
	options := savefixture.Defaults()
	for index, table := range options.Tables {
		if table.Entry != "BuddyStoneParam.param" {
			continue
		}
		options.Tables[index].Init = func(rowID int32, row []byte) {
			// Every row different, and different from every recipe target, so
			// neither a uniform reader nor a coincidence can pass.
			binary.LittleEndian.PutUint32(row[8:], uint32(1000+rowID))
			binary.LittleEndian.PutUint32(row[12:], uint32(2000+rowID))
			binary.LittleEndian.PutUint16(row[28:], uint16(40+rowID))
			binary.LittleEndian.PutUint16(row[30:], uint16(int16(-2-rowID)))
			binary.LittleEndian.PutUint32(row[32:], uint32(3000+rowID))
			binary.LittleEndian.PutUint32(row[36:], uint32(4000+rowID))
		}
	}
	save, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := savecore.ReadVanillaSnapshot(context.Background(), writeSave(t, save), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Stone) == 0 {
		t.Fatal("no stone rows were recorded")
	}
	for rowID, values := range snapshot.Stone {
		want := savecore.StoneRow{
			"eliminateTargetEntityId":         int64(1000 + rowID),
			"summonedEventFlagId":             int64(2000 + rowID),
			"activateRange":                   int64(40 + rowID),
			"overwriteReturnRange":            int64(-2 - rowID),
			"overwriteActivateRegionEntityId": int64(3000 + rowID),
			"warnRegionEntityId":              int64(4000 + rowID),
		}
		for _, name := range savecore.StoneFieldNames() {
			if values[name] != want[name] {
				t.Errorf("stone row %d %s = %d, want %d", rowID, name, values[name], want[name])
			}
		}
	}
}

// overwriteReturnRange is the one signed field and the recipe's own target for
// it is -1, so the shipped values a reset restores are exactly where a sign
// mistake hides. Reading it as unsigned records 65535, and a reset would then
// write 65535 back and call the save vanilla.
//
// Seeded negative on purpose: the default fixture uses a positive 50, which
// every decoding of a 16-bit field agrees on and so proves nothing here.
func TestVanillaSnapshotReadsTheSignedStoneFieldAsSigned(t *testing.T) {
	options := savefixture.Defaults()
	for index, table := range options.Tables {
		if table.Entry != "BuddyStoneParam.param" {
			continue
		}
		options.Tables[index].Init = func(_ int32, row []byte) {
			shipped := int16(-1)
			binary.LittleEndian.PutUint16(row[30:], uint16(shipped))
		}
	}
	save, err := savefixture.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := savecore.ReadVanillaSnapshot(context.Background(), writeSave(t, save), nil)
	if err != nil {
		t.Fatal(err)
	}
	for rowID, values := range snapshot.Stone {
		if values["overwriteReturnRange"] != -1 {
			t.Fatalf("stone row %d overwriteReturnRange = %d, want -1 rather than an unsigned 65535",
				rowID, values["overwriteReturnRange"])
		}
	}
}
