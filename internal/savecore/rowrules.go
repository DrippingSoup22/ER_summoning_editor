package savecore

// What one row patch has to satisfy, table by table. Every rule here is about
// the contents of a patch - which fields it names and what values they carry -
// and none of them is about the manifest that carries it.

import (
	"fmt"
	"math"
	"strings"
)

// goodsWriteSurface is which of the three goods fields each operation may name.
//
// Naming the set per operation is what keeps them disjoint where they share a
// table. A conversion cannot reach the price and a cost edit cannot re-point a
// formation, so the two can be staged against one Ash without either being able
// to do the other's work by accident.
//
// The first two must name their whole set: an operation that could leave out
// half of what it is for would be describing a different edit. A reset names a
// subset, because a reset yields where it meets a more specific staged change -
// it says "put back everything I have not said otherwise about", and the field
// another part is writing is a field it has been said otherwise about. The
// union across the parts still determines every field exactly once.
var goodsWriteSurface = map[string]goodsSurface{
	operationConvert: {fields: []string{"refId_default"}},
	operationCost:    {fields: []string{"consumeMP", "consumeHP"}},
	operationReset:   {fields: []string{"refId_default", "consumeMP", "consumeHP"}, partial: true},
}

type goodsSurface struct {
	fields  []string
	partial bool
}

func validateGoodsPatches(plan *manifest) error {
	surface, writes := goodsWriteSurface[plan.Operation]
	if !writes && len(plan.GoodsPatches) > 0 {
		return fmt.Errorf("%s writes no goods row", plan.Operation)
	}
	allowed := make(map[string]bool, len(surface.fields))
	for _, name := range surface.fields {
		allowed[name] = true
	}
	seenGoods := make(map[int32]bool, len(plan.GoodsPatches))
	for index, patch := range plan.GoodsPatches {
		if patch.RowID < 0 || seenGoods[patch.RowID] {
			return fmt.Errorf("goods patch %d has invalid or duplicate row ID %d", index+1, patch.RowID)
		}
		seenGoods[patch.RowID] = true
		if len(patch.Set) == 0 {
			return fmt.Errorf("goods patch %d sets no field", index+1)
		}
		if !surface.partial && len(patch.Set) != len(surface.fields) {
			return fmt.Errorf("goods patch %d must set exactly %s", index+1, strings.Join(surface.fields, ", "))
		}
		for name, value := range patch.Set {
			if !allowed[name] {
				return fmt.Errorf("goods patch %d sets %s, which %s does not write",
					index+1, name, plan.Operation)
			}
			low, high := goodsFields[name].valueRange()
			if value != math.Trunc(value) || value < low || value > high {
				return fmt.Errorf("goods patch %d sets %s to %v, which is outside %v..%v",
					index+1, name, value, low, high)
			}
		}
	}
	return nil
}

func validateBuddyPatches(plan *manifest) error {
	seenBuddy := make(map[int32]bool, len(plan.BuddyPatches))
	for index, patch := range plan.BuddyPatches {
		if patch.RowID < 0 || seenBuddy[patch.RowID] {
			return fmt.Errorf("Buddy patch %d has invalid or duplicate row ID %d", index+1, patch.RowID)
		}
		seenBuddy[patch.RowID] = true
		// The switch writes the same one field, in either direction: 0 lifts
		// the restriction and the shipped value puts it back. It may not reach
		// any other field, because a switch about where you may summon has no
		// business naming an actor. A conversion carries the switch, and this
		// stays its whole BuddyParam write surface.
		if plan.Operation == operationArea || plan.Operation == operationConvert {
			value, present := patch.Set["appearOnAroundSekihi"]
			if len(patch.Set) != 1 || !present || value < 0 || value > 1 {
				return fmt.Errorf("Buddy patch %d must only set appearOnAroundSekihi to 0 or 1", index+1)
			}
			continue
		}
		// Authoring and a reset both write the whole row. A reset restores
		// every field because an authored formation can have changed any of
		// them and one changed field is reason enough to put the row back;
		// authoring writes buddyAuthoredFields, which is every field carrying
		// part of the answer to what this slot delivers - see the note there
		// for why that set is stated rather than grown.
		//
		// The eleven reinforcement fields are the one difference between the
		// two. Authoring writes them only when the formation is being scaled to
		// the creature it delivers, and leaves them alone otherwise - a row
		// inserted into a block copies its template's ladder, which is already
		// the right one. A reset has to put back whatever authoring wrote, so
		// it writes every field in buddyFields.
		// The strength switch writes the ladder and nothing else. It has no
		// business naming an actor: it changes how hard the summon a formation
		// already holds hits, and a switch able to rewrite npcParamId would be
		// a switch able to change which creature is summoned.
		if plan.Operation == operationStrength {
			if err := validateBuddyLadderPatch(index, patch); err != nil {
				return err
			}
			continue
		}
		if err := validateBuddyRowPatch(index, patch, plan.Operation == operationReset); err != nil {
			return err
		}
	}
	return nil
}

