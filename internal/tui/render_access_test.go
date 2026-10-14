package tui

import (
	"strings"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/ui"
)

// TestRenderAccessShowsGuidanceAndNoFolderRows asserts that a modeAccess frame
// renders the ui.AccessMessage guidance for its status and contains no folder
// rows (Req 8.1, 8.2).
func TestRenderAccessShowsGuidanceAndNoFolderRows(t *testing.T) {
	// A folder whose FormatLine text must never appear in the access frame,
	// since modeAccess renders guidance only and no folder rows.
	folder := model.ScannedFolder{
		Hash:    "abc123",
		AbsPath: "/base/abc123",
		SizeKB:  2048,
		Domain:  "example.com",
	}
	folderLine := ui.FormatLine(folder)

	cases := []struct {
		name   string
		status model.AccessStatus
	}{
		{"access blocked", model.StatusAccessBlocked},
		{"not found", model.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{mode: modeAccess, status: tc.status}
			frame := string(render(m))

			msg := ui.AccessMessage(tc.status)
			if !strings.Contains(frame, msg) {
				t.Errorf("access frame missing guidance for %v\nwant substring: %q\nframe: %q",
					tc.status, msg, frame)
			}

			// No folder rows: a constructed folder's FormatLine text must not
			// appear, and no inverted-selection escape should be emitted.
			if strings.Contains(frame, folderLine) {
				t.Errorf("access frame unexpectedly contains a folder row: %q", folderLine)
			}
			if strings.Contains(frame, escInvert) {
				t.Errorf("access frame unexpectedly contains the inverted-row escape %q", escInvert)
			}
		})
	}
}
