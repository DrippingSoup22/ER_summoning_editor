package application

// The player's own characters, summoned as people.
//
// Every filled slot of the loaded save is offered as a person on one card, the
// only card that changes with the save: it is made from the slots Load read,
// which is who the characters are and nothing more. What a character wears is
// read at Save, from the slot as it is then, for the characters a formation
// will name - and copied whole, unjudged (internal/character rows.go). So a
// save opened after a character changed weapons summons them with the new
// ones on the next Save.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/strength"
)

// CustomCategory is the player's own, last in the catalog: two cards, the save's characters on the card below, offered when
// the save has one, and the characters built in the editor (builds.go,
// CustomCharacterCard), always offered, empty until the first is built.
const CustomCategory = "custom"

// The card the save's slots are offered on.
const (
	charactersCard     = "characters"
	charactersCardName = "Your characters"
	// characterShell is the person shell a character's body copies and whose
	// think row it thinks with: Nepheli Loux Puppet's, the shell every person
	// the panel offers rides (ui/gio DefaultPersonShell).
	characterShell = int32(100000020)
)

// characterCard is the card of the loaded save's characters, or false when no
// slot holds one.
func (session *Session) characterCard() (HierarchyCard, bool) {
	card := HierarchyCard{Card: charactersCard, Name: charactersCardName}
	kind := HierarchyType{}
	for _, person := range session.loaded.people {
		if !person.Used {
			continue
		}
		kind.Entries = append(kind.Entries, HierarchyEntry{
			Key:   "slot:" + strconv.Itoa(person.Slot),
			Trunk: "person", Name: CharacterLabel(person),
			Ships: character.RowOf(person.Slot), Model: "c0000", Offer: "ship",
			Validity: "valid",
			// The character's own body, on the shell's think row (PersonShell),
			// pictured as its starting class.
			Tags: map[string]string{
				"shell":       strconv.Itoa(int(character.BodyOf(person.Slot))),
				"shell_think": strconv.Itoa(int(characterShell)),
				"class_art":   ClassArt(slices.Index(character.ClassNames, person.Class)),
			},
		})
	}
	if len(kind.Entries) == 0 {
		return card, false
	}
	card.Types = []HierarchyType{kind}
	return card, true
}

// ClassArt is the picture a character shows, by its starting class: the one drawn for the class's CharaInitParam row, which
// the catalog's Starting class card ships - 3100 the Vagabond, every second
// row on to 3118 the Wretch - or "" for a class the game does not use.
func ClassArt(class int) string {
	if class < 0 || class >= len(character.ClassNames) {
		return ""
	}
	return strconv.Itoa(3100 + 2*class)
}

// CharacterLabel is how a character is told apart from the others: the slot,
// the name, the level and the starting class. Names repeat - a player who
// names every character alike is the ordinary case - so the slot leads.
func CharacterLabel(person CharacterState) string {
	label := fmt.Sprintf("Slot %d · %s · level %d", person.Slot+1, person.Name, person.Level)
	if person.Class != "" {
		label += " · " + person.Class
	}
	return label
}