func validateStonePatches(plan *manifest) error {
	if plan.Operation != operationConvert && plan.Operation != operationArea {
		return validateStoneRowPatches(plan.BuddyStoneRowPatches, fieldNames(stoneFields))
	}
	// The monument switch has two halves: where a summon may be called (five
	// fields) and the once-per-rest limit (summonedEventFlagId). A half that
	// is on rides on the one all-row patch at the recipe's values; a half that
	// is off is put back on every row from the shipped snapshot, and those
	// values are read rather than known in advance, so every one has to be
	// proved to fit the field it is going into.
	on := map[string]bool{}
	if len(plan.BuddyStonePatches) == 1 {
		for name, value := range plan.BuddyStonePatches[0].Set {
			if field, known := stoneFields[name]; !known || value != field.want {
				return fmt.Errorf("BuddyStone recipe field %s is not one of the six at its recipe value", name)
			}
			on[name] = true
		}
		rest := on[restField]
		switch {
		case len(on) == 1 && rest, len(on) == len(stoneFields)-1 && !rest, len(on) == len(stoneFields):
		default:
			return fmt.Errorf("the all-row BuddyStone patch must set a whole half of the monument switch, or both")
		}
	}
	var off []string
	for _, name := range fieldNames(stoneFields) {
		if !on[name] {
			off = append(off, name)
		}
	}
	if (len(off) == 0) != (len(plan.BuddyStoneRowPatches) == 0) {
		return fmt.Errorf("BuddyStone rows are restored exactly when a half of the monument switch is off")
	}
	return validateStoneRowPatches(plan.BuddyStoneRowPatches, off)
}

// validateBuddyRowPatch checks one restored BuddyParam row.
//
// It is the counterpart of validateStoneRowPatches and holds the same rule: a
// reset writes recorded data, so every value has to be proved to fit the field
// it is going into. The two places in this product where the number written is
// not known in advance are both resets.
func validateBuddyRowPatch(index int, patch rowPatch, whole bool) error {
	required := buddyAuthoredFields
	if whole {
		required = buddyFields
	}
	if len(patch.Set) < len(required) || len(patch.Set) > len(buddyFields) {
		return fmt.Errorf("Buddy patch %d sets %d fields, want %d to %d",
			index+1, len(patch.Set), len(required), len(buddyFields))
	}
	for name := range patch.Set {
		if _, known := buddyFields[name]; !known {
			return fmt.Errorf("Buddy patch %d sets %s, which is not a BuddyParam field this product writes", index+1, name)
		}
	}
	for name := range required {
		if _, ok := patch.Set[name]; !ok {
			return fmt.Errorf("Buddy patch %d does not set %s", index+1, name)
		}
	}
	for name, value := range patch.Set {
		field := buddyFields[name]
		// Every field on this row is an integer except the three that say
		// where the actor stands, which are f32 and whose whole point is the
		// fraction: the shipped layouts sit at 1.3 and 0.75.
		if field.kind != "f32" && value != math.Trunc(value) {
			return fmt.Errorf("Buddy patch %d field %s is not a whole number: %g", index+1, name, value)
		}
		low, high := field.valueRange()
		if value < low || value > high {
			return fmt.Errorf("Buddy patch %d field %s is %g, outside the %s range %g..%g",
				index+1, name, value, field.kind, low, high)
		}
	}
	return nil
}

// validateBuddyLadderPatch holds a strength patch to the eleven reinforcement
// fields, all of them and only them.
//
// All eleven because a ladder with a gap is a formation that is the creature at
// some upgrade levels and a helper at others, which no player asked for and
// nothing on screen would explain. Only those eleven because the switch is
// about strength: the identity fields say which creature and which trigger, and
// those are the formation's business.
func validateBuddyLadderPatch(index int, patch rowPatch) error {
	if len(patch.Set) != len(buddyDopingFields) {
		return fmt.Errorf("strength patch %d sets %d fields, want the %d reinforcement levels",
			index+1, len(patch.Set), len(buddyDopingFields))
	}
	for name, value := range patch.Set {
		field, isLadder := buddyDopingFields[name]
		if !isLadder {
			return fmt.Errorf("strength patch %d sets %s, which is not a reinforcement level", index+1, name)
		}
		if value != math.Trunc(value) {
			return fmt.Errorf("strength patch %d field %s is not a whole number: %g", index+1, name, value)
		}
		low, high := field.valueRange()
		if value < low || value > high {
			return fmt.Errorf("strength patch %d field %s is %g, outside the %s range %g..%g",
				index+1, name, value, field.kind, low, high)
		}
	}
	return nil
}

