package tui

import (
	"math/rand"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// Feature: interactive-tui-mode, Property 6: Selection index stays in range
//
// For any sequence of navigation keypresses applied to a non-empty
// Folder_List, the Selection_Index always satisfies 0 <= index < len(folders).
//
// This builds a Model in modeList with a random non-empty folders slice, then
// applies a random sequence of KeyUp/KeyDown keypresses via handleKey. After
// every keypress it asserts the invariant holds.
//
// Validates: Requirements 3.1, 3.6, 6.3
func TestProperty6_SelectionIndexStaysInRange(t *testing.T) {
	const iterations = 500 // >= 100 iterations
	rng := rand.New(rand.NewSource(0xBADC0FFEE))

	navKeys := []Key{KeyUp, KeyDown}

	for iter := 0; iter < iterations; iter++ {
		// Random non-empty folders slice (length 1..30).
		n := rng.Intn(30) + 1
		folders := make([]model.ScannedFolder, n)
		for i := range folders {
			folders[i] = model.ScannedFolder{Hash: "h", AbsPath: "/p", SizeKB: int64(i)}
		}

		m := Model{
			folders: folders,
			index:   rng.Intn(n), // random valid starting index
			mode:    modeList,
		}

		// Baseline invariant before any navigation.
		if m.index < 0 || m.index >= len(m.folders) {
			t.Fatalf("iteration %d: initial index %d out of range [0,%d)", iter, m.index, len(m.folders))
		}

		// Apply a random sequence of navigation keypresses.
		steps := rng.Intn(100) + 1
		for s := 0; s < steps; s++ {
			k := navKeys[rng.Intn(len(navKeys))]
			m, _ = m.handleKey(k)

			if m.index < 0 || m.index >= len(m.folders) {
				t.Fatalf("iteration %d step %d: index %d out of range [0,%d) after key %v",
					iter, s, m.index, len(m.folders), k)
			}
		}
	}
}

// TestProperty6_RebuildKeepsIndexInRange covers the post-deletion rebuild
// clause of Property 6: for any prior index and any rebuilt non-empty folder
// list, rebuildAfterDelete leaves the Selection_Index in range.
//
// Validates: Requirements 3.1, 3.6, 6.3
func TestProperty6_RebuildKeepsIndexInRange(t *testing.T) {
	const iterations = 300 // >= 100 iterations
	rng := rand.New(rand.NewSource(0x5EED))

	for iter := 0; iter < iterations; iter++ {
		// A previously-loaded session at some arbitrary (possibly stale) index.
		prevLen := rng.Intn(30) + 1
		prev := Model{
			folders:          make([]model.ScannedFolder, prevLen),
			index:            rng.Intn(prevLen),
			mode:             modeList,
			sessionHasLoaded: true,
		}

		// The re-scan yields a new non-empty list, often shorter.
		newLen := rng.Intn(30) + 1
		folders := make([]model.ScannedFolder, newLen)
		for i := range folders {
			folders[i] = model.ScannedFolder{Hash: "h", AbsPath: "/p", SizeKB: int64(i)}
		}

		rebuilt := rebuildAfterDelete(prev, folders, model.StatusOK)

		if rebuilt.index < 0 || rebuilt.index >= len(rebuilt.folders) {
			t.Fatalf("iteration %d: rebuilt index %d out of range [0,%d) (prev index %d, prev len %d)",
				iter, rebuilt.index, len(rebuilt.folders), prev.index, prevLen)
		}
	}
}
