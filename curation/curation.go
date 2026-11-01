// Package curation holds the judgements this product makes about the game,
// hand-written and not derived from anything.
//
// It is the opposite of reference/. That directory is written out of the
// research repository by its build, through the author's reference sync tool: every byte of
// it comes from a research file, and editing it by hand is pointless because
// the next build overwrites the edit. This directory
// is never written by any tool. It is edited by a person, and the reason it is
// its own package rather than two more columns in the projected catalog is
// exactly that: the two must not be able to be confused for one another.
//
// What lives here is a question the regulation cannot answer. "Is this a boss"
// is the first of them. Research established that there is no boss flag in the
// game data at all - getSoul is zero on Malenia, Maliketh, Morgott and Radagon,
// teamType does not separate them, and the rows carrying boss runes and
// boss-sounding names are the soldiers named after them - so boss-ness is a
// map and design property, and where the map is ambiguous somebody has to
// decide. This is where the deciding is written down.
package curation

import (
	"bytes"
	"embed"
	"encoding/csv"
	"fmt"
	"io"
)

//go:embed entities.csv pictures.csv covers.csv
var files embed.FS

// A curated row says two things about what it is, and they are two because
// they answer two different questions: what a thing IS (Kind) and where a
// player meets it. One column for both mixes tiers with kinds - a donkey filed
// as a person, a hostile shaman among the NPCs - and cannot hold people built
// from `CharaInitParam` rather than from a `cXXXX` model.

// Kind is what a thing IS. It is the axis that does not move: adding people to
// the catalog adds rows with Kind Person, not a new vocabulary.
//
// It is also the merge rule. Two rows of different Kind may never share a card
// - see SeparateFrom.
type Kind string

const (
	// Creature is an animal, a monster, or a person-shaped thing the game
	// builds as an enemy model rather than as a character.
	Creature Kind = "creature"
	// Person is somebody met by name. Gatekeeper Gostoc is built on the
	// Commoner, Miriel on the Turtle, Smithing Master Iji on the Troll: the
	// game says they share a rig, which is exactly what wearing a costume
	// means, and none of them is a variant of the creature underneath.
	Person Kind = "person"
	// Construct is a machine or an emplacement: the ballista, the trebuchet,
	// the dragon-faced flamethrower, and the pair of eyes on a basilisk statue
	// that blights anything in sight.
	Construct Kind = "construct"
	// Scenery is what is in the actor tables without being an actor: the
	// dummies the engine keeps for bullets and caravans, and the corpse models
	// a boss leaves behind.
	Scenery Kind = "scenery"
)

var kinds = map[string]Kind{
	string(Creature): Creature, string(Person): Person,
	string(Construct): Construct, string(Scenery): Scenery,
}

// Standing is what the game DOES with a thing: a ladder from most of a fight
// to least of it.
type Standing string

const (
	Boss     Standing = "boss"
	Miniboss Standing = "miniboss"
	Enemy    Standing = "enemy"
	// Passive is placed but not hostile: wildlife, merchants, quest
	// characters, and the horses, which are ridden into a fight without being
	// the thing that fights.
	Passive Standing = "passive"
	// Inert is not an actor at all. It is separate from Delivers, which is a
	// tested verdict about one summon; this is a judgement about what the row
	// is.
	Inert Standing = "inert"
)

var standings = map[string]Standing{
	string(Boss): Boss, string(Miniboss): Miniboss, string(Enemy): Enemy,
	string(Passive): Passive, string(Inert): Inert,
}

// SeparateFrom reports whether two kinds may never share a card.
//
// Any difference separates. A person is not a variant of the creature whose
// skeleton they borrow, and neither is a machine: merging either would put the
// thing in a list it does not belong to and take a filter away from whoever
// went looking for it.
//
// It is a rule about the FILE and not about the game data. The game is happy
// to say Gostoc and the Commoner animate on one skeleton, and it is right.
func SeparateFrom(a, b Kind) bool { return a != b }

// Entity is one curated family.
type Entity struct {
	// Model is the family key and the join. A name identifies nothing - the
	// DLC ships a second Deer, Basilisk, Rat and Fingercreeper.
	Model string
	// Kind is what this thing is, and Standing is what the game does with it.
	// Two columns rather than one: see the note above Kind.
	Kind     Kind
	Standing Standing
	// Family is the model this one is browsed under: one card in the spirits
	// catalog for every model that names it, with all their movesets behind
	// it. Empty means this family is its own card.
	//
	// It is not SameAs. SameAs is about a picture and Family is about a card,
	// and the four Fingercreepers show why both are needed: they are one card,
	// and a 5m Giant Fingercreeper does not have to be drawn with the same
	// picture as a 0.8m Small one. Every SameAs row is also a Family row -
	// nothing shares an image without sharing a card - and 16 Family rows are
	// not SameAs rows.
	//
	// One hop only, and the loader enforces it: a family named here must not
	// itself name one. Chains would let a card's identity depend on the order
	// two unrelated edits landed in.
	Family string
	// SameAs names the model this family is another version of: the same
	// creature under a second model number, which the game does whenever the
	// DLC re-issues a base enemy and whenever one enemy needs two skeletons.
	// Empty means this family stands for itself.
	//
	// The spirits panel reads it for ONE thing: which model's portrait a
	// family borrows. It does not merge the browsable list - the four
	// Fingercreepers are still four cards, sharing one picture - and
	// TestSharingAPictureMeansSharingACard is what holds that line.
	//
	// It is also not the same relation as catalog.Family.Rig, which is the
	// game's own statement that two models animate on one skeleton. The rig is
	// within a generation and this is across one: c5551 retargets onto c5550
	// and shares its picture with c4240. 84 of the 98 rows here leave their
	// rig cluster that way. See archive/history/editor/IDENTITY.md.
	SameAs string
	// Card is what a merged card is CALLED, on the root row only.
	//
	// A card's name is normally derived: the longest run of words its members
	// share, so six Soldiers become "Soldier" without anybody writing it down.
	// That derivation has nothing to say about a card whose members share no
	// words - Flying Dragon, Decaying Ekzykes, Elder Dragon Greyoll, Jagged
	// Peak Drake are all one creature and the words do not overlap - and it
	// gives the wrong answer where they share the wrong ones: Death Bird and
	// Death Rite Bird would derive "Bird".
	//
	// So a card that the words cannot name says its own name here, and that
	// name wins outright. Empty is the common case and keeps the derivation.
	// A member row must leave it empty: the loader refuses a name on a row
	// that is browsed under something else, because it would never be read.
	Card string
	// Delivers is "no" for a family that was authored into a slot and did not
	// arrive: no model, nothing to collide with, and an enemy alongside it
	// neither attacked nor was attacked. Empty means nothing is known against
	// it, which is the case for all but a handful.
	//
	// The panel drops these entirely rather than offering a card that cannot
	// summon anything. That is a stronger claim than any other column here
	// makes, so it is only ever written from a seen test, and the reason
	// belongs in Note beside it.
	//
	// It exists because catalog.Family.Compatibility, which is where a verdict
	// like this belongs, lives in reference/ - projected out of research and
	// never hand-edited. Until research carries the result, curation does.
	Delivers string
	// Note is for the reader, and for the curator six months later. Nothing
	// reads it.
	Note string
}

