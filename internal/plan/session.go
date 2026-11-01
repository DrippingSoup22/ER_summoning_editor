package plan

// Composition: one editing session's worth of staged operations, written as a
// single manifest.
//
// An editor is not a sequence of commands. A player converts one Ash, authors
// another, restores a third, and presses Save once - so what reaches the save
// core is one plan carrying several operations, not several plans applied in
// turn. Applying them in turn would mean rebuilding a 26 MB regulation once per
// operation and producing one audit artifact per operation for what the player
// performed as a single edit.
//
// The composition relaxes no rule. G1 refused to build a validator permissive
// enough to describe two operations at once, because such a validator stops
// rejecting either one's mistakes, and that argument is untouched here: each
// part stays a strict single-operation manifest, and the save core validates
// every one of them exactly as it would alone before checking what is true of
// the combination. This file only assembles; savecore decides.

import "fmt"

// OperationConvert is the operation value a conversion carries inside a
// session. On its own a conversion still leaves the field empty, so a
// single-conversion commit produces the bytes it always has; see Compose.
const OperationConvert = "convert"

// OperationSession is the operation value a composed manifest carries. Its own
// patch and structural fields are always empty: a session manifest describes
// nothing itself and is entirely the sum of its parts.
const OperationSession = "session"

// ErrNothingStaged is returned for a commit with no operations in it. It is a
// distinct failure from a manifest that is invalid: there is nothing wrong with
// the request except that it asks for no work.
var ErrNothingStaged = fmt.Errorf("a commit needs at least one staged change")

// Compose assembles the parts of one commit into a single manifest.
//
// A commit carrying exactly one part returns that part unchanged rather than
// wrapping it. That is not an optimisation. The single-operation manifests are
// the audit artifacts this product has always produced and the ones its
// documentation shows, and a session wrapper around a lone conversion would
// change those bytes to say nothing new. A session manifest therefore appears
// exactly when there is genuinely more than one operation in the save.
func Compose(sourceSHA256 string, parts []*Manifest) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(parts) == 0 {
		return nil, ErrNothingStaged
	}
	for index, part := range parts {
		if part == nil {
			return nil, fmt.Errorf("staged change %d is missing", index+1)
		}
		if len(part.Parts) != 0 {
			return nil, fmt.Errorf("staged change %d is itself a session; a commit is one level deep", index+1)
		}
		switch part.Operation {
		case "", OperationConvert, OperationReset, OperationAuthor, OperationCost, OperationStrength, OperationArea, OperationTint, OperationPrune, OperationSharedEffects, OperationCharacters, OperationOwnEffects:
		default:
			return nil, fmt.Errorf("staged change %d has unknown operation %q", index+1, part.Operation)
		}
		part.SourceSHA256 = sourceSHA256
	}
	if len(parts) == 1 {
		return parts[0], nil
	}

	composed := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationSession}
	for _, part := range parts {
		// Only inside a session, and only once the part is known to be one of
		// the three. A composed manifest has to say which operation each part
		// is, because there is no longer one operation for the whole file to
		// imply it.
		if part.Operation == "" {
			part.Operation = OperationConvert
		}
		composed.Parts = append(composed.Parts, part)
	}
	return composed, nil
}
