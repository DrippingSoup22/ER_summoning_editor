package application_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// Putrid Corpse Ashes: four shipped rows, one block, and a formation the
// fixture really carries. The actor is a world row with no clone beside it, so
// authoring it exercises the whole path rather than the half that only rewrites
// values.
const (
	authoringHost    = "Putrid Corpse Ashes"
	authoringTrigger = int32(215000)
	authoringRows    = 4
)

func worldActorSlot() application.AuthoredSlot {
	return application.AuthoredSlot{Npc: savefixture.CloneableWorldActors[0]}
}

func slotsOf(count int) []application.AuthoredSlot {
	slots := make([]application.AuthoredSlot, 0, count)
	for index := 0; index < count; index++ {
		slots = append(slots, worldActorSlot())
	}
	return slots
}

func authoringRequest(input, output string, slots int) application.Request {
	return application.Request{
		InputPath: input, OutputPath: output,
		Authoring: &application.AuthoringRequest{Host: authoringHost, Slots: slotsOf(slots)},
	}
}

func outputBeside(t *testing.T, input, name string) string {
	t.Helper()
	return filepath.Join(filepath.Dir(input), name)
}

// The workflow G1 exists to expose: a player names an Ash and five actors, and
// the save comes back summoning five of them.
//
// Everything below the service already had a test. What did not exist was a way
// to ask for it, which is the same as saying the capability was unreachable.
func TestAuthoringFillsAHostBlockAndAddsWhatItNeeds(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	output := outputBeside(t, input, "authored.dat")
	service := newService(t)

	const slots = 5
	preview, err := service.PreviewAuthoring(context.Background(), authoringRequest(input, output, slots))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Formation.TriggerSpEffectID != authoringTrigger {
		t.Errorf("preview edits formation %d, want the host's own %d",
			preview.Formation.TriggerSpEffectID, authoringTrigger)
	}
	if preview.Formation.ShippedRows != authoringRows {
		t.Errorf("preview says the block holds %d rows, want %d", preview.Formation.ShippedRows, authoringRows)
	}
	if len(preview.Formation.Slots) != slots {
		t.Fatalf("preview plans %d slots, want %d", len(preview.Formation.Slots), slots)
	}
	inserted := 0
	for _, slot := range preview.Formation.Slots {
		if slot.Inserted {
			inserted++
		}
		// What the user named is a world row; what the game reads is its clone.
		// A preview that showed the world row would be describing an actor that
		// is not on the Spirit Summon team.
		want := worldActorSlot().Npc + formation.CloneOffset
		if slot.Npc != want || slot.Think != want {
			t.Errorf("slot %d resolves to %d/%d, want %d for both", slot.Slot, slot.Npc, slot.Think, want)
		}
	}
	if inserted != slots-authoringRows {
		t.Errorf("preview inserts %d rows, want %d", inserted, slots-authoringRows)
	}
	if len(preview.Formation.Clones) != 1 {
		t.Fatalf("preview plans %d clones, want 1", len(preview.Formation.Clones))
	}
	if clone := preview.Formation.Clones[0]; !clone.InsertsNpc || !clone.InsertsThink {
		t.Errorf("clone %+v does not insert both halves, but the fixture holds neither", clone)
	}
	if preview.Formation.Retired != nil {
		t.Errorf("a formation larger than its block retired rows %v", preview.Formation.Retired)
	}

	result, err := service.Author(context.Background(), authoringRequest(input, output, slots))
	if err != nil {
		t.Fatal(err)
	}
	if result.GoodsRows != 0 {
		t.Errorf("authoring wrote %d goods rows, want none", result.GoodsRows)
	}
	if result.BuddyStoneRows != 0 {
		t.Errorf("authoring wrote %d BuddyStone rows, want none", result.BuddyStoneRows)
	}
	// One BuddyParam row and the two halves of one actor.
	if result.RowsInserted != 3 {
		t.Errorf("authoring inserted %d rows, want 3", result.RowsInserted)
	}
	if result.RowsRemoved != 0 {
		t.Errorf("authoring removed %d rows, want none", result.RowsRemoved)
	}

	// Read the result back through the product's own reader: the block is one
	// row longer and every row names the summon.
	written, err := savecore.ReadFormations(context.Background(), output, []int32{authoringTrigger})
	if err != nil {
		t.Fatal(err)
	}
	block := written.Blocks[authoringTrigger]
	if len(block.Rows) != slots {
		t.Fatalf("the authored block holds %d rows, want %d", len(block.Rows), slots)
	}
	summon := worldActorSlot().Npc + formation.CloneOffset
	for _, row := range block.Rows {
		if row.Npc != summon || row.Think != summon {
			t.Errorf("row %d names %d/%d after authoring, want %d", row.RowID, row.Npc, row.Think, summon)
		}
	}
	if !written.NpcRows[summon] || !written.NpcThinkRows[summon] {
		t.Error("the actor the block names was not inserted into both actor tables")
	}
}

