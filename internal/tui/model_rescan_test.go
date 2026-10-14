package tui

import (
	"path/filepath"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
)

// containerSuffix mirrors scanner's fixed macOS container path so the test can
// seed hashed child folders where scanner.Scan expects to find them.
const containerSuffix = "Library/Containers/com.apple.Safari/Data/Library/WebKit/WebsiteData/Default"

// TestApplyDelete_RescanAfterSuccessfulDeletion exercises the re-scan-and-rebuild
// path of applyDelete end-to-end against a Safari-shaped fake tree.
//
// It seeds several hashed child folders of differing recursive size under the
// container suffix, builds a Model from an initial scan, deletes the currently
// selected folder, and asserts the rebuilt list:
//   - no longer contains the deleted folder's AbsPath (Req 6.1),
//   - is the descending top-15 of the current scan (Req 6.2),
//   - carries a Selection_Index that is in range (Req 6.3).
//
// _Requirements: 6.1, 6.2, 6.3_
func TestApplyDelete_RescanAfterSuccessfulDeletion(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())

	// Seed hashed child folders with files of distinct sizes so each folder
	// has SizeKB > 0 and the descending ordering is deterministic.
	//
	//   aaa -> ~3 KB  (largest)
	//   bbb -> ~2 KB
	//   ccc -> ~1 KB  (smallest)
	seed := []struct {
		hash  string
		bytes int
	}{
		{"aaa", 3000},
		{"bbb", 2000},
		{"ccc", 1000},
	}
	for _, s := range seed {
		rel := filepath.Join(containerSuffix, s.hash, "data.bin")
		if _, err := fake.WriteFile(rel, make([]byte, s.bytes)); err != nil {
			t.Fatalf("seeding %s: %v", s.hash, err)
		}
	}

	base := scanner.BasePath(fake.Root())

	// Build the initial Model from a real scan of the seeded tree.
	folders, status, err := scanner.Scan(fake, base)
	if err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	m := newModel(folders, status)

	if len(m.folders) != len(seed) {
		t.Fatalf("expected %d folders after initial scan, got %d", len(seed), len(m.folders))
	}

	// Target the second-largest folder (bbb) to make the index non-trivial.
	m.index = 1
	target := m.folders[m.index]

	rebuilt := applyDelete(m, fake, base)

	// The deleted folder's AbsPath is gone from the rebuilt list (Req 6.1).
	for i, f := range rebuilt.folders {
		if f.AbsPath == target.AbsPath {
			t.Fatalf("deleted folder %q still present in rebuilt list at index %d", target.AbsPath, i)
		}
	}

	// The rebuilt list has one fewer folder than before.
	if got, want := len(rebuilt.folders), len(seed)-1; got != want {
		t.Fatalf("expected %d folders after deletion, got %d", want, got)
	}

	// The rebuilt list honours the Listing_Limit and descending-size ordering
	// (Req 6.2).
	if len(rebuilt.folders) > listingLimit {
		t.Fatalf("rebuilt list length %d exceeds listingLimit %d", len(rebuilt.folders), listingLimit)
	}
	for i := 1; i < len(rebuilt.folders); i++ {
		if rebuilt.folders[i-1].SizeKB < rebuilt.folders[i].SizeKB {
			t.Fatalf("rebuilt list not ordered by descending SizeKB at index %d: %d < %d",
				i, rebuilt.folders[i-1].SizeKB, rebuilt.folders[i].SizeKB)
		}
	}

	// The Selection_Index is in range for the rebuilt, non-empty list (Req 6.3).
	if rebuilt.index < 0 || rebuilt.index >= len(rebuilt.folders) {
		t.Fatalf("selection index %d out of range for rebuilt list of length %d",
			rebuilt.index, len(rebuilt.folders))
	}
}
