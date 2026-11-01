package application

// The items a built character can carry (internal/items), for the character
// builder's catalog.

import (
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/items"
)

// BuildItem is one item the builder offers: its table and row, its name, and
// the category and subcategory the catalog groups it by. The icon is named
// by Key.
type BuildItem = items.Item

// The tables a BuildItem is in.
const (
	ItemWeapon    = items.Weapon
	ItemProtector = items.Protector
	ItemAccessory = items.Accessory
	ItemMagic     = items.Magic
	ItemGoods     = items.Goods
	ItemGem       = items.Gem
)

// BuildItems is every item the builder offers.
func BuildItems() ([]BuildItem, error) { return items.All() }

// itemName is an item's name, and whether the builder offers it.
func itemName(table string, id int32) (string, bool) {
	all, err := items.All()
	if err != nil {
		return "", false
	}
	for _, item := range all {
		if item.Table == table && item.ID == id {
			return strings.TrimSpace(item.Name), true
		}
	}
	return "", false
}
