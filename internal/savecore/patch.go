// Package savecore contains the complete, product-owned save boundary for v1.
// It supports only the three PARAM edits emitted by the Summoning Editor and
// deliberately exposes no generic game-data mutation API.
package savecore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// Info is the non-private metadata a frontend needs before constructing a
// source-pinned edit plan.
type Info struct {
	SHA256            string
	RegulationVersion uint32
	CompressedSize    int
	EncryptedCapacity int
	DecompressedSize  int
}

// Summary contains verification metadata and no private save contents.
type Summary struct {
	InputPath          string
	OutputPath         string
	InputSHA256        string
	OutputSHA256       string
	RegulationVersion  uint32
	OriginalStreamSize int
	NewStreamSize      int
	Capacity           int
	GoodsRows          int
	BuddyRows          int
	BuddyStoneRows     int
	// RowsInserted and RowsRemoved are what the structural stage did: how many
	// rows the save gained and lost across all tables. A patch count alone
	// cannot say, because an authored formation writes the same four fields to
	// a row it added and to a row that shipped.
	RowsInserted int
	RowsRemoved  int
}

// Inspect validates a decrypted PS4 save and returns display-safe metadata.
func Inspect(ctx context.Context, path string) (*Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := loadSave(path)
	if err != nil {
		return nil, err
	}
	return &Info{
		SHA256: regulation.Sha256Hex(loaded.FileBytes), RegulationVersion: loaded.Version,
		CompressedSize: loaded.CompressedSize, EncryptedCapacity: loaded.CiphertextLen,
		DecompressedSize: len(loaded.BND4),
	}, nil
}

