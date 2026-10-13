// Package selection_test holds the property tests for the selection package of
// the "safari-data-tool" feature. The property tests use the standard-library
// testing/quick harness (minimum 100 iterations each) so the module stays
// dependency-free, consistent with the design's pure-Go goal.
package selection_test

import (
	"errors"
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/selection"
)

// randomFolders builds a random slice of ScannedFolder values whose SizeKB
// fields span a small range so collisions (equal sizes) are common, exercising
// the stable-sort / ordering guarantees.
func randomFolders(rng *rand.Rand) []model.ScannedFolder {
	n := rng.Intn(40) // 0..39 folders
	folders := make([]model.ScannedFolder, n)
	for i := range folders {
		folders[i] = model.ScannedFolder{
			Hash:    "hash" + itoa(i),
			AbsPath: "/base/hash" + itoa(i),
			SizeKB:  int64(rng.Intn(50)), // deliberately small range -> ties
			Domain:  "example" + itoa(i) + ".com",
		}
	}
	return folders
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isNonIncreasing reports whether the folders are ordered by non-increasing
// SizeKB.
func isNonIncreasing(folders []model.ScannedFolder) bool {
	for i := 1; i < len(folders); i++ {
		if folders[i-1].SizeKB < folders[i].SizeKB {
			return false
		}
	}
	return true
}

// sizeMultiset counts occurrences of each SizeKB value, giving a size-based
// multiset comparison independent of folder identity/order.
func sizeMultiset(folders []model.ScannedFolder) map[int64]int {
	m := make(map[int64]int, len(folders))
	for _, f := range folders {
		m[f.SizeKB]++
	}
	return m
}

func equalSizeMultiset(a, b map[int64]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Property 1: Top-N selection ("safari-data-tool").
//
// For any set of scanned folders and any positive count, the listing returns
// exactly min(count, n) folders ordered by non-increasing size, and returns all
// folders when count is at least the number of folders (count defaults to 15
// when unspecified).
//
// **Validates: Requirements 1.1, 1.2, 1.4**
func TestTopN_Property1_Selection(t *testing.T) {
	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		folders := randomFolders(rng)
		// positive count in 1..60 so we hit both count < n and count >= n.
		count := rng.Intn(60) + 1

		got := selection.TopN(folders, count)

		// 1. Length is exactly min(count, len(folders)).
		wantLen := min(count, len(folders))
		if len(got) != wantLen {
			t.Errorf("len(TopN)=%d, want min(%d,%d)=%d (seed=%d)",
				len(got), count, len(folders), wantLen, seed)
			return false
		}

		// 2. Result is ordered by non-increasing SizeKB.
		if !isNonIncreasing(got) {
			t.Errorf("TopN result not ordered by non-increasing SizeKB (seed=%d)", seed)
			return false
		}

		// 3. When count >= len(folders), all folders are returned (same
		//    size-multiset as the full input).
		if count >= len(folders) {
			if !equalSizeMultiset(sizeMultiset(got), sizeMultiset(folders)) {
				t.Errorf("count>=n but result is not a permutation of input by size (seed=%d)", seed)
				return false
			}
		}

		// 4. The selected head must be a size-multiset subset of the full
		//    sorted input: TopN takes the largest entries, so comparing the
		//    result's sizes against the top wantLen sizes of a full sort must
		//    match.
		full := selection.TopN(folders, len(folders)+1) // full, sorted desc
		if !equalSizeMultiset(sizeMultiset(got), sizeMultiset(full[:wantLen])) {
			t.Errorf("TopN head sizes differ from full-sort head sizes (seed=%d)", seed)
			return false
		}

		return true
	}

	// Minimum 100 iterations.
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}

// Property 1 (default count): the default count of 15 behaves per the min()
// rule — fewer than 15 folders returns all of them, more than 15 returns 15.
//
// **Validates: Requirements 1.1, 1.2, 1.4**
func TestTopN_Property1_DefaultCount15(t *testing.T) {
	const defaultCount = 15

	// Fewer than 15 folders: all are returned.
	few := make([]model.ScannedFolder, 10)
	for i := range few {
		few[i] = model.ScannedFolder{SizeKB: int64(i)}
	}
	if got := selection.TopN(few, defaultCount); len(got) != len(few) {
		t.Errorf("default count with %d folders: len=%d, want %d", len(few), len(got), len(few))
	}

	// More than 15 folders: exactly 15 are returned, largest first.
	many := make([]model.ScannedFolder, 30)
	for i := range many {
		many[i] = model.ScannedFolder{SizeKB: int64(i)}
	}
	got := selection.TopN(many, defaultCount)
	if len(got) != defaultCount {
		t.Fatalf("default count with %d folders: len=%d, want %d", len(many), len(got), defaultCount)
	}
	if !isNonIncreasing(got) {
		t.Errorf("default-count result not ordered by non-increasing SizeKB")
	}
	// The largest size present (29) must be first.
	if got[0].SizeKB != 29 {
		t.Errorf("default-count head SizeKB=%d, want 29", got[0].SizeKB)
	}
}

// Property 2: Count validation rejects non-positive values ("safari-data-tool").
//
// For any integer count that is not a positive integer, count validation
// returns an input error; for any positive count it returns nil.
//
// **Validates: Requirements 1.5**
func TestValidateCount_Property2_RejectsNonPositive(t *testing.T) {
	// Non-positive counts (<= 0) must return ErrInvalidCount.
	nonPositive := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		count := -rng.Intn(1 << 31) // 0 .. -(2^31-1)

		err := selection.ValidateCount(count)
		if !errors.Is(err, selection.ErrInvalidCount) {
			t.Errorf("ValidateCount(%d)=%v, want ErrInvalidCount (seed=%d)", count, err, seed)
			return false
		}
		return true
	}
	if err := quick.Check(nonPositive, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}

	// Positive counts (>= 1) must return nil.
	positive := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		count := rng.Intn(1<<31) + 1 // 1 .. 2^31

		if err := selection.ValidateCount(count); err != nil {
			t.Errorf("ValidateCount(%d)=%v, want nil (seed=%d)", count, err, seed)
			return false
		}
		return true
	}
	if err := quick.Check(positive, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}
