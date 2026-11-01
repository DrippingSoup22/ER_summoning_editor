package application

// The strength of the game's own summon rows.
//
// A creature the player drops is cloned and carries its role in its recipe
// (strength.go). FromSoftware's own summon rows - the creatures of its Ashes
// and the person shells - are never cloned, so a role chosen for one is the
// row's: every Ash that summons it changes with it, and on a shell every person
// and every character body built on it. That is a staged change of its own: it
// appears in the pending list, it can be dropped, and it is written at Save.
//
// It works by DIFFERENCE: only a row whose values differ from what its role
// says is written. Choosing the role a row already has writes nothing, and the
// Helper over a row an earlier Fighter left puts the shipped values back
// (reference/vanilla.json). A staged reset puts every such row back.

import (
	"fmt"
	"slices"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/strength"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// StagedStrength is the roles chosen for the game's own summon rows, as a
// pending change: how many rows it turns into Fighters and into Helpers.
type StagedStrength struct {
	Fighters, Helpers int

	rows []plan.RowValues
}

// RowFighter reports whether a summon row the save holds will summon a Fighter
// - the role the player chose for it, or a Helper under a staged reset, or what
// the file holds - and whether the player may choose: only a row the game
// ships takes a role of its own. A clone this editor wrote keeps the role it
// was written with; another role is another slot of the creature.
func (session *Session) RowFighter(row int32) (fighter, choosable bool) {
	if session.loaded == nil {
		return false, false
	}
	current, held := session.loaded.state.SummonRows[row]
	if !held {
		return false, false
	}
	if _, shipped := session.loaded.shipped[row]; !shipped {
		return unlocked(current), false
	}
	if chosen, asked := session.rowRoles[row]; asked {
		return chosen, true
	}
	if session.restoration != nil {
		return false, true
	}
	return unlocked(current), true
}

// SetRowFighter chooses the role of one of the game's own summon rows, for
// every slot that summons it, and stages what that writes.
func (session *Session) SetRowFighter(row int32, fighter bool) error {
	if session.loaded == nil {
		return ErrNoSaveLoaded
	}
	if _, choosable := session.RowFighter(row); !choosable {
		return fmt.Errorf("row %d: %w", row, ErrHeldRowStrength)
	}
	session.rowRoles[row] = fighter
	session.restageStrength()
	return nil
}

// DropStrength forgets every role chosen for the game's own rows, which is
// what removing its entry from the pending list means.
func (session *Session) DropStrength() {
	session.rowRoles = map[int32]bool{}
	session.strength = nil
}

// restageStrength recomputes the pending change from the roles chosen, or
// drops it when the save already holds every one of them.
func (session *Session) restageStrength() {
	staged := &StagedStrength{}
	rows := make([]int32, 0, len(session.rowRoles))
	for row := range session.rowRoles {
		rows = append(rows, row)
	}
	slices.Sort(rows)
	for _, row := range rows {
		fighter := session.rowRoles[row]
		current := session.loaded.state.SummonRows[row]
		changed := differing(current, shippedRowAs(session.loaded.shipped[row], current, fighter))
		if len(changed) == 0 {
			continue
		}
		staged.rows = append(staged.rows, plan.RowValues{RowID: row, Set: changed})
		if fighter {
			staged.Fighters++
		} else {
			staged.Helpers++
		}
	}
	session.strength = nil
	if len(staged.rows) > 0 {
		session.strength = staged
	}
}

// helpersBack is what a staged reset writes to the game's own summon rows:
// every one an earlier Fighter left, back to its shipped values - less the
// rows the player chose a role for, which is the more specific change.
func (session *Session) helpersBack() []plan.RowValues {
	var back []plan.RowValues
	for _, row := range sortedFamilies(session.loaded.shipped) {
		current, held := session.loaded.state.SummonRows[row]
		if _, chosen := session.rowRoles[row]; chosen || !held {
			continue
		}
		if changed := differing(current, shippedRowAs(session.loaded.shipped[row], current, false)); len(changed) > 0 {
			back = append(back, plan.RowValues{RowID: row, Set: changed})
		}
	}
	return back
}

// shippedRowAs is what one of the game's own summon rows holds for a role.
// The Helper is the shipped record, put back only on a row carrying an
// earlier Fighter's fingerprint: a save can differ from the record for reasons
// that are not its role. The Fighter is the record with the taxes taken off and
// the body's health, since a shipped summon's health is a Helper's.
func shippedRowAs(shipped, current savecore.NpcRow, fighter bool) savecore.NpcRow {
	if !fighter {
		if !unlocked(current) && !scaled(current) {
			return nil
		}
		return shipped
	}
	return untaxed(shipped, strength.Body(int32(shipped[savecore.NpcHealthField])))
}

// shippedSummonRows is the shipped record of every summon row the game ships.
func shippedSummonRows() (map[int32]savecore.NpcRow, error) {
	snapshot, err := reference.Vanilla()
	if err != nil {
		return nil, fmt.Errorf("read the embedded shipped-value snapshot: %w", err)
	}
	rows := make(map[int32]savecore.NpcRow, len(snapshot.NpcCloneRows))
	for _, row := range snapshot.NpcCloneRows {
		if values, shipped := snapshot.NpcRow(row); shipped {
			rows[row] = savecore.NpcRow(values)
		}
	}
	return rows, nil
}

// untaxed is a row with the damage taxes and any world scaling taken off and
// the creature's own health written.
func untaxed(current savecore.NpcRow, health int32) savecore.NpcRow {
	wanted := copyRow(current)
	wanted[savecore.NpcHealthField] = int64(health)
	for _, name := range savecore.NpcResidentFields() {
		if effect := int32(wanted[name]); strength.Taxed(effect) || isScaling(effect) {
			wanted[name] = 0
		}
	}
	return wanted
}

// isScaling reports whether an effect is a world-scaling one: the twenty region
// tiers and the New Game + catch-up effects, which an earlier build wrote on a
// Fighter as its New Game + cycle.
func isScaling(effect int32) bool { return effect >= 7000 && effect < 7700 }

func copyRow(row savecore.NpcRow) savecore.NpcRow {
	copied := make(savecore.NpcRow, len(row))
	for name, value := range row {
		copied[name] = value
	}
	return copied
}

// differing is the fields of wanted that current does not already hold.
func differing(current, wanted savecore.NpcRow) map[string]int64 {
	changed := map[string]int64{}
	for name, value := range wanted {
		if current[name] != value {
			changed[name] = value
		}
	}
	return changed
}

// unlocked reports whether a summon row has had the balance package taken off
// it: a Fighter.
func unlocked(row savecore.NpcRow) bool {
	for _, name := range savecore.NpcResidentFields() {
		if int32(row[name]) == strength.Taxes[0] {
			return false
		}
	}
	return true
}

// scaled reports whether a summon row carries a world-scaling effect, which an
// earlier build wrote on a Fighter as its New Game + cycle.
func scaled(row savecore.NpcRow) bool {
	for _, name := range savecore.NpcResidentFields() {
		if isScaling(int32(row[name])) {
			return true
		}
	}
	return false
}
