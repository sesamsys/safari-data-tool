// Package deleter_test holds the property and unit tests for the deleter
// package of the "safari-data-tool" feature. The property tests use the
// standard-library testing/quick harness (minimum 100 iterations each) so the
// module stays dependency-free, consistent with the design's pure-Go goal.
package deleter_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/deleter"
	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
)

// quickConfig runs each property for at least 100 iterations, satisfying the
// design's minimum-iteration requirement for property-based tests.
var quickConfig = &quick.Config{MaxCount: 200}

// ----------------------------------------------------------------------------
// Random-value helpers
// ----------------------------------------------------------------------------

// randString builds a deterministic-but-varied token from a pseudo-random
// seed. It never returns the empty string so generated domains/hashes are
// always "set" (Match treats "" as not-provided).
func randString(seed uint32, prefix string) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	n := int(seed%6) + 3 // length 3..8
	var b strings.Builder
	b.WriteString(prefix)
	s := seed
	for i := 0; i < n; i++ {
		s = s*1664525 + 1013904223 // LCG step
		b.WriteByte(alphabet[int(s>>8)%len(alphabet)])
	}
	return b.String()
}

// randFolders produces a non-empty, de-duplicated slice of ScannedFolder from a
// seed. Domains and hashes are unique across the slice so that a value picked
// from one folder cannot accidentally be satisfied by another.
func randFolders(seed uint32) []model.ScannedFolder {
	count := int(seed%5) + 1 // 1..5 folders
	folders := make([]model.ScannedFolder, 0, count)
	for i := 0; i < count; i++ {
		s := seed*uint32(i+1) + uint32(i)*2654435761
		domain := randString(s, "d") + "." + randString(s>>3, "tld")
		hash := randString(s>>1, "h")
		folders = append(folders, model.ScannedFolder{
			Hash:    hash,
			AbsPath: "/base/" + hash,
			SizeKB:  int64(s % 10000),
			Domain:  domain,
			Hosts:   []string{domain},
		})
	}
	return folders
}

// containsDomain / containsHash report whether any folder satisfies a field
// using the same semantics as Match (case-insensitive domain, exact hash).
func containsDomain(folders []model.ScannedFolder, domain string) bool {
	for _, f := range folders {
		if strings.EqualFold(f.Domain, domain) {
			return true
		}
	}
	return false
}

