package application

// Sharing a clone between formations.
//
// A world creature's summon-side clone for one recipe is ONE row: its
// destination is key + formation.CloneOffset, arithmetic by docs/SAFETY.md rule
// 1. Two Ashes staged in one session that both summon the same creature
// therefore both compile an insertion of the same row, because each is
// compiled against the save as it was loaded - and the save core refuses a row
// two parts insert.
//
// So the commit resolves it rather than the player: when two formations insert
// the same clone WITH THE SAME RECIPE, the first (in commit order) inserts it
// and the rest name the row it inserted. That writes exactly the save a player
// would get by authoring the second Ash after the first had been written.
//
// Two formations never want one clone row two ways in the ordinary course: at
// Save every staged formation is placed with one
// allocator (clonealloc.go), which gives each recipe of a creature its own
// clone key (formation.CloneBands) - a Guard in one Ash and a Sentinel of the
// same creature in another are two rows. CreatureTwoWaysError is kept as the
// guard that says so. It fires for a player only through a defect; the one
// staged set that reaches it on purpose is a research seat that asks for one
// creature's knobs by itself (AuthoredSlot.Residents, BareRow) in one Ash and
// not in another, since those are recorded per creature rather than per
// recipe.

import (
	"fmt"
	"slices"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
)

// CreatureTwoWaysError reports one clone row that two staged formations want
// written two different ways. That means two manifests placed their recipes
// with different allocators - a defect - since one allocator gives each recipe
// its own row.
type CreatureTwoWaysError struct {
	// Table is "NpcParam" or "NpcThinkParam", or "NpcParam copy" for a shell
	// copied onto a team.
	Table string
	// Source is the world (or shell) row both formations clone.
	Source int32
	// First and Second are the two Ashes, in commit order.
	First, Second string
}

func (err *CreatureTwoWaysError) Error() string {
	return fmt.Sprintf("%s and %s both summon row %d (%s) but configure it differently, and it has one summon-side row",
		err.First, err.Second, err.Source, err.Table)
}

// Unwrap places the error in the category a frontend acts on: drop one of the
// two staged changes, or make them agree.
func (err *CreatureTwoWaysError) Unwrap() error { return ErrStagedChangesCannotCombine }

// shareClones returns the formation manifests with every clone an earlier
// formation already inserts removed from the later ones, and refuses a clone
// two formations would write differently. The staged manifests are not
// changed: a formation that shares a clone in this commit may be the first to
// insert it in the next, once the other is unstaged.
func shareClones(staged []*StagedFormation) ([]*plan.Manifest, error) {
	// A formation writes a manifest a block, and the Giant Rat's three name
	// one set of clones, which the first of them inserts.
	var hosts []string
	var manifests []*plan.Manifest
	for _, formation := range staged {
		for _, manifest := range formation.manifests {
			hosts, manifests = append(hosts, formation.HostName), append(manifests, manifest)
		}
	}
	npcFirst := map[int32]int{}
	thinkFirst := map[int32]int{}
	copyFirst := map[int32]int{}
	npcRecipes := make([]map[int32]string, len(manifests))
	thinkRecipes := make([]map[int32]string, len(manifests))
	copyRecipes := make([]map[int32]string, len(manifests))
	shared := make([]*plan.Manifest, len(manifests))

	for index, manifest := range manifests {
		npcRecipes[index] = npcCloneRecipes(manifest)
		thinkRecipes[index] = thinkCloneRecipes(manifest)
		copyRecipes[index] = summonCopyRecipes(manifest)

		dropNpc, err := claimShared(hosts, index, "NpcParam", manifest.NpcCloneSources, npcFirst, npcRecipes)
		if err != nil {
			return nil, err
		}
		dropThink, err := claimShared(hosts, index, "NpcThinkParam", manifest.NpcThinkCloneSources, thinkFirst, thinkRecipes)
		if err != nil {
			return nil, err
		}
		dropCopy, err := claimShared(hosts, index, "NpcParam copy", manifest.NpcSummonCopies, copyFirst, copyRecipes)
		if err != nil {
			return nil, err
		}
		if len(dropNpc)+len(dropThink)+len(dropCopy) == 0 {
			shared[index] = manifest
			continue
		}
		shared[index] = withoutClones(manifest, dropNpc, dropThink, dropCopy)
	}
	return shared, nil
}

// claimShared records the sources manifest index inserts and answers with the
// ones an earlier manifest already inserts the same way. A source an earlier
// one inserts differently is refused, naming the two Ashes (hosts).
func claimShared(hosts []string, index int, table string, sources []int32,
	first map[int32]int, recipes []map[int32]string) (map[int32]bool, error) {
	drop := map[int32]bool{}
	for _, source := range sources {
		earlier, claimed := first[source]
		if !claimed {
			first[source] = index
			continue
		}
		if recipes[earlier][source] != recipes[index][source] {
			return nil, &CreatureTwoWaysError{
				Table: table, Source: source,
				First: hosts[earlier], Second: hosts[index],
			}
		}
		drop[source] = true
	}
	return drop, nil
}

