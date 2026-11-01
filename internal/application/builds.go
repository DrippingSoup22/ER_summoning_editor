package application

// Characters built from choices (internal/character build.go): what the
// character builder makes, and a console batch's test characters.
//
// A built character is the player's until they delete it. It has a number, 0
// to character.Builds - 1, and is kept in the save whether or not a formation
// names it - person character.BuiltRowOf(n) on body character.BuiltBodyOf(n). A
// build staged under its number writes its rows and its body at Save; a
// deletion staged removes them, and is refused while a formation names the
// character. One the save holds that this session neither rebuilt nor deleted
// stays as it is (characters.go). A reset, which puts the regulation back as
// the game ships it, removes them all.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/DrippingSoup22/ER_shared/regulation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/items"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// What a frontend builds a character from, re-exported so it need not import
// the character package (internal/character build.go says what each means).
type (
	Build        = character.Build
	BuildWeapon  = character.Weapon
	BuildStack   = character.Stack
	BuildChoices = character.Choices
	WeaponChoice = character.WeaponChoice
	AshChoice    = character.AshChoice
	// BuildShortfalls are the weapon and spell requirements a build does not
	// meet (BuildChoices.Shortfalls): the builder warns of them and builds
	// anyway.
	BuildShortfalls = character.Shortfalls
	BuildUnmet      = character.Unmet
)

// The limits and names a builder offers.
const (
	// MaxCustomCharacters is how many custom characters a save holds.
	MaxCustomCharacters = character.Builds
	MaxCarried          = character.MaxCarried
	// MaxNameLength is the longest name a custom character takes.
	MaxNameLength = character.MaxNameLength
	// BuildQuickSlots is how many quick items a character chooses: the
	// first of the ten holds the flask every custom character carries,
	// NPCFlask.
	BuildQuickSlots = character.BuildQuickSlots
	NPCFlask        = character.NPCFlask
	BuildTemplateA  = character.TemplateA
	BuildTemplateB  = character.TemplateB
	// Hand slots are in the save's order, L1 R1 L2 R2 L3 R3.
	LeftHand  = character.LeftHand
	RightHand = character.RightHand
)

var (
	Affinities    = character.Affinities
	ClassNames    = character.ClassNames
	TemplateNames = character.TemplateNames
)

// PlainBuild is a new character: the Warrior template of either body type,
// every attribute at 1, nothing equipped. The builder starts it on a class
// (BuildChoices.Classes).
func PlainBuild(typeA bool) Build { return character.Plain(typeA) }

// StagedBuild is one built character, checked against the loaded save's
// regulation when it was staged.
type StagedBuild struct {
	N     int
	Build character.Build
	// Name and Summary are how the character is named (CustomCharacter).
	Name, Summary string
	// New is true when the save holds no character n: one made rather than
	// one changed.
	New  bool
	rows []character.Row
	hp   int32
}

// CustomCharacter is one built character as the save will hold it: what the
// save holds, with what is staged over it.
type CustomCharacter struct {
	N     int
	Build character.Build
	// Name is the player's name for it or "Custom character 1"
	// (BuildName), and Summary its level and main weapon.
	Name, Summary string
	// Staged is true when a build is staged for it, Deleting when its
	// deletion is.
	Staged, Deleting bool
	// Named is true while a formation the save will hold summons it, which a
	// deletion is refused for.
	Named bool
}

// StageBuild checks a build against the loaded save and stages it as built
// character n, replacing what n was and any deletion of it. It returns nil
// when the save already holds exactly this build, and stages nothing.
func (session *Session) StageBuild(n int, build character.Build) (*StagedBuild, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	loaded, err := regulation.Load(session.loaded.info.Path)
	if err != nil {
		return nil, err
	}
	if digest := sha256.Sum256(loaded.FileBytes); hex.EncodeToString(digest[:]) != session.loaded.info.SHA256 {
		return nil, fmt.Errorf("%s changed since it was loaded: %w", session.loaded.info.Path, ErrSourceMismatch)
	}
	rows, hp, err := build.Rows(n, loaded.BND4)
	if err != nil {
		return nil, err
	}
	delete(session.dropped, n)
	held, holds := session.loaded.builds[n]
	if holds && held == build {
		delete(session.builds, n)
		return nil, nil
	}
	staged := &StagedBuild{N: n, Build: build, Name: BuildName(n, build), Summary: buildSummary(build), New: !holds,
		rows: rows, hp: hp}
	if session.builds == nil {
		session.builds = make(map[int]*StagedBuild)
	}
	session.builds[n] = staged
	return staged, nil
}