// Apply strictly decodes a v1 manifest, proves every changed byte belongs to a
// declared row, writes a distinct output atomically, and reopens it before
// publication.
//
// Cancellation is checked between stages rather than inside them: reading 26 MB
// and recompressing it are single blocking calls that cannot be interrupted
// part-way. What the context does guarantee is that a cancelled operation never
// publishes an output file — the last check sits immediately before the write.
func Apply(ctx context.Context, savePath, outPath string, encodedManifest io.Reader) (*Summary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inputAbsolute, err := filepath.Abs(savePath)
	if err != nil {
		return nil, err
	}
	outputAbsolute, err := filepath.Abs(outPath)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Clean(inputAbsolute), filepath.Clean(outputAbsolute)) {
		return nil, fmt.Errorf("output must differ from input")
	}

	plan, err := decodeManifest(encodedManifest)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, ErrManifestInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := loadSave(savePath)
	if err != nil {
		return nil, err
	}
	inputHash := regulation.Sha256Hex(loaded.FileBytes)
	if inputHash != plan.SourceSHA256 {
		return nil, fmt.Errorf("input SHA-256 is %s, manifest requires %s: %w", inputHash, plan.SourceSHA256, ErrSourceMismatch)
	}
	originalBND4 := bytes.Clone(loaded.BND4)

	// The structural stage first, and separately, because a patch and a row-set
	// change cannot share a proof: see internal/savecore/structural.go. What
	// the patches below then run against is a regulation whose row set is
	// already final, so every one of them is an ordinary in-place write into a
	// row that exists.
	if plan.hasStructuralWork() {
		changed, err := applyStructuralChanges(loaded.BND4, plan)
		if err != nil {
			return nil, err
		}
		if err := verifyStructuralStage(loaded.BND4, changed, plan); err != nil {
			return nil, fmt.Errorf("structural stage: %w", err)
		}
		loaded.BND4 = changed
	}
	// The baseline the byte-window proof is stated against. It is the
	// post-structural regulation rather than the input, because the two have
	// different lengths whenever a row was added or removed, and comparing
	// across that would be comparing two different address spaces.
	patchBaseline := bytes.Clone(loaded.BND4)

	goodsRanges, err := applyGoods(loaded.BND4, plan.GoodsPatches)
	if err != nil {
		return nil, err
	}
	buddyRanges, err := applyBuddy(loaded.BND4, plan.BuddyPatches, plan.Operation != operationConvert)
	if err != nil {
		return nil, err
	}
	npcRanges, err := applyNpc(loaded.BND4, plan.NpcPatches)
	if err != nil {
		return nil, err
	}
	stoneRanges, stoneRows, err := applyStoneChanges(loaded.BND4, plan)
	if err != nil {
		return nil, err
	}
	vfxRanges, err := applyVfx(loaded.BND4, plan.VfxPatches)
	if err != nil {
		return nil, err
	}
	effectRanges, err := applyEffects(loaded.BND4, plan.EffectPatches)
	if err != nil {
		return nil, err
	}
	personRanges, err := applyPersons(loaded.BND4, plan.PersonPatches)
	if err != nil {
		return nil, err
	}
	characterRanges, err := applyCharacterRows(loaded.BND4, plan.CharacterRows)
	if err != nil {
		return nil, err
	}
	// After the NpcParam patches, so a body copies its shell as the commit
	// leaves it.
	bodyRanges, err := applyCharacterBodies(loaded.BND4, plan.CharacterBodies)
	if err != nil {
		return nil, err
	}
	ranges := append(append(append(append(append(append(append(append(goodsRanges, buddyRanges...), npcRanges...), stoneRanges...), vfxRanges...), effectRanges...), personRanges...), characterRanges...), bodyRanges...)
	if bytes.Equal(originalBND4, loaded.BND4) {
		return nil, fmt.Errorf("manifest changed no save bytes: %w", ErrNoChange)
	}
	if err := verifyBoundedMutation(patchBaseline, loaded.BND4, ranges); err != nil {
		return nil, fmt.Errorf("bounded mutation: %w", err)
	}
	// On the regulation as it will be written, after every patch: the strength
	// switch rewrites resident slots too.
	if err := verifyOwnEffectsNamed(loaded.BND4); err != nil {
		return nil, fmt.Errorf("own effect rows: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	outputBytes, writeResult, err := regulation.Build(loaded)
	if err != nil {
		return nil, err
	}
	if len(outputBytes) != len(loaded.FileBytes) {
		return nil, fmt.Errorf("rebuilt save size changed from %d to %d", len(loaded.FileBytes), len(outputBytes))
	}
	// The last opportunity to abandon the operation without leaving a file
	// behind. Everything after this point publishes.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := regulation.WriteAtomicValidated(outPath, outputBytes, func(candidatePath string) error {
		candidate, err := loadSave(candidatePath)
		if err != nil {
			return err
		}
		if candidate.Version != loaded.Version {
			return fmt.Errorf("regulation version changed from %d to %d", loaded.Version, candidate.Version)
		}
		if !bytes.Equal(candidate.BND4, loaded.BND4) {
			return fmt.Errorf("reopened BND4 differs from the intended edit")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	currentInput, err := os.ReadFile(savePath)
	if err != nil {
		return nil, fmt.Errorf("recheck input: %w", err)
	}
	if currentHash := regulation.Sha256Hex(currentInput); currentHash != inputHash {
		return nil, fmt.Errorf("input changed during operation: %s -> %s", inputHash, currentHash)
	}
	return &Summary{
		InputPath: savePath, OutputPath: outPath,
		InputSHA256: inputHash, OutputSHA256: regulation.Sha256Hex(outputBytes),
		RegulationVersion:  loaded.Version,
		OriginalStreamSize: writeResult.OriginalStreamSize,
		NewStreamSize:      writeResult.NewStreamSize, Capacity: writeResult.Capacity,
		GoodsRows: len(plan.GoodsPatches), BuddyRows: len(plan.BuddyPatches),
		BuddyStoneRows: stoneRows,
		RowsInserted: len(plan.NpcCloneSources) + len(plan.NpcThinkCloneSources) +
			len(plan.NpcSummonCopies) + len(plan.BuddyRowInsertions) + countIDs(plan.characterInsertions()) +
			len(plan.OwnEffectInsertions),
		RowsRemoved: len(plan.BuddyRowRemovals) + len(plan.NpcCloneRemovals) +
			len(plan.NpcThinkCloneRemovals) + len(plan.CharacterRowRemovals) + len(plan.OwnEffectRemovals),
	}, nil
}
