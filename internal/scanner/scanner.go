// Package scanner enumerates the hashed per-domain folders under Safari's
// WebsiteData directory, sizing and resolving each one, and classifies the
// overall access status.
package scanner

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/originparse"
	"github.com/sesamsys/safari-data-tool/internal/sizer"
)

// containerSuffix is the fixed path, relative to the user's home directory, of
// Safari's per-origin WebsiteData store inside its sandbox container (Req 7.1).
const containerSuffix = "Library/Containers/com.apple.Safari/Data/Library/WebKit/WebsiteData/Default"

// BasePath derives the absolute WebsiteData base path for the given home
// directory by joining it with the fixed macOS container suffix.
func BasePath(home string) string {
	return filepath.Join(home, containerSuffix)
}

// Scan enumerates the direct child directories of base, sizing and resolving
// the domain of each, and returns the collected folders alongside an
// AccessStatus describing what was observed.
//
// Behaviour:
//   - If base does not exist, no folders are returned and the status is
//     StatusNotFound (Req 6.3).
//   - If base exists but its directory cannot be read, it is treated as having
//     zero readable children, yielding StatusAccessBlocked (Req 6.1).
//   - Each child directory is sized (Req 3.1) and domain-resolved (Req 2.1);
//     any folder whose size cannot be computed is skipped so one unreadable
//     folder never aborts the whole scan.
func Scan(f fsx.FileSystem, base string) ([]model.ScannedFolder, model.AccessStatus, error) {
	if _, err := f.Stat(base); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, Classify(false, 0), nil
		}
		// The base exists in some form but cannot be stat-ed (e.g. permission
		// denied): treat it as present with no readable children.
		return nil, Classify(true, 0), nil
	}

	entries, err := f.ReadDir(base)
	if err != nil {
		// Base exists but its contents are unreadable: zero readable children.
		return nil, Classify(true, 0), nil
	}

	var folders []model.ScannedFolder
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		abs := filepath.Join(base, name)

		size, sizeErr := sizer.SizeKB(f, abs)
		if sizeErr != nil {
			// Skip folders we cannot size (unreadable) and keep going.
			continue
		}

		domain, hosts := originparse.Resolve(f, abs)
		folders = append(folders, model.ScannedFolder{
			Hash:    name,
			AbsPath: abs,
			SizeKB:  size,
			Domain:  domain,
			Hosts:   hosts,
		})
	}

	return folders, Classify(true, len(folders)), nil
}
