package plan

// Authoring: turning a formation the player composed into the rows it becomes.
//
// It is the third operation, beside a conversion and a reset, and it is a
// separate one rather than a wider conversion because the three write disjoint
// surfaces. A conversion re-points a host Ash's goods rows at another shipped
// formation and clears one BuddyParam flag. Authoring fills the host's OWN
// block with actors the player chose, which means writing the three actor
// fields of a formation row and adding rows the game never shipped - and no
// goods row at all, which is what leaves it structurally unable to reach
// enable_multi.

import (
	"fmt"
	"slices"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// OperationAuthor is the operation value an authored manifest carries.
const OperationAuthor = "author"

// AuthoredAppearOnAroundSekihi is what authoring writes to the placement flag
// of every row it fills.
//
// It is 0 on 150 of the 170 shipped BuddyParam rows and 1 on the other 20, and
// the 20 are the ones research measured spawning their formation at an
// arbitrary distant monument. For an authored formation that is never what the
// player asked for: they chose actors and expect them where they are standing.
// Writing 0 therefore makes an authored block behave like the ordinary
// majority, and it is the same byte research validated as part of the
// shipping fix. A reset puts the shipped value back, in both directions.
const AuthoredAppearOnAroundSekihi = int64(0)

// InertTrigger is how the game itself spells a formation row that summons
// nothing: six shipped rows carry it. A formation smaller than the block it is
// authored into retires the surplus rows this way rather than deleting them, so
// an ordinary value patch undoes it.
const InertTrigger = int64(-1)

// NoNpcPlayerInit is a formation row that summons a creature rather than a
// person, and it is what authoring writes to every row it fills.
//
// npcPlayerInitParamId names a CharaInitParam row, and when it is set the game
// builds that human being INSTEAD of the creature npcParamId names. Twelve
// shipped rows carry one - Mimic Tear's second row, the five Puppets, and the
// DLC NPC Ashes. Authoring into one of those hosts without clearing this is
// what made the identification run deliver ten hosts' own people at the
// requested count. A reset puts the shipped value back, so a Puppet authored
// over is still a Puppet afterwards.
const NoNpcPlayerInit = int64(-1)

// OrdinarySummonAnimation is the animation a summoned creature arrives with:
// 1830, on 157 of the 170 shipped rows. The thirteen exceptions are the human
// Ashes, which arrive the way a person does, and Mimic Tear's own 1831.
// Writing it changes nothing on an ordinary host and keeps a creature from
// arriving on a person's animation on the thirteen.
const OrdinarySummonAnimation = int64(1830)

// PersonSummonAnimation is the animation a summoned person arrives with:
// 60501 on five of the six shipped Puppets (the Mimic Tear's second row has
// 60500, the DLC NPC Ashes 60506-60509). A person slot writes it, so a human
// does not emerge the way a creature does.
const PersonSummonAnimation = int64(60501)

// personOf is the CharaInitParam row a slot summons, or the no-person
// spelling for a creature.
func personOf(actor formation.Actor) int64 {
	if actor.Person > 0 {
		return int64(actor.Person)
	}
	return NoNpcPlayerInit
}

// animationOf is the emergence animation for the slot: its own where it has
// one, a person's on a person slot, the ordinary one otherwise.
func animationOf(actor formation.Actor) int64 {
	if actor.Arrival > 0 {
		return int64(actor.Arrival)
	}
	if actor.Person > 0 {
		return PersonSummonAnimation
	}
	return OrdinarySummonAnimation
}

// Stance is whether a summoned actor follows the player or holds its ground.
// It is formation.Stance, which owns the type and the reasoning; re-exported
// here so a caller staging a formation does not need a second import.
type Stance = formation.Stance

// Placement is formation.Placement, re-exported so a caller staging a
// formation does not need a second import to say where an actor stands.
type Placement = formation.Placement

var (
	StanceFollow = formation.StanceFollow
	StanceWander = formation.StanceWander
	StanceHold   = formation.StanceHold
	StanceStill  = formation.StanceStill
)

// ErrNothingToWrite is returned for a formation that fills no row. It is
// distinct from a formation that is invalid: formation.Validate refuses an
// empty one, so reaching here means a compile produced no work.
var ErrNothingToWrite = fmt.Errorf("an authored formation writes at least one row")

// ErrUnknownRow is returned when the manifest would have to describe a row the
// save does not hold. Authoring only ever meets this while retiring a surplus
// row, which by definition is a row the block already had.
var ErrUnknownRow = fmt.Errorf("the save does not hold a row this plan names")

// BuildAuthor turns a compiled formation into the manifest that writes it.
func BuildAuthor(sourceSHA256 string, compiled formation.Compiled, current Vanilla, strength Strength) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(compiled.Edits)+len(compiled.Inserts) == 0 {
		return nil, fmt.Errorf("an authored formation fills at least one row: %w", ErrNothingToWrite)
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationAuthor}

	// Clones first, in the manifest as in the write: the rows a formation names
	// have to exist before anything can name them. Compile has already answered
	// each table independently, because a save can hold one half of a pair and
	// not the other.
	for _, clone := range compiled.Clones {
		if clone.Npc != nil {
			manifest.NpcCloneSources = append(manifest.NpcCloneSources, clone.Npc.CopyFrom)
			// A level's own residents - the Follower's 296010 - ride the same
			// entry as the creature's grants, exactly as a slot's asked-for
			// residents do.
			level := formation.LevelResidents(clone.Source.Perception)
			// The health and the grants are the creature's, whatever band its
			// clone is in; the role, the presence and the marker are the
			// recipe's.
			presence, marker, fighter := clone.Source.Presence, clone.Source.Marker, clone.Source.Fighter
			if want, asked := strength[clone.Npc.CopyFrom%formation.CloneOffset]; asked {
				residents := append([]int32(nil), want.Residents...)
				for _, effect := range level {
					if !slices.Contains(residents, effect) {
						residents = append(residents, effect)
					}
				}
				hp := want.HelperHP
				if fighter {
					hp = want.FighterHP
				}
				manifest.NpcCloneStrength = append(manifest.NpcCloneStrength,
					CloneStrength{Source: clone.Npc.CopyFrom, HP: hp, Residents: residents,
						Removes: append([]int32(nil), want.Removes...), Presence: presence, Marker: marker, KeepBalance: !fighter})
			} else if len(level) > 0 || presence != 0 || marker != 0 || fighter {
				manifest.NpcCloneStrength = append(manifest.NpcCloneStrength,
					CloneStrength{Source: clone.Npc.CopyFrom, Residents: level, Presence: presence, Marker: marker, KeepBalance: !fighter})
			}
			if clone.Source.Team != formation.TeamSummon {
				manifest.NpcCloneTeams = append(manifest.NpcCloneTeams,
					CloneTeam{Source: clone.Npc.CopyFrom, Team: uint8(clone.Source.Team)})
			}
			if clone.Source.WorldBranch {
				manifest.NpcCloneWorldBranch = append(manifest.NpcCloneWorldBranch, clone.Npc.CopyFrom)
				if clone.Source.WorldBranchUnheld {
					manifest.NpcCloneWorldBranchUnheld = append(manifest.NpcCloneWorldBranchUnheld, clone.Npc.CopyFrom)
				}
			}
			// The sheathed state is resident on the actor row but named by the
			// think row, so the entry carries both: the writer reads the
			// effect off the world think row and takes it off the clone.
			if formation.WeaponAlwaysOut(clone.Source.Perception) {
				manifest.NpcCloneWeaponOut = append(manifest.NpcCloneWeaponOut,
					CloneWeaponOut{Source: clone.Npc.CopyFrom, Think: clone.Source.Think})
			}
		}
		if clone.Think != nil {
			manifest.NpcThinkCloneSources = append(manifest.NpcThinkCloneSources, clone.Think.CopyFrom)
			if !clone.Source.Perception.Valid() {
				return nil, fmt.Errorf("actor %d asks for perception %q, which is not one of %v",
					clone.Source.Npc, clone.Source.Perception, formation.Perceptions)
			}
			entry := ThinkPerception{Source: clone.Think.CopyFrom, Level: clone.Source.Perception.Name(),
				WorldBranch: clone.Source.WorldBranch && !clone.Source.WorldBranchUnheld,
				NoTurn:      clone.Source.NoTurn}
			// Every level whose reach is measured BEYOND THE BODY needs it:
			// the ladder's Follower, Guard and Sentinel are all "constant +
			// body", like the eyes level.
			if formation.MeasuredFromTheBody(clone.Source.Perception) {
				entry.Body = clone.Source.Body
				entry.Front = clone.Source.Front
			}
			// The share box's think half: a level whose share the player
			// chooses, left on its own senses, fights whoever hits it. The
			// Statue and the Loner fix their own damage rate and never set it.
			if _, choosable, _ := formation.LevelShare(clone.Source.Perception); choosable {
				on, err := formation.ResolveShare(clone.Source)
				if err != nil {
					return nil, fmt.Errorf("actor %d: %w", clone.Source.Npc, err)
				}
				entry.OwnTarget = !on
			}
			manifest.NpcThinkPerception = append(manifest.NpcThinkPerception, entry)
		}
	}

	// A shell on a team is a copy of the shell with the team on it; the copy
	// is what the formation row names (Compile moved the actor there).
	for _, copied := range compiled.Copies {
		manifest.NpcSummonCopies = append(manifest.NpcSummonCopies, copied.CopyFrom)
		manifest.NpcCloneTeams = append(manifest.NpcCloneTeams,
			CloneTeam{Source: copied.CopyFrom, Team: uint8(copied.Source.Team)})
	}

	// A row's ladder is its Ash's (ladderOf), or the characters' for a row
	// summoning one of the player's characters.
	ladderFor := func(actor formation.Actor, ladderOf int32) ([formation.DopingLevels]int32, bool) {
		if formation.IsCharacter(actor.Person) && current.CharacterLadder != [formation.DopingLevels]int32{} {
			return current.CharacterLadder, true
		}
		ash, known := current.Ladders[ladderOf]
		return ash, known
	}
	for _, edit := range compiled.Edits {
		ladder, known := ladderFor(edit.Actor, edit.RowID)
		manifest.BuddyPatches = append(manifest.BuddyPatches,
			filledRow(edit.RowID, compiled.Trigger, edit.Actor, edit.Stance, edit.Place, ladder, known))
	}
	for _, insert := range compiled.Inserts {
		manifest.BuddyRowInsertions = append(manifest.BuddyRowInsertions, BuddyRowInsertion{
			RowID: insert.RowID, CopyFrom: insert.CopyFrom, InsertAfter: insert.InsertAfter,
		})
		ladder, known := ladderFor(insert.Actor, insert.CopyFrom)
		manifest.BuddyPatches = append(manifest.BuddyPatches,
			filledRow(insert.RowID, compiled.Trigger, insert.Actor, insert.Stance, insert.Place, ladder, known))
	}
	for _, rowID := range compiled.Retired {
		values, known := current.Buddy[rowID]
		if !known {
			return nil, fmt.Errorf("row %d is being retired but the save does not hold it: %w", rowID, ErrUnknownRow)
		}
		set := make(map[string]any, len(RestorableBuddyFields))
		for _, name := range RestorableBuddyFields {
			value, present := values[name]
			if !present {
				return nil, fmt.Errorf("row %d has no current %s: %w", rowID, name, ErrUnknownRow)
			}
			set[name] = value
		}
		// A retired row takes its Ash's ladder back, so it never names the
		// characters' ladder after the characters are gone.
		if ash, known := current.Ladders[rowID]; known {
			for level, effect := range ash {
				set[fmt.Sprintf("dopingSpEffect_lv%d", level)] = float64(effect)
			}
		}
		set["triggerSpEffectId"] = InertTrigger
		manifest.BuddyPatches = append(manifest.BuddyPatches, RowPatch{
			RowID: rowID, Name: "Authored: retired surplus row", Set: set,
		})
	}
	return manifest, nil
}

