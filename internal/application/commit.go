package application

// Committing: turning everything staged into one write.
//
// Two things happen here and they are deliberately separate. PrepareCommit
// snapshots the staged set into an immutable plan; Commit writes that plan.
// They are apart because a desktop frontend asks the operating system where to
// save, and a native dialog is modal to the operating system rather than to the
// application - so a plan assembled after the dialog opened could silently
// include or omit an edit the player made while it was up. Freeze the plan,
// then ask where it goes.
//
// One write, not a chain of them. Goods, buddy, stone and both actor tables all
// live in the one regulation, so a commit is one structural stage, one patch
// stage, one bounded-mutation proof and one atomic publication however many
// operations it carries. Applying the staged changes one after another through
// temporary files would mean rebuilding 26 MB once per operation and producing
// one audit artifact per operation for what the player performed as a single
// edit.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"slices"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/formation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// CommitSummary is what one commit would do, in the terms the player recognises
// on top of the rows underneath them.
type CommitSummary struct {
	Save        SaveInfo
	Formations  []*StagedFormation
	Conversions []*StagedConversion
	Costs       []*StagedCost
	Restoration *StagedRestoration
	// Grants are the Ashes going into a character's inventory. They write no
	// regulation row, so none of the counts below covers them.
	Grants []*StagedGrant
	// Strength is the roles chosen for the game's own summon rows, when any
	// is staged.
	Strength *StagedStrength

	// GoodsRows, BuddyRows and StoneRows are how many rows the write touches,
	// after everything overridden below has been taken out.
	GoodsRows int
	BuddyRows int
	StoneRows int
	// NpcRows is how many of the game's own summon rows the write gives
	// another role, a reset's included. Counted apart
	// from BuddyRows because they are a different table and a different claim:
	// one is which creature a formation summons, the other is what that
	// creature is worth.
	NpcRows int
	// RowsInserted and RowsRemoved are how the save's row set changes.
	RowsInserted int
	RowsRemoved  int
	// RestorationOverridden is how many rows a staged reset would have restored
	// or removed that another staged change claimed instead. It is worth
	// reporting: a player who resets and then edits has asked for two things,
	// and this is the number that says the second one won where they met.
	RestorationOverridden int
}

// CommitPlan is an immutable snapshot of everything staged, ready to write.
//
// Nothing about it can change once it exists. Later staging does not reach it,
// and it carries the manifest bytes rather than a way to rebuild them, so what
// Commit writes is exactly what the summary describes.
type CommitPlan struct {
	summary  CommitSummary
	manifest []byte
	// grants are the items to put in a character's inventory. They are not in
	// the manifest and cannot be: it describes regulation rows, and a grant
	// writes inside a character slot. Carried on the plan so that what Save
	// writes is still decided entirely before the dialog opens.
	grants []*StagedGrant
	// regulation reports whether anything at all is being written to the
	// regulation. A commit of grants alone has an empty manifest and copies the
	// save instead of patching it.
	regulation bool
}

// Summary describes what this plan would do.
func (commit *CommitPlan) Summary() CommitSummary { return commit.summary }

// Manifest is the audit artifact this plan would write, byte for byte. It is a
// copy: a caller cannot change what gets applied by editing what it was shown.
func (commit *CommitPlan) Manifest() []byte { return bytes.Clone(commit.manifest) }

