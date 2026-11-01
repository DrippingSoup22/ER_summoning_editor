package catalog_test

import (
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

func loadFamilies(t *testing.T) []catalog.Family {
	t.Helper()
	families, err := catalog.LoadFamilies()
	if err != nil {
		t.Fatalf("load the embedded entity catalog: %v", err)
	}
	return families
}

func TestEveryFamilyIsBrowsable(t *testing.T) {
	seen := make(map[int32]string)
	models := make(map[string]bool)
	for _, family := range loadFamilies(t) {
		if models[family.Model] {
			t.Errorf("%s is listed twice", family.Model)
		}
		models[family.Model] = true
		// A blank tile is not something a player can choose, and 24 families
		// carry no Paramdex name at all.
		if family.Name == "" {
			t.Errorf("%s has no name to show", family.Model)
		}
		if len(family.Variants) == 0 {
			t.Errorf("%s opens onto nothing", family.Model)
		}
		if _, known := map[catalog.Compatibility]bool{catalog.Untested: true, catalog.Ships: true, catalog.Warns: true, catalog.Refused: true}[family.Compatibility]; !known {
			t.Errorf("%s ships as %q, which is not a verdict", family.Model, family.Compatibility)
		}
		previous := int32(-1)
		for _, variant := range family.Variants {
			if variant.Npc <= previous {
				t.Errorf("%s lists %d after %d", family.Model, variant.Npc, previous)
			}
			previous = variant.Npc
			if where, twice := seen[variant.Npc]; twice {
				t.Errorf("row %d is in both %s and %s", variant.Npc, where, family.Model)
			}
			seen[variant.Npc] = family.Model
			if variant.Name == "" {
				t.Errorf("%s row %d has no name to show", family.Model, variant.Npc)
			}
			if variant.Npc >= formation.CloneOffset {
				t.Errorf("%s lists %d, which is a summon row and not a world row", family.Model, variant.Npc)
			}
		}
	}
}

func TestDecodeRefusesACatalogItCannotTrust(t *testing.T) {
	const family = "model,name,external_name,provenance,hit_radius,hit_height,rig,compatibility,artwork\n" +
		"c3661,Putrid Corpse,Putrid Corpse,base_placed,0.3,1.8,c3660,untested,\n"
	const actor = "npc_id,think_id,name,model,hp,area,class,arena,artwork,granted,removed\n36610000,36610000,Putrid Corpse,c3661,376,,Basic,,,,\n"

	for _, test := range []struct {
		name           string
		family, actors string
		want           string
	}{
		{"a family catalog missing a column",
			"model,name\nc3661,Putrid Corpse\n", actor, "missing"},
		{"an actor catalog missing a column",
			family, "npc_id,name\n36610000,Putrid Corpse\n", "missing"},
		{"a verdict this product cannot interpret",
			strings.Replace(family, ",untested,", ",probably-fine,", 1), actor, "not a verdict"},
		{"the same model twice",
			family + "c3661,Putrid Corpse,,base_placed,0.3,1.8,c3660,untested,\n", actor, "twice"},
		{"the same row twice",
			family, actor + "36610000,36610000,Putrid Corpse,c3661,376,,Basic,,,,\n", "twice"},
		{"a row already in the summon namespace",
			family, "npc_id,think_id,name,model,hp,area,class,arena,artwork,granted,removed\n136610000,136610000,Putrid Corpse,c3661,376,,Basic,,,,\n",
			"not a world row"},
		{"an actor whose family is not listed",
			family, actor + "43110000,43110000,Godrick Soldier,c4311,192,,Basic,,,,\n", "does not list"},
		{"a family with no rows",
			family + "c4311,Godrick Soldier,,base_placed,0.4,1.9,c4310,untested,\n", actor, "no rows"},
		{"a family whose collision bounds are not numbers",
			strings.Replace(family, ",0.3,1.8,", ",wide,1.8,", 1), actor, "hit_radius"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := catalog.DecodeFamilies(strings.NewReader(test.family), strings.NewReader(test.actors))
			if err == nil {
				t.Fatal("decoded a catalog that should have been refused")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refused with %q, which does not mention %q", err, test.want)
			}
		})
	}
}
