package application_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

const pricedHost = "Lone Wolf Ashes"

// commitTo writes everything staged to a fresh file and returns its path.
func commitTo(t *testing.T, session *application.Session) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "committed.dat")
	if _, err := session.Commit(context.Background(), mustPrepare(t, session), output, ""); err != nil {
		t.Fatal(err)
	}
	return output
}

func readSave(t *testing.T, path string) []byte {
	t.Helper()
	save, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return save
}

func mustStageCost(t *testing.T, session *application.Session, host string, price plan.Price) *application.StagedCost {
	t.Helper()
	staged, err := session.StageCost(host, price)
	if err != nil {
		t.Fatal(err)
	}
	return staged
}

// A reset and a price are not in conflict, and the resolution has to be
// per field: the reset still restores the trigger of the rows whose price the
// player has just set.
func TestAPriceStagedBesideAResetKeepsTheFieldsEachNamed(t *testing.T) {
	t.Parallel()
	converted := commitTo(t, func() *application.Session {
		session := loadedSession(t, writeVanilla(t))
		if _, err := session.StageConversion(pricedHost, "Wandering Noble Ashes"); err != nil {
			t.Fatal(err)
		}
		return session
	}())

	session := loadedSession(t, converted)
	if _, err := session.StageReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	mustStageCost(t, session, pricedHost, plan.Price{FP: 3})

	output := commitTo(t, session)
	reader, err := savefixture.Open(readSave(t, output))
	if err != nil {
		t.Fatal(err)
	}
	for level := int32(0); level < savefixture.LevelRows; level++ {
		rowID := int32(232000) + level
		// The reset won the trigger: the conversion is undone.
		trigger, _, err := reader.GoodsTrigger(rowID)
		if err != nil {
			t.Fatal(err)
		}
		if want := savefixture.GoodsFamilies[232000][level]; trigger != want {
			t.Errorf("row %d delivers %d, want the shipped %d the reset restores", rowID, trigger, want)
		}
		// The price won its own two fields: the reset yielded them.
		fp, _, err := reader.GoodsPrice(rowID)
		if err != nil {
			t.Fatal(err)
		}
		if fp != 3 {
			t.Errorf("row %d costs %d FP, want the staged 3 the reset should have yielded to", rowID, fp)
		}
	}
}
