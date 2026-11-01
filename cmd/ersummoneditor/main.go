// Command ersummoneditor is the command-line frontend for the UI-independent
// Summoning Editor application service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

type conversionFlags []string

func (values *conversionFlags) String() string { return strings.Join(*values, ",") }
func (values *conversionFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	// Interrupting a conversion must not leave a half-written save. The
	// application checks this context immediately before publishing, so
	// Ctrl-C either lands before the write or after it completes.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("ersummoneditor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	savePath := flags.String("save", "", "input decrypted PS4 memory.dat")
	outPath := flags.String("out", "", "distinct output save path")
	manifestPath := flags.String("manifest-out", "", "optional path for the generated JSON audit manifest")
	dryRun := flags.Bool("dry-run", false, "report what would change and write nothing")
	list := flags.Bool("list", false, "print every Spirit Ash that can be used and exit")
	reset := flags.Bool("reset", false, "restore every Spirit Ash to shipped behavior; takes no -convert")
	author := flags.String("author", "", "Ash whose own formation to rewrite, by name or goods-family ID; needs -slots")
	var requested conversionFlags
	flags.Var(&requested, "convert", "host=destination using an Ash name or goods-family ID; repeatable")
	var slots conversionFlags
	flags.Var(&slots, "slots", "actors for -author: npc[:think][@x/z[/facing]][~perception][xN], comma-separated; repeatable")
	perception := flags.String("perception", "",
		"the ROLE every -slots actor plays, unless the slot says ~otherwise: follower (the default: 2 m beyond its body, fights what you fight), guard (12 m), sentinel (30 m, narrow cone), hunter (50 m and a 25 m nose, off the leash - needs the wander stance), statue (acts never), loner (its own senses - what an enemy gets)")
	fighter := flags.Bool("fighter", false,
		"summon every -slots creature as a Fighter rather than a Helper: no balance package, and the creature's own health")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	service, err := application.New()
	if err != nil {
		return err
	}
	// Listing needs no save and no conversion, so it is answered before the
	// arguments those require are checked.
	if *list {
		return reportAshes(service, stdout)
	}

	// A reset names no conversion, so it is dispatched before the parser that
	// requires one. Asking for both is still refused, by the application rather
	// than here, so a GUI gets the same answer.
	if *reset {
		conversions, err := parseResetConversions(requested)
		if err != nil {
			return err
		}
		request := application.Request{
			InputPath: *savePath, OutputPath: *outPath,
			ManifestPath: *manifestPath, Conversions: conversions,
		}
		if *dryRun {
			return reportResetPreview(ctx, service, request, stdout)
		}
		return reportReset(ctx, service, request, stdout)
	}

	// Authoring, like a reset, names no conversion, so it too is dispatched
	// before the parser that requires one. A request carrying both is refused by
	// the application rather than here, so a GUI gets the same answer.
	if *author != "" || len(slots) > 0 {
		authoring, err := parseAuthoring(*author, slots, *fighter, *perception)
		if err != nil {
			return err
		}
		conversions, err := parseResetConversions(requested)
		if err != nil {
			return err
		}
		request := application.Request{
			InputPath: *savePath, OutputPath: *outPath,
			ManifestPath: *manifestPath, Conversions: conversions, Authoring: authoring,
		}
		if *dryRun {
			return reportAuthoringPreview(ctx, service, request, stdout)
		}
		return reportAuthoring(ctx, service, request, stdout)
	}

	conversions, err := parseConversions(requested)
	if err != nil {
		return err
	}
	request := application.Request{
		InputPath: *savePath, OutputPath: *outPath,
		ManifestPath: *manifestPath, Conversions: conversions,
	}
	if *dryRun {
		return reportPreview(ctx, service, request, stdout)
	}
	return reportApply(ctx, service, request, stdout)
}

