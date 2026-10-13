// Package sizer computes recursive folder sizes with du -sk semantics.
package sizer

import (
	"io/fs"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
)

// SizeKB returns the recursive size of root in 1024-byte blocks, equivalent to
// `du -sk root`: it walks the whole tree and sums the sizes of regular files,
// rounding each file's byte size up to the next 1024-byte block before adding
// it to the total (Req 3.1).
//
// Rounding per file (ceiling to a 1024-byte block) rather than rounding the
// aggregate byte total is what makes the result additive: the size of a tree
// equals the sum of the sizes of its subtrees, because each file contributes
// the same block count regardless of how the tree is partitioned.
//
// Symlinks are not followed: filepath.WalkDir (used by the FileSystem
// implementations) does not descend into symlinked directories, and only
// entries whose type is a regular file are counted, so symlinks themselves are
// skipped. This mirrors `du`'s default behaviour and avoids cycles.
func SizeKB(f fsx.FileSystem, root string) (int64, error) {
	var totalKB int64
	err := f.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Count regular files only; skip directories, symlinks, and other
		// non-regular entries.
		if !d.Type().IsRegular() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		// Round this file's byte size up to the next whole 1024-byte block.
		totalKB += (info.Size() + 1023) / 1024
		return nil
	})
	if err != nil {
		return 0, err
	}
	return totalKB, nil
}
