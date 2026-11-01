package plan

// A prune: the creature rows this editor inserted that no formation row names
// any more, removed when the save is written.
//
// The file is written from the whole configuration at Save. Every creature gets exactly the rows its slots need -
// one per recipe, shared by every slot that uses it - and a row nothing
// summons is not written. Before it only a reset removed anything, so a player
// who reconfigured a creature, or took it out of a container, left its old
// rows in the save for good.
//
// Which rows are unused is decided by the application against every formation
// row in the save (clonealloc.go); this only spells the decision as a
// manifest, and the save core holds it to the clone bands as it holds a reset.

import (
	"fmt"
	"slices"
)

// OperationPrune is the operation value a prune manifest carries.
const OperationPrune = "prune"

// BuildPrune is the manifest that removes the given NpcParam and NpcThinkParam
// rows. Both lists are sorted, so one staged set always writes the same bytes.
func BuildPrune(sourceSHA256 string, npc, think []int32) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(npc)+len(think) == 0 {
		return nil, fmt.Errorf("a prune needs at least one creature row to remove")
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationPrune}
	manifest.NpcCloneRemovals = slices.Sorted(slices.Values(npc))
	manifest.NpcThinkCloneRemovals = slices.Sorted(slices.Values(think))
	if len(manifest.NpcCloneRemovals) == 0 {
		manifest.NpcCloneRemovals = nil
	}
	if len(manifest.NpcThinkCloneRemovals) == 0 {
		manifest.NpcThinkCloneRemovals = nil
	}
	return manifest, nil
}
