package catalog

// The entity catalog: every actor the game ships, grouped the way a player
// browses them.
//
// Two levels rather than one. A flat list of 6,279 NpcParam rows shows the same
// name a dozen times with nothing to choose between the copies, because most
// rows are one enemy re-tuned per region. The cXXXX model is what a player
// recognises, so that is the browsable level, and the rows sit behind it.
//
// Nothing here knows what a save holds. A family listed here is a family the
// game defines; whether this particular save can summon any of it is a question
// only the loaded save can answer, and it is asked in internal/application.

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// Compatibility is what phase H has found out about summoning a family.
//
// Every family ships as Untested and the value is a reserved column: the whole
// vocabulary is fixed here so that filling one in later is a data change and
// not a format change. Unknown values are refused at load rather than passed
// through, because a verdict a frontend cannot interpret is worse than no
// verdict - it would be shown as if it meant something.
type Compatibility string

const (
	// Untested is the shipping default: nobody has summoned this family yet.
	Untested Compatibility = "untested"
	// Ships means a compatibility sweep summoned it and it behaved.
	Ships Compatibility = "ship"
	// Warns means it works with a caveat the frontend must show.
	Warns Compatibility = "warn"
	// Refused means it is known to break, and the frontend must not offer it.
	Refused Compatibility = "refuse"
)

var compatibilities = map[string]Compatibility{
	string(Untested): Untested, string(Ships): Ships,
	string(Warns): Warns, string(Refused): Refused,
}

// Family is one cXXXX model: the level the player browses.
type Family struct {
	// Model is the family key, "c" followed by NpcParam ID / 10000. It is the
	// only unique one: the DLC ships a second Deer, Basilisk, Rat and
	// Fingercreeper, so a name identifies nothing.
	Model string
	// Name is what to show, and is never empty. It falls back to ExternalName
	// and then to Model, because 24 families carry no Paramdex name at all and
	// a blank tile is not a thing a player can choose.
	Name string
	// ExternalName is the model-level name from the SOTE-aware roster, present
	// on 408 of 445 families. It is kept beside Name rather than folded into it
	// so a frontend can show it as the disambiguator it is.
	ExternalName string
	// Provenance is which game the model belongs to, as research classified it:
	// base_placed, base_unplaced, dlc_c5xxx, dlc_c62xx or unclassified_system.
	Provenance string
	// HitRadius and HitHeight are the largest collision bounds in the family.
	// They are the field public tools cap when converting a large actor, and
	// sorting families by radius reproduces the boss roster by size.
	HitRadius, HitHeight float64
	// Rig is the model this family borrows its animations from, from
	// NpcParam.RetargetReferenceChrId, or "" when it borrows none. It is the
	// game's own and only statement that two model families are built on one
	// skeleton, and so the floor of the identity rule: two families with no rig
	// path between them are never the same creature.
	//
	// It does not settle the other direction. A rig is a skeleton and the game
	// hangs distinct characters on borrowed ones - Gatekeeper Gostoc on the
	// Commoner, Miriel on the Turtle - so a shared rig makes two families
	// candidates and nothing more. See archive/history/editor/IDENTITY.md and curation/entities.csv.
	//
	// It may name a model this catalog does not list: c4310, c4350, c4370 and
	// c4380 are rig templates that no world row stands on, and the soldiers,
	// knights, foot soldiers and diggers all point at one of them.
	Rig           string
	Compatibility Compatibility
	// Artwork is the key this family's image is stored under. Empty until
	// phase J decides where the images come from.
	Artwork string
	// Variants are the family's rows, ordered by ID. Every family has at least
	// one; a family with none would be a browsable tile that opens onto
	// nothing.
	Variants []Variant
}

