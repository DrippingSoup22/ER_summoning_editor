package catalog

import (
	"fmt"
	"strings"
	"testing"
)

const catalogHeader = "goods_family,name,level_rows,trigger_families,all_counts\n"

func oneRow(triggers, counts string) string {
	return catalogHeader + fmt.Sprintf("232000,Test Ashes,11,%s,%s\n", triggers, counts)
}

// docs/SAFETY.md refuses a formation larger than ten because the game discards
// the rows past it. Nothing enforced that, so a catalog could offer one and the
// player would silently receive a smaller formation than they picked.
func TestCatalogRefusesAFormationLargerThanTheGameDelivers(t *testing.T) {
	_, err := DecodeAshes(strings.NewReader(oneRow("232000", "232000:11")))
	if err == nil {
		t.Fatal("a formation of 11 was accepted")
	}
	if !strings.Contains(err.Error(), "discarded") {
		t.Fatalf("err = %v, want it to explain why the size is refused", err)
	}
	// The boundary itself is allowed; the rule is "larger than", not "as large".
	if _, err := DecodeAshes(strings.NewReader(oneRow("232000", "232000:10"))); err != nil {
		t.Fatalf("a formation of exactly %d was refused: %v", MaxFormationSize, err)
	}
}

func TestResolveByNameAndGoodsFamily(t *testing.T) {
	ashes, err := LoadAshes()
	if err != nil {
		t.Fatal(err)
	}
	loneWolf, err := Resolve(ashes, "lone wolf ashes")
	if err != nil {
		t.Fatal(err)
	}
	if loneWolf.GoodsFamily != 232000 {
		t.Fatalf("Lone Wolf goods family = %d", loneWolf.GoodsFamily)
	}
	bloodfiend, err := Resolve(ashes, "2201000")
	if err != nil {
		t.Fatal(err)
	}
	if bloodfiend.Name != "Bloodfiend Hexer's Ashes" || bloodfiend.TriggerFamilies[0] != 20201000 {
		t.Fatalf("unexpected Bloodfiend record: %+v", bloodfiend)
	}
}