// parseAuthoring reads the actor list. The syntax is the shortest thing that
// covers what the model allows: an NpcParam row, optionally the think row when
// it is not the same number, optionally a repeat count.
//
// A world actor is named by its world row. Nothing here computes the
// summon-side row: that is arithmetic internal/formation owns, and keeping it
// there is what stops a caller naming a destination of its own.
func parseAuthoring(host string, arguments []string, fighter bool, perception string) (*application.AuthoringRequest, error) {
	defaultPerception, err := parsePerception(perception)
	if err != nil {
		return nil, fmt.Errorf("-perception: %w", err)
	}
	request := &application.AuthoringRequest{Host: strings.TrimSpace(host)}
	for _, argument := range arguments {
		for _, entry := range strings.Split(argument, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			slot, repeat, err := parseSlot(entry)
			if err != nil {
				return nil, err
			}
			if !slot.perceptionSaid {
				slot.Perception = defaultPerception
			}
			slot.Fighter = fighter
			// This command offers no stance and no share box, so every ladder
			// level is given its own: the follow type the Statue, the Hunter
			// and the Loner cannot be themselves without, and the level's
			// default share (on for the Follower, off for the rest) - rather
			// than being refused.
			if stance, known := application.LevelStance(slot.AuthoredSlot); known {
				slot.Stance = stance
			}
			for index := 0; index < repeat; index++ {
				request.Slots = append(request.Slots, slot.AuthoredSlot)
			}
		}
	}
	// Both halves are required together. Either one alone is a half-typed
	// command rather than a request with a sensible default.
	if request.Host == "" {
		return nil, fmt.Errorf("-slots needs -author to say which Ash the formation belongs to")
	}
	if len(request.Slots) == 0 {
		return nil, fmt.Errorf("-author needs at least one -slots actor")
	}
	if len(request.Slots) > application.MaxFormationSlots {
		return nil, fmt.Errorf("%d actors were named; a formation holds at most %d",
			len(request.Slots), application.MaxFormationSlots)
	}
	return request, nil
}

// parsedSlot is an authored slot plus whether the entry named its own
// perception, so the -perception default applies only where it did not.
type parsedSlot struct {
	application.AuthoredSlot
	perceptionSaid bool
}

// parsePerception reads a perception level by name.
//
// The ladder only: "follower" (the default), "statue", "guard", "sentinel",
// "hunter", "loner". "" means the Follower, because it is the zero value, so an
// empty -perception means "the default", not "unset".
func parsePerception(text string) (application.Perception, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "follower" {
		return application.PerceptionFollower, nil
	}
	for _, level := range application.Ladder {
		if string(level) == text {
			return level, nil
		}
	}
	return "", fmt.Errorf("%q is not a perception; want follower (the default), statue, guard, sentinel, hunter or loner", text)
}

// parseSlot reads one actor: npc[:think][@x/z[/facing]][~perception][xN].
//
// The placement is metres sideways and metres forward from the player, and an
// optional facing in degrees. Slashes rather than commas because commas already
// separate slots. The perception, after a tilde, is how far this actor looks
// for a target on its own; without it the -perception default applies.
//
// There is no height, and there is no field for one: BuddyParam carries
// x_offset, z_offset and y_angle, and y_angle is a rotation. A summon cannot be
// placed in the air or under the ground through this product because the game
// gives it nowhere to write that.
func parseSlot(entry string) (parsedSlot, int, error) {
	repeat := 1
	if body, count, found := strings.Cut(entry, "x"); found {
		parsed, err := strconv.Atoi(strings.TrimSpace(count))
		if err != nil || parsed < 1 {
			return parsedSlot{}, 0, fmt.Errorf("-slots %q: %q is not a repeat count", entry, count)
		}
		entry, repeat = strings.TrimSpace(body), parsed
	}
	var perception application.Perception
	perceptionSaid := false
	if body, level, found := strings.Cut(entry, "~"); found {
		parsed, err := parsePerception(level)
		if err != nil {
			return parsedSlot{}, 0, fmt.Errorf("-slots %q: %w", entry, err)
		}
		entry, perception, perceptionSaid = strings.TrimSpace(body), parsed, true
	}
	var placement *application.Placement
	if body, where, found := strings.Cut(entry, "@"); found {
		place, err := parsePlacement(where)
		if err != nil {
			return parsedSlot{}, 0, fmt.Errorf("-slots %q: %w", entry, err)
		}
		entry, placement = strings.TrimSpace(body), &place
	}
	npcText, thinkText, hasThink := strings.Cut(entry, ":")
	npc, err := parseRowID(npcText)
	if err != nil {
		return parsedSlot{}, 0, fmt.Errorf("-slots %q: %w", entry, err)
	}
	slot := parsedSlot{AuthoredSlot: application.AuthoredSlot{Npc: npc, Placement: placement, Perception: perception}, perceptionSaid: perceptionSaid}
	if hasThink {
		think, err := parseRowID(thinkText)
		if err != nil {
			return parsedSlot{}, 0, fmt.Errorf("-slots %q: %w", entry, err)
		}
		slot.Think = think
	}
	return slot, repeat, nil
}