// PrepareCommit snapshots every staged change into one immutable plan.
//
// It writes nothing and changes nothing about the session, so a frontend may
// call it whenever it wants to describe what Save would do.
func (session *Session) PrepareCommit() (*CommitPlan, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	if session.PendingCount() == 0 {
		return nil, ErrNothingStaged
	}
	if err := session.validateStaged(); err != nil {
		return nil, err
	}
	staged := session.Staged()
	sourceSHA256 := session.loaded.info.SHA256

	// Order is fixed rather than incidental: conversions, then formations by
	// host, then the restoration. Two commits of one staged set must produce
	// the same manifest bytes, or a diff between two audit artifacts stops
	// meaning anything.
	//
	// Narrowing runs in the opposite direction, from the most specific staged
	// change to the least. A formation names one block; a conversion applies a
	// blanket recipe to every row that ships the monument flag; a reset speaks
	// for the whole save. Where they meet, the one that named the row wins.
	// The creature rows are decided HERE, over the whole save (clonealloc.go):
	// every staged formation placed together, a recipe the save already holds
	// reused where it stands, every other recipe on the lowest free band, and
	// the rows this editor inserted that nothing will name pruned. A staged
	// reset removes this editor's rows by its own rule, so beside one nothing
	// is pruned or freed. Two Ashes that summon one recipe then share its row:
	// the first inserts it and the rest name it (share.go).
	placedFormations, pruneNpc, pruneThink, err := session.placeStaged(staged)
	if err != nil {
		return nil, err
	}
	staged.Formations = placedFormations
	formations, err := shareClones(staged.Formations)
	if err != nil {
		return nil, err
	}

	var parts []*plan.Manifest
	if len(staged.Costs) > 0 {
		costs := make([]plan.Cost, 0, len(staged.Costs))
		for _, staged := range staged.Costs {
			costs = append(costs, staged.cost)
		}
		// One part for all of them, like conversions, though for a duller
		// reason: they need no shipping recipe and could have been one part
		// each. One is fewer manifest entries saying the same thing.
		priced, err := plan.BuildCost(sourceSHA256, costs)
		if err != nil {
			return nil, err
		}
		parts = append(parts, priced)
	}
	// The monument switches, resolved once for the whole session. A
	// conversion carries them and so does the standalone part, so exactly one
	// of the two emits them: two parts both claiming the stone table is a
	// composition the session checker refuses, and rightly.
	switching, err := session.areaRecipe()
	if err != nil {
		return nil, err
	}
	recipe := conversionRecipe(session.loaded.state)
	if switching != nil {
		recipe = *switching
	}

	if len(staged.Conversions) > 0 {
		conversions := make([]plan.Conversion, 0, len(staged.Conversions))
		for _, conversion := range staged.Conversions {
			conversions = append(conversions, conversion.conversion)
		}
		// One part for all of them, not one each. Conversions are not
		// independent: they share the single shipping recipe that makes any of
		// them work, and a recipe written twice is two parts claiming the whole
		// stone table.
		converted, err := plan.Build(sourceSHA256, conversions, recipe)
		if err != nil {
			return nil, err
		}
		if err := narrowConversion(converted, formations); err != nil {
			return nil, err
		}
		parts = append(parts, converted)
	} else if switching != nil {
		// Nothing to convert, and the player moved the switch. Without this the
		// switch would do nothing at all on a save whose formations are all
		// authored, which is the case it was added for: authoring writes the
		// host's own block and has nothing to say to the monument table.
		area, err := plan.BuildArea(sourceSHA256, recipe)
		if err != nil {
			return nil, err
		}
		narrowArea(area, formations)
		parts = append(parts, area)
	}
	parts = append(parts, formations...)
	// The prune rides only on a commit that writes the regulation anyway: a
	// save whose only change is an item grant is copied, as it always was.
	if len(parts) > 0 && len(pruneNpc)+len(pruneThink) > 0 {
		pruned, err := plan.BuildPrune(sourceSHA256, pruneNpc, pruneThink)
		if err != nil {
			return nil, err
		}
		parts = append(parts, pruned)
	}
	// The game's own summon rows: the roles chosen for them, and beside a
	// staged reset every other one an earlier Fighter left, back to a Helper.
	// After the formations, because the two write different rows: a formation
	// inserts its clone and carries its own recipe, and these are only ever
	// rows FromSoftware ships, which nothing prunes.
	var summonRows []plan.RowValues
	if staged.Strength != nil {
		summonRows = append(summonRows, staged.Strength.rows...)
	}
	if staged.Restoration != nil {
		summonRows = append(summonRows, session.helpersBack()...)
	}
	if len(summonRows) > 0 {
		change, err := plan.BuildStrength(sourceSHA256, summonRows, "Summon strength")
		if err != nil {
			return nil, err
		}
		parts = append(parts, change)
	}

	// The shared effect writes, decided over the save as it will be written
	// (sharedeffects.go). Like the prune it rides only on a commit that writes
	// the regulation anyway - a reset among them, which puts them back.
	if len(parts) > 0 || staged.Restoration != nil || len(staged.Builds)+len(staged.DroppedBuilds) > 0 {
		// The colour switch an earlier build wrote, put back (tint.go). It
		// writes one field of SpEffectVfxParam that nothing else touches.
		tint, err := session.tintPart(staged.Restoration != nil)
		if err != nil {
			return nil, err
		}
		if tint != nil {
			parts = append(parts, tint)
		}
		shared, err := session.sharedEffectsPart(formations, staged.Restoration != nil)
		if err != nil {
			return nil, err
		}
		if shared != nil {
			parts = append(parts, shared)
		}
		// The player's own characters, decided the same way (characters.go).
		characters, err := session.charactersPart(formations, staged.Restoration != nil, len(summonRows) > 0)
		if err != nil {
			return nil, err
		}
		if characters != nil {
			parts = append(parts, characters)
		}
		// The effect rows of our own the clones name, decided the same way
		// (owneffects.go).
		own, err := session.ownEffectsPart(formations, staged.Formations, pruneNpc, staged.Restoration != nil)
		if err != nil {
			return nil, err
		}
		if own != nil {
			parts = append(parts, own)
		}
	}

	overridden := 0
	if staged.Restoration != nil {
		// The restoration goes last and yields where it meets anything else.
		// That is the whole conflict rule, and it falls out of what the two
		// mean rather than being a table of cases: a reset says "put back
		// everything I have not said otherwise about", so a row another staged
		// change names is a row it has been said otherwise about.
		if staged.Restoration.manifest != nil {
			narrowed, dropped := narrowRestoration(staged.Restoration.manifest, parts, staged.Formations)
			overridden = dropped
			if narrowed != nil {
				parts = append(parts, narrowed)
			}
		}
	}
	if len(parts) == 0 && len(staged.Grants) == 0 {
		// Reachable only through a staged reset that every other staged change
		// completely overrode, which is a save that would be written exactly as
		// it was read.
		return nil, fmt.Errorf("every staged change is already true of this save: %w", ErrNoChange)
	}
	if len(parts) == 0 {
		// Grants alone. There is no manifest to compose, because nothing in the
		// regulation changes; the save is copied and the items are written into
		// the copy.
		summary := CommitSummary{Save: session.loaded.info}
		return &CommitPlan{summary: summary, grants: staged.Grants}, nil
	}

	manifest, err := plan.Compose(sourceSHA256, parts)
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	if err := manifest.Encode(&encoded); err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}

	summary := countRows(parts, session.loaded.state.StoneRows)
	summary.Save = session.loaded.info
	summary.Grants = staged.Grants
	summary.Formations = staged.Formations
	summary.Conversions = staged.Conversions
	summary.Costs = staged.Costs
	summary.Restoration = staged.Restoration
	summary.Strength = staged.Strength
	summary.RestorationOverridden = overridden

	return &CommitPlan{manifest: encoded.Bytes(), summary: summary, grants: staged.Grants, regulation: true}, nil
}

