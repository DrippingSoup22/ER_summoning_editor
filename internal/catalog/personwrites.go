package catalog

// The person writes: fields of SHIPPED CharaInitParam rows an armed person
// needs, carried by the hierarchy's
// chara_set column as
// field=shipped>value pairs. The application writes them only while an Ash
// holds the person, and puts the shipped values back once none does.

import (
	"fmt"
	"strconv"
	"strings"
)

// PersonWriteNeed is one field one armed person needs moved.
type PersonWriteNeed struct {
	// Init is the CharaInitParam row the entry ships, and the row written.
	Init    int32
	Field   string
	Shipped float64
	Value   float64
}

// PersonWriteNeeds reads every entry's chara_set. Only an offered person
// entry may carry one, and a row says each field once.
func PersonWriteNeeds(categories []HierarchyCategory) ([]PersonWriteNeed, error) {
	var needs []PersonWriteNeed
	for _, category := range categories {
		for _, card := range category.Cards {
			for _, kind := range card.Types {
				for _, entry := range kind.Entries {
					raw := entry.Tags["chara_set"]
					if raw == "" {
						continue
					}
					if entry.Trunk != "person" || entry.Offer == "hold" || entry.Ships <= 0 {
						return nil, fmt.Errorf("entry %s carries chara_set but is no offered person", entry.Key)
					}
					seen := map[string]bool{}
					for _, pair := range strings.Split(raw, "|") {
						field, values, ok := strings.Cut(pair, "=")
						was, now, ok2 := strings.Cut(values, ">")
						shipped, err1 := strconv.ParseFloat(was, 64)
						value, err2 := strconv.ParseFloat(now, 64)
						if !ok || !ok2 || field == "" || err1 != nil || err2 != nil {
							return nil, fmt.Errorf("entry %s: chara_set %q is not field=shipped>value", entry.Key, pair)
						}
						if seen[field] {
							return nil, fmt.Errorf("entry %s names %s twice", entry.Key, field)
						}
						seen[field] = true
						needs = append(needs, PersonWriteNeed{Init: entry.Ships, Field: field, Shipped: shipped, Value: value})
					}
				}
			}
		}
	}
	return needs, nil
}
