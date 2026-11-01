package application

// Shared effect writes, decided at Save over the whole save.
//
// Two entries need fields of SHIPPED effect rows that the world's own fights
// read too - the combined Ancestor and Rykard below half health - and they
// ship behind a strong warning, with the write present only while an Ash holds
// one of them. So this is decided like
// the prune, from what the file will summon once it is written: a field is
// written while any formation row still names a clone of an entry that needs
// it, and put back to what the game ships once none does. A field is written
// only when the save does not already say the right thing, and a value this
// editor did not write is never "put back" over - only one of ours is.

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// checkSharedEffectNeeds holds the catalog's needs to the save core's surface:
// the same fields, the same two values each, and one value per field however
// many entries need it. A catalog asking for a write the save core would
// refuse is caught when the service starts, not at a player's Save.
func checkSharedEffectNeeds(needs []catalog.SharedEffectNeed) error {
	surface := map[savecore.SharedEffectKey]savecore.SharedEffectWrite{}
	for _, write := range savecore.SharedEffectWrites() {
		surface[savecore.SharedEffectKey{Row: write.Row, Field: write.Field}] = write
	}
	asked := map[savecore.SharedEffectKey]bool{}
	for _, need := range needs {
		key := savecore.SharedEffectKey{Row: need.Row, Field: need.Field}
		write, known := surface[key]
		if !known {
			return fmt.Errorf("the catalog needs %d.%s, which the save core may not write", need.Row, need.Field)
		}
		if need.Shipped != write.Shipped || need.Value != write.Written {
			return fmt.Errorf("the catalog moves %d.%s from %g to %g, the save core from %g to %g",
				need.Row, need.Field, need.Shipped, need.Value, write.Shipped, write.Written)
		}
		asked[key] = true
	}
	for key := range surface {
		if !asked[key] {
			return fmt.Errorf("the save core may write %d.%s, which no catalog entry needs", key.Row, key.Field)
		}
	}
	return nil
}

// checkPersonWriteNeeds holds the catalog's person needs to the save core's
// surface the same way: the same fields, the same two values each, one value
// per field however many entries need it, and nothing the catalog does not ask.
func checkPersonWriteNeeds(needs []catalog.PersonWriteNeed) error {
	surface := map[savecore.PersonKey]savecore.PersonWrite{}
	for _, write := range savecore.PersonWrites() {
		surface[savecore.PersonKey{Row: write.Row, Field: write.Field}] = write
	}
	asked := map[savecore.PersonKey]bool{}
	for _, need := range needs {
		key := savecore.PersonKey{Row: need.Init, Field: need.Field}
		write, known := surface[key]
		if !known {
			return fmt.Errorf("the catalog arms %d.%s, which the save core may not write", need.Init, need.Field)
		}
		if need.Shipped != write.Shipped || need.Value != write.Written {
			return fmt.Errorf("the catalog moves %d.%s from %g to %g, the save core from %g to %g",
				need.Init, need.Field, need.Shipped, need.Value, write.Shipped, write.Written)
		}
		asked[key] = true
	}
	for key := range surface {
		if !asked[key] {
			return fmt.Errorf("the save core may write %d.%s, which no catalog entry needs", key.Row, key.Field)
		}
	}
	return nil
}

// personsAfter is every CharaInitParam row a formation row in the written
// save will point at, read as summonedAfter reads the creatures.
func personsAfter(read *savecore.FormationState, formations []*plan.Manifest, resetting bool) map[int32]bool {
	named := map[int32]bool{}
	rewritten := map[int32]bool{}
	for _, manifest := range formations {
		for _, patch := range manifest.BuddyPatches {
			rewritten[patch.RowID] = true
			if value, present := patch.Set["npcPlayerInitParamId"]; present {
				if row, ok := asRowID(value); ok && row > 0 {
					named[row] = true
				}
			}
		}
	}
	if !resetting {
		for rowID, person := range read.Persons {
			if !rewritten[rowID] {
				named[person] = true
			}
		}
	}
	return named
}

// summonedAfter is every world row a clone in the written save will be of:
// what the formation rows name once the staged formations are written - and,
// beside a staged reset, only what they name, since the reset puts every other
// row back to the actor the game ships.
func summonedAfter(read *savecore.FormationState, formations []*plan.Manifest, resetting bool) (map[int32]bool, error) {
	snapshot, err := reference.Vanilla()
	if err != nil {
		return nil, fmt.Errorf("read the embedded shipped-value snapshot: %w", err)
	}
	named := map[int32]bool{}
	rewritten := map[int32]bool{}
	for _, manifest := range formations {
		for _, patch := range manifest.BuddyPatches {
			rewritten[patch.RowID] = true
			if value, present := patch.Set["npcParamId"]; present {
				if row, ok := asRowID(value); ok {
					named[row] = true
				}
			}
		}
	}
	if !resetting {
		for rowID, actors := range read.Actors {
			if !rewritten[rowID] {
				named[actors.Npc] = true
			}
		}
	}
	sources := map[int32]bool{}
	for row := range named {
		if slices.Contains(snapshot.NpcCloneRows, row) {
			continue
		}
		if source, ok := formation.CloneSource(row); ok {
			sources[source] = true
		}
	}
	return sources, nil
}

