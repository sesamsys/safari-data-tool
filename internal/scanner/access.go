package scanner

import "github.com/sesamsys/safari-data-tool/internal/model"

// Classify maps the observed state of the base path to an AccessStatus.
//
//   - baseExists == false            -> StatusNotFound: the Safari website-data
//     directory does not exist (Req 6.3).
//   - baseExists && childCount == 0  -> StatusAccessBlocked: the base exists but
//     no readable child folders were enumerated, which on macOS typically means
//     Full Disk Access has not been granted (Req 6.1).
//   - otherwise                      -> StatusOK: the base exists with at least
//     one readable folder.
func Classify(baseExists bool, childCount int) model.AccessStatus {
	switch {
	case !baseExists:
		return model.StatusNotFound
	case childCount == 0:
		return model.StatusAccessBlocked
	default:
		return model.StatusOK
	}
}
