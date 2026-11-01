// Package items is what the character builder offers a character to carry:
// every weapon, shield, catalyst, bow and arrow, the armour and talismans, and
// the spells, goods and Ashes of War the game's AI can use. the author's item pack tool writes the list
// from the merchant editor's vendored item set; regenerate, never hand-edit.
package items

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"
	"sync"
)

//go:embed items.csv
var listed []byte

// The tables an item's row is in. A row id alone names nothing: a goods row
// and a talisman may share one.
const (
	Weapon    = "weapon"
	Protector = "protector"
	Accessory = "accessory"
	Magic     = "magic"
	Goods     = "goods"
	// Gem is an Ash of War, an EquipParamGem row.
	Gem = "gem"
)

// Item is one thing the builder offers.
type Item struct {
	Table string
	ID    int32
	Name  string
	// Category and Subcategory are the merchant editor's: melee_armaments and
	// Daggers, tools and Throwing Pots, head, and so on.
	Category    string
	Subcategory string
}

// Key names the item across tables, and its icon: weapon-2000000.
func (item Item) Key() string { return item.Table + "-" + strconv.Itoa(int(item.ID)) }

var (
	once  sync.Once
	all   []Item
	ready error
)

// All is every item, in the list's order: by category, then as the merchant
// editor sorts them.
func All() ([]Item, error) {
	once.Do(func() { all, ready = parse(listed) })
	return all, ready
}

func parse(data []byte) ([]Item, error) {
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("items.csv: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("items.csv is empty")
	}
	out := make([]Item, 0, len(records)-1)
	for line, record := range records[1:] {
		if len(record) != 5 {
			return nil, fmt.Errorf("items.csv line %d has %d fields, want 5", line+2, len(record))
		}
		id, err := strconv.ParseInt(record[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("items.csv line %d: %w", line+2, err)
		}
		out = append(out, Item{Table: record[0], ID: int32(id), Name: record[2], Category: record[3], Subcategory: record[4]})
	}
	return out, nil
}
