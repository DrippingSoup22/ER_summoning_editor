package catalog

// The shared effect writes: fields of SHIPPED effect rows an entry needs, and
// the warning a player is shown for it.
// The world's own fight reads the same
// rows, so the application writes them only while an Ash holds the entry.

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// SharedEffectNeed is one field one entry needs moved.
type SharedEffectNeed struct {
	// Npc is the world row the entry ships; any clone of it is the entry.
	Npc     int32
	Row     int32
	Field   string
	Shipped float64
	Value   float64
	Warning string
}

// LoadSharedEffectNeeds parses the embedded table.
func LoadSharedEffectNeeds() ([]SharedEffectNeed, error) {
	return DecodeSharedEffectNeeds(reference.SharedEffectWriteCatalog())
}

// DecodeSharedEffectNeeds parses the table from any reader.
func DecodeSharedEffectNeeds(reader io.Reader) ([]SharedEffectNeed, error) {
	records, err := csv.NewReader(reader).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read the shared effect writes: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("the shared effect writes have no header")
	}
	column := map[string]int{}
	for index, name := range records[0] {
		column[name] = index
	}
	for _, name := range []string{"npc_id", "row", "field", "shipped", "value", "warning"} {
		if _, present := column[name]; !present {
			return nil, fmt.Errorf("the shared effect writes are missing the %s column", name)
		}
	}
	needs := make([]SharedEffectNeed, 0, len(records)-1)
	for line, record := range records[1:] {
		npc, err1 := strconv.ParseInt(record[column["npc_id"]], 10, 32)
		row, err2 := strconv.ParseInt(record[column["row"]], 10, 32)
		shipped, err3 := strconv.ParseFloat(record[column["shipped"]], 64)
		value, err4 := strconv.ParseFloat(record[column["value"]], 64)
		for _, err := range []error{err1, err2, err3, err4} {
			if err != nil {
				return nil, fmt.Errorf("shared effect write %d: %w", line+1, err)
			}
		}
		if record[column["field"]] == "" || record[column["warning"]] == "" {
			return nil, fmt.Errorf("shared effect write %d names no field or no warning", line+1)
		}
		needs = append(needs, SharedEffectNeed{Npc: int32(npc), Row: int32(row), Field: record[column["field"]],
			Shipped: shipped, Value: value, Warning: record[column["warning"]]})
	}
	return needs, nil
}