// placeStaged places every staged formation over the whole save
// (placeFormations) and answers with a copy of each carrying what Save writes
// for it, together with the creature rows the prune removes. The session's own
// staged formations are not changed: PrepareCommit changes nothing about the
// session.
func (session *Session) placeStaged(staged Staged) ([]*StagedFormation, []int32, []int32, error) {
	var units []authoredUnit
	for _, formation := range staged.Formations {
		units = append(units, formation.units...)
	}
	placed, err := placeFormations(context.Background(), session.loaded.info.Path, session.loaded.info.SHA256,
		session.loaded.formations, units, session.service.families, staged.Restoration == nil)
	if err != nil {
		return nil, nil, nil, err
	}
	out := make([]*StagedFormation, len(staged.Formations))
	first := 0
	for index, formation := range staged.Formations {
		copied := *formation
		copied.Requested = slices.Clone(formation.Requested)
		copied.Formation = placed.described[first]
		copied.manifests = placed.manifests[first : first+len(formation.units)]
		first += len(formation.units)
		out[index] = &copied
	}
	return out, placed.pruneNpc, placed.pruneThink, nil
}

// oneShot performs a whole operation in one call, through a session.
//
// Service is one call per intent and Session is many intents per commit, but
// they are not two ways of writing a save: there is one, and this is where the
// first shape borrows it. Everything that decides what reaches the file - what
// a part may write, how parts compose, what the paths must not collide with,
// when the manifest is written - lives on the session, so a rule added there
// reaches the command line without being written a second time.
//
// The caller keeps whatever belongs to its own shape: which requests are
// refused together, and how a staging failure is worded.
func (service *Service) oneShot(ctx context.Context, request Request,
	stage func(*Session) error) (*Result, error) {

	session := service.NewSession()
	if err := session.Load(ctx, request.InputPath); err != nil {
		return nil, err
	}
	if err := stage(session); err != nil {
		return nil, err
	}
	commit, err := session.PrepareCommit()
	if err != nil {
		return nil, err
	}
	result, err := session.Commit(ctx, commit, request.OutputPath, request.ManifestPath)
	if result != nil {
		// Commit hands back the result AND an error when the save was written
		// but the session could not reopen it. That matters to an editor,
		// which is about to keep working on the file it just made. A one-shot
		// call is finished: the save core validated the output before
		// publishing it, so a failure to reopen afterwards is not a failure of
		// the operation, and reporting one while leaving the file on disk
		// would be the least true thing this could say.
		//
		// Everything else after the write already works this way - a grant
		// that fails is reported in the result rather than failing the commit.
		return result, nil
	}
	return nil, err
}