// sharedEffectsPart is the write, or nil when the save already says what it
// should.
func (session *Session) sharedEffectsPart(formations []*plan.Manifest, resetting bool) (*plan.Manifest, error) {
	needs := session.service.shared
	if len(needs) == 0 && len(session.service.persons) == 0 {
		return nil, nil
	}
	sources, err := summonedAfter(session.loaded.formations, formations, resetting)
	if err != nil {
		return nil, err
	}
	type decision struct {
		value, shipped float64
		by             []string
	}
	decided := map[savecore.SharedEffectKey]*decision{}
	var order []savecore.SharedEffectKey
	for _, need := range needs {
		key := savecore.SharedEffectKey{Row: need.Row, Field: need.Field}
		one := decided[key]
		if one == nil {
			one = &decision{value: need.Shipped, shipped: need.Shipped}
			decided[key] = one
			order = append(order, key)
		}
		if sources[need.Npc] {
			one.value = need.Value
			one.by = append(one.by, fmt.Sprint(need.Npc))
		}
	}
	var values []plan.SharedEffectValue
	for _, key := range order {
		one := decided[key]
		current, held := session.loaded.state.SharedEffects[key]
		if !held || sameValue(current, one.value) {
			continue
		}
		why := "put back: no Ash holds an entry that needs it"
		if len(one.by) > 0 {
			why = "needed while an Ash holds " + strings.Join(one.by, ", ")
		} else if !session.wroteShared(key, current) {
			// Not ours to put back: a value this editor never writes.
			continue
		}
		values = append(values, plan.SharedEffectValue{Row: key.Row, Field: key.Field, Value: one.value,
			Why: fmt.Sprintf("SpEffectParam %d.%s: %s", key.Row, key.Field, why)})
	}
	persons := session.personWritesPart(formations, resetting)
	if len(values) == 0 && len(persons) == 0 {
		return nil, nil
	}
	return plan.BuildSharedEffects(session.loaded.info.SHA256, values, persons)
}

// personWritesPart decides every person field as sharedEffectsPart decides an
// effect field: the armed value while a formation row in the written save
// points at the person, the shipped value once none does - and only a value
// this editor writes is ever put back.
func (session *Session) personWritesPart(formations []*plan.Manifest, resetting bool) []plan.PersonValue {
	if len(session.service.persons) == 0 {
		return nil
	}
	summoned := personsAfter(session.loaded.formations, formations, resetting)
	type decision struct {
		value float64
		held  bool
	}
	decided := map[savecore.PersonKey]*decision{}
	var order []savecore.PersonKey
	for _, need := range session.service.persons {
		key := savecore.PersonKey{Row: need.Init, Field: need.Field}
		one := decided[key]
		if one == nil {
			one = &decision{value: need.Shipped}
			decided[key] = one
			order = append(order, key)
		}
		if summoned[need.Init] {
			one.value, one.held = need.Value, true
		}
	}
	var values []plan.PersonValue
	for _, key := range order {
		one := decided[key]
		current, held := session.loaded.state.PersonWrites[key]
		if !held || sameValue(current, one.value) {
			continue
		}
		why := "put back: no Ash holds this person"
		if one.held {
			why = "armed while an Ash holds this person"
		} else if !session.wrotePerson(key, current) {
			continue // not ours to put back
		}
		values = append(values, plan.PersonValue{Row: key.Row, Field: key.Field, Value: one.value,
			Why: fmt.Sprintf("CharaInitParam %d: %s", key.Row, why)})
	}
	return values
}

// wrotePerson reports whether a field holds the value an armed person needs,
// the only value of it this editor ever writes.
func (session *Session) wrotePerson(key savecore.PersonKey, current float64) bool {
	for _, need := range session.service.persons {
		if need.Init == key.Row && need.Field == key.Field && sameValue(current, need.Value) {
			return true
		}
	}
	return false
}

// wroteShared reports whether a field holds the value an entry needs, which is
// the only value of it this editor ever writes.
func (session *Session) wroteShared(key savecore.SharedEffectKey, current float64) bool {
	for _, need := range session.service.shared {
		if need.Row == key.Row && need.Field == key.Field && sameValue(current, need.Value) {
			return true
		}
	}
	return false
}

// sameValue compares as the save stores: effectEndurance is an f32, so 0.1
// reads back as 0.10000000149.
func sameValue(a, b float64) bool {
	return math.Float32bits(float32(a)) == math.Float32bits(float32(b))
}