// parsePlacement reads "x/z" or "x/z/facing".
//
// The bounds are savecore's, which are the research patcher's: far wider than
// anything the game ships, because placing a 27-metre Furnace Golem inside the
// shipped four-metre envelope puts it on top of the player. A caller past the
// envelope is running a placement test, and nothing has seen what the
// game does out there.
func parsePlacement(text string) (application.Placement, error) {
	parts := strings.Split(strings.TrimSpace(text), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return application.Placement{}, fmt.Errorf("%q is not a placement; want x/z or x/z/facing", text)
	}
	values := make([]float64, len(parts))
	for index, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return application.Placement{}, fmt.Errorf("%q is not a number", strings.TrimSpace(part))
		}
		values[index] = value
	}
	place := application.Placement{X: values[0], Z: values[1]}
	if len(values) == 3 {
		place.Angle = values[2]
	}
	if err := application.ValidPlacement(place); err != nil {
		return application.Placement{}, err
	}
	return place, nil
}

func parseRowID(text string) (int32, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 32)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%q is not a row ID", strings.TrimSpace(text))
	}
	return int32(value), nil
}

func reportAuthoringPreview(ctx context.Context, service *application.Service, request application.Request, stdout io.Writer) error {
	preview, err := service.PreviewAuthoring(ctx, request)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "dry run; nothing was written\n")
	fmt.Fprintf(stdout, "save %s (regulation %d)\n", preview.Save.SHA256, preview.Save.RegulationVersion)
	describeAuthoredFormation(preview.Formation, stdout)
	fmt.Fprintf(stdout, "would write %d BuddyParam rows and insert %d rows\n",
		preview.BuddyRows, preview.RowsInserted)
	return nil
}

func reportAuthoring(ctx context.Context, service *application.Service, request application.Request, stdout io.Writer) error {
	result, err := service.Author(ctx, request)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created %s\n", result.OutputPath)
	fmt.Fprintf(stdout, "input SHA-256:  %s\n", result.InputSHA256)
	fmt.Fprintf(stdout, "output SHA-256: %s\n", result.OutputSHA256)
	if result.Authored != nil {
		describeAuthoredFormation(*result.Authored, stdout)
	}
	fmt.Fprintf(stdout, "regulation: %d; verified rows: buddy=%d inserted=%d\n",
		result.RegulationVersion, result.BuddyRows, result.RowsInserted)
	return nil
}

// describeAuthoredFormation prints the formation slot by slot, because the row
// IDs are the only way to check afterwards that the save says what was asked
// for, and the clone lines are the only notice that an actor the game never
// offered as a summon is being made into one.
func describeAuthoredFormation(authored application.AuthoredFormation, stdout io.Writer) {
	fmt.Fprintf(stdout, "%s: formation %d, %d shipped rows\n",
		authored.HostName, authored.TriggerSpEffectID, authored.ShippedRows)
	for _, slot := range authored.Slots {
		note := ""
		if slot.Inserted {
			note = "  [new row]"
		}
		fmt.Fprintf(stdout, "  slot %d: row %d -> %d/%d at %g/%g facing %g%s\n",
			slot.Slot, slot.RowID, slot.Npc, slot.Think, slot.Place.X, slot.Place.Z, slot.Place.Angle, note)
	}
	for _, clone := range authored.Clones {
		fmt.Fprintf(stdout, "  clone: world actor %d becomes summon %d\n", clone.SourceNpc, clone.Npc)
	}
	if len(authored.Retired) > 0 {
		fmt.Fprintf(stdout, "  retired rows: %v\n", authored.Retired)
	}
}

