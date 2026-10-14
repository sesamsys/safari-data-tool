package tui

import (
	"fmt"
	"strings"

	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/ui"
)

const (
	escClear      = "\x1b[2J\x1b[H"
	escInvert     = "\x1b[7m" // reverse video for the selected row (Req 4.1)
	escReset      = "\x1b[0m"
	listLegend    = "↑/↓ move   d delete   q quit"           // Key_Legend for list / access / all-clear
	confirmLegend = "y confirm   n cancel"                   // Key_Legend for the confirmation prompt
	allClearMsg   = "No Safari website-data folders remain." // All_Clear_Message (Req 9.3)

	// crlf ends every rendered line. The event loop runs the terminal in raw
	// mode, which disables output post-processing (OPOST/ONLCR), so a bare
	// "\n" moves the cursor down a row without returning it to column 0 and
	// the rows stair-step across the screen. Emitting an explicit carriage
	// return + line feed keeps each row left-aligned regardless of terminal
	// mode and keeps render self-contained.
	crlf = "\r\n"
)

// render produces the full ANSI frame for the current model. It is pure: given
// the same Model it always yields the same bytes, so tests render into a buffer
// and assert on the output.
//
// The Key_Legend is context-sensitive (Req 2.6). In every mode except
// modeConfirm it is the last line of the frame. In modeConfirm the legend
// shows the confirm/cancel keys and is placed above the prompt so the
// "Proceed? [y/N]:" line stays last and the operator's input appears directly
// after it (Req 5.6, 5.7).
func render(m Model) []byte {
	var b strings.Builder
	b.WriteString(escClear)

	switch m.mode {
	case modeAccess:
		// Access guidance only; no folder rows (Req 8.1, 8.2). The list
		// legend is the final line.
		b.WriteString(ui.AccessMessage(m.status))
		b.WriteString(crlf)
		b.WriteString(listLegend) // Key_Legend (Req 2.6)
		b.WriteString(crlf)
	case modeAllClear:
		// Everything was cleared this session: show the all-clear message
		// instead of the ui.AccessMessage guidance, with no folder rows
		// (Req 9.3, 9.4). The list legend is the final line.
		b.WriteString(allClearMsg)
		b.WriteString(crlf)
		b.WriteString(listLegend) // Key_Legend (Req 9.4)
		b.WriteString(crlf)
	default: // modeList and modeConfirm both render the folder rows.
		for i, folder := range m.folders {
			line := ui.FormatLine(folder) // reuse existing row format (Req 2.5)
			if i == m.index {
				b.WriteString(escInvert + line + escReset) // Req 4.1
			} else {
				b.WriteString(line) // Req 4.2
			}
			b.WriteString(crlf)
		}
		if m.errMsg != "" {
			b.WriteString("Delete failed: " + m.errMsg + crlf) // Req 5.5
		}
		if m.mode == modeConfirm {
			// Confirmation legend first, then the prompt as the final line so
			// the cursor sits right after "Proceed? [y/N]:" (Req 5.6, 5.7).
			b.WriteString(confirmLegend)
			b.WriteString(crlf)
			b.WriteString(renderConfirm(m.folders[m.index])) // Req 5.2
		} else {
			// modeList: the list legend is the final line (Req 2.6).
			b.WriteString(listLegend)
			b.WriteString(crlf)
		}
	}

	return []byte(b.String())
}

// renderConfirm shows the selected folder's Domain, Hash, and Size, matching
// the detail set used by ui.Confirm (Req 5.2). It ends with the "Proceed?"
// prompt and no trailing newline so the cursor rests on the prompt line. Other
// lines end in CRLF so the block renders correctly under raw mode (see the
// crlf constant).
func renderConfirm(folder model.ScannedFolder) string {
	return fmt.Sprintf(
		"About to clear this site's data (permanent, no undo); a tiny origin metadata marker is kept so the folder stays identifiable:%[1]s  Domain: %[2]s%[1]s  Hash:   %[3]s%[1]s  Size:   %[4]s%[1]sProceed? [y/N]: ",
		crlf, folder.Domain, folder.Hash, ui.FormatSize(folder.SizeKB),
	)
}