// StageBuildDeletion stages removing built character n from the save, which
// is refused while a formation summons it. A character only staged, never
// written, is simply unstaged.
func (session *Session) StageBuildDeletion(n int) error {
	if session.loaded == nil {
		return ErrNoSaveLoaded
	}
	if session.buildNamed(n) {
		return fmt.Errorf("custom character %d: %w", n+1, ErrBuildNamed)
	}
	delete(session.builds, n)
	if _, held := session.loaded.state.CharacterRows[builtRowKey(n)]; held {
		if session.dropped == nil {
			session.dropped = make(map[int]bool)
		}
		session.dropped[n] = true
	}
	return nil
}

// UnstageBuild drops whatever is staged for built character n - a build or a
// deletion - and reports whether anything was.
func (session *Session) UnstageBuild(n int) bool {
	_, built := session.builds[n]
	dropped := session.dropped[n]
	delete(session.builds, n)
	delete(session.dropped, n)
	return built || dropped
}

// CustomCharacters is every built character the save will hold, and every one
// being deleted, by number.
func (session *Session) CustomCharacters() []CustomCharacter {
	if session.loaded == nil {
		return nil
	}
	named := session.namedBuilds()
	numbers := map[int]bool{}
	for n := range session.loaded.builds {
		numbers[n] = true
	}
	for n := range session.builds {
		numbers[n] = true
	}
	var out []CustomCharacter
	for _, n := range slices.Sorted(maps.Keys(numbers)) {
		one := CustomCharacter{N: n, Build: session.loaded.builds[n], Deleting: session.dropped[n], Named: named[n]}
		if staged, ok := session.builds[n]; ok {
			one.Build, one.Staged = staged.Build, true
		}
		one.Name, one.Summary = BuildName(n, one.Build), buildSummary(one.Build)
		out = append(out, one)
	}
	return out
}

// FreeBuild is the lowest number no built character has, held or staged, and
// false when all are taken.
func (session *Session) FreeBuild() (int, bool) {
	if session.loaded == nil {
		return 0, false
	}
	for n := range character.Builds {
		_, held := session.loaded.state.CharacterRows[builtRowKey(n)]
		if _, staged := session.builds[n]; !held && !staged {
			return n, true
		}
	}
	return 0, false
}

// stagedBuilds is every staged build, by number.
func (session *Session) stagedBuilds() []*StagedBuild {
	builds := make([]*StagedBuild, 0, len(session.builds))
	for _, n := range slices.Sorted(maps.Keys(session.builds)) {
		builds = append(builds, session.builds[n])
	}
	return builds
}

// namedBuilds is every built character a formation the save will hold
// summons: the staged formations', and the held ones' they do not rewrite.
func (session *Session) namedBuilds() map[int]bool {
	formations := make([]*plan.Manifest, 0, len(session.formations))
	for _, staged := range session.formations {
		formations = append(formations, staged.manifests...)
	}
	out := map[int]bool{}
	for row := range personsAfter(session.loaded.formations, formations, session.restoration != nil) {
		if n, ok := character.BuildOf(row); ok {
			out[n] = true
		}
	}
	return out
}

func (session *Session) buildNamed(n int) bool { return session.namedBuilds()[n] }

// buildsAfter is every custom character the written save will hold, as the
// character write decides it (characters.go): a staged one, and one the save
// holds that is not being deleted - beside a reset, only while a staged
// formation names it.
func (session *Session) buildsAfter(formations []*plan.Manifest, resetting bool) map[int]bool {
	named := map[int]bool{}
	for row := range personsAfter(session.loaded.formations, formations, resetting) {
		if n, ok := character.BuildOf(row); ok {
			named[n] = true
		}
	}
	out := map[int]bool{}
	for n := range character.Builds {
		_, holds := session.loaded.state.CharacterRows[builtRowKey(n)]
		_, building := session.builds[n]
		if !session.dropped[n] && (building || holds && (!resetting || named[n])) {
			out[n] = true
		}
	}
	return out
}

