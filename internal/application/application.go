// Package application is the UI-independent Summoning Editor workflow.
// Command-line and graphical frontends should translate user input into a
// Request and render the returned Result; save and game behavior stays here.
//
// The three entry points are meant to be used in order. Inspect answers "can
// this file be edited at all", cheaply enough to run when a file is chosen.
// Preview answers "what exactly would change", writing nothing. Apply performs
// the edit. Preview and Apply share one preparation step, so what Preview
// promises is what Apply does.
package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// supportedRegulationVersions is every version this build accepts: versions
// proven row-for-row identical, so the embedded catalog, the shipping recipe
// and the vanilla snapshot describe each of them exactly.
//
// A version is only ever added on evidence, never on a version number looking
// close enough. `ereffecteditor regdiff` compared the whole regulation of a
// 1.17 save (11701000) against a 1.17.1 one (11711000), and that one against
// a 1.17.2 one (11721000, 2026-10-24), and reported changed_tables 0 and
// changed_rows 0 both times: not one PARAM row differs. Calibration 1.17.3
// (11730034, 2026-11-01) changes one row against 11721000, and one field of
// it: Bullet 20003309, Piquebone Arrow (Fletched), atkId_Bullet 0 to 1. This
// editor reads and writes no Bullet row, so for everything it touches the
// four are identical. The snapshot is read from the 1.17.3 baseline; read from
// 1.17, 1.17.1 and 1.17.2, every value was the same.
//
// Anything that changes a row this editor reads or writes must fail this gate
// rather than widen it.
var supportedRegulationVersions = []uint32{11701000, 11711000, 11721000, 11730034}

// SupportedRegulations names every supported version, for a message.
func SupportedRegulations() string {
	names := make([]string, len(supportedRegulationVersions))
	for i, version := range supportedRegulationVersions {
		names[i] = strconv.FormatUint(uint64(version), 10)
	}
	return strings.Join(names, " or ")
}

// RegulationSupported reports whether this build has been verified against a
// version. It is the only place the set is consulted, so adding a version is
// one edit and cannot half-apply.
func RegulationSupported(version uint32) bool {
	for _, known := range supportedRegulationVersions {
		if version == known {
			return true
		}
	}
	return false
}

// AshOption is one entry a frontend may display in its picker.
type AshOption struct {
	GoodsFamily int32
	Name        string
	// FormationSize is how many actors the base (+0) formation summons.
	FormationSize int
	// LargestFormation is the most it summons at any upgrade level. It differs
	// from FormationSize only for a family that varies by level.
	LargestFormation int
	// VariesByLevel means this family delivers different formations as it is
	// upgraded, so choosing it as a destination carries that progression over.
	VariesByLevel bool
}

// ConversionRequest selects a host Ash and destination formation by exact
// case-insensitive name or numeric goods-family ID.
type ConversionRequest struct {
	Host        string
	Destination string
}

// Request contains all inputs needed for one save operation.
//
// Which operation is decided by the method called, not by the fields set. The
// three are mutually exclusive and each refuses the other's inputs, so a
// request cannot half-describe two edits.
type Request struct {
	InputPath    string
	OutputPath   string
	ManifestPath string
	Conversions  []ConversionRequest
	// Authoring is set for Author and PreviewAuthoring only.
	Authoring *AuthoringRequest
}

// SaveInfo describes a save file without changing it.
//
// Supported is a field rather than an error because a file picker wants to
// describe what it found, not to fail. Preview and Apply do refuse an
// unsupported version, because there is no valid plan for one.
type SaveInfo struct {
	Path              string
	SHA256            string
	RegulationVersion uint32
	Supported         bool
}

// ResolvedConversion records the concrete catalog rows selected by a request.
type ResolvedConversion struct {
	HostGoodsFamily        int32
	HostName               string
	DestinationGoodsFamily int32
	DestinationName        string
	TriggerSpEffectID      int32
}

