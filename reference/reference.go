// Package reference embeds the runtime catalog. The authoritative copy is in
// the author's research tooling, which writes this folder; it is never edited by hand.
package reference

import (
	"bytes"
	"embed"
)

//go:embed catalog-ashes.csv catalog-families.csv catalog-actors.csv catalog-hierarchy.csv think-turns.csv bodies.csv shared-effect-writes.csv spell-copies.csv vanilla.json
var files embed.FS

// ThinkTurnCatalog returns a fresh reader for the embedded turn table: one row
// per world think row with a battle goal, saying whether that goal's battle
// script carries a Turn action ("yes", "no", or "unread" where the corpus
// holds no script for it).
func ThinkTurnCatalog() *bytes.Reader {
	return bytes.NewReader(mustRead("think-turns.csv"))
}

// BodyCatalog returns a fresh reader for the embedded body table: one row per
// creature model, its collision radius and how far it reaches ahead and
// behind.
func BodyCatalog() *bytes.Reader {
	return bytes.NewReader(mustRead("bodies.csv"))
}

// SharedEffectWriteCatalog returns a fresh reader for the embedded shared
// effect writes: one row per field of a shipped effect row an entry needs
// while an Ash holds it.
func SharedEffectWriteCatalog() *bytes.Reader {
	return bytes.NewReader(mustRead("shared-effect-writes.csv"))
}

// SpellCopyCatalog returns a fresh reader for the embedded spell copies: one
// row per player spell with an NPC copy, the Magic row a summoned character
// casts in its place.
func SpellCopyCatalog() *bytes.Reader {
	return bytes.NewReader(mustRead("spell-copies.csv"))
}

// AshCatalog returns a fresh reader for the embedded Spirit Ash catalog.
func AshCatalog() *bytes.Reader {
	return bytes.NewReader(mustRead("catalog-ashes.csv"))
}

// FamilyCatalog returns a fresh reader for the embedded entity families: one
// row per cXXXX model.
func FamilyCatalog() *bytes.Reader {
	return bytes.NewReader(mustRead("catalog-families.csv"))
}

// ActorCatalog returns a fresh reader for the embedded world actors: one row
// per NpcParam row the game ships, each naming the family it belongs to.
func ActorCatalog() *bytes.Reader {
	return bytes.NewReader(mustRead("catalog-actors.csv"))
}

func mustRead(name string) []byte {
	body, err := files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return body
}

// HierarchyCatalog returns a fresh reader for the embedded browse hierarchy:
// one row per reachable signature, carrying the four levels a player navigates
// - category, card, type, variant - plus the npc row to summon and how much
// research measured separating that signature from its nearest sibling.
func HierarchyCatalog() *bytes.Reader {
	return bytes.NewReader(mustRead("catalog-hierarchy.csv"))
}
