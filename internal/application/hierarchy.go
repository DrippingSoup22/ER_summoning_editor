package application

import (
	"fmt"
	"strconv"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// The browse hierarchy, narrowed to what the loaded save can deliver: the
// category read per entry from the game files, the card the model family, the
// type and the variant the collapse - the grouping research decided, and what
// the spirits panel builds from.

// HierarchyCategory and the levels under it are the catalog's types, re-exported
// so a frontend naming one does not have to import the catalog package.
type (
	HierarchyCategory = catalog.HierarchyCategory
	HierarchyCard     = catalog.HierarchyCard
	HierarchyType     = catalog.HierarchyType
	HierarchyEntry    = catalog.HierarchyEntry
)

// AshesCategory is the category of the game's own Spirit Ashes: every creature an Ash
// the game ships holds, one card each, delivered AS SHIPPED - FromSoftware's
// own rows, the Ash's own stance, a person on the Ash's own shell. The panel
// opens on it.
const AshesCategory = "ashes"

// AsShipped reports whether an entry is delivered exactly as the game ships
// it: an Ashes card. Its slot names FromSoftware's summon-side rows, which are
// never written, on the stance ShippedStance reads.
//
// FROMSOFTWARE'S ROW IS THE FOLLOWER: the product's Follower level was built
// as a copy of what FromSoftware ships its Ashes on, so an Ashes card is a
// creature on the Follower in its band-1 row, and nothing is written for it.
// Another level on it needs a second row of the creature (ErrAshLevelOwed).
func AsShipped(entry HierarchyEntry) bool {
	return entry.Tags["category"] == AshesCategory
}

// AshShare is the share an Ashes card's Follower resolves to so that it agrees
// with the stance its Ash ships: off where the Ash does not take the player's
// target (the wanderers), the Follower's own default otherwise.
func AshShare(stance Stance) Share {
	if stance.DisablePCTargetShare == 1 {
		return formation.ShareOff
	}
	return formation.ShareDefault
}

// ashLevelsBuilt refuses a level the product cannot yet deliver on a creature
// row FromSoftware ships as an Ash: that row is the Follower and is never
// written. PerceptionAsHeld - a slot re-staged as the file holds it - is not
// a level and is let through. A Fighter is refused on every row that is not
// cloned, which keeps the strength it has (ErrHeldRowStrength).
func ashLevelsBuilt(slots []AuthoredSlot) error {
	snapshot, err := reference.Vanilla()
	if err != nil {
		return err
	}
	shipped := make(map[int32]bool, len(snapshot.NpcCloneRows))
	for _, row := range snapshot.NpcCloneRows {
		shipped[row] = true
	}
	for index, slot := range slots {
		actor := slot.resolve()
		if actor.Fighter && (actor.Person != 0 || actor.Npc >= formation.CloneOffset) {
			return fmt.Errorf("slot %d, row %d: %w", index+1, actor.Npc, ErrHeldRowStrength)
		}
		if actor.Person != 0 || !shipped[actor.Npc] {
			continue
		}
		if actor.Perception != PerceptionFollower && actor.Perception != PerceptionAsHeld {
			return fmt.Errorf("slot %d, row %d, level %q: %w", index+1, actor.Npc, actor.Perception.Name(), ErrAshLevelOwed)
		}
	}
	return nil
}

// ShippedStance is the stance an Ashes card's Ash ships it on (the follow_type
// and share_off tags); ok is false for any other entry, or one that does not
// carry them.
func ShippedStance(entry HierarchyEntry) (Stance, bool) {
	follow, errFollow := strconv.Atoi(entry.Tags["follow_type"])
	share, errShare := strconv.Atoi(entry.Tags["share_off"])
	if !AsShipped(entry) || errFollow != nil || errShare != nil {
		return Stance{}, false
	}
	return Stance{PcFollowType: follow, DisablePCTargetShare: share}, true
}

// PersonShell is the shell a person entry names and the shell's think row
// (the shell and shell_think tags, an Ashes person's own); own is false when
// the entry names none and rides the frontend's default shell.
func PersonShell(entry HierarchyEntry) (shell, think int32, own bool, err error) {
	if entry.Tags["shell"] == "" {
		return 0, 0, false, nil
	}
	s, errShell := strconv.ParseInt(entry.Tags["shell"], 10, 32)
	t, errThink := strconv.ParseInt(entry.Tags["shell_think"], 10, 32)
	if errShell != nil || errThink != nil || s <= 0 || t <= 0 {
		return 0, 0, false, fmt.Errorf("entry %s names an unreadable shell %q / %q", entry.Key, entry.Tags["shell"], entry.Tags["shell_think"])
	}
	return int32(s), int32(t), true, nil
}

// Hierarchy returns the browse hierarchy as the loaded save can deliver it.
//
// A creature entry whose npc row or think row the save does not carry, or
// whose summon-side twin is out of range, is dropped; a person entry is kept
// as long as the save carries a person shell (every 1.17 save does), because
// the pointer reaches any CharaInitParam row. A held entry
// - offer "hold" - is dropped here too: it ships nothing, and a frontend has
// nothing to draw for it. A type or a card left with nothing under it goes
// with it: an empty tile is worse than an absent one, because a player cannot
// tell it from a bug.
//
// A fresh slice every call, like Entities and Ashes: a frontend holds this for
// as long as a save is loaded and must not be able to edit the service's copy.
func (session *Session) Hierarchy() ([]HierarchyCategory, error) {
	if !session.Loaded() {
		return nil, ErrNoSaveLoaded
	}
	whole, err := catalog.ReadHierarchy(reference.HierarchyCatalog())
	if err != nil {
		return nil, err
	}
	read := session.loaded.formations

	out := make([]HierarchyCategory, 0, len(whole))
	for _, category := range whole {
		kept := HierarchyCategory{Name: category.Name}
		for _, card := range category.Cards {
			keptCard := HierarchyCard{Card: card.Card, Name: card.Name, Artwork: card.Artwork}
			for _, one := range card.Types {
				keptType := HierarchyType{Name: one.Name}
				for _, entry := range one.Entries {
					if entry.Offer == "hold" || entry.Ships == 0 {
						continue
					}
					switch entry.Trunk {
					case "creature":
						if !read.NpcRows[entry.Ships] || entry.Think <= 0 || !read.NpcThinkRows[entry.Think] {
							continue
						}
						// A summon is built by cloning into the summon-side
						// twin of the world row, so a row whose twin is out of
						// range can never be delivered however complete the
						// catalog is.
						if _, err := formation.SummonRow(entry.Ships); err != nil {
							continue
						}
					case "person":
						// A person rides one of the game's own shells; the
						// shell's rows are summon-side already and every 1.17
						// save carries them. The pointer reaches any row. An
						// Ashes person names its own shell (the shell tag) and
						// is kept only where the save holds it.
						if shell, think, own, err := PersonShell(entry); err != nil {
							return nil, err
						} else if own && (!read.NpcRows[shell] || !read.NpcThinkRows[think]) {
							continue
						}
					default:
						continue
					}
					keptType.Entries = append(keptType.Entries, entry)
				}
				if len(keptType.Entries) > 0 {
					keptCard.Types = append(keptCard.Types, keptType)
				}
			}
			if len(keptCard.Types) > 0 {
				kept.Cards = append(kept.Cards, keptCard)
			}
		}
		if len(kept.Cards) > 0 {
			out = append(out, kept)
		}
	}
	// The player's own characters last, in one category: the save's, then
	// those built in the editor, whose card the frontend puts in after it
	// (CustomCharacterCard).
	custom := HierarchyCategory{Name: CustomCategory}
	if card, any := session.characterCard(); any {
		custom.Cards = append(custom.Cards, card)
	}
	return append(out, custom), nil
}