// PlannedConversion is a resolved conversion plus what the save says today, so
// a frontend can show the change rather than only the intent.
type PlannedConversion struct {
	ResolvedConversion
	UpgradeLevels int
	// CurrentTrigger is what the host family's base row points at now.
	CurrentTrigger int32
	// LevelTriggers is the trigger each upgrade level would be given, read from
	// what the destination family itself carries at that level.
	LevelTriggers []int32
	// LevelFormations is the same list with the upgrade level taken back out,
	// leaving the formation a user would recognise.
	LevelFormations []int32
	// VariesByLevel means those formations are not all the same, so the host
	// will summon a different formation as it is upgraded.
	VariesByLevel bool
	// AlreadyApplied means every row of this family already points at the
	// requested formation, so applying would change nothing for it.
	AlreadyApplied bool
}

// Preview is the complete description of an edit that has not happened.
type Preview struct {
	Save          SaveInfo
	Conversions   []PlannedConversion
	TouchedTables []string
	// GoodsRows, BuddyRows and StoneRows are how many rows each table would
	// have written. StoneRows counts the whole table: the expanded-area recipe
	// is applied to every row, including Ashes the user did not select.
	GoodsRows int
	BuddyRows int
	StoneRows int
	// ExpandedAreaAlreadyApplied means the stone recipe is already present on
	// every row, so this save has been edited before.
	ExpandedAreaAlreadyApplied bool
	// Manifest is exactly the audit artifact Apply would write.
	Manifest []byte
}

// Result contains display-safe verification metadata and no private save data.
type Result struct {
	InputPath         string
	OutputPath        string
	InputSHA256       string
	OutputSHA256      string
	RegulationVersion uint32
	GoodsRows         int
	BuddyRows         int
	BuddyStoneRows    int
	// RowsInserted and RowsRemoved are how many rows the save gained and lost.
	// Only authoring adds and only a reset removes, so on a conversion both are
	// zero - which is itself worth reporting.
	RowsInserted int
	RowsRemoved  int
	Conversions  []ResolvedConversion
	// Authored describes the formation that was written, on an authoring run.
	Authored *AuthoredFormation
	// ItemsGranted is how many Ashes were put into a character's inventory,
	// and ItemsRefused says why any were not. A refused item does not fail the
	// commit - the save is written and correct either way - so this is the
	// only place it is reported.
	ItemsGranted int
	ItemsRefused []string
}

// resultFrom is the part of a Result that every operation fills in the same
// way: what the save core did to the file. What is left over is what only one
// operation knows - the conversions it resolved, the formation it authored,
// the items it granted - and each caller adds its own on top.
func resultFrom(summary savecore.Summary) *Result {
	return &Result{
		InputPath:         summary.InputPath,
		OutputPath:        summary.OutputPath,
		InputSHA256:       summary.InputSHA256,
		OutputSHA256:      summary.OutputSHA256,
		RegulationVersion: summary.RegulationVersion,
		GoodsRows:         summary.GoodsRows,
		BuddyRows:         summary.BuddyRows,
		BuddyStoneRows:    summary.BuddyStoneRows,
		RowsInserted:      summary.RowsInserted,
		RowsRemoved:       summary.RowsRemoved,
	}
}

// Service owns the immutable catalog used by every frontend.
type Service struct {
	ashes    []catalog.Ash
	families []catalog.Family
	// shared is what each entry needs of shipped effect rows (sharedeffects.go).
	shared []catalog.SharedEffectNeed
	// persons is what each armed person needs of its own CharaInitParam row
	// (sharedeffects.go, the person half).
	persons []catalog.PersonWriteNeed
	// rules is what each creature allows, by world row (look.go).
	rules map[int32]CreatureRules
}