// Variant is one NpcParam world row: one actor a slot can name.
type Variant struct {
	// Npc is the world row. It is never a summon-side row - those are the
	// game's own clones, counted against the family they copy.
	Npc int32
	// Think is the NpcThinkParam row this actor thinks with, or -1 where
	// research could not pair one. An unpaired actor cannot be summoned,
	// because a formation slot names both.
	Think int32
	// Name is what to show, and is never empty; it falls back to the family's.
	Name string
	// HP is the pre-scaling base value. World strength is partly supplied by
	// area-scaling effects, so this ranks variants against each other and not
	// against anything a player has fought.
	HP int32
	// Granted are the effects the map's event script places on this one actor
	// and no row holds - the Stormhill knight's Bloody Slash is 14469 and
	// 14475. A clone made from the row alone lacks them and behaves as the
	// plain creature; one carrying them behaves as the placed actor, so the
	// author carries them unless a slot asks for the bare row.
	// Empty for 5,988 of the 6,279 rows.
	Granted []int32
	// Removed are resident effects the row carries that its summon must not:
	// a composite that takes one off (the God-Devouring Serpent's 11937, 'Character Respawn', which keeps a
	// summoned Serpent from dying). Empty elsewhere.
	Removed []int32
	// Area is the world-area scaling effect this row carries, in the band
	// 7010-7200, or 0 where it carries none - which 3,351 of the 6,279 rows
	// do. It is the multiplier the region applies to HP and attack, and so it
	// is the difference between the row's numbers and what the player fought.
	//
	// A summon never gets it: internal/savecore strips the band from every
	// clone, exactly as no shipped summon row carries one. It is here so the
	// editor can say what the creature is worth in the world, and so a
	// formation can be scaled to it. See docs/DESIGN.md (Summon strength).
	Area int32
	// Artwork is the key this variant's image is stored under. Empty means it
	// uses its family's, which is the usual case and the reason this cannot
	// just be derived from the row ID.
	Artwork string
	// Class is what research classified this row as, verbatim: Boss, Miniboss,
	// MinorBoss, DragonMiniboss, NightMiniboss, Evergaol, Basic, Helper,
	// Wildlife or Scarab.
	//
	// Empty is a real and common answer - 2,679 of the 6,279 rows carry no
	// class - so a reader must treat it as "not classified" and never as a
	// default bucket. Research's own word is kept rather than a friendlier
	// one: which of these a player would call a boss is a judgement, and it
	// belongs where the grid is drawn, not where provenance is.
	//
	// It also covers the base game ONLY. Research measured zero coverage
	// across all 1,463 DLC rows, so anything built on Class alone files every
	// DLC boss as unclassified. Use Arena for that.
	Class string
	// Arena is the tier of the GameAreaParam arena this row stands in -
	// "great", "major", "minor" or "field" - and empty for a row that stands
	// in no arena at all, which most do not.
	//
	// This is the boss signal, and the reason it exists beside Class: it is
	// derived from the arenas rather than from an external dataset, so it
	// covers the DLC as well as the base game. The regulation has no boss
	// flag - research checked getSoul, teamType and hp and none of them
	// separate a boss from a soldier named after one - and the resident 90400
	// package is a necessary condition rather than a sufficient one: it also
	// lands on Wolf, Imp, Skeleton and Living Jar. An arena is the map saying
	// a fight happens here, which is what boss-ness actually is.
	Arena string
}

// Summonable reports whether the variant is one a formation can name at all.
//
// Two rows fail it, and both are listed anyway because both are part of what
// the game holds: an actor research could not pair a think row for, and the
// NpcParam row numbered 0, which the clone arithmetic has no destination for.
func (v Variant) Summonable() bool {
	if v.Think < 0 {
		return false
	}
	_, err := formation.SummonRow(v.Npc)
	return err == nil
}

// LoadFamilies parses the embedded entity catalog, in the order the catalog
// lists the families - which the author's reference sync tool writes by model number, so
// that a browsable grid keeps the two games in their own blocks.
func LoadFamilies() ([]Family, error) {
	return DecodeFamilies(reference.FamilyCatalog(), reference.ActorCatalog())
}

// DecodeFamilies parses the two halves of the entity catalog and joins them.
//
// Exported for the same reason DecodeAshes is: the author's reference sync tool has to read
// the files it has just written rather than the ones compiled into it.
func DecodeFamilies(families, actors io.Reader) ([]Family, error) {
	records, err := readTable(families, "entity family catalog",
		[]string{"model", "name", "external_name", "provenance", "hit_radius", "hit_height", "rig", "compatibility", "artwork"})
	if err != nil {
		return nil, err
	}
	byModel := make(map[string]*Family, len(records))
	decoded := make([]*Family, 0, len(records))
	for _, record := range records {
		family, err := decodeFamily(record)
		if err != nil {
			return nil, err
		}
		if _, twice := byModel[family.Model]; twice {
			return nil, fmt.Errorf("entity family catalog lists %s twice", family.Model)
		}
		byModel[family.Model] = family
		decoded = append(decoded, family)
	}

	rows, err := readTable(actors, "actor catalog",
		[]string{"npc_id", "think_id", "name", "model", "hp", "area", "class", "arena", "artwork", "granted", "removed"})
	if err != nil {
		return nil, err
	}
	seen := make(map[int32]bool, len(rows))
	for _, record := range rows {
		variant, model, err := decodeVariant(record)
		if err != nil {
			return nil, err
		}
		if seen[variant.Npc] {
			return nil, fmt.Errorf("actor catalog lists row %d twice", variant.Npc)
		}
		seen[variant.Npc] = true
		family, known := byModel[model]
		if !known {
			return nil, fmt.Errorf("actor %d names family %s, which the family catalog does not list", variant.Npc, model)
		}
		if variant.Name == "" {
			variant.Name = family.Name
		}
		family.Variants = append(family.Variants, variant)
	}

	result := make([]Family, 0, len(decoded))
	for _, family := range decoded {
		if len(family.Variants) == 0 {
			return nil, fmt.Errorf("family %s has no rows", family.Model)
		}
		sort.Slice(family.Variants, func(i, j int) bool {
			return family.Variants[i].Npc < family.Variants[j].Npc
		})
		result = append(result, *family)
	}
	return result, nil
}