// validateStoneRowPatches checks the values a reset restores.
//
// A conversion writes six compile-time constants, so its only real check is
// that the manifest did not ask for different ones. A reset writes recorded
// data, so every value has to be proved to fit the field it is going into: a
// stone row patch is the one place in this product where the number written is
// not known in advance.
func validateStoneRowPatches(patches []rowPatch, fields []string) error {
	seen := make(map[int32]bool, len(patches))
	for index, patch := range patches {
		if patch.RowID < 0 || seen[patch.RowID] {
			return fmt.Errorf("BuddyStone patch %d has invalid or duplicate row ID %d", index+1, patch.RowID)
		}
		seen[patch.RowID] = true
		if len(patch.Set) != len(fields) {
			return fmt.Errorf("BuddyStone patch %d sets %d fields, want %d", index+1, len(patch.Set), len(fields))
		}
		for _, name := range fields {
			field := stoneFields[name]
			value, ok := patch.Set[name]
			if !ok {
				return fmt.Errorf("BuddyStone patch %d does not set %s", index+1, name)
			}
			if value != math.Trunc(value) {
				return fmt.Errorf("BuddyStone patch %d field %s is not a whole number: %g", index+1, name, value)
			}
			low, high := field.valueRange()
			if value < low || value > high {
				return fmt.Errorf("BuddyStone patch %d field %s is %g, outside the %s range %g..%g",
					index+1, name, value, field.kind, low, high)
			}
		}
	}
	return nil
}

// validateVfxPatches holds the colour switch to one field of one row.
//
// Three clauses, and each refuses a different mistake. Only the tint operation
// may carry one at all, so no other operation can smuggle a global visual edit
// in under a per-Ash intent - the same rule that keeps the monument recipe off
// a conversion. Only spiritVfxRow may be named, which is what makes "this
// cannot reach another visual effect" true by construction rather than by
// review. And only the two shipped values are accepted: 200 is PhantomParam's
// Spirit Summon profile and 0 is the game's own no-overwrite spelling, so both
// positions of the switch write a number the game already uses and neither
// invents one.
func validateVfxPatches(plan *manifest) error {
	if len(plan.VfxPatches) == 0 {
		return nil
	}
	if plan.Operation != operationTint && plan.Operation != operationSession {
		return fmt.Errorf("operation %q may not write SpEffectVfxParam: %w",
			plan.Operation, ErrManifestInvalid)
	}
	if len(plan.VfxPatches) != 1 {
		return fmt.Errorf("the colour switch writes one row, got %d: %w",
			len(plan.VfxPatches), ErrManifestInvalid)
	}
	patch := plan.VfxPatches[0]
	if patch.RowID != spiritVfxRow {
		return fmt.Errorf("the colour switch may only write SpEffectVfxParam row %d, got %d: %w",
			spiritVfxRow, patch.RowID, ErrManifestInvalid)
	}
	if len(patch.Set) != len(vfxFields) {
		return fmt.Errorf("the colour switch sets %d fields, want %d: %w",
			len(patch.Set), len(vfxFields), ErrManifestInvalid)
	}
	value, present := patch.Set["phantomParamOverwriteId"]
	if !present {
		return fmt.Errorf("the colour switch does not set phantomParamOverwriteId: %w", ErrManifestInvalid)
	}
	// Three shipped profiles, and only three. 200 is the white spirit look,
	// 410 is the neutral one the off position writes, and 0 is what an older
	// build's off position wrote - kept so its file is still recognised rather
	// than refused.
	if value != float64(noPhantomProfile) &&
		value != float64(spiritPhantomProfile) &&
		value != float64(neutralPhantomProfile) {
		return fmt.Errorf("the colour switch writes %g; the only shipped values are %d, %d and %d: %w",
			value, noPhantomProfile, spiritPhantomProfile, neutralPhantomProfile, ErrManifestInvalid)
	}
	return nil
}