// New loads and validates the embedded catalogs once.
func New() (*Service, error) {
	ashes, err := catalog.LoadAshes()
	if err != nil {
		return nil, err
	}
	families, err := catalog.LoadFamilies()
	if err != nil {
		return nil, err
	}
	shared, err := catalog.LoadSharedEffectNeeds()
	if err != nil {
		return nil, err
	}
	if err := checkSharedEffectNeeds(shared); err != nil {
		return nil, err
	}
	whole, err := catalog.ReadHierarchy(reference.HierarchyCatalog())
	if err != nil {
		return nil, err
	}
	persons, err := catalog.PersonWriteNeeds(whole)
	if err != nil {
		return nil, err
	}
	if err := checkPersonWriteNeeds(persons); err != nil {
		return nil, err
	}
	return &Service{ashes: ashes, families: families, shared: shared, persons: persons, rules: creatureRulesOf(whole)}, nil
}

// Ashes returns a fresh picker list so a frontend cannot mutate service state.
func (service *Service) Ashes() []AshOption {
	options := make([]AshOption, len(service.ashes))
	for index, ash := range service.ashes {
		options[index] = optionFor(ash)
	}
	return options
}

// optionFor is the one place a catalog entry becomes a picker entry, so a
// session read model and a picker list cannot describe the same Ash
// differently.
func optionFor(ash catalog.Ash) AshOption {
	return AshOption{
		GoodsFamily:      ash.GoodsFamily,
		Name:             ash.Name,
		FormationSize:    ash.FormationSizes[ash.TriggerFamilies[0]],
		LargestFormation: ash.LargestFormation(),
		VariesByLevel:    len(ash.TriggerFamilies) > 1,
	}
}

// Inspect reports what a save is, without planning or changing anything.
//
// It fails only when the file cannot be read as a decrypted PS4 save. A save
// from another game version comes back with Supported false, because a picker
// needs to say which version it found rather than refuse to describe the file.
func (service *Service) Inspect(ctx context.Context, path string) (*SaveInfo, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrPathRequired
	}
	info, err := savecore.Inspect(ctx, path)
	if err != nil {
		return nil, err
	}
	return &SaveInfo{
		Path: path, SHA256: info.SHA256,
		RegulationVersion: info.RegulationVersion,
		Supported:         RegulationSupported(info.RegulationVersion),
	}, nil
}

// Preview reports exactly what Apply would do, and writes nothing.
//
// The output path may be empty here: a frontend shows the user what a
// conversion would do before asking where to save it. Any path that is supplied
// is validated exactly as Apply would validate it.
func (service *Service) Preview(ctx context.Context, request Request) (*Preview, error) {
	prepared, err := service.prepare(ctx, request, optionalOutputPath)
	if err != nil {
		return nil, err
	}

	state := prepared.state
	planned := make([]PlannedConversion, 0, len(prepared.resolved))
	for index, conversion := range prepared.conversions {
		applied := true
		for level := 0; level < conversion.UpgradeLevels; level++ {
			row := state.Goods[conversion.HostGoodsFamily+int32(level)]
			if row.Trigger != conversion.LevelTriggers[level] {
				applied = false
				break
			}
		}
		formations := levelFormations(conversion.LevelTriggers)
		planned = append(planned, PlannedConversion{
			ResolvedConversion: prepared.resolved[index],
			UpgradeLevels:      conversion.UpgradeLevels,
			CurrentTrigger:     state.Goods[conversion.HostGoodsFamily].Trigger,
			LevelTriggers:      conversion.LevelTriggers,
			LevelFormations:    formations,
			VariesByLevel:      variesByLevel(formations),
			AlreadyApplied:     applied,
		})
	}

	return &Preview{
		Save:                       *prepared.save,
		Conversions:                planned,
		TouchedTables:              prepared.manifest.TouchedTables(),
		GoodsRows:                  len(prepared.manifest.GoodsPatches),
		BuddyRows:                  len(prepared.manifest.BuddyPatches),
		StoneRows:                  stoneRowsAfter(state.StoneRows, prepared.manifest),
		ExpandedAreaAlreadyApplied: state.ExpandedAreaApplied(),
		Manifest:                   prepared.encoded,
	}, nil
}

