package tui

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/selection"
)

// prop2RandomFolders builds a random slice of ScannedFolder values with varying
// SizeKB, Hash, Domain, and AbsPath. The length spans 0 to well above the
// listingLimit (15) so the top-15 truncation path is exercised, and the SizeKB
// range is deliberately small so equal sizes (ties) are common, stressing the
// stable ordering that TopN guarantees.
func prop2RandomFolders(rng *rand.Rand) []model.ScannedFolder {
	n := rng.Intn(30) // 0..29 folders: crosses the 15-entry limit boundary
	folders := make([]model.ScannedFolder, n)
	for i := range folders {
		id := prop2Itoa(i)
		folders[i] = model.ScannedFolder{
			Hash:    "hash" + id,
			AbsPath: "/base/hash" + id,
			SizeKB:  int64(rng.Intn(40)), // small range -> frequent ties
			Domain:  "example" + id + ".com",
		}
	}
	return folders
}

func prop2Itoa(i int) string {
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

// TestNewModel_Property2_FolderListShape validates Property 2.
//
// Feature: interactive-tui-mode, Property 2: Folder list is always the
// descending top-15 of the scan.
//
// For any set of scanned folders with status StatusOK, the Model built by
// newModel has a folders slice equal to selection.TopN(folders, 15) — ordered
// by descending size and limited to fifteen entries. Because newModel delegates
// list shaping to selection.TopN (the same function used by the post-deletion
// rebuild), this is the single source of truth for list shape at both initial
// build and after a rebuild.
//
// **Validates: Requirements 2.2, 2.3, 2.4, 6.1, 6.2**
func TestNewModel_Property2_FolderListShape(t *testing.T) {
	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		folders := prop2RandomFolders(rng)

		got := newModel(folders, model.StatusOK).folders
		want := selection.TopN(folders, listingLimit)

		if !reflect.DeepEqual(got, want) {
			t.Errorf("newModel folders mismatch (seed=%d, inputLen=%d):\n got=%+v\nwant=%+v",
				seed, len(folders), got, want)
			return false
		}

		// The list is never longer than the Listing_Limit (Req 2.3).
		if len(got) > listingLimit {
			t.Errorf("folder list length %d exceeds listingLimit %d (seed=%d)",
				len(got), listingLimit, seed)
			return false
		}

		// The list is ordered by non-increasing SizeKB (Req 2.2, 2.4).
		for i := 1; i < len(got); i++ {
			if got[i-1].SizeKB < got[i].SizeKB {
				t.Errorf("folder list not ordered by descending SizeKB at index %d (seed=%d)",
					i, seed)
				return false
			}
		}

		return true
	}

	// Minimum 100 iterations.
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}
