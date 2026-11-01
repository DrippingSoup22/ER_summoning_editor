// Package catalog reads the provenance-locked Spirit Ash catalog embedded in
// the product. It contains product lookup logic, not game-format logic.
package catalog

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// MaxFormationSize is the largest formation the game delivers intact.
//
// It is formation.MaxSlots and not a second copy of the number. The catalog
// enforces the ceiling on shipped data and the authoring model enforces it on
// what a player builds, but it is one rule about one game: two constants that
// happened to agree would only stay agreeing by luck.
const MaxFormationSize = formation.MaxSlots

// Ash is one selectable Spirit Ash family and the shipped formation triggers
// it may use. TriggerFamilies[0] is the base (+0) formation.
type Ash struct {
	GoodsFamily     int32
	Name            string
	UpgradeLevels   int
	TriggerFamilies []int32
	// FormationSizes is how many actors each of those triggers summons.
	FormationSizes map[int32]int
}

// LargestFormation is the biggest formation this family can deliver, across
// every level. Frontends show it; the loader checks it.
func (a Ash) LargestFormation() int {
	largest := 0
	for _, size := range a.FormationSizes {
		if size > largest {
			largest = size
		}
	}
	return largest
}

// LoadAshes parses the embedded catalog.
func LoadAshes() ([]Ash, error) { return DecodeAshes(reference.AshCatalog()) }

// DecodeAshes parses a catalog from any reader.
//
// Exported for the author's reference sync tool, which must read the catalog it has just
// written to disk rather than the one compiled into the running binary: during
// a refresh those are different files, and the goods rows it goes on to extract
// have to come from the new one.
func DecodeAshes(reader io.Reader) ([]Ash, error) {
	csvReader := csv.NewReader(reader)
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read ash catalog header: %w", err)
	}
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[name] = index
	}
	for _, required := range []string{"goods_family", "name", "level_rows", "trigger_families", "all_counts"} {
		if _, ok := columns[required]; !ok {
			return nil, fmt.Errorf("ash catalog is missing %q", required)
		}
	}

	var ashes []Ash
	for line := 2; ; line++ {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read ash catalog line %d: %w", line, err)
		}
		family, err := parseInt32(record[columns["goods_family"]])
		if err != nil {
			return nil, fmt.Errorf("ash catalog line %d goods_family: %w", line, err)
		}
		levels, err := strconv.Atoi(record[columns["level_rows"]])
		if err != nil || levels <= 0 {
			return nil, fmt.Errorf("ash catalog line %d has invalid level_rows %q", line, record[columns["level_rows"]])
		}
		var triggers []int32
		for _, text := range strings.Split(record[columns["trigger_families"]], "|") {
			trigger, err := parseInt32(text)
			if err != nil {
				return nil, fmt.Errorf("ash catalog line %d trigger_families: %w", line, err)
			}
			triggers = append(triggers, trigger)
		}
		if len(triggers) == 0 {
			return nil, fmt.Errorf("ash catalog line %d has no trigger family", line)
		}
		name := record[columns["name"]]
		sizes, err := parseFormationSizes(record[columns["all_counts"]], triggers)
		if err != nil {
			return nil, fmt.Errorf("ash catalog line %d (%s): %w", line, name, err)
		}
		ashes = append(ashes, Ash{
			GoodsFamily: family, Name: name, UpgradeLevels: levels,
			TriggerFamilies: triggers, FormationSizes: sizes,
		})
	}
	return ashes, nil
}

// parseFormationSizes reads the "trigger:count|trigger:count" column and holds
// the catalog to the one size rule docs/SAFETY.md states.
//
// The check lives at load rather than at conversion time because a catalog that
// offers a formation the game would truncate is wrong for every request, not
// just the one that happens to pick it. Refusing to load says so once, at the
// moment the bad data arrives, instead of once per user who chooses it.
func parseFormationSizes(column string, triggers []int32) (map[int32]int, error) {
	sizes := make(map[int32]int, len(triggers))
	for _, pair := range strings.Split(column, "|") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		triggerText, countText, found := strings.Cut(pair, ":")
		if !found {
			return nil, fmt.Errorf("all_counts entry %q must be trigger:count", pair)
		}
		trigger, err := parseInt32(triggerText)
		if err != nil {
			return nil, fmt.Errorf("all_counts entry %q: %w", pair, err)
		}
		count, err := strconv.Atoi(strings.TrimSpace(countText))
		if err != nil || count <= 0 {
			return nil, fmt.Errorf("all_counts entry %q has an invalid count", pair)
		}
		if count > MaxFormationSize {
			return nil, fmt.Errorf("formation %d summons %d, and anything past %d is discarded by the game",
				trigger, count, MaxFormationSize)
		}
		sizes[trigger] = count
	}
	// Every trigger a conversion can select must have a known size, or the size
	// rule would be enforced on some formations and not others.
	for _, trigger := range triggers {
		if _, ok := sizes[trigger]; !ok {
			return nil, fmt.Errorf("formation %d has no entry in all_counts", trigger)
		}
	}
	return sizes, nil
}

// Resolve finds an Ash by exact case-insensitive name or numeric goods-family
// ID. Numeric lookup deliberately uses the goods family, not a trigger ID.
func Resolve(ashes []Ash, selector string) (Ash, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Ash{}, fmt.Errorf("empty Ash selector")
	}
	if family, err := parseInt32(selector); err == nil {
		for _, ash := range ashes {
			if ash.GoodsFamily == family {
				return ash, nil
			}
		}
		return Ash{}, fmt.Errorf("no Spirit Ash has goods family %d", family)
	}
	for _, ash := range ashes {
		if strings.EqualFold(ash.Name, selector) {
			return ash, nil
		}
	}
	return Ash{}, fmt.Errorf("unknown Spirit Ash %q", selector)
}

func parseInt32(text string) (int32, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", text)
	}
	return int32(value), nil
}