// Apply validates and executes one complete conversion operation.
func (service *Service) Apply(ctx context.Context, request Request) (*Result, error) {
	if err := checkPaths(request, requireOutputPath); err != nil {
		return nil, err
	}
	if request.Authoring != nil {
		return nil, ErrAuthoringTakesNoConversions
	}
	// Resolved before the save is opened, so a name nobody has fails without a
	// 26 MB read, and so two conversions naming one host are refused by the
	// layer that knows what the user typed. Staging alone would not do it: a
	// second conversion for the same host REPLACES the first there, which is
	// right for an editor and wrong for one command that named both.
	pairs, err := service.resolvePairs(request.Conversions)
	if err != nil {
		return nil, err
	}
	return service.oneShot(ctx, request, func(session *Session) error {
		for _, pair := range pairs {
			// Staged by row rather than by the name the user typed, so the
			// session cannot resolve to anything but what was just checked.
			if _, err := session.StageConversion(
				hostSelector(pair.host.GoodsFamily), hostSelector(pair.destination.GoodsFamily)); err != nil {
				return err
			}
		}
		return nil
	})
}

// preparation is everything Preview and Apply both need. Sharing it is what
// makes a preview binding: neither can resolve, validate, or plan differently
// from the other.
type preparation struct {
	save        *SaveInfo
	state       *savecore.State
	conversions []plan.Conversion
	resolved    []ResolvedConversion
	manifest    *plan.Manifest
	encoded     []byte
}

// Whether an output path has to be present yet. Apply needs one; Preview does
// not, because a frontend asks what a conversion would do before it asks the
// user where to put the result. Collisions are still checked whenever a path is
// supplied, so a preview cannot approve an arrangement Apply would refuse.
const (
	requireOutputPath  = true
	optionalOutputPath = false
)

func (service *Service) prepare(ctx context.Context, request Request, needOutput bool) (*preparation, error) {
	if err := checkPaths(request, needOutput); err != nil {
		return nil, err
	}
	if request.Authoring != nil {
		return nil, ErrAuthoringTakesNoConversions
	}

	pairs, err := service.resolvePairs(request.Conversions)
	if err != nil {
		return nil, err
	}
	state, err := service.readState(ctx, request.InputPath, goodsRowsFor(pairs))
	if err != nil {
		return nil, err
	}
	save := &SaveInfo{
		Path: request.InputPath, SHA256: state.SHA256,
		RegulationVersion: state.RegulationVersion,
		Supported:         RegulationSupported(state.RegulationVersion),
	}
	if !save.Supported {
		return nil, &UnsupportedRegulationError{Found: save.RegulationVersion}
	}

	conversions, resolved, err := planConversions(pairs, state)
	if err != nil {
		return nil, err
	}

	manifest, err := plan.Build(save.SHA256, conversions, conversionRecipe(state))
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	if err := manifest.Encode(&encoded); err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return &preparation{
		save: save, state: state, conversions: conversions, resolved: resolved,
		manifest: manifest, encoded: encoded.Bytes(),
	}, nil
}

// readState opens the save once for everything prepare needs: its identity, its
// regulation version, and the current value of every goods row involved.
//
// A missing row is ambiguous on its own — a save from another game version can
// lack rows this one has — so that one failure is re-read as a version question
// before it is reported as a corrupt save.
func (service *Service) readState(ctx context.Context, path string, rows []int32) (*savecore.State, error) {
	state, err := savecore.ReadState(ctx, path, rows)
	if err == nil {
		return state, nil
	}
	if errors.Is(err, ErrRowNotFound) {
		if info, inspectErr := savecore.Inspect(ctx, path); inspectErr == nil &&
			!RegulationSupported(info.RegulationVersion) {
			return nil, &UnsupportedRegulationError{
				Found: info.RegulationVersion,
			}
		}
	}
	return nil, fmt.Errorf("read input save: %w", err)
}