// Load parses the embedded curation file.
func Load() ([]Entity, error) { return Decode(bytes.NewReader(mustRead("entities.csv"))) }

// Decode parses a curation file. Exported so a test can hand it a bad one and
// check that it is refused, without writing a file to do it.
func Decode(source io.Reader) ([]Entity, error) {
	records, err := csv.NewReader(source).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse the curation file: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("the curation file holds no rows")
	}
	columns := make(map[string]int, len(records[0]))
	for index, name := range records[0] {
		columns[name] = index
	}
	// name and provenance are in the file for the person editing it - "c5410"
	// is not something anybody can make a judgement about - and are read by
	// nothing. They are still required, so that a file which has lost them
	// fails here rather than becoming unreadable by hand.
	for _, needed := range []string{"model", "name", "provenance", "kind", "standing", "family", "card", "same_as", "delivers", "note"} {
		if _, held := columns[needed]; !held {
			return nil, fmt.Errorf("the curation file is missing %q", needed)
		}
	}

	entities := make([]Entity, 0, len(records)-1)
	seen := make(map[string]bool, len(records)-1)
	for _, record := range records[1:] {
		model := record[columns["model"]]
		if model == "" {
			return nil, fmt.Errorf("the curation file holds a row with no model")
		}
		if seen[model] {
			return nil, fmt.Errorf("the curation file lists %s twice", model)
		}
		seen[model] = true
		kind, known := kinds[record[columns["kind"]]]
		if !known {
			return nil, fmt.Errorf("%s is curated as kind %q, which is not a kind",
				model, record[columns["kind"]])
		}
		standing, ranked := standings[record[columns["standing"]]]
		if !ranked {
			return nil, fmt.Errorf("%s is curated as standing %q, which is not a standing",
				model, record[columns["standing"]])
		}
		delivers := record[columns["delivers"]]
		if delivers != "" && delivers != "no" {
			return nil, fmt.Errorf("%s says it delivers %q; the only thing that "+
				"column ever says is \"no\"", model, delivers)
		}
		entities = append(entities, Entity{
			Model: model, Kind: kind, Standing: standing, Family: record[columns["family"]],
			Card: record[columns["card"]], SameAs: record[columns["same_as"]],
			Delivers: delivers, Note: record[columns["note"]],
		})
	}
	if err := checkFamilies(entities); err != nil {
		return nil, err
	}
	return entities, nil
}

// checkFamilies holds the two rules that make a merged card well defined: the
// model a row is browsed under has to exist, and it has to be a card itself.
//
// The second is the one worth stating. Allowing A -> B -> C would mean a card's
// membership depended on how far a reader chose to follow the column, and two
// edits made months apart could quietly build a chain nobody intended. One hop
// makes the file say exactly what it means: every row either IS a card or names
// the one card it belongs to.
func checkFamilies(entities []Entity) error {
	known := make(map[string]Entity, len(entities))
	for _, entity := range entities {
		known[entity.Model] = entity
	}
	for _, entity := range entities {
		if entity.Family == "" {
			continue
		}
		if entity.Family == entity.Model {
			return fmt.Errorf("%s is browsed under itself", entity.Model)
		}
		root, listed := known[entity.Family]
		if !listed {
			return fmt.Errorf("%s is browsed under %s, which the curation file does not list",
				entity.Model, entity.Family)
		}
		if root.Family != "" {
			return fmt.Errorf("%s is browsed under %s, which is itself browsed under %s",
				entity.Model, entity.Family, root.Family)
		}
		if entity.Card != "" {
			return fmt.Errorf("%s names the card %q and is itself browsed under %s: "+
				"only the row that IS a card can name one",
				entity.Model, entity.Card, entity.Family)
		}
		if SeparateFrom(entity.Kind, root.Kind) {
			return fmt.Errorf("%s is a %s and is browsed under %s, which is a %s: "+
				"two kinds never share a card",
				entity.Model, entity.Kind, entity.Family, root.Kind)
		}
	}
	return nil
}

func mustRead(name string) []byte {
	body, err := files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return body
}
