// Package tui implements the interactive, full-screen terminal UI for
// browsing and deleting the largest Safari website-data folders. All decision
// logic lives in a pure Model with pure transition functions; terminal I/O and
// rendering are isolated behind separate interfaces and pure helpers.
package tui

import (
	"github.com/sesamsys/safari-data-tool/internal/deleter"
	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
	"github.com/sesamsys/safari-data-tool/internal/selection"
)

// mode is the current screen the TUI is showing.
type mode int

const (
	modeList     mode = iota // browsing the folder list
	modeConfirm              // confirmation prompt for the selected folder
	modeAccess               // non-OK access status at startup; guidance only
	modeAllClear             // all folders cleared during this session (Req 9)
)

// Key is a decoded keypress. Arrow keys arrive as three-byte escape sequences
// and command keys as single bytes; both decode into one of these values.
type Key int

const (
	KeyNone       Key = iota // empty input
	KeyUp                    // ESC [ A
	KeyDown                  // ESC [ B
	KeyDelete                // 'd' or 'D'
	KeyConfirmYes            // 'y' or 'Y'
	KeyConfirmNo             // 'n' or 'N'
	KeyQuit                  // 'q' or 'Q'
	KeyOther                 // anything else; ignored by the model
)

// action is a side effect the event loop must perform after a transition.
type action int

const (
	actNone   action = iota
	actDelete        // delete folders[index], then re-scan and rebuild
	actQuit          // stop the loop
)

// Model is the pure, I/O-free state of the TUI. Every transition is a method
// that returns a new Model; none of them touch the terminal, filesystem, or
// clock directly.
type Model struct {
	folders []model.ScannedFolder // the Folder_List, already TopN-limited
	index   int                   // Selection_Index into folders
	mode    mode
	status  model.AccessStatus // from the most recent scan
	errMsg  string             // last deletion failure, shown then cleared
	quit    bool               // set by KeyQuit; stops the event loop
	// sessionHasLoaded is Session_Has_Loaded: true once a StatusOK scan in this
	// session produced a non-empty Folder_List. It lets the TUI distinguish an
	// empty post-deletion re-scan (All_Clear_State) from a genuine startup
	// access block, since scanner.Classify maps both to StatusAccessBlocked
	// (Req 2.7, 9.1).
	sessionHasLoaded bool
}

const listingLimit = 15 // Listing_Limit (Req 2.3)

// newModel builds a Model from a scan result. When the status is OK it takes
// the top-15 by size as the Folder_List and selects the top item, setting
// sessionHasLoaded when that list is non-empty (Req 2.2, 2.3, 2.4, 2.7, 3.1,
// 9.1); otherwise it enters modeAccess (Req 8.1, 8.2).
//
// newModel alone cannot decide the All_Clear_State: it sees only the current
// scan, not the session history. The caller threads sessionHasLoaded forward
// (see rebuildAfterDelete) and promotes an empty re-scan to modeAllClear.
func newModel(folders []model.ScannedFolder, status model.AccessStatus) Model {
	if status != model.StatusOK {
		return Model{mode: modeAccess, status: status}
	}
	top := selection.TopN(folders, listingLimit)
	return Model{
		folders:          top,
		index:            0,
		mode:             modeList,
		status:           status,
		sessionHasLoaded: len(top) > 0, // Req 2.7, 9.1
	}
}

// moveDown advances the selection, wrapping from the last folder to the first
// (Req 3.2, 3.4). It is a no-op on an empty list.
func (m Model) moveDown() Model {
	if n := len(m.folders); n > 0 {
		m.index = (m.index + 1) % n
	}
	return m
}

// moveUp retreats the selection, wrapping from the first folder to the last
// (Req 3.3, 3.5). It is a no-op on an empty list.
func (m Model) moveUp() Model {
	if n := len(m.folders); n > 0 {
		m.index = (m.index + n - 1) % n
	}
	return m
}

// handleKey applies a keypress to the model and returns the updated model plus
// any side-effect action for the loop to perform.
func (m Model) handleKey(k Key) (Model, action) {
	switch m.mode {
	case modeList:
		switch k {
		case KeyUp:
			return m.moveUp(), actNone
		case KeyDown:
			return m.moveDown(), actNone
		case KeyDelete: // 'd'/'D' opens the confirmation (Req 5.1)
			if len(m.folders) > 0 {
				m.mode = modeConfirm
			}
			return m, actNone
		case KeyQuit:
			m.quit = true
			return m, actQuit
		}
	case modeConfirm:
		switch k {
		case KeyConfirmYes: // 'y'/'Y' confirms (Req 5.3)
			return m, actDelete
		case KeyConfirmNo: // 'n'/'N' cancels, index unchanged (Req 5.4)
			m.mode = modeList
			return m, actNone
		}
	case modeAllClear:
		// The all-clear screen stays interactive: quit exits, every other key
		// is ignored (Req 9.5, 9.6).
		if k == KeyQuit {
			m.quit = true
			return m, actQuit
		}
	}
	return m, actNone
}

// applyDelete removes the selected folder and rebuilds the model from a fresh
// scan on success (Req 5.3, 6.1, 6.2, 6.3). On failure it records the error and
// returns to the list without changing the folder set (Req 5.5).
func applyDelete(m Model, f fsx.FileSystem, base string) Model {
	target := m.folders[m.index]
	if err := deleter.Delete(f, base, target.AbsPath); err != nil {
		m.mode = modeList
		m.errMsg = err.Error()
		return m
	}
	folders, status, _ := scanner.Scan(f, base)
	return rebuildAfterDelete(m, folders, status)
}

// rebuildAfterDelete builds the post-deletion Model from a fresh scan while
// carrying the prior session history forward. It is pure, so the all-clear
// disambiguation can be driven directly in tests.
//
// scanner.Classify maps "base exists but has zero children" to
// StatusAccessBlocked, so a re-scan that cleared the last folder is
// indistinguishable from a startup Full Disk Access block on the status alone.
// We disambiguate with the prior model's sessionHasLoaded: if the session had
// already loaded a non-empty list and the re-scan now yields no folders, the
// operator deleted everything, so we enter modeAllClear rather than modeAccess
// (Req 6.4, 8.4, 9.2). Any other non-OK status at this point still falls
// through to modeAccess guidance.
func rebuildAfterDelete(prev Model, folders []model.ScannedFolder, status model.AccessStatus) Model {
	rebuilt := newModel(folders, status)
	// sessionHasLoaded is sticky across the rebuild (Req 9.1).
	rebuilt.sessionHasLoaded = rebuilt.sessionHasLoaded || prev.sessionHasLoaded

	if rebuilt.mode == modeList && len(rebuilt.folders) > 0 {
		// Keep the selection in range near the previous position (Req 6.3).
		rebuilt.index = clamp(prev.index, 0, len(rebuilt.folders)-1)
		return rebuilt
	}
	if len(rebuilt.folders) == 0 && rebuilt.sessionHasLoaded {
		// Everything was deleted during a loaded session: all-clear, not a
		// genuine access block (Req 6.4, 8.4, 9.2).
		rebuilt.mode = modeAllClear
	}
	return rebuilt
}

// clamp constrains v to the inclusive range [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