// npcCloneRecipes is, per NpcParam clone source, everything the manifest says
// about how that clone is written. Two formations that say the same thing may
// share the row.
func npcCloneRecipes(manifest *plan.Manifest) map[int32]string {
	recipes := map[int32]string{}
	for _, source := range manifest.NpcCloneSources {
		var strength *plan.CloneStrength
		for _, entry := range manifest.NpcCloneStrength {
			if entry.Source == source {
				entry := entry
				strength = &entry
			}
		}
		recipes[source] = fmt.Sprintf("strength=%v team=%d world=%t unheld=%t weapon=%d",
			strengthKey(strength), teamOf(manifest, source),
			slices.Contains(manifest.NpcCloneWorldBranch, source),
			slices.Contains(manifest.NpcCloneWorldBranchUnheld, source),
			weaponOutOf(manifest, source))
	}
	return recipes
}

// thinkCloneRecipes is, per NpcThinkParam clone source, the perception entry
// it is written with.
func thinkCloneRecipes(manifest *plan.Manifest) map[int32]string {
	recipes := map[int32]string{}
	for _, source := range manifest.NpcThinkCloneSources {
		recipes[source] = "none"
		for _, entry := range manifest.NpcThinkPerception {
			if entry.Source == source {
				recipes[source] = fmt.Sprintf("%+v", entry)
			}
		}
	}
	return recipes
}

// summonCopyRecipes is, per shell copied into the copy band, the team it is
// copied onto: the copy is the shell byte for byte with teamType moved.
func summonCopyRecipes(manifest *plan.Manifest) map[int32]string {
	recipes := map[int32]string{}
	for _, source := range manifest.NpcSummonCopies {
		recipes[source] = fmt.Sprintf("team=%d", teamOf(manifest, source))
	}
	return recipes
}

func strengthKey(strength *plan.CloneStrength) string {
	if strength == nil {
		return "none"
	}
	return fmt.Sprintf("hp=%d residents=%v keep=%t", strength.HP, strength.Residents, strength.KeepBalance)
}

func teamOf(manifest *plan.Manifest, source int32) uint8 {
	for _, entry := range manifest.NpcCloneTeams {
		if entry.Source == source {
			return entry.Team
		}
	}
	return 0
}

func weaponOutOf(manifest *plan.Manifest, source int32) int32 {
	for _, entry := range manifest.NpcCloneWeaponOut {
		if entry.Source == source {
			return entry.Think
		}
	}
	return 0
}

// withoutClones is a copy of manifest that no longer inserts the named clones,
// nor carries the entries that describe how they are written. Every other field
// is the original's; no slice of the original is modified.
func withoutClones(manifest *plan.Manifest, npc, think, copies map[int32]bool) *plan.Manifest {
	out := *manifest
	out.NpcCloneSources = keepSources(manifest.NpcCloneSources, npc)
	out.NpcThinkCloneSources = keepSources(manifest.NpcThinkCloneSources, think)
	out.NpcSummonCopies = keepSources(manifest.NpcSummonCopies, copies)
	out.NpcCloneWorldBranch = keepSources(manifest.NpcCloneWorldBranch, npc)
	out.NpcCloneWorldBranchUnheld = keepSources(manifest.NpcCloneWorldBranchUnheld, npc)
	out.NpcCloneStrength = nil
	for _, entry := range manifest.NpcCloneStrength {
		if !npc[entry.Source] {
			out.NpcCloneStrength = append(out.NpcCloneStrength, entry)
		}
	}
	out.NpcCloneWeaponOut = nil
	for _, entry := range manifest.NpcCloneWeaponOut {
		if !npc[entry.Source] {
			out.NpcCloneWeaponOut = append(out.NpcCloneWeaponOut, entry)
		}
	}
	// A team names a clone or a copy by its source, and the two never share a
	// number: a clone's source is a world row below 100000000, a copy's a shell
	// above it.
	out.NpcCloneTeams = nil
	for _, entry := range manifest.NpcCloneTeams {
		if !npc[entry.Source] && !copies[entry.Source] {
			out.NpcCloneTeams = append(out.NpcCloneTeams, entry)
		}
	}
	out.NpcThinkPerception = nil
	for _, entry := range manifest.NpcThinkPerception {
		if !think[entry.Source] {
			out.NpcThinkPerception = append(out.NpcThinkPerception, entry)
		}
	}
	return &out
}

func keepSources(sources []int32, drop map[int32]bool) []int32 {
	var kept []int32
	for _, source := range sources {
		if !drop[source] {
			kept = append(kept, source)
		}
	}
	return kept
}