// reportAshes prints the catalog so a user can learn a name without opening the
// CSV. Either selector in the printed line works with -convert.
func reportAshes(service *application.Service, stdout io.Writer) error {
	options := service.Ashes()
	fmt.Fprintf(stdout, "%d Spirit Ash families; either the ID or the name works with -convert\n\n", len(options))
	width := 0
	for _, option := range options {
		if len(option.Name) > width {
			width = len(option.Name)
		}
	}
	for _, option := range options {
		summons := fmt.Sprintf("summons %d", option.FormationSize)
		if option.VariesByLevel {
			summons = fmt.Sprintf("summons %d, rising to %d as it is upgraded",
				option.FormationSize, option.LargestFormation)
		}
		fmt.Fprintf(stdout, "  %-9d  %-*s  %s\n", option.GoodsFamily, width, option.Name, summons)
	}
	return nil
}

func reportPreview(ctx context.Context, service *application.Service, request application.Request, stdout io.Writer) error {
	preview, err := service.Preview(ctx, request)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "dry run; nothing was written\n")
	fmt.Fprintf(stdout, "save %s (regulation %d)\n", preview.Save.SHA256, preview.Save.RegulationVersion)
	for _, conversion := range preview.Conversions {
		status := ""
		if conversion.AlreadyApplied {
			status = "  [already applied]"
		}
		fmt.Fprintf(stdout, "  %s: %d -> %d (%s)%s\n",
			conversion.HostName, conversion.CurrentTrigger,
			conversion.TriggerSpEffectID, conversion.DestinationName, status)
		// A destination that changes formation as it is upgraded carries that
		// progression over, so the single arrow above does not describe it.
		if conversion.VariesByLevel {
			fmt.Fprintf(stdout, "    varies by upgrade level: %s\n",
				describeLevels(conversion.LevelFormations))
		}
	}
	fmt.Fprintf(stdout, "would write: %s\n", strings.Join(preview.TouchedTables, ", "))
	fmt.Fprintf(stdout, "rows: goods=%d buddy=%d buddy-stone=%d\n",
		preview.GoodsRows, preview.BuddyRows, preview.StoneRows)
	if preview.ExpandedAreaAlreadyApplied {
		fmt.Fprintln(stdout, "note: this save already carries the expanded-area recipe")
	}
	// The recipe reaches every Ash, not only the ones being converted, and
	// docs/SAFETY.md requires that be stated rather than discovered.
	fmt.Fprintf(stdout, "note: the once-per-rest limit is lifted for every Ash, not only the converted ones, at all %d monuments (the 39 sleeping ones woken)\n",
		preview.StoneRows)
	return nil
}

func reportApply(ctx context.Context, service *application.Service, request application.Request, stdout io.Writer) error {
	result, err := service.Apply(ctx, request)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created %s\n", result.OutputPath)
	fmt.Fprintf(stdout, "input SHA-256:  %s\n", result.InputSHA256)
	fmt.Fprintf(stdout, "output SHA-256: %s\n", result.OutputSHA256)
	fmt.Fprintf(stdout, "regulation: %d; verified rows: goods=%d buddy=%d buddy-stone=%d\n",
		result.RegulationVersion, result.GoodsRows, result.BuddyRows, result.BuddyStoneRows)
	return nil
}

// describeLevels collapses a per-level trigger list into the level ranges that
// share a formation, which is how the game presents it and how a user thinks
// about it: "+0..+4 gives one thing, +5..+9 another".
func describeLevels(formations []int32) string {
	var parts []string
	for start := 0; start < len(formations); {
		end := start
		for end+1 < len(formations) && formations[end+1] == formations[start] {
			end++
		}
		if start == end {
			parts = append(parts, fmt.Sprintf("+%d -> %d", start, formations[start]))
		} else {
			parts = append(parts, fmt.Sprintf("+%d..+%d -> %d", start, end, formations[start]))
		}
		start = end + 1
	}
	return strings.Join(parts, ", ")
}

