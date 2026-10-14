package tui

import (
	"math/rand"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// TestProperty5WrapAroundNavigation drives moveDown and moveUp with random
// non-empty folder lists and random starting indices, asserting that
// navigation is modular over the list length: moveDown yields (i+1) mod n and
// moveUp yields (i+n-1) mod n. This makes wrap-around at both ends a direct
// consequence of the modular arithmetic (moving down from the last item lands
// on the first; moving up from the first lands on the last).
//
// Feature: interactive-tui-mode, Property 5: Navigation wraps around and is modular
//
// Validates: Requirements 3.2, 3.3, 3.4, 3.5
func TestProperty5WrapAroundNavigation(t *testing.T) {
	const iterations = 1000 // well above the 100-iteration minimum
	rng := rand.New(rand.NewSource(1))

	for iter := 0; iter < iterations; iter++ {
		n := rng.Intn(30) + 1 // non-empty list, length 1..30
		i := rng.Intn(n)      // starting index in [0, n)

		m := Model{folders: makeFolders(n), index: i}

		if got, want := m.moveDown().index, (i+1)%n; got != want {
			t.Fatalf("iter %d: n=%d i=%d: moveDown().index = %d, want (i+1)%%n = %d",
				iter, n, i, got, want)
		}
		if got, want := m.moveUp().index, (i+n-1)%n; got != want {
			t.Fatalf("iter %d: n=%d i=%d: moveUp().index = %d, want (i+n-1)%%n = %d",
				iter, n, i, got, want)
		}
	}
}

// makeFolders returns a slice of n placeholder ScannedFolders. Navigation
// depends only on the list length, so the folder contents are irrelevant here.
func makeFolders(n int) []model.ScannedFolder {
	folders := make([]model.ScannedFolder, n)
	return folders
}
