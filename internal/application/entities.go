package application

// What to call the actor rows the loaded save can summon.
//
// internal/catalog knows what the game defines. Only a save says which of those
// rows this player's regulation actually has - a save from someone without the
// DLC is missing 1,463 of them - and only the save says whether an actor
// already has a summon-side twin.

import (
	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
)

// summonable reports whether the loaded save can summon variant, and what to
// call the row if it can.
func (session *Session) summonable(family catalog.Family, variant catalog.Variant) (name string, summon int32, shipped, ok bool) {
	read := session.loaded.formations
	if !variant.Summonable() || !read.NpcRows[variant.Npc] || !read.NpcThinkRows[variant.Think] {
		return "", 0, false, false
	}
	// SummonRow cannot fail here: Summonable already asked it.
	summon, _ = formation.SummonRow(variant.Npc)
	name = DisplayName(variant.Name)
	if name == "" {
		name = family.Name
	}
	return name, summon, read.NpcRows[summon], true
}

// ActorLabel is what to call one actor row: its cleaned display name and the
// family it belongs to.
type ActorLabel struct {
	Name, Family string
	// Model is the family's model id, "c5170". Family is the family's NAME and
	// cannot stand in for it: the curated lists in hazard.go are keyed by
	// model, and two families can share a name.
	Model string
}

// ActorLabels names every actor row this save can summon, under both the
// world row an authored slot carries and the summon-side row a shipped
// formation names: a formation the game ships names the clone, an authored
// one the world row, and a display of what a save holds has to label both.
func (session *Session) ActorLabels() (map[int32]ActorLabel, error) {
	if !session.Loaded() {
		return nil, ErrNoSaveLoaded
	}
	labels := make(map[int32]ActorLabel, len(session.service.families)*4)
	for _, family := range session.service.families {
		for _, variant := range family.Variants {
			name, summon, _, ok := session.summonable(family, variant)
			if !ok {
				continue
			}
			label := ActorLabel{Name: name, Family: family.Name, Model: family.Model}
			for _, row := range [...]int32{variant.Npc, summon} {
				if _, known := labels[row]; !known {
					labels[row] = label
				}
			}
		}
	}
	return labels, nil
}