// Commit writes a prepared plan to a new save.
//
// The input is never touched, the output must not exist, and the manifest is
// written only once the save does - an audit artifact for an edit that never
// happened is a record of something the player does not have.
//
// On success the session advances to the output and its staged changes are
// cleared, so the player carries on editing the file they just made rather than
// the one they started from. If that reload fails the save is still written and
// still correct: Commit then returns both the result and an error saying the
// session did not follow, and the caller should offer the output as a fresh
// Load rather than treat the write as failed.
func (session *Session) Commit(ctx context.Context, commit *CommitPlan, outputPath, manifestPath string) (*Result, error) {
	if session.loaded == nil {
		return nil, ErrNoSaveLoaded
	}
	if commit == nil {
		return nil, ErrNothingStaged
	}
	// The plan pins the save it was made against, and so does the manifest
	// inside it. Refusing here says which of the two moved; the save core would
	// otherwise report a hash mismatch for a file the player never chose.
	if commit.summary.Save.SHA256 != session.loaded.info.SHA256 {
		return nil, fmt.Errorf("this plan was made for a different save: %w", ErrSourceMismatch)
	}
	request := Request{
		InputPath: session.loaded.info.Path, OutputPath: outputPath, ManifestPath: manifestPath,
	}
	if err := checkPaths(request, requireOutputPath); err != nil {
		return nil, err
	}
	if err := manifestPathAvailable(manifestPath); err != nil {
		return nil, err
	}

	var summary savecore.Summary
	if commit.regulation {
		applied, err := savecore.Apply(ctx, request.InputPath, outputPath, bytes.NewReader(commit.manifest))
		if err != nil {
			return nil, fmt.Errorf("apply staged changes: %w", err)
		}
		summary = *applied
	} else {
		// Grants alone: nothing in the regulation changes, so the save is
		// copied and the items are written into the copy. The input is still
		// only ever read.
		copied, err := copySave(request.InputPath, outputPath, session.loaded.info)
		if err != nil {
			return nil, err
		}
		summary = *copied
	}
	if err := writeManifestBeside(manifestPath, commit.manifest, summary.OutputPath); err != nil {
		return nil, err
	}
	result := resultFrom(summary)
	for _, conversion := range commit.summary.Conversions {
		result.Conversions = append(result.Conversions, conversion.ResolvedConversion)
	}
	if len(commit.summary.Formations) == 1 {
		authored := commit.summary.Formations[0].Formation
		result.Authored = &authored
	}

	// The items go in after the regulation, into this commit's own output. A
	// grant that fails does not fail the commit: the regulation edits are
	// already written and already correct, and discarding a good save because
	// an item could not be added would cost the player what they came for.
	granted, failures := applyGrants(outputPath, commit.grants)
	result.ItemsGranted = granted
	for _, failure := range failures {
		result.ItemsRefused = append(result.ItemsRefused, failure.Error())
	}
	// The digest is taken again because the grants were written after the one
	// the save core produced, and a digest that described the file before the
	// last write to it is a false record - the same objection this product
	// makes to a manifest for a save that was never written.
	//
	// Recomputed unconditionally rather than only when a grant was made: a step
	// added after this one would otherwise reintroduce the same staleness
	// silently, and re-reading a file already on disk is cheap beside writing
	// it.
	if digest, err := digestOf(outputPath); err != nil {
		return result, fmt.Errorf("%s was written, but its digest could not be taken: %w", outputPath, err)
	} else {
		result.OutputSHA256 = digest
	}

	if err := session.Load(ctx, outputPath); err != nil {
		return result, fmt.Errorf("%s was written, but the session could not reopen it: %w", outputPath, err)
	}
	return result, nil
}