// charactersPart is the character write, or nil when the save already holds
// what it should. A slot's character a formation will name gets its rows and
// its body built from its slot now, written where they differ from the save's;
// a slot's row no formation names is removed. A slot named but emptied since
// keeps the rows it has: there is nothing to build them from, and they are
// still the character the formation was made with. A built character is kept
// until deleted (builds.go): a staged build is written, a staged deletion
// removed, and one the save holds kept - beside a reset, only while a staged
// formation names it. A body copies its shell, so a role staged for a shell,
// which writes the shell, rewrites every body with it.
func (session *Session) charactersPart(formations []*plan.Manifest, resetting, switching bool) (*plan.Manifest, error) {
	held := session.loaded.state.CharacterRows
	wanted, named := []int{}, map[int]bool{}
	for row := range personsAfter(session.loaded.formations, formations, resetting) {
		if slot, ok := character.SlotOf(row); ok {
			wanted = append(wanted, slot)
		}
		if n, ok := character.BuildOf(row); ok {
			named[n] = true
		}
	}
	if len(wanted)+len(named)+len(session.builds) == 0 && len(held) == 0 {
		return nil, nil
	}
	slices.Sort(wanted)

	var file []byte
	keep := map[savecore.CharacterRowKey]bool{}
	var writes []plan.CharacterWrite
	var bodies []plan.CharacterBody
	// write stages one character's rows and four bodies - the Helper and the
	// Fighter, each in the Puppet coat and in its own colours, whichever its
	// formations name - where they differ from the save's. The Helper carries
	// 2.7x the character's health, as a creature's Helper does (strength.go).
	write := func(rows []character.Row, body plan.CharacterBody, name string) error {
		for _, row := range rows {
			key := savecore.CharacterRowKey{Table: row.Table, RowID: row.ID}
			keep[key] = true
			current, exists := held[key]
			if exists && bytes.Equal(current, row.Data) {
				continue
			}
			writes = append(writes, plan.CharacterWrite{Table: row.Table, RowID: row.ID, Data: row.Data, Insert: !exists})
		}
		shell, ok := session.loaded.state.PersonShells[characterShell]
		if !ok {
			return fmt.Errorf("the save holds no person shell %d: %w", characterShell, ErrRowNotFound)
		}
		for at := range character.Bodies {
			one := body
			one.OwnColours, one.Fighter = at&1 != 0, at&2 != 0
			one.RowID, one.Effects = character.BodyFor(body.RowID, one.OwnColours, one.Fighter), slices.Clone(body.Effects)
			if !one.Fighter {
				one.HP = strength.Helper(body.HP)
			}
			key := savecore.CharacterRowKey{Table: "NpcParam", RowID: one.RowID}
			keep[key] = true
			wantedBody, err := savecore.CharacterBody(shell, one.HP, one.Effects, one.OwnColours, one.Fighter)
			if err != nil {
				return fmt.Errorf("%s's body: %w", name, err)
			}
			current, exists := held[key]
			if exists && !switching && bytes.Equal(current, wantedBody) {
				continue
			}
			one.Insert = !exists
			bodies = append(bodies, one)
		}
		return nil
	}
	for n := range character.Builds {
		_, holds := held[builtRowKey(n)]
		staged, building := session.builds[n]
		switch {
		case session.dropped[n] && named[n]:
			return nil, fmt.Errorf("custom character %d is being deleted: %w", n+1, ErrBuildNamed)
		case session.dropped[n]:
			// Its rows are not kept, so they are removed below.
		case building:
			if err := write(staged.rows, builtBody(n, staged.hp), fmt.Sprintf("custom character %d", n+1)); err != nil {
				return nil, err
			}
		case holds && (!resetting || named[n]):
			for key := range held {
				if belongsTo(key, character.BuiltRowOf(n), character.BuiltBodyOf(n)) {
					keep[key] = true
				}
			}
			// The body is written again where it is not what it should be: a
			// role staged for its shell, or one written before free casting.
			if build, read := session.loaded.builds[n]; read {
				if err := write(nil, builtBody(n, build.HP()), fmt.Sprintf("custom character %d", n+1)); err != nil {
					return nil, err
				}
			}
		case named[n]:
			return nil, fmt.Errorf("a formation names custom character %d, which is neither staged nor in the save: %w", n+1, ErrRowNotFound)
		}
	}
	for _, slot := range wanted {
		if !session.loaded.people[slot].Used {
			for key := range held {
				if belongsTo(key, character.RowOf(slot), character.BodyOf(slot)) {
					keep[key] = true
				}
			}
			continue
		}
		if file == nil {
			read, err := session.readLoadedFile()
			if err != nil {
				return nil, err
			}
			file = read
		}
		loadout, err := character.ReadLoadout(file, slot)
		if err != nil {
			return nil, err
		}
		// What is active on the character, and free casting: a summoned
		// character spends no FP.
		body := plan.CharacterBody{RowID: character.BodyOf(slot), Shell: characterShell,
			HP: int32(loadout.HP), Effects: append(slices.Clone(loadout.Effects), savecore.FreeCasting)}
		if err := write(loadout.Rows(slot), body, session.loaded.people[slot].Name); err != nil {
			return nil, err
		}
	}
	var removals []plan.CharacterRowKey
	for key := range held {
		if !keep[key] {
			removals = append(removals, plan.CharacterRowKey{Table: key.Table, RowID: key.RowID})
		}
	}
	if len(writes)+len(bodies)+len(removals) == 0 {
		return nil, nil
	}
	return plan.BuildCharacters(session.loaded.info.SHA256, writes, bodies, removals)
}

// builtBody is custom character n's body: its health, and free casting
// resident (savecore.FreeCasting) - a summoned character spends no FP.
func builtBody(n int, hp int32) plan.CharacterBody {
	return plan.CharacterBody{RowID: character.BuiltBodyOf(n), Shell: characterShell, HP: hp,
		Effects: []int32{savecore.FreeCasting}}
}

// isOwnBody reports whether an actor is a character on one of its own four
// bodies, which the character write builds at Save and the save need not hold
// yet.
func isOwnBody(actor formation.Actor) bool {
	body, _, _, ok := character.BodyAt(actor.Npc)
	return ok && body == ownBodyOf(actor.Person)
}

// ownBodyOf is a character's first body - the Helper in its Puppet coat - by
// its CharaInitParam row: zero for a person who is no character of the
// player's.
func ownBodyOf(person int32) int32 {
	if slot, ok := character.SlotOf(person); ok {
		return character.BodyOf(slot)
	}
	if n, ok := character.BuildOf(person); ok {
		return character.BuiltBodyOf(n)
	}
	return 0
}

// belongsTo reports whether a character row is one character's: its
// CharaInitParam and FaceParam row at first, a custom weapon at first plus a
// hand, or its body.
func belongsTo(key savecore.CharacterRowKey, first, body int32) bool {
	if key.Table == "NpcParam" {
		first = body
	}
	return key.RowID >= first && key.RowID < first+10
}

// readLoadedFile reads the loaded save again, for what Load did not: what a
// character wears. It must still be the file every staged change was planned
// against.
func (session *Session) readLoadedFile() ([]byte, error) {
	file, err := os.ReadFile(session.loaded.info.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", session.loaded.info.Path, err)
	}
	if digest := sha256.Sum256(file); hex.EncodeToString(digest[:]) != session.loaded.info.SHA256 {
		return nil, fmt.Errorf("%s changed since it was loaded: %w", session.loaded.info.Path, ErrSourceMismatch)
	}
	return file, nil
}