func containsHash(folders []model.ScannedFolder, hash string) bool {
	for _, f := range folders {
		if f.Hash == hash {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Task 5.3 — Property 9: Deletion target matching
// safari-data-tool / Property 9
// Validates: Requirements 4.1, 4.2, 4.3
// ----------------------------------------------------------------------------

// TestProperty9_Match_PresentFieldFound asserts that when a target names an
// existing folder's domain and/or hash (with case variation on the domain to
// exercise case-insensitivity), Match returns a folder satisfying every
// provided field. safari-data-tool / Property 9.
func TestProperty9_Match_PresentFieldFound(t *testing.T) {
	property := func(seed uint32) bool {
		folders := randFolders(seed)
		pick := folders[int(seed)%len(folders)]

		// Vary case on the chosen domain to exercise case-insensitive match.
		varied := pick.Domain
		if seed%2 == 0 {
			varied = strings.ToUpper(varied)
		} else {
			varied = strings.ToLower(varied)
		}

		cases := []deleter.Target{
			{Domain: varied},             // domain only (4.1)
			{Hash: pick.Hash},            // hash only (4.2)
			{Domain: varied, Hash: pick.Hash}, // both (4.3 — AND)
		}

		for _, tgt := range cases {
			got, err := deleter.Match(folders, tgt)
			if err != nil {
				return false
			}
			if tgt.Domain != "" && !strings.EqualFold(got.Domain, tgt.Domain) {
				return false
			}
			if tgt.Hash != "" && got.Hash != tgt.Hash {
				return false
			}
		}
		return true
	}

	if err := quick.Check(property, quickConfig); err != nil {
		t.Fatalf("safari-data-tool Property 9 (present field found) failed: %v", err)
	}
}

// TestProperty9_Match_AbsentFieldNoMatch asserts that when a target names a
// domain and/or hash that no folder satisfies, Match returns ErrNoMatch.
// safari-data-tool / Property 9.
func TestProperty9_Match_AbsentFieldNoMatch(t *testing.T) {
	property := func(seed uint32) bool {
		folders := randFolders(seed)

		// Construct values guaranteed absent from the slice.
		absentDomain := randString(seed, "zzz-absent-") + ".example"
		for containsDomain(folders, absentDomain) {
			absentDomain += "x"
		}
		absentHash := randString(seed>>2, "zzz-absent-")
		for containsHash(folders, absentHash) {
			absentHash += "x"
		}

		// Also build a mismatched AND target: an existing domain paired with an
		// absent hash must not match (both fields required when both are set).
		existingDomain := folders[int(seed)%len(folders)].Domain

		cases := []deleter.Target{
			{Domain: absentDomain},
			{Hash: absentHash},
			{Domain: absentDomain, Hash: absentHash},
			{Domain: existingDomain, Hash: absentHash},
		}

		for _, tgt := range cases {
			_, err := deleter.Match(folders, tgt)
			if !errors.Is(err, deleter.ErrNoMatch) {
				return false
			}
		}
		return true
	}

	if err := quick.Check(property, quickConfig); err != nil {
		t.Fatalf("safari-data-tool Property 9 (absent field no match) failed: %v", err)
	}
}

// TestProperty9_Match_EmptyTargetNoMatch is an edge-case check: an entirely
// empty target (neither field set) matches nothing. safari-data-tool /
// Property 9.
func TestProperty9_Match_EmptyTargetNoMatch(t *testing.T) {
	folders := randFolders(42)
	if _, err := deleter.Match(folders, deleter.Target{}); !errors.Is(err, deleter.ErrNoMatch) {
		t.Fatalf("expected ErrNoMatch for empty target, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// Task 5.4 — Property 10: Path-safety requires a direct child of base
// safari-data-tool / Property 10
// Validates: Requirements 5.1, 5.2, 5.3
// ----------------------------------------------------------------------------

// segment produces a single non-empty, separator-free path segment from a seed.
func segment(seed uint32) string {
	return randString(seed, "s")
}

// TestProperty10_PathSafety_DirectChildAccepted asserts that a direct child of
// base (base/<segment>) is accepted (nil error). safari-data-tool / Property 10.
func TestProperty10_PathSafety_DirectChildAccepted(t *testing.T) {
	property := func(baseSeed, childSeed uint32) bool {
		base := filepath.Join(os.TempDir(), "sdt-base-"+segment(baseSeed))
		child := filepath.Join(base, segment(childSeed))
		return deleter.ValidatePath(base, child) == nil
	}
	if err := quick.Check(property, quickConfig); err != nil {
		t.Fatalf("safari-data-tool Property 10 (direct child accepted) failed: %v", err)
	}
}

// TestProperty10_PathSafety_BaseItselfRejected asserts that the base itself is
// rejected with ErrPathUnsafe. safari-data-tool / Property 10.
func TestProperty10_PathSafety_BaseItselfRejected(t *testing.T) {
	property := func(baseSeed uint32) bool {
		base := filepath.Join(os.TempDir(), "sdt-base-"+segment(baseSeed))
		// Pass the same path (optionally un-cleaned) as the target.
		target := base
		if baseSeed%2 == 0 {
			target = base + string(filepath.Separator) + "." // base/. -> base after clean
		}
		return errors.Is(deleter.ValidatePath(base, target), deleter.ErrPathUnsafe)
	}
	if err := quick.Check(property, quickConfig); err != nil {
		t.Fatalf("safari-data-tool Property 10 (base itself rejected) failed: %v", err)
	}
}

// TestProperty10_PathSafety_NestedDescendantRejected asserts that a grandchild
// (base/child/grandchild) is rejected with ErrPathUnsafe. safari-data-tool /
// Property 10.
func TestProperty10_PathSafety_NestedDescendantRejected(t *testing.T) {
	property := func(baseSeed, childSeed, grandSeed uint32) bool {
		base := filepath.Join(os.TempDir(), "sdt-base-"+segment(baseSeed))
		nested := filepath.Join(base, segment(childSeed), segment(grandSeed))
		return errors.Is(deleter.ValidatePath(base, nested), deleter.ErrPathUnsafe)
	}
	if err := quick.Check(property, quickConfig); err != nil {
		t.Fatalf("safari-data-tool Property 10 (nested descendant rejected) failed: %v", err)
	}
}

// TestProperty10_PathSafety_TraversalEscapeRejected asserts that a target using
// `..` to escape the base is rejected with ErrPathUnsafe. safari-data-tool /
// Property 10.
func TestProperty10_PathSafety_TraversalEscapeRejected(t *testing.T) {
	property := func(baseSeed, childSeed uint32) bool {
		base := filepath.Join(os.TempDir(), "sdt-base-"+segment(baseSeed))
		// base/<child>/../../<child> cleans to a sibling/parent outside base.
		escape := filepath.Join(base, segment(childSeed), "..", "..", segment(childSeed))
		if err := deleter.ValidatePath(base, escape); !errors.Is(err, deleter.ErrPathUnsafe) {
			return false
		}
		// A direct `..` sibling of base also escapes.
		sibling := filepath.Join(base, "..", segment(childSeed))
		return errors.Is(deleter.ValidatePath(base, sibling), deleter.ErrPathUnsafe)
	}
	if err := quick.Check(property, quickConfig); err != nil {
		t.Fatalf("safari-data-tool Property 10 (traversal escape rejected) failed: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Task 5.5 — Unit test for Delete against a fake base
// Requirements: 4.6, 5.2
// ----------------------------------------------------------------------------

// TestDelete_RemovesDirectChild creates a fake base with a child folder holding
// a file and asserts Delete removes the child (Stat reports not-exist).
// Requirements 4.6, 5.2.
func TestDelete_RemovesDirectChild(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())
	base, err := fake.Mkdir("base")
	if err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	child, err := fake.Mkdir("base/child")
	if err != nil {
		t.Fatalf("mkdir child: %v", err)
	}
	if _, err := fake.WriteFile("base/child/data.txt", []byte("payload")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Sanity: the child exists before deletion.
	if _, err := fake.Stat(child); err != nil {
		t.Fatalf("child should exist before delete: %v", err)
	}

	if err := deleter.Delete(fake, base, child); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	if _, err := fake.Stat(child); !os.IsNotExist(err) {
		t.Fatalf("expected child to be gone (not-exist), got err=%v", err)
	}
}

// TestDelete_RefusesOutsideBase asserts that an outside-base target is refused
// with ErrPathUnsafe before any removal, leaving the target intact.
// Requirements 4.6, 5.2.
func TestDelete_RefusesOutsideBase(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())
	base, err := fake.Mkdir("base")
	if err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	// A sibling of base: lives under the fake root but is not a child of base.
	sibling, err := fake.Mkdir("sibling")
	if err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	if _, err := fake.WriteFile("sibling/keep.txt", []byte("keep")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := deleter.Delete(fake, base, sibling); !errors.Is(err, deleter.ErrPathUnsafe) {
		t.Fatalf("expected ErrPathUnsafe for sibling target, got %v", err)
	}
	if _, err := fake.Stat(sibling); err != nil {
		t.Fatalf("sibling should still exist after refused delete: %v", err)
	}

	// The base itself must likewise be refused and remain intact.
	if err := deleter.Delete(fake, base, base); !errors.Is(err, deleter.ErrPathUnsafe) {
		t.Fatalf("expected ErrPathUnsafe for base-as-target, got %v", err)
	}
	if _, err := fake.Stat(base); err != nil {
		t.Fatalf("base should still exist after refused delete: %v", err)
	}
}
