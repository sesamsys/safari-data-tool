// Package deleter handles deletion-target matching, path-safety validation,
// and the actual recursive removal of a hashed folder.
//
// The core logic stays pure: Match and ValidatePath operate on plain values and
// return typed sentinel errors (ErrNoMatch, ErrPathUnsafe) rather than printing,
// so callers in the cli layer map them to messages and exit codes. All real disk
// access goes through the fsx.FileSystem interface so Delete is testable against
// fakes.
package deleter

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
)

// originFileName is the exact base name of the WebKit metadata sidecar files
// that selective deletion preserves. Nothing else is treated as metadata.
const originFileName = "origin"

// ErrNoMatch is returned by Match when no scanned folder satisfies the target.
var ErrNoMatch = errors.New("no matching folder found")

// ErrPathUnsafe is returned by ValidatePath and Delete when the resolved target
// path is not a direct child of the base path (the base itself, a nested
// descendant, or a traversal that escapes the base once cleaned).
var ErrPathUnsafe = errors.New("deletion target is not a direct child of the base path")

// Target identifies a deletion request by domain and/or hash.
//
// Either field may be empty when the operator did not provide it. Callers must
// validate that at least one field is set before calling Match; an entirely
// empty Target matches nothing and yields ErrNoMatch.
type Target struct {
	Domain string // from --domain; empty if not provided
	Hash   string // from --hash;   empty if not provided
}

// Match selects the folder whose resolved Domain equals Target.Domain and/or
// whose Hash equals Target.Hash. If both fields are given, a folder must satisfy
// both; if only one is given, that field alone must match. Domain comparison is
// case-insensitive on the resolved host; Hash comparison is exact. An empty
// Target (neither field set) matches nothing. Returns ErrNoMatch when nothing
// matches (Req 4.1, 4.2, 4.3).
func Match(folders []model.ScannedFolder, t Target) (model.ScannedFolder, error) {
	hasDomain := t.Domain != ""
	hasHash := t.Hash != ""
	if !hasDomain && !hasHash {
		return model.ScannedFolder{}, ErrNoMatch
	}

	for _, f := range folders {
		if hasDomain && !strings.EqualFold(f.Domain, t.Domain) {
			continue
		}
		if hasHash && f.Hash != t.Hash {
			continue
		}
		return f, nil
	}
	return model.ScannedFolder{}, ErrNoMatch
}

// ValidatePath verifies that target resolves to an absolute path that is a
// direct child of base (Req 5.1, 5.2, 5.3). It absolutizes and cleans both
// paths, then rejects the target (returning ErrPathUnsafe) when it equals the
// base itself or when its parent directory is not exactly the base. Requiring
// filepath.Dir(target) == base rejects both nested descendants and any `..`
// escape once the path is cleaned. Returns nil when the target is a safe direct
// child.
func ValidatePath(base, target string) error {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return ErrPathUnsafe
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return ErrPathUnsafe
	}

	b := filepath.Clean(absBase)
	t := filepath.Clean(absTarget)

	if t == b {
		return ErrPathUnsafe
	}
	if filepath.Dir(t) != b {
		return ErrPathUnsafe
	}
	return nil
}

// Delete removes the selected folder recursively after the caller has matched,
// validated, and confirmed it (Req 4.6). It re-runs ValidatePath(base, target)
// as a defensive final gate, returning its error if the target is unsafe, and
// otherwise delegates removal to f.RemoveAll(target). It remains available as
// the full-wipe primitive reused by DeletePreservingOrigin's no-sidecar
// fallback.
func Delete(f fsx.FileSystem, base, target string) error {
	if err := ValidatePath(base, target); err != nil {
		return err
	}
	return f.RemoveAll(target)
}

// DeletePreservingOrigin clears a hashed folder's data while preserving every
// file whose base name is exactly "origin" (the WebKit metadata sidecar) and
// the directory chain leading to each. The now-tiny target folder remains and
// still resolves to its domain on the next scan, instead of vanishing or
// becoming "Unknown Domain". This is the default deletion behavior.
//
// It applies the same safety guarantee as Delete: ValidatePath(base, target)
// runs first (unchanged guard), so the target must be a validated direct child
// of base before any walking or removal happens; everything removed is strictly
// inside that validated target. Returns ValidatePath's error (ErrPathUnsafe)
// when the target is unsafe, removing nothing.
//
// Algorithm (two passes, remove-only, through fsx.FileSystem):
//
//   - Pass 1 walks target collecting every regular file named "origin" and, for
//     each, marks it and every ancestor directory up to and including target as
//     "protected". If no "origin" file exists there is nothing to preserve, so
//     the whole folder is removed via Delete (the folder then disappears from
//     the next scan).
//   - Pass 2 walks target top-down and RemoveAll's any entry not in the
//     protected set, returning fs.SkipDir after removing a directory so the walk
//     never descends into an already-removed subtree. Protected directories and
//     target itself are kept.
//
// Symlink-escape safety is inherited from filepath.WalkDir (which the fsx
// implementations use and which does not follow symlinked directories): every
// removed path is produced by walking inside the validated target.
func DeletePreservingOrigin(f fsx.FileSystem, base, target string) error {
	if err := ValidatePath(base, target); err != nil {
		return err
	}

	cleanTarget := filepath.Clean(target)

	// Pass 1: collect the set of paths to protect (origin files and the
	// directory chain leading to each, up to and including the target).
	protected := map[string]struct{}{cleanTarget: {}}
	foundOrigin := false
	walkErr := f.WalkDir(cleanTarget, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != originFileName {
			return nil
		}
		// Mark the origin file and every ancestor up to the target.
		foundOrigin = true
		p := filepath.Clean(path)
		for {
			protected[p] = struct{}{}
			if p == cleanTarget {
				break
			}
			parent := filepath.Dir(p)
			if parent == p { // defensive: cannot climb above the target
				break
			}
			p = parent
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	// No sidecar to preserve: remove the whole folder (full-wipe fallback).
	if !foundOrigin {
		return f.RemoveAll(cleanTarget)
	}

	// Pass 2: remove every entry not in the protected set. SkipDir after
	// removing a directory so the walk does not descend into a removed subtree.
	return f.WalkDir(cleanTarget, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		p := filepath.Clean(path)
		if _, ok := protected[p]; ok {
			// Protected directory (or the target): keep walking into it so we
			// reach the non-protected siblings of the preserved origin files.
			return nil
		}
		if removeErr := f.RemoveAll(p); removeErr != nil {
			return removeErr
		}
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
}