// goodsRowsFor lists every goods row prepare has to look at: the host's, to
// report what it says today, and the destination's, to learn which formation it
// delivers at each level.
func goodsRowsFor(pairs []resolvedPair) []int32 {
	var rows []int32
	for _, pair := range pairs {
		for level := 0; level < pair.host.UpgradeLevels; level++ {
			rows = append(rows, pair.host.GoodsFamily+int32(level))
		}
		for level := 0; level < pair.destination.UpgradeLevels; level++ {
			rows = append(rows, pair.destination.GoodsFamily+int32(level))
		}
	}
	return rows
}

// planConversions turns resolved catalog entries into per-level plans by first
// reading and validating what the destination family ships at every level.
//
// Ordinary families then pin that one formation's +0 trigger across every host
// row. Giant Rat is the one multi-formation family in the locked catalog; it
// keeps its shipped progression because each formation has reinforcement slots
// only for the levels that originally reach it.
func planConversions(pairs []resolvedPair, state *savecore.State) ([]plan.Conversion, []ResolvedConversion, error) {
	conversions := make([]plan.Conversion, 0, len(pairs))
	resolved := make([]ResolvedConversion, 0, len(pairs))
	for _, pair := range pairs {
		triggers, err := destinationLevelTriggers(pair.host, pair.destination, state)
		if err != nil {
			return nil, nil, err
		}
		conversions = append(conversions, plan.Conversion{
			HostGoodsFamily: pair.host.GoodsFamily,
			HostName:        pair.host.Name,
			UpgradeLevels:   pair.host.UpgradeLevels,
			LevelTriggers:   triggers,
			DestinationName: pair.destination.Name,
		})
		resolved = append(resolved, ResolvedConversion{
			HostGoodsFamily:        pair.host.GoodsFamily,
			HostName:               pair.host.Name,
			DestinationGoodsFamily: pair.destination.GoodsFamily,
			DestinationName:        pair.destination.Name,
			TriggerSpEffectID:      triggers[0],
		})
	}
	return conversions, resolved, nil
}

func destinationLevelTriggers(host, destination catalog.Ash, state *savecore.State) ([]int32, error) {
	// Every shipped family has eleven levels, so this cannot fire today. It is
	// here because a silent mismatch would misalign the whole progression, and
	// no clamp would be obviously right.
	if host.UpgradeLevels != destination.UpgradeLevels {
		return nil, &LevelCountMismatchError{
			HostName: host.Name, HostLevels: host.UpgradeLevels,
			DestinationName: destination.Name, DestinationLevels: destination.UpgradeLevels,
		}
	}
	shipped := make(map[int32]bool, len(destination.TriggerFamilies))
	for _, family := range destination.TriggerFamilies {
		shipped[family] = true
	}
	shippedTriggers := make([]int32, destination.UpgradeLevels)
	for level := range shippedTriggers {
		trigger := state.Goods[destination.GoodsFamily+int32(level)].Trigger
		// Every shipped row is its formation family plus its own upgrade level,
		// so subtracting the level recovers the family. Giant Rat +5 is 233105,
		// not 233100: the family changes while the level keeps counting.
		//
		// The catalog enumerates what the game ships. A destination whose family
		// is not in its own set was re-pointed by an earlier edit, and copying
		// that would spread one edit into a second family silently.
		if !shipped[trigger-int32(level)] {
			return nil, &DestinationNotShippedError{
				Name: destination.Name, GoodsFamily: destination.GoodsFamily,
				Level: level, Found: trigger,
			}
		}
		shippedTriggers[level] = trigger
	}
	if len(destination.TriggerFamilies) == 1 {
		triggers := make([]int32, host.UpgradeLevels)
		for level := range triggers {
			triggers[level] = shippedTriggers[0]
		}
		return triggers, nil
	}
	return shippedTriggers, nil
}