// A preview is binding: what it shows is the manifest the write uses, not a
// second plan built from the same request.
func TestAnAuthoringPreviewPromisesExactlyWhatAuthorWrites(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	output := outputBeside(t, input, "authored.dat")
	manifestPath := outputBeside(t, input, "authored.json")
	service := newService(t)

	preview, err := service.PreviewAuthoring(context.Background(), authoringRequest(input, output, 5))
	if err != nil {
		t.Fatal(err)
	}
	request := authoringRequest(input, output, 5)
	request.ManifestPath = manifestPath
	if _, err := service.Author(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preview.Manifest, written) {
		t.Error("the manifest Author wrote differs from the one Preview promised")
	}
	if strings.Contains(string(written), "goods_patches") {
		t.Error("an authored manifest carries a goods patch")
	}
}

// Everything an authoring request can be wrong about, refused before the save
// is written and with an error a frontend can act on.
func TestAuthoringRefusesWhatItCannotWriteHonestly(t *testing.T) {
	t.Parallel()
	input := writeVanilla(t)
	output := outputBeside(t, input, "authored.dat")
	service := newService(t)

	converted := outputBeside(t, input, "converted.dat")
	if _, err := service.Apply(context.Background(), application.Request{
		InputPath: input, OutputPath: converted,
		Conversions: []application.ConversionRequest{{Host: authoringHost, Destination: "Land Squirt Ashes"}},
	}); err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name    string
		request application.Request
		want    any
	}{
		{
			// A converted host delivers someone else's formation, so authoring
			// into it would edit an item the user did not name.
			name: "a host an earlier conversion re-pointed",
			request: application.Request{InputPath: converted, OutputPath: output,
				Authoring: &application.AuthoringRequest{Host: authoringHost, Slots: slotsOf(3)}},
			want: new(application.HostRePointedError),
		},
		{
			// The game delivers the shipped Kristoff whatever his block says.
			name: "the Ash the game binds itself",
			request: application.Request{InputPath: input, OutputPath: output,
				Authoring: &application.AuthoringRequest{Host: "Ancient Dragon Knight Kristoff", Slots: slotsOf(3)}},
			want: new(application.HostFixedError),
		},
		{
			name: "an actor this save has no row for",
			request: application.Request{InputPath: input, OutputPath: output,
				Authoring: &application.AuthoringRequest{Host: authoringHost, Slots: []application.AuthoredSlot{{Npc: 44444444}}}},
			want: new(application.UnknownActorError),
		},
		{
			name: "an Ash the catalog does not name",
			request: application.Request{InputPath: input, OutputPath: output,
				Authoring: &application.AuthoringRequest{Host: "Fire Giant Ashes", Slots: slotsOf(1)}},
			want: new(application.UnknownAshError),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := service.Author(context.Background(), testCase.request)
			if err == nil {
				t.Fatal("the request was accepted")
			}
			if !errors.As(err, &testCase.want) {
				t.Fatalf("err = %v (%T), want %T", err, err, testCase.want)
			}
			if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("a refused request created %s", output)
			}
		})
	}
}

// The share box a frontend offers: the Follower always shares and offers no
// box, since by itself it notices two metres and off it would fight nothing;
// the Guard, the Sentinel and the Hunter offer one, off at first. (The writer
// still takes a Follower off - the Putrid Corpse's Ash ships that way.)
func TestTheFollowerOffersNoShareBox(t *testing.T) {
	if on, choosable := application.ShareOffered(application.PerceptionFollower); !on || choosable {
		t.Errorf("the Follower offers share %t, choosable %t; want always on", on, choosable)
	}
	for _, level := range []application.Perception{application.PerceptionGuard,
		application.PerceptionSentinel, application.PerceptionHunter} {
		if on, choosable := application.ShareOffered(level); on || !choosable {
			t.Errorf("the %s offers share %t, choosable %t; want a choice, off at first", level.Name(), on, choosable)
		}
	}
}