// copySave writes the input to a new path unchanged, for a commit that only
// grants items. It goes through the same never-overwrite check as any other
// output and reports the same summary shape, so the caller cannot tell which
// of the two paths produced the file it is holding.
func copySave(inputPath, outputPath string, info SaveInfo) (*savecore.Summary, error) {
	file, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", inputPath, err)
	}
	if err := writeNewFile(outputPath, file); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(file)
	return &savecore.Summary{
		InputPath: inputPath, OutputPath: outputPath,
		InputSHA256: info.SHA256, OutputSHA256: hex.EncodeToString(digest[:]),
		RegulationVersion: info.RegulationVersion,
	}, nil
}

// narrowConversion drops the monument-origin fix for rows an authored formation
// already writes.
//
// The two agree about the byte. A conversion clears appearOnAroundSekihi on the
// twenty rows that ship it set, so that a formation summoned through the
// converted Ash spawns where the player is standing rather than at a distant
// monument; an authored row writes the same 0 as part of writing the whole row.
// They are not in conflict, but they are two parts claiming one row, and a
// composition that let both through would be a composition that could not tell
// this case from a real disagreement. The formation named the row, so it keeps
// it.
//
// A retired row is the one case where the values differ: retiring writes back
// whatever the save holds, which may be the 1 the game shipped. That is
// harmless and deliberate - a retired row carries no trigger, so nothing ever
// resolves it, and the placement of a formation that summons nothing is not a
// property anything can observe.
//
// Emptying the recipe entirely would leave a conversion the save core refuses,
// because part one of the shipping fix without part two is a bug rather than a
// partial fix. It cannot happen with the shipped catalog: twelve of the twenty
// monument rows belong to Giant Rat Ashes, which varies by upgrade level and
// therefore cannot be authored at all. It is reported rather than assumed away,
// because that reasoning is about a catalog and catalogs are data.
func narrowConversion(conversion *plan.Manifest, formations []*plan.Manifest) error {
	if len(formations) == 0 {
		return nil
	}
	claimed := make(map[int32]bool)
	for _, formation := range formations {
		for _, patch := range formation.BuddyPatches {
			claimed[patch.RowID] = true
		}
	}
	kept := make([]plan.RowPatch, 0, len(conversion.BuddyPatches))
	for _, patch := range conversion.BuddyPatches {
		if claimed[patch.RowID] {
			continue
		}
		kept = append(kept, patch)
	}
	if len(kept) == 0 {
		return fmt.Errorf(
			"every monument-origin row this conversion fixes is also written by an authored formation, so the conversion has no shipping fix left to carry: %w",
			ErrStagedChangesCannotCombine)
	}
	conversion.BuddyPatches = kept
	return nil
}

