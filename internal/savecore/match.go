package savecore

// Recognising a clone the save already holds.
//
// A creature's clone is its world row with a recipe written over it, and a
// creature may have one clone per recipe (the clone bands, fields.go). An
// author compiles against the save as it was loaded, and without this it could
// not tell a clone an earlier session wrote the same way from one written some
// other way: a player who reopened a save and changed a creature's level would
// get the old level, silently. MatchClones answers the question with
// bytes: it renders each clone the manifest would insert through the very
// recipe functions the writer uses, and looks for a row of the same creature,
// in any clone band, that already holds exactly those bytes, its role's
// strength included (sameNpcClone).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// CloneMatches maps a clone key the manifest would insert to a row the save
// already holds that is that clone, byte for byte, per table.
type CloneMatches struct {
	Npc   map[int32]int32
	Think map[int32]int32
}

// MatchClones reads the save at path and reports, for every clone the author
// manifest would insert, an existing row that already is that clone. It writes
// nothing. A manifest for another save is refused, as Apply refuses it.
func MatchClones(ctx context.Context, path string, encodedManifest io.Reader) (*CloneMatches, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	plan, err := decodeManifest(encodedManifest)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, ErrManifestInvalid)
	}
	loaded, err := loadSave(path)
	if err != nil {
		return nil, err
	}
	if hash := regulation.Sha256Hex(loaded.FileBytes); hash != plan.SourceSHA256 {
		return nil, fmt.Errorf("input SHA-256 is %s, manifest requires %s: %w", hash, plan.SourceSHA256, ErrSourceMismatch)
	}
	matches := &CloneMatches{Npc: map[int32]int32{}, Think: map[int32]int32{}}
	recipes := cloneRecipesOf(plan)
	if len(plan.NpcCloneSources) > 0 {
		recipe, err := npcCloneRecipe(loaded.BND4, recipes.strength, recipes.teams, recipes.worldBranch, recipes.unheld, recipes.weaponOut)
		if err != nil {
			return nil, err
		}
		if matches.Npc, err = matchActorClones(loaded.BND4, npcLayout, plan.NpcCloneSources, recipe, sameNpcClone); err != nil {
			return nil, err
		}
	}
	if len(plan.NpcThinkCloneSources) > 0 {
		recipe, err := thinkCloneRecipe(recipes.perception)
		if err != nil {
			return nil, err
		}
		if matches.Think, err = matchActorClones(loaded.BND4, npcThinkLayout, plan.NpcThinkCloneSources, recipe, bytes.Equal); err != nil {
			return nil, err
		}
	}
	return matches, nil
}

// sameNpcClone compares two NpcParam clones: every byte must agree, and the
// resident effects must agree as a set of slots filled, whatever slot each
// sits in. The strength is compared too, because the role is the recipe's: a
// Helper and a Fighter of one creature are two rows.
func sameNpcClone(held, wanted []byte) bool {
	if len(held) != len(wanted) {
		return false
	}
	masked := func(row []byte) ([]byte, map[int32]int) {
		copied := bytes.Clone(row)
		residents := map[int32]int{}
		for _, offset := range npcResidentEffectOffsets {
			effect := readS32(copied, offset)
			writeS32(copied, offset, 0)
			if effect > 0 {
				residents[effect]++
			}
		}
		return copied, residents
	}
	heldBytes, heldResidents := masked(held)
	wantedBytes, wantedResidents := masked(wanted)
	return bytes.Equal(heldBytes, wantedBytes) && maps.Equal(heldResidents, wantedResidents)
}

// matchActorClones renders each key's clone - a copy of its world row with the
// recipe written over it, exactly as insertActorClonesFrom writes one - and
// answers with the lowest existing row of that creature, in any clone band,
// that same judges to be that clone.
func matchActorClones(bnd4 []byte, layout regulation.TableLayout, keys []int32, recipe func(int32, []byte) error, same func(held, wanted []byte) bool) (map[int32]int32, error) {
	table, err := regulation.OpenTable(bnd4, layout)
	if err != nil {
		return nil, err
	}
	bySource := map[int32][]int32{}
	for _, row := range table.Rows {
		if inCloneBand(row.ID) {
			bySource[row.ID%cloneRowOffset] = append(bySource[row.ID%cloneRowOffset], row.ID)
		}
	}
	matches := map[int32]int32{}
	for _, key := range keys {
		source, ok := cloneKeySource(key)
		if !ok {
			return nil, fmt.Errorf("%s: %d is not a clone key", layout.Entry, key)
		}
		world, found := table.ByID[source]
		if !found {
			return nil, fmt.Errorf("%s: actor %d was not found: %w", layout.Entry, source, ErrRowNotFound)
		}
		rendered := bytes.Clone(table.Data[world.DataOffset : world.DataOffset+layout.RowSize])
		if err := recipe(key, rendered); err != nil {
			return nil, fmt.Errorf("%s clone key %d: %w", layout.Entry, key, err)
		}
		candidates := slices.Clone(bySource[source])
		slices.Sort(candidates)
		for _, id := range candidates {
			row := table.ByID[id]
			if same(table.Data[row.DataOffset:row.DataOffset+layout.RowSize], rendered) {
				matches[key] = id
				break
			}
		}
	}
	return matches, nil
}