// filledRow is one formation row naming one actor. All four restorable fields
// are written because a row that names an actor without naming its trigger is a
// row in no formation at all, and because the save core validates the set as a
// whole rather than field by field.
//
// The ladder is the Ash's own - the row's as the game ships it, or for an
// inserted row the one of the row it copies - or the characters'. It is
// written whenever it is known, so a ladder an earlier build of this editor
// changed goes back.
func filledRow(rowID, trigger int32, actor formation.Actor, stance formation.Stance,
	place formation.Placement, ladder [formation.DopingLevels]int32, known bool) RowPatch {
	patch := RowPatch{
		RowID: rowID,
		Name:  "Authored: formation row",
		Set: map[string]any{
			"triggerSpEffectId":    int64(trigger),
			"npcParamId":           int64(actor.Npc),
			"npcThinkParamId":      int64(actor.Think),
			"appearOnAroundSekihi": AuthoredAppearOnAroundSekihi,
			"disablePCTargetShare": int64(stance.DisablePCTargetShare),
			"pcFollowType":         int64(stance.PcFollowType),
			"npcPlayerInitParamId": personOf(actor),
			"generateAnimId":       animationOf(actor),
			// Where the actor stands, always stated. An unwritten placement is
			// the template's, and the template's on a single-row host is one
			// point for the whole formation.
			"x_offset": place.X,
			"z_offset": place.RowZ(),
			"y_angle":  place.Angle,
		},
	}
	if known {
		for level, effect := range ladder {
			patch.Set[fmt.Sprintf("dopingSpEffect_lv%d", level)] = int64(effect)
		}
	}
	return patch
}
