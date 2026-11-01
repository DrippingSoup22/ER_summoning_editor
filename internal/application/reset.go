package application

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// ResetPreview describes a reset that has not happened.
//
// It is the reset counterpart of Preview and shares its contract: what it
// promises is what Reset writes, because both are the session's staged reset
// (Session.StageReset).
type ResetPreview struct {
	Save *SaveInfo
	// GoodsRows, BuddyRows and StoneRows are how many rows would actually be
	// written: only those whose current value differs from the shipped one. A
	// save with one converted Ash restores eleven goods rows, not all 924.
	GoodsRows  int
	BuddyRows  int
	StoneRows  int
	OutputPath string
	// AlreadyVanilla means nothing differs from shipped values, so there is
	// nothing to restore. A frontend should say "this save is unedited" rather
	// than report a failure the user cannot act on.
	AlreadyVanilla bool
	// StoneRecipeApplied reports the expanded-area recipe on every stone row,
	// which is what a conversion by this product leaves behind.
	StoneRecipeApplied bool
	// SummonRows and Colours are the game's own summon rows a Fighter left and
	// the colour switch, which the reset puts back too (StagedRestoration).
	SummonRows int
	Colours    bool
	// Characters is how many custom characters the reset deletes.
	Characters int
}

// PreviewReset reports exactly what Reset would do, and writes nothing.
func (service *Service) PreviewReset(ctx context.Context, request Request) (*ResetPreview, error) {
	if err := checkPaths(request, optionalOutputPath); err != nil {
		return nil, err
	}
	// A reset restores every row the product can write, including any a
	// conversion or an authored formation would have just changed, so the
	// requests contradict each other and there is no order that satisfies both.
	if len(request.Conversions) > 0 || request.Authoring != nil {
		return nil, ErrResetTakesNoConversions
	}
	session := service.NewSession()
	if err := session.Load(ctx, request.InputPath); err != nil {
		return nil, err
	}
	save := session.Save()
	preview := &ResetPreview{Save: &save, OutputPath: request.OutputPath}
	staged, err := session.StageReset(ctx)
	if errors.Is(err, ErrAlreadyVanilla) {
		preview.AlreadyVanilla = true
		return preview, nil
	}
	if err != nil {
		return nil, err
	}
	preview.GoodsRows, preview.BuddyRows, preview.StoneRows = staged.GoodsRows, staged.BuddyRows, staged.StoneRows
	preview.StoneRecipeApplied, preview.SummonRows, preview.Colours = staged.StoneRecipeApplied, staged.SummonRows, staged.Colours
	preview.Characters = staged.Characters
	return preview, nil
}

// Reset returns a save to shipped behavior, whatever edited it and however
// many times.
//
// It restores an absolute target state rather than replaying history backwards,
// so it needs no record of the conversions that were applied: a save edited
// three times by three versions of this tool resets in one step. That is the
// whole reason the shipped values are embedded rather than kept in a manifest,
// which only ever knows about the one conversion that produced it.
//
// Like every other operation here, it never touches the input: the result is a
// distinct file, written atomically and reopened before it is published.
func (service *Service) Reset(ctx context.Context, request Request) (*Result, error) {
	// Kept here rather than left to the session, because they are facts about
	// this request shape: a reset names no conversion, and a one-shot call has
	// to know its output path before it does any work rather than at the write.
	if err := checkPaths(request, requireOutputPath); err != nil {
		return nil, err
	}
	if len(request.Conversions) > 0 || request.Authoring != nil {
		return nil, ErrResetTakesNoConversions
	}
	return service.oneShot(ctx, request, func(session *Session) error {
		_, err := session.StageReset(ctx)
		if errors.Is(err, ErrAlreadyVanilla) {
			// Not a failure of the request so much as the answer to it, and
			// the wording a command line has always given.
			return fmt.Errorf("nothing to restore: %w", ErrNoChange)
		}
		return err
	})
}