// heldBuilds is how many built characters the save holds.
func (session *Session) heldBuilds() int {
	count := 0
	for n := range character.Builds {
		if _, held := session.loaded.state.CharacterRows[builtRowKey(n)]; held {
			count++
		}
	}
	return count
}

// builtRowKey is built character n's CharaInitParam row, which says whether
// the save holds the character.
func builtRowKey(n int) savecore.CharacterRowKey {
	return savecore.CharacterRowKey{Table: character.TableCharaInit, RowID: character.BuiltRowOf(n)}
}

// readBuilds reads back every built character the save holds. One whose rows
// do not read as a build - not written by this editor - is left out of the
// list and kept in the save.
func readBuilds(path string, rows map[savecore.CharacterRowKey][]byte) (map[int]character.Build, error) {
	out := map[int]character.Build{}
	any := false
	for n := range character.Builds {
		_, held := rows[builtRowKey(n)]
		any = any || held
	}
	if !any {
		return out, nil
	}
	loaded, err := regulation.Load(path)
	if err != nil {
		return nil, err
	}
	faces, err := character.TemplateFaces(loaded.BND4)
	if err != nil {
		return nil, err
	}
	find := func(table string, id int32) []byte { return rows[savecore.CharacterRowKey{Table: table, RowID: id}] }
	for n := range character.Builds {
		if build, held, err := character.ReadBuild(n, find, faces); held && err == nil {
			out[n] = build
		}
	}
	return out, nil
}

// BuildName is how built character n is called: the name the player gave it,
// else "Custom character 1".
func BuildName(n int, build Build) string {
	if build.Name != "" {
		return build.Name
	}
	return fmt.Sprintf("Custom character %d", n+1)
}

// buildSummary tells built characters apart: "level 27 · Longsword".
func buildSummary(build character.Build) string {
	summary := fmt.Sprintf("level %d", build.Level())
	if weapon, ok := mainWeapon(build); ok {
		if name, ok := itemName(items.Weapon, weapon); ok {
			return summary + " · " + name
		}
	}
	return summary
}

// mainWeapon is the weapon a character leads with: the right hand's first,
// else the first it holds.
func mainWeapon(build character.Build) (int32, bool) {
	for _, hand := range []int{character.RightHand, character.LeftHand, 3, 2, 5, 4} {
		if weapon := build.Hands[hand]; weapon.Base != 0 {
			return weapon.Base, true
		}
	}
	return 0, false
}

// CustomCard is the custom characters' card, in the characters category.
const CustomCard = "custom"

// CustomCharacterCard is the card the custom characters are offered on: one
// type per character the save will hold, in number order, each a person on
// its own body (summary tag: its Summary; class_art: ClassArt).
// It holds no type before the first
// is built, and changes as builds are staged, so it is asked for apart from
// Hierarchy.
func (session *Session) CustomCharacterCard() HierarchyCard {
	card := HierarchyCard{Card: CustomCard, Name: "Custom characters"}
	for _, one := range session.CustomCharacters() {
		if one.Deleting {
			continue
		}
		card.Types = append(card.Types, HierarchyType{Name: one.Name, Entries: []HierarchyEntry{{
			Key: "build:" + strconv.Itoa(one.N), Trunk: "person", Name: "",
			Ships: character.BuiltRowOf(one.N), Model: "c0000", Offer: "ship", Validity: "valid",
			Tags: map[string]string{
				"shell":       strconv.Itoa(int(character.BuiltBodyOf(one.N))),
				"shell_think": strconv.Itoa(int(characterShell)),
				"build":       strconv.Itoa(one.N),
				"summary":     one.Summary,
				"class_art":   ClassArt(one.Build.Class),
			},
		}}})
	}
	return card
}

// BuildChoices is what each weapon can be built as and what each Ash of War
// goes on, read from the loaded save's regulation (internal/character
// ashes.go).
func (session *Session) BuildChoices() (BuildChoices, error) {
	if session.loaded == nil {
		return BuildChoices{}, ErrNoSaveLoaded
	}
	loaded, err := regulation.Load(session.loaded.info.Path)
	if err != nil {
		return BuildChoices{}, err
	}
	return character.ReadChoices(loaded.BND4)
}
