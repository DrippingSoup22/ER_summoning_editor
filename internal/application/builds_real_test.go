package application_test

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/character"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// A custom character is the player's until they delete it: written by itself,
// with no formation naming it - a CharaInitParam row, a FaceParam row and two
// bodies with the Vigor's health and free casting, one in the Puppet coat and
// one in its own colours, whose effect rows the save then holds - read back as
// it was built, name and all, and kept by later saves; its look is the body a
// formation names, and the row naming it climbs the characters' ladder until a
// creature takes its place; a deletion is refused while a formation summons it
// and removes every row once none does, the effect rows with the last character.
func TestACustomCharacterIsKeptUntilDeleted(t *testing.T) {
	t.Parallel()
	const shell, godrickSoldier = int32(100000020), int32(43110000)
	directory := t.TempDir()
	choices := character.Plain(false)
	choices.Name = "Rennala's Pupil"
	choices.Attributes[0], choices.Attributes[3] = 40, 99
	choices.Hands[character.RightHand] = character.Weapon{Base: 2000000, Affinity: 1}
	choices.Quick[0] = character.Stack{Row: 300, Count: 5}

	var state *savecore.State
	save := func(session *application.Session, name string) (string, map[savecore.CharacterRowKey][]byte) {
		t.Helper()
		path := filepath.Join(directory, name)
		if _, err := session.Commit(context.Background(), mustPrepare(t, session), path, ""); err != nil {
			t.Fatal(err)
		}
		read, err := savecore.ReadState(context.Background(), path, nil)
		if err != nil {
			t.Fatal(err)
		}
		state = read
		return path, read.CharacterRows
	}

	session := loadedSession(t, restorableSavePath(t))
	if _, err := session.StageBuild(3, choices); err != nil {
		t.Fatal(err)
	}
	built, rows := save(session, "built.dat")
	ownBody := character.BodyFor(character.BuiltBodyOf(3), true, false)
	body, ok := rows[savecore.CharacterRowKey{Table: "NpcParam", RowID: character.BuiltBodyOf(3)}]
	own, twin := rows[savecore.CharacterRowKey{Table: "NpcParam", RowID: ownBody}]
	fighter, alone := rows[savecore.CharacterRowKey{Table: "NpcParam", RowID: character.BodyFor(character.BuiltBodyOf(3), false, true)}]
	if len(rows) != 2+character.Bodies || !ok || !twin || !alone {
		t.Fatalf("the custom character is %d rows (body %v, own colours %v, Fighter %v)", len(rows), ok, twin, alone)
	}
	// NpcParam.hp: Vigor 40 is 1450, the Fighter's; the Helper carries 2.7x
	// it and the balance package 296000, the Fighter neither.
	if helperHP, fighterHP := binary.LittleEndian.Uint32(body[36:]), binary.LittleEndian.Uint32(fighter[36:]); helperHP != 3915 || fighterHP != 1450 {
		t.Errorf("the Helper carries %d health and the Fighter %d, want 3915 and Vigor 40's 1450", helperHP, fighterHP)
	}
	if !state.OwnEffects[savecore.FreeCasting] || !state.OwnEffects[savecore.OwnColoursPresence] ||
		!slices.Contains(state.OwnEffectNames[character.BuiltBodyOf(3)], savecore.FreeCasting) ||
		!slices.Contains(state.OwnEffectNames[ownBody], savecore.FreeCasting) ||
		!slices.Contains(state.OwnEffectNames[ownBody], savecore.OwnColoursPresence) {
		t.Errorf("the bodies name %v and %v", state.OwnEffectNames[character.BuiltBodyOf(3)], state.OwnEffectNames[ownBody])
	}
	// NpcParam's resident effects, spEffectID0-31: the Puppets' coat (295200)
	// and particles (295201) on the one body, neither on the other.
	residents := func(row []byte) []int32 {
		var out []int32
		for _, run := range [][2]int{{76, 8}, {376, 8}, {488, 16}} {
			for slot := range run[1] {
				out = append(out, int32(binary.LittleEndian.Uint32(row[run[0]+4*slot:])))
			}
		}
		return out
	}
	if puppet, coloured := residents(body), residents(own); !slices.Contains(puppet, 295200) || !slices.Contains(puppet, 295201) ||
		slices.Contains(coloured, 295200) || slices.Contains(coloured, 295201) {
		t.Errorf("the puppet body carries %v and the own-colours body %v", puppet, coloured)
	}
	if !slices.Contains(residents(body), 296000) || slices.Contains(residents(fighter), 296000) {
		t.Errorf("the Helper carries %v and the Fighter %v", residents(body), residents(fighter))
	}

	// The Cerulean Hidden Tear for good: no FP for spells and skills, items
	// still used up (the one flask stays one drink), no glow.
	loaded, err := regulation.Load(built)
	if err != nil {
		t.Fatal(err)
	}
	effects, effectRows, err := regulation.LoadRowsByID(loaded.BND4, "SpEffectParam.param", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	effect := effects[effectRows[savecore.FreeCasting].DataOffset:]
	f32 := func(offset int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(effect[offset:])) }
	if f32(740) != 0 || f32(744) != 0 || f32(752) != 0 || f32(900) != 1 || f32(8) != -1 ||
		int32(binary.LittleEndian.Uint32(effect[368:])) != -1 {
		t.Errorf("free casting costs arts %v, sorcery %v, incantation %v, items %v, lasts %v",
			f32(740), f32(744), f32(752), f32(900), f32(8))
	}

	reopened := loadedSession(t, built)
	if listed := reopened.CustomCharacters(); len(listed) != 1 || listed[0].N != 3 || listed[0].Build != choices ||
		listed[0].Name != choices.Name {
		t.Fatalf("the reopened save lists %+v", listed)
	}
	if count := reopened.PendingCount(); count != 0 {
		t.Errorf("reopening staged %d changes", count)
	}
	summoned := application.AuthoredSlot{Npc: character.BuiltBodyOf(3), Think: shell, Person: character.BuiltRowOf(3),
		Look: application.LookOwnColours}
	staged, err := reopened.StageFormation(authoringHost, []application.AuthoredSlot{summoned, summoned})
	if err != nil {
		t.Fatal(err)
	}
	if npc := staged.Formation.Slots[0].Npc; npc != ownBody {
		t.Errorf("the character in its own colours stands on %d, want %d", npc, ownBody)
	}
	named, rows := save(reopened, "named.dat")
	if len(rows) != 2+character.Bodies {
		t.Errorf("summoning the character left %d of its rows", len(rows))
	}
	// The rows summoning the character climb the characters' ladder: a share
	// of its health and damage, all of it at +10.
	blockIn := func(path string) savecore.FormationBlock {
		t.Helper()
		read, err := savecore.ReadFormations(context.Background(), path, []int32{authoringTrigger})
		if err != nil {
			t.Fatal(err)
		}
		return read.Blocks[authoringTrigger]
	}
	ashLadders := map[int32][11]int32{}
	for _, row := range blockIn(restorableSavePath(t)).Rows {
		ashLadders[row.RowID] = row.Doping
	}
	for _, row := range blockIn(named).Rows {
		if row.Doping != savecore.CharacterLadder() {
			t.Errorf("row %d summoning the character climbs %v, want %v", row.RowID, row.Doping, savecore.CharacterLadder())
		}
	}
	if loaded, err = regulation.Load(named); err != nil {
		t.Fatal(err)
	}
	if effects, effectRows, err = regulation.LoadRowsByID(loaded.BND4, "SpEffectParam.param", "", 0); err != nil {
		t.Fatal(err)
	}
	// SpEffectParam: maxHpRate 16, physicsAttackPowerRate 72,
	// staminaAttackRate 200; the 50% rung is its level-5 row.
	half := effects[effectRows[savecore.CharacterLadder()[5]].DataOffset:]
	for _, offset := range []int{16, 72, 200} {
		if rate := math.Float32frombits(binary.LittleEndian.Uint32(half[offset:])); rate != 0.5 {
			t.Errorf("the 50%% rung holds %v at offset %d", rate, offset)
		}
	}
	if err := loadedSession(t, named).StageBuildDeletion(3); !errors.Is(err, application.ErrBuildNamed) {
		t.Errorf("deleting a summoned character: %v, want ErrBuildNamed", err)
	}

	unnaming := loadedSession(t, named)
	if _, err := unnaming.StageFormation(authoringHost, []application.AuthoredSlot{{Npc: godrickSoldier}}); err != nil {
		t.Fatal(err)
	}
	unnamed, rows := save(unnaming, "unnamed.dat")
	if len(rows) != 2+character.Bodies {
		t.Errorf("no formation names the character and the save keeps %d of its rows, want %d", len(rows), 2+character.Bodies)
	}
	// The creature's row and the row it retired both climb the Ash's ladder again.
	creature := blockIn(unnamed)
	if len(creature.Rows) != 1 {
		t.Fatalf("one creature stands on %d rows", len(creature.Rows))
	}
	for _, row := range slices.Concat(creature.Rows, creature.Inert) {
		if row.Doping != ashLadders[row.RowID] {
			t.Errorf("row %d climbs %v, want its Ash's %v", row.RowID, row.Doping, ashLadders[row.RowID])
		}
	}

	deleting := loadedSession(t, unnamed)
	if err := deleting.StageBuildDeletion(3); err != nil {
		t.Fatal(err)
	}
	if _, gone := save(deleting, "gone.dat"); len(gone) != 0 || state.OwnEffects[savecore.FreeCasting] ||
		state.OwnEffects[savecore.OwnColoursPresence] || state.OwnEffects[savecore.CharacterLadder()[5]] {
		t.Errorf("the deleted character left %d rows, free casting held %t, own colours %t, the ladder %t", len(gone),
			state.OwnEffects[savecore.FreeCasting], state.OwnEffects[savecore.OwnColoursPresence],
			state.OwnEffects[savecore.CharacterLadder()[5]])
	}
}
