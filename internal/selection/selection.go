// Package selection provides helpers for choosing which scanned folders to
// present to the user, including top-N selection by size and count validation.
package selection

import (
	"errors"
	"sort"

	"github.com/sesamsys/safari-data-tool/internal/model"
)

// ErrInvalidCount is returned by ValidateCount when the requested count is not
// a positive integer.
var ErrInvalidCount = errors.New("count must be a positive integer")

// TopN returns the largest-by-size folders, sorted by descending SizeKB using a
// stable sort, limited to the first min(count, len(folders)) entries. It
// operates on a copy, so the caller's slice ordering is left untouched. For
// count <= 0 it returns an empty slice; callers validate count separately with
// ValidateCount.
func TopN(folders []model.ScannedFolder, count int) []model.ScannedFolder {
	if count <= 0 {
		return []model.ScannedFolder{}
	}

	sorted := make([]model.ScannedFolder, len(folders))
	copy(sorted, folders)

	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SizeKB > sorted[j].SizeKB
	})

	if count > len(sorted) {
		count = len(sorted)
	}
	return sorted[:count]
}

// ValidateCount rejects non-positive counts, returning ErrInvalidCount.
func ValidateCount(count int) error {
	if count <= 0 {
		return ErrInvalidCount
	}
	return nil
}