func decodeFamily(record map[string]string) (*Family, error) {
	model := record["model"]
	if !strings.HasPrefix(model, "c") || model == "c" {
		return nil, fmt.Errorf("entity family catalog holds %q, which is not a cXXXX model", model)
	}
	radius, err := strconv.ParseFloat(record["hit_radius"], 64)
	if err != nil {
		return nil, fmt.Errorf("family %s has hit_radius %q", model, record["hit_radius"])
	}
	height, err := strconv.ParseFloat(record["hit_height"], 64)
	if err != nil {
		return nil, fmt.Errorf("family %s has hit_height %q", model, record["hit_height"])
	}
	verdict, known := compatibilities[record["compatibility"]]
	if !known {
		return nil, fmt.Errorf("family %s has compatibility %q, which is not a verdict this product understands",
			model, record["compatibility"])
	}
	name := record["name"]
	if name == "" {
		name = record["external_name"]
	}
	if name == "" {
		name = model
	}
	return &Family{
		Model: model, Name: name, ExternalName: record["external_name"],
		Provenance: record["provenance"], HitRadius: radius, HitHeight: height,
		Rig: record["rig"], Compatibility: verdict, Artwork: record["artwork"],
	}, nil
}

func decodeVariant(record map[string]string) (Variant, string, error) {
	npc, err := parseInt32(record["npc_id"])
	if err != nil {
		return Variant{}, "", fmt.Errorf("actor catalog npc_id: %w", err)
	}
	// The catalog describes the world. A summon-side row here would be an
	// actor whose clone arithmetic has already been applied, which
	// internal/formation would then not apply again - it would read the row as
	// its own summon and the player would get a different actor than the one
	// they dropped.
	if npc < 0 || npc >= formation.CloneOffset {
		return Variant{}, "", fmt.Errorf("actor catalog holds %d, which is not a world row", npc)
	}
	think, err := parseInt32(record["think_id"])
	if err != nil {
		return Variant{}, "", fmt.Errorf("actor %d think_id: %w", npc, err)
	}
	hp, err := parseInt32(record["hp"])
	if err != nil {
		return Variant{}, "", fmt.Errorf("actor %d hp: %w", npc, err)
	}
	// Empty is the ordinary answer and is not a failure: over half the rows
	// stand in no scaled region at all.
	var area int32
	if record["area"] != "" {
		if area, err = parseInt32(record["area"]); err != nil {
			return Variant{}, "", fmt.Errorf("actor %d area: %w", npc, err)
		}
	}
	var granted []int32
	for _, field := range strings.Split(record["granted"], "|") {
		if field = strings.TrimSpace(field); field != "" {
			id, err := parseInt32(field)
			if err != nil {
				return Variant{}, "", fmt.Errorf("actor %d granted %q: %w", npc, field, err)
			}
			granted = append(granted, id)
		}
	}
	var removed []int32
	for _, field := range strings.Split(record["removed"], "|") {
		if field = strings.TrimSpace(field); field != "" {
			id, err := parseInt32(field)
			if err != nil {
				return Variant{}, "", fmt.Errorf("actor %d removed %q: %w", npc, field, err)
			}
			removed = append(removed, id)
		}
	}
	return Variant{
		Npc: npc, Think: think, Name: record["name"], HP: hp, Area: area,
		Class: record["class"], Arena: record["arena"], Artwork: record["artwork"],
		Granted: granted, Removed: removed,
	}, record["model"], nil
}

// readTable reads a whole CSV into named records, refusing one that is missing
// a column this product reads.
//
// Refusing at load is the same choice parseFormationSizes makes: a catalog with
// the wrong shape is wrong for every request, so saying so once when the file
// arrives beats discovering it per lookup.
func readTable(reader io.Reader, what string, required []string) ([]map[string]string, error) {
	records, err := csv.NewReader(reader).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("%s holds no rows", what)
	}
	header := records[0]
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[name] = index
	}
	for _, name := range required {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf("%s is missing %q", what, name)
		}
	}
	rows := make([]map[string]string, 0, len(records)-1)
	for _, record := range records[1:] {
		row := make(map[string]string, len(required))
		for _, name := range required {
			row[name] = record[columns[name]]
		}
		rows = append(rows, row)
	}
	return rows, nil
}