func parseConversions(arguments []string) ([]application.ConversionRequest, error) {
	var conversions []application.ConversionRequest
	for _, argument := range arguments {
		for _, pair := range strings.Split(argument, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			host, destination, found := strings.Cut(pair, "=")
			if !found {
				return nil, fmt.Errorf("-convert %q must be host=destination", pair)
			}
			conversions = append(conversions, application.ConversionRequest{
				Host: strings.TrimSpace(host), Destination: strings.TrimSpace(destination),
			})
		}
	}
	if len(conversions) == 0 {
		return nil, fmt.Errorf("at least one -convert host=destination is required")
	}
	return conversions, nil
}

// reportResetPreview says what a reset would do, and whether there is anything
// to do at all.
func reportResetPreview(ctx context.Context, service *application.Service, request application.Request, stdout io.Writer) error {
	preview, err := service.PreviewReset(ctx, request)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "dry run; nothing was written\n")
	fmt.Fprintf(stdout, "save %s (regulation %d)\n", preview.Save.SHA256, preview.Save.RegulationVersion)
	if preview.AlreadyVanilla {
		fmt.Fprintln(stdout, "this save is already unedited; a reset would change nothing")
		return nil
	}
	if preview.StoneRecipeApplied {
		fmt.Fprintln(stdout, "the expanded-area recipe is present and would be removed")
	}
	// Only rows whose value differs from the shipped one are written, so these
	// counts are the size of the edit being undone, not the size of the table.
	fmt.Fprintf(stdout, "would restore shipped values: goods=%d buddy=%d buddy-stone=%d\n",
		preview.GoodsRows, preview.BuddyRows, preview.StoneRows)
	if preview.SummonRows > 0 {
		fmt.Fprintf(stdout, "would make %d of the game's own summon rows Helpers again\n", preview.SummonRows)
	}
	if preview.Colours {
		fmt.Fprintln(stdout, "would put the game's spirit look back on every summon")
	}
	if preview.Characters > 0 {
		fmt.Fprintf(stdout, "would delete %d custom characters\n", preview.Characters)
	}
	if preview.StoneRows > 0 {
		fmt.Fprintln(stdout, "note: the once-per-rest limit returns for every Ash, not only the converted ones")
	}
	return nil
}

func reportReset(ctx context.Context, service *application.Service, request application.Request, stdout io.Writer) error {
	result, err := service.Reset(ctx, request)
	if err != nil {
		if errors.Is(err, application.ErrNoChange) {
			return fmt.Errorf("this save is already unedited; there is nothing to reset")
		}
		return err
	}
	fmt.Fprintf(stdout, "created %s\n", result.OutputPath)
	fmt.Fprintf(stdout, "input SHA-256:  %s\n", result.InputSHA256)
	fmt.Fprintf(stdout, "output SHA-256: %s\n", result.OutputSHA256)
	fmt.Fprintf(stdout, "regulation: %d; restored rows: goods=%d buddy=%d buddy-stone=%d\n",
		result.RegulationVersion, result.GoodsRows, result.BuddyRows, result.BuddyStoneRows)
	// Removals are the half of a reset a restored-value count cannot show: an
	// authored formation leaves rows behind that have no shipped value to
	// restore, and taking them out is what makes the block the shipped one again.
	if result.RowsRemoved > 0 {
		fmt.Fprintf(stdout, "removed %d rows this editor had added\n", result.RowsRemoved)
	}
	return nil
}

// parseResetConversions exists so -reset -convert reaches the application's own
// refusal rather than being silently ignored here. Parsing it the ordinary way
// would fail first with "at least one -convert is required", which is the
// opposite of what happened.
func parseResetConversions(arguments []string) ([]application.ConversionRequest, error) {
	for _, argument := range arguments {
		if strings.TrimSpace(argument) != "" {
			return parseConversions(arguments)
		}
	}
	return nil, nil
}
