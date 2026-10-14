package tui

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
)

// TestRun_AllClearLoopReturnsZero drives tui.Run end-to-end over a Safari-shaped
// tree that holds exactly one hashed folder. Deleting that last remaining folder
// through the loop re-scans to an empty result in a session that has already
// loaded, which rebuildAfterDelete promotes to modeAllClear rather than a
// startup access block. The subsequent quit keypress must take the ordinary
// actQuit path so Run returns 0, and the all-clear frame must have been rendered
// before quitting.
//
// Script: KeyDelete (open confirmation) -> KeyConfirmYes (delete the last
// folder, re-scan to empty, enter modeAllClear) -> KeyQuit (exit with code 0).
//
// _Requirements: 8.4, 9.7_
func TestRun_AllClearLoopReturnsZero(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())

	// Seed exactly one hashed child folder with a data file but NO `origin`
	// sidecar so its recursive SizeKB > 0 and the initial scan yields a
	// non-empty StatusOK list. Under the new default (preserve `origin`),
	// all-clear now relies on this no-origin folder being FULLY removed by the
	// selective delete (D2), leaving an empty re-scan that reaches modeAllClear.
	rel := filepath.Join(containerSuffix, "onlyhash", "data.bin")
	if _, err := fake.WriteFile(rel, make([]byte, 2000)); err != nil {
		t.Fatalf("seeding the single folder: %v", err)
	}

	base := scanner.BasePath(fake.Root())

	term := NewFakeTerminal(KeyDelete, KeyConfirmYes, KeyQuit)
	var stdout, stderr bytes.Buffer

	code := Run(term, fake, base, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit code 0 after clearing the last folder and quitting, got %d (stderr=%q)",
			code, stderr.String())
	}

	// The all-clear frame must have been rendered before the quit keypress was
	// consumed, confirming the loop reached modeAllClear and drew its frame.
	if !strings.Contains(term.Output(), allClearMsg) {
		t.Fatalf("expected the rendered output to contain the all-clear message %q, got %q",
			allClearMsg, term.Output())
	}

	// Raw mode was entered and restored on the quit path (sanity on the loop
	// lifecycle; the all-clear state is reached only after a loaded session).
	if !term.MakeRawCalled {
		t.Fatalf("expected MakeRaw to be called for the interactive loop")
	}
	if !term.RestoreCalled {
		t.Fatalf("expected the terminal to be restored on quit")
	}
}

// TestHandleKey_ModeAllClear asserts the interactive contract of the all-clear
// screen directly on the pure handleKey transition: KeyQuit exits (actQuit, with
// the quit flag set), and every other key is ignored (actNone) while the model
// stays in modeAllClear.
//
// _Requirements: 9.5, 9.6_
func TestHandleKey_ModeAllClear(t *testing.T) {
	t.Run("KeyQuit returns actQuit and sets quit", func(t *testing.T) {
		m := Model{mode: modeAllClear}

		returned, act := m.handleKey(KeyQuit)

		if act != actQuit {
			t.Errorf("handleKey(KeyQuit) action = %v, want actQuit", act)
		}
		if !returned.quit {
			t.Errorf("handleKey(KeyQuit) quit = false, want true")
		}
	})

	t.Run("every non-quit key is ignored and mode stays modeAllClear", func(t *testing.T) {
		nonQuitKeys := []Key{
			KeyUp,
			KeyDown,
			KeyDelete,
			KeyConfirmYes,
			KeyConfirmNo,
			KeyOther,
			KeyNone,
		}

		for _, k := range nonQuitKeys {
			m := Model{mode: modeAllClear}

			returned, act := m.handleKey(k)

			if act != actNone {
				t.Errorf("handleKey(%v) action = %v, want actNone", k, act)
			}
			if returned.mode != modeAllClear {
				t.Errorf("handleKey(%v) mode = %d, want modeAllClear (%d)", k, returned.mode, modeAllClear)
			}
			if returned.quit {
				t.Errorf("handleKey(%v) quit = true, want false", k)
			}
		}
	})
}
