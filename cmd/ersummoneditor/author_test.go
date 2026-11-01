package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

// The command a console batch actually runs: name an Ash, name actors, get a
// save. Until this existed the authoring path was reachable only from a test,
// which is the same as saying nobody could use it.
func TestAuthoringFromTheCommandLine(t *testing.T) {
	input := writeVanillaSave(t)
	output := filepath.Join(t.TempDir(), "authored.dat")
	world := savefixture.CloneableWorldActors[0]
	slots := strings.Join([]string{itoa(world), "x5"}, "")

	dry, _, err := runCLI(t, "-save", input, "-author", "Putrid Corpse Ashes", "-slots", slots, "-dry-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"nothing was written",
		"Putrid Corpse Ashes: formation 215000",
		"slot 5:",
		"[new row]",
		"clone: world actor " + itoa(world),
	} {
		if !strings.Contains(dry, want) {
			t.Errorf("dry run does not mention %q:\n%s", want, dry)
		}
	}
	if _, err := os.Stat(output); err == nil {
		t.Error("the dry run created the output save")
	}

	stdout, _, err := runCLI(t, "-save", input, "-out", output,
		"-author", "Putrid Corpse Ashes", "-slots", slots)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "created "+output) {
		t.Errorf("output does not report the created save:\n%s", stdout)
	}
	// One BuddyParam row and the two halves of the actor it names.
	if !strings.Contains(stdout, "inserted=3") {
		t.Errorf("output does not report three inserted rows:\n%s", stdout)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("the authored save was not created: %v", err)
	}
}

// A formation the command line can express but the game cannot deliver, and
// half-typed commands, are refused before the save is opened.
func TestTheAuthoringFlagsRefuseWhatTheyCannotMean(t *testing.T) {
	world := itoa(savefixture.CloneableWorldActors[0])
	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "actors with no Ash to put them in",
			args: []string{"-save", "absent.dat", "-slots", world},
			want: "needs -author",
		},
		{
			name: "an Ash with no actors",
			args: []string{"-save", "absent.dat", "-author", "Putrid Corpse Ashes"},
			want: "at least one -slots",
		},
		{
			name: "more actors than a formation holds",
			args: []string{"-save", "absent.dat", "-author", "Putrid Corpse Ashes", "-slots", world + "x11"},
			want: "at most 10",
		},
		{
			name: "an actor that is not a row ID",
			args: []string{"-save", "absent.dat", "-author", "Putrid Corpse Ashes", "-slots", "wolf"},
			want: "is not a row ID",
		},
		{
			name: "a repeat count that is not a count",
			args: []string{"-save", "absent.dat", "-author", "Putrid Corpse Ashes", "-slots", world + "xmany"},
			want: "is not a repeat count",
		},
		{
			// Refused by the application, not by the parser, so a GUI making the
			// same mistake gets the same answer.
			name: "a conversion and an authored formation together",
			args: []string{"-save", "absent.dat", "-out", "out.dat",
				"-author", "Putrid Corpse Ashes", "-slots", world,
				"-convert", "Lone Wolf Ashes=Giant Rat Ashes"},
			want: "cannot be asked for together",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := runCLI(t, testCase.args...)
			if err == nil {
				t.Fatal("the command was accepted")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to mention %q", err, testCase.want)
			}
		})
	}
}

func itoa(value int32) string { return strconv.FormatInt(int64(value), 10) }

// The perception: the Follower by default, a -perception default for the
// formation, and a per-slot ~level that wins over it. Only the ladder is
// accepted, on either path.
func TestASlotsPerceptionIsTheFollowerUnlessSaidOtherwise(t *testing.T) {
	world := itoa(savefixture.CloneableWorldActors[0])
	request, err := parseAuthoring("Putrid Corpse Ashes", []string{world, world + "~guard", world + "@1/2~hunter"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := []application.Perception{request.Slots[0].Perception, request.Slots[1].Perception, request.Slots[2].Perception}; got[0] != application.PerceptionFollower ||
		got[1] != application.PerceptionGuard || got[2] != application.PerceptionHunter {
		t.Errorf("perceptions = %q, want follower, guard, hunter", got)
	}
	if request.Slots[2].Placement == nil || request.Slots[2].Placement.Z != 2 {
		t.Errorf("the placement before the tilde was lost: %+v", request.Slots[2])
	}
	request, err = parseAuthoring("Putrid Corpse Ashes", []string{world, world + "~statue"}, false, "sentinel")
	if err != nil {
		t.Fatal(err)
	}
	if request.Slots[0].Perception != application.PerceptionSentinel || request.Slots[1].Perception != application.PerceptionStatue {
		t.Errorf("the -perception default did not apply where the slot said nothing: %+v", request.Slots)
	}
	for _, refused := range []string{"hawk", "world", "eyes", "as-held"} {
		if _, err := parseAuthoring("Putrid Corpse Ashes", []string{world + "~" + refused}, false, ""); err == nil {
			t.Errorf("per-slot perception %q was accepted", refused)
		}
		if _, err := parseAuthoring("Putrid Corpse Ashes", []string{world}, false, refused); err == nil {
			t.Errorf("-perception %q was accepted", refused)
		}
	}
}