// levelFormations recovers the formation family behind each level's trigger.
// This is the part a user recognises — the formation — with the upgrade level
// taken back out.
func levelFormations(triggers []int32) []int32 {
	formations := make([]int32, len(triggers))
	if len(triggers) == 0 {
		return formations
	}
	fixed := true
	for _, trigger := range triggers[1:] {
		if trigger != triggers[0] {
			fixed = false
			break
		}
	}
	if fixed {
		for level := range formations {
			formations[level] = triggers[0]
		}
		return formations
	}
	for level, trigger := range triggers {
		formations[level] = trigger - int32(level)
	}
	return formations
}

// variesByLevel reports whether a destination delivers different formations
// across its upgrade levels, so a frontend can say so rather than imply one.
//
// It compares formations, not triggers: every family's raw trigger changes at
// every level, because the level is part of it. Only a change of formation
// family is a change of what gets summoned.
func variesByLevel(formations []int32) bool {
	for _, formation := range formations[1:] {
		if formation != formations[0] {
			return true
		}
	}
	return false
}

// resolvedPair is a request turned into two catalog entries. What each level
// delivers is not known yet: that comes from the save.
type resolvedPair struct {
	host        catalog.Ash
	destination catalog.Ash
}

func (service *Service) resolvePairs(requests []ConversionRequest) ([]resolvedPair, error) {
	if len(requests) == 0 {
		return nil, ErrNoConversions
	}
	pairs := make([]resolvedPair, 0, len(requests))
	claimed := make(map[int32]string, len(requests))
	for _, request := range requests {
		host, err := catalog.Resolve(service.ashes, request.Host)
		if err != nil {
			return nil, &UnknownAshError{Role: "host", Selector: request.Host, cause: err}
		}
		if err := refuseFixed(host.GoodsFamily, host.Name); err != nil {
			return nil, err
		}
		destination, err := catalog.Resolve(service.ashes, request.Destination)
		if err != nil {
			return nil, &UnknownAshError{Role: "destination", Selector: request.Destination, cause: err}
		}
		// Caught here as well as in plan.Build, because only this layer knows
		// the names the user typed, and the second conversion would silently
		// overwrite the first.
		if previous, taken := claimed[host.GoodsFamily]; taken {
			return nil, &DuplicateHostError{GoodsFamily: host.GoodsFamily, First: previous, Second: host.Name}
		}
		claimed[host.GoodsFamily] = host.Name
		pairs = append(pairs, resolvedPair{host: host, destination: destination})
	}
	return pairs, nil
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

// manifestPathAvailable reports an occupied manifest path before the save work
// starts, so the user is not told the path is taken only after a 26 MB rebuild.
func manifestPathAvailable(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("manifest %s already exists: %w", path, ErrOutputExists)
	}
	// Any other stat failure is left to the O_EXCL create below, which is the
	// real guarantee. This check only exists to fail fast on the common case.
	return nil
}

// writeManifestBeside writes the audit manifest for a save that now exists.
//
// The order is the point. Writing the manifest first would leave an audit
// artifact for an edit that never happened whenever the save write failed or
// was cancelled - a record of a conversion the user does not have. An empty
// path means the caller was not asked for one.
func writeManifestBeside(path string, body []byte, savePath string) error {
	if path == "" {
		return nil
	}
	if err := writeNewFile(path, body); err != nil {
		return fmt.Errorf("save %s was created, but its manifest could not be written: %w", savePath, err)
	}
	return nil
}

func writeNewFile(path string, body []byte) error {
	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		// Lost the race against the check above, or against another writer.
		// O_EXCL is the guarantee; the earlier check is only for a good message.
		return fmt.Errorf("%s already exists: %w", path, ErrOutputExists)
	}
	if err != nil {
		return err
	}
	if _, err := handle.Write(body); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Sync(); err != nil {
		handle.Close()
		return err
	}
	return handle.Close()
}