// narrowArea drops from the monument switch the rows an authored formation is
// already writing.
//
// Twenty BuddyParam rows carry the origin flag and every one of them is also a
// formation block, so a save whose formations are authored will collide with
// the switch on any of them it happens to use. There is no conflict to
// resolve: authoring writes the same 0 the switch's on position writes, and the
// off position hands back the shipped value on rows nobody authored.
//
// Unlike narrowConversion this does not refuse when every row is claimed. A
// conversion with no origin fix left has lost the half that makes it work; the
// switch has not, because the stone table is the half that decides where a
// summon may be called and no formation touches it.
func narrowArea(area *plan.Manifest, formations []*plan.Manifest) {
	if len(formations) == 0 {
		return
	}
	claimed := make(map[int32]bool)
	for _, formation := range formations {
		for _, patch := range formation.BuddyPatches {
			claimed[patch.RowID] = true
		}
	}
	kept := make([]plan.RowPatch, 0, len(area.BuddyPatches))
	for _, patch := range area.BuddyPatches {
		if claimed[patch.RowID] {
			continue
		}
		kept = append(kept, patch)
	}
	area.BuddyPatches = kept
}

// narrowRestoration drops from a staged reset everything a more specific staged
// change already writes, and reports how many rows it gave up.
//
// A reset and an edit are not really in conflict, which is why this resolves
// rather than refuses. A reset means "put back everything I have not said
// otherwise about", so a row another staged change names is exactly a row it
// has been said otherwise about. What the player gets is a save restored to
// shipped behaviour except where they asked for something else - which is what
// resetting and then editing plainly means.
//
// It returns nil when nothing is left to restore.
// goodsClaim is one field of one goods row: the unit a staged change claims in
// EquipParamGoods, which is the one table two staged changes may share.
type goodsClaim struct {
	row   int32
	field string
}

func narrowRestoration(restoration *plan.Manifest, others []*plan.Manifest, formations []*StagedFormation) (*plan.Manifest, int) {
	// Goods claims are per field and the rest are per row, for the reason the
	// save core's composition check gives: a conversion re-points a row and a
	// cost edit prices it, and a reset that yielded the whole row to either
	// would stop restoring the field neither of them writes.
	claimedGoods := make(map[goodsClaim]bool)
	claimedBuddy := make(map[int32]bool)
	// Rows something else needs, which is every row another change writes into
	// plus every clone-namespace actor a formation points at. The second half
	// matters and is easy to miss: a formation composed against this save may
	// reuse a clone that already exists, so it inserts nothing and still cannot
	// have that row removed underneath it.
	neededRows := make(map[int32]bool)
	wholeStoneTable := false
	for _, other := range others {
		for _, patch := range other.GoodsPatches {
			for name := range patch.Set {
				claimedGoods[goodsClaim{row: patch.RowID, field: name}] = true
			}
		}
		for _, patch := range other.BuddyPatches {
			claimedBuddy[patch.RowID] = true
			neededRows[patch.RowID] = true
		}
		if len(other.BuddyStonePatches)+len(other.BuddyStoneRowPatches)+len(other.StoneInsertions)+len(other.StoneRemovals) > 0 {
			wholeStoneTable = true
		}
	}
	for _, staged := range formations {
		for _, slot := range staged.Formation.Slots {
			for _, rowID := range [2]int32{slot.Npc, slot.Think} {
				if rowID >= formation.CloneOffset {
					neededRows[rowID] = true
				}
			}
		}
	}

	narrowed := &plan.Manifest{
		SourceSHA256: restoration.SourceSHA256,
		Operation:    restoration.Operation,
	}
	dropped := 0
	for _, patch := range restoration.GoodsPatches {
		kept := make(map[string]any, len(patch.Set))
		for name, value := range patch.Set {
			if !claimedGoods[goodsClaim{row: patch.RowID, field: name}] {
				kept[name] = value
			}
		}
		if len(kept) == 0 {
			dropped++
			continue
		}
		patch.Set = kept
		narrowed.GoodsPatches = append(narrowed.GoodsPatches, patch)
	}
	for _, patch := range restoration.BuddyPatches {
		if claimedBuddy[patch.RowID] {
			dropped++
			continue
		}
		narrowed.BuddyPatches = append(narrowed.BuddyPatches, patch)
	}
	// The monument switches say where every stone row stands, so a per-row
	// restoration of that table cannot survive beside them at all: the
	// switches are what was said otherwise about it, and a conversion does not
	// work without its recipe - part one without part two makes a formation
	// spawn at a distant monument and appear to vanish.
	if wholeStoneTable {
		dropped += len(restoration.BuddyStoneRowPatches)
	} else {
		narrowed.BuddyStoneRowPatches = restoration.BuddyStoneRowPatches
		narrowed.StoneRemovals = restoration.StoneRemovals
	}
	// Removals are the same rule seen from the other side: a row something else
	// needs is a row this reset no longer takes out. The block a formation was
	// authored into therefore keeps the size an earlier edit left it, with any
	// surplus rows retired rather than deleted - a reset on its own still
	// removes them.
	narrowed.BuddyRowRemovals = keepUnneeded(restoration.BuddyRowRemovals, neededRows)
	narrowed.NpcCloneRemovals = keepUnneeded(restoration.NpcCloneRemovals, neededRows)
	narrowed.NpcThinkCloneRemovals = keepUnneeded(restoration.NpcThinkCloneRemovals, neededRows)
	dropped += removedRows(restoration) - removedRows(narrowed)

	if len(narrowed.GoodsPatches)+len(narrowed.BuddyPatches)+
		len(narrowed.BuddyStoneRowPatches)+removedRows(narrowed) == 0 {
		return nil, dropped
	}
	return narrowed, dropped
}