// shippedVanilla reads the embedded record of what the game shipped, in the
// shape a reset compares against, together with the regulation version it was
// taken from.
//
// The key sets come through unchanged rather than being derived here: which
// rows the game shipped is a fact about the regulation, and this layer has no
// way to know it that is not the snapshot.
func shippedVanilla() (plan.Vanilla, error) {
	snapshot, err := reference.Vanilla()
	if err != nil {
		return plan.Vanilla{}, fmt.Errorf("read the embedded shipped-value snapshot: %w", err)
	}
	shipped := plan.Vanilla{
		Goods:             make(map[int32]map[string]int64, len(snapshot.Goods)),
		Buddy:             make(map[int32]map[string]float64, len(snapshot.Buddy)),
		Stone:             make(map[int32]map[string]int64, len(snapshot.Stone)),
		NpcCloneRows:      snapshot.NpcCloneRows,
		NpcThinkCloneRows: snapshot.NpcThinkCloneRows,
	}
	for rowID := range snapshot.Goods {
		values, ok := snapshot.GoodsRow(rowID)
		if !ok {
			return plan.Vanilla{}, fmt.Errorf("shipped snapshot is missing goods row %d", rowID)
		}
		shipped.Goods[rowID] = values
	}
	for rowID := range snapshot.Buddy {
		values, ok := snapshot.BuddyRow(rowID)
		if !ok {
			return plan.Vanilla{}, fmt.Errorf("shipped snapshot is missing buddy row %d", rowID)
		}
		shipped.Buddy[rowID] = values
	}
	for rowID := range snapshot.Stone {
		values, ok := snapshot.StoneRow(rowID)
		if !ok {
			return plan.Vanilla{}, fmt.Errorf("shipped snapshot is missing stone row %d", rowID)
		}
		shipped.Stone[rowID] = values
	}
	return shipped, nil
}

// currentVanilla reads what a save says now for every field a reset restores.
//
// It uses the same bounded reader the writer is paired with, so what is
// compared is exactly what would be written. Everything it reports that the
// shipped record does not name was added by this editor, and is what a reset
// removes - so of the stone rows the game did not ship it reports only the
// woken monuments, the one kind this editor adds.
func currentVanilla(ctx context.Context, path string, shipped plan.Vanilla) (plan.Vanilla, *savecore.VanillaSnapshot, error) {
	goodsRows := make([]int32, 0, len(shipped.Goods))
	for rowID := range shipped.Goods {
		goodsRows = append(goodsRows, rowID)
	}
	slices.Sort(goodsRows)

	snapshot, err := savecore.ReadVanillaSnapshot(ctx, path, goodsRows)
	if err != nil {
		return plan.Vanilla{}, nil, err
	}
	current := plan.Vanilla{
		Goods:             make(map[int32]map[string]int64, len(snapshot.Goods)),
		Buddy:             make(map[int32]map[string]float64, len(snapshot.Buddy)),
		Stone:             make(map[int32]map[string]int64, len(snapshot.Stone)),
		NpcCloneRows:      snapshot.NpcCloneRows,
		NpcThinkCloneRows: snapshot.NpcThinkCloneRows,
	}
	for rowID, values := range snapshot.Goods {
		current.Goods[rowID] = maps.Clone(values)
	}
	for rowID, values := range snapshot.Buddy {
		current.Buddy[rowID] = maps.Clone(values)
	}
	sleeping := savecore.SleepingMonuments()
	for rowID, values := range snapshot.Stone {
		if _, ships := shipped.Stone[rowID]; ships || slices.Contains(sleeping, rowID) {
			current.Stone[rowID] = maps.Clone(values)
		}
	}
	return current, snapshot, nil
}

// stoneRecipeApplied reports the expanded-area recipe sitting on every stone
// row, which is the state a conversion by this product leaves behind. It is
// descriptive only: what gets restored is decided row by row, not by this.
func stoneRecipeApplied(snapshot *savecore.VanillaSnapshot) bool {
	recipe := map[string]int64{
		"activateRange": 65535, "overwriteReturnRange": -1,
		"overwriteActivateRegionEntityId": 0, "warnRegionEntityId": 0,
		"summonedEventFlagId": 0, "eliminateTargetEntityId": 0,
	}
	if len(snapshot.Stone) == 0 {
		return false
	}
	for _, values := range snapshot.Stone {
		for name, want := range recipe {
			if values[name] != want {
				return false
			}
		}
	}
	return true
}
