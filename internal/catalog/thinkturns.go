package catalog

// Which think rows run a battle script that cannot turn.
//
// An eye cone is a width capped at 180 degrees, so no think row sees behind its
// actor; a creature keeps a target that slips behind it only by turning, and
// turning is its battle script's business. Research read every battle script
// for a GOAL_COMMON_Turn act (tools/gates/think_turns.py) and joined the
// answer to the world think rows. The ladder gives a creature that cannot turn
// a nose on its three middle levels, because nothing else covers its back
// (docs/DESIGN.md, the ladder).

import (
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// DecodeThinkTurns reads the turn table and returns the world think rows whose
// battle script has NO Turn action. "unread" - no script in the corpus for the
// goal - is left out: a nose is added on evidence, never on its absence.
func DecodeThinkTurns(source io.Reader) (map[int32]bool, error) {
	records, err := readTable(source, "think turn table", []string{"think_id", "battle_goal", "turns"})
	if err != nil {
		return nil, err
	}
	never := make(map[int32]bool)
	for _, record := range records {
		think, err := strconv.ParseInt(record["think_id"], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("think turn table: think_id %q: %w", record["think_id"], err)
		}
		switch record["turns"] {
		case "no":
			never[int32(think)] = true
		case "yes", "unread":
		default:
			return nil, fmt.Errorf("think turn table: row %d says turns %q, want yes, no or unread", think, record["turns"])
		}
	}
	return never, nil
}

var neverTurns = sync.OnceValues(func() (map[int32]bool, error) {
	return DecodeThinkTurns(reference.ThinkTurnCatalog())
})

// NeverTurns reports whether a think row's battle script has no Turn action.
// It takes either side of the pair: a clone thinks with its world row's script.
func NeverTurns(think int32) (bool, error) {
	never, err := neverTurns()
	if err != nil {
		return false, err
	}
	if think >= formation.CloneOffset {
		// A think clone in any band thinks with its world row's script.
		think %= formation.CloneOffset
	}
	return never[think], nil
}