// keepUnneeded returns the removals that survive: a row something else staged
// needs is a row this reset no longer takes out.
func keepUnneeded(rows []int32, needed map[int32]bool) []int32 {
	var kept []int32
	for _, rowID := range rows {
		if !needed[rowID] {
			kept = append(kept, rowID)
		}
	}
	return kept
}

func removedRows(manifest *plan.Manifest) int {
	return len(manifest.BuddyRowRemovals) + len(manifest.NpcCloneRemovals) + len(manifest.NpcThinkCloneRemovals) +
		len(manifest.CharacterRowRemovals) + len(manifest.OwnEffectRemovals) + len(manifest.StoneRemovals)
}

// countRows totals what the commit writes. stoneRows is the one count that
// cannot come from a list length: the all-row recipe writes the whole table, so
// its size has to come from the save.
func countRows(parts []*plan.Manifest, stoneTableRows int) CommitSummary {
	var summary CommitSummary
	for _, part := range parts {
		summary.GoodsRows += len(part.GoodsPatches)
		summary.BuddyRows += len(part.BuddyPatches)
		summary.NpcRows += len(part.NpcPatches)
		summary.RowsInserted += len(part.NpcCloneSources) + len(part.NpcThinkCloneSources) + len(part.BuddyRowInsertions) +
			len(part.OwnEffectInsertions) + len(part.StoneInsertions)
		for _, row := range part.CharacterRows {
			if row.Insert {
				summary.RowsInserted++
			}
		}
		for _, body := range part.CharacterBodies {
			if body.Insert {
				summary.RowsInserted++
			}
		}
		summary.RowsRemoved += removedRows(part)
		if len(part.BuddyStonePatches) > 0 {
			// At most one part carries the all-row recipe, and a per-row
			// restoration sits beside it only in the same part, on rows it
			// already counts - savecore refuses the other arrangements - so
			// this assigns where the others add.
			summary.StoneRows = stoneRowsAfter(stoneTableRows, part)
			continue
		}
		summary.StoneRows += len(part.BuddyStoneRowPatches)
	}
	return summary
}

// stoneRowsAfter is how many stone rows the all-row recipe writes: the table
// as the part leaves it, the woken monuments in and the sleeping ones out.
func stoneRowsAfter(rows int, part *plan.Manifest) int {
	return rows - len(part.StoneRemovals) + len(part.StoneInsertions)
}

// digestOf is the SHA-256 of a file on disk, for reporting what was actually
// written rather than what was about to be.
func digestOf(path string) (string, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(file)
	return hex.EncodeToString(sum[:]), nil
}
