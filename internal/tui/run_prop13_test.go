package tui

import (
	"io"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/model"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
	"github.com/sesamsys/safari-data-tool/internal/ui"
)

// Feature: interactive-tui-mode, Property 13: Post-deletion empty re-scan in a loaded session yields the all-clear state
//
// For any Model with sessionHasLoaded true whose post-deletion re-scan returns
// an empty / StatusAccessBlocked result, the resulting Model is in modeAllClear,
// its frame contains the All_Clear_Message and the Key_Legend with no folder
// rows and no ui.AccessMessage guidance, and a subsequent quit makes the loop
// return 0; when sessionHasLoaded is false the same empty result stays
// modeAccess.
//
// The property is exercised in two layers:
//
//	A) the pure rebuildAfterDelete + render path, driven directly, and
//	B) the end-to-end tui.Run loop over a Safari-shaped fsx.FakeFS seeded with a
//	   single folder so the first scan loads the session and the post-deletion
//	   re-scan is empty.
//
// Validates: Requirements 6.4, 8.4, 9.2, 9.3, 9.4, 9.7

// TestProperty13_PostDeletionAllClear_Pure drives rebuildAfterDelete and render
// directly: a loaded session whose re-scan is empty / StatusAccessBlocked lands
// in modeAllClear with the all-clear frame, while an unloaded session with the
// same empty result stays in modeAccess.
func TestProperty13_PostDeletionAllClear_Pure(t *testing.T) {
	const iterations = 200 // well above the 100-iteration minimum
	rng := rand.New(rand.NewSource(13))

	blockedGuidance := ui.AccessMessage(model.StatusAccessBlocked)

	for iter := 0; iter < iterations; iter++ {
		// A random prior model that has already loaded a non-empty list this
		// session. The specific folder contents / index are irrelevant to the
		// disambiguation, so vary them to broaden coverage.
		prev := Model{
			folders:          prop13RandomFolders(rng),
			index:            rng.Intn(20),
			mode:             modeList,
			status:           model.StatusOK,
			sessionHasLoaded: true,
		}

		// Post-deletion re-scan yields no folders and (per scanner.Classify of
		// an empty base) StatusAccessBlocked.
		rebuilt := rebuildAfterDelete(prev, nil, model.StatusAccessBlocked)

		if rebuilt.mode != modeAllClear {
			t.Fatalf("iter %d: loaded session + empty re-scan: mode = %d, want modeAllClear (%d)",
				iter, rebuilt.mode, modeAllClear)
		}

		frame := string(render(rebuilt))

		if !strings.Contains(frame, allClearMsg) {
			t.Fatalf("iter %d: all-clear frame missing All_Clear_Message %q:\n%s", iter, allClearMsg, frame)
		}
		if !strings.Contains(frame, listLegend) {
			t.Fatalf("iter %d: all-clear frame missing Key_Legend %q:\n%s", iter, listLegend, frame)
		}
		if strings.Contains(frame, blockedGuidance) {
			t.Fatalf("iter %d: all-clear frame must not contain ui.AccessMessage guidance:\n%s", iter, frame)
		}
		// No folder rows: none of the previous folders' rendered lines appear.
		for _, f := range prev.folders {
			if row := ui.FormatLine(f); strings.Contains(frame, row) {
				t.Fatalf("iter %d: all-clear frame unexpectedly contains folder row %q:\n%s", iter, row, frame)
			}
		}

		// Conversely, an unloaded session with the same empty result stays in
		// modeAccess (genuine startup access block, not all-clear).
		unloadedPrev := Model{sessionHasLoaded: false}
		unloaded := rebuildAfterDelete(unloadedPrev, nil, model.StatusAccessBlocked)
		if unloaded.mode != modeAccess {
			t.Fatalf("iter %d: unloaded session + empty re-scan: mode = %d, want modeAccess (%d)",
				iter, unloaded.mode, modeAccess)
		}
	}
}

// TestProperty13_PostDeletionAllClear_Loop drives the whole tui.Run loop over a
// Safari-shaped fake tree seeded with exactly one hashed folder. The initial
// scan is StatusOK non-empty (so sessionHasLoaded becomes true); deleting that
// folder empties the base, so the re-scan is StatusAccessBlocked and the TUI
// reaches modeAllClear. A trailing quit keypress makes Run return 0 and the
// captured output contains the all-clear frame.
func TestProperty13_PostDeletionAllClear_Loop(t *testing.T) {
	const iterations = 100 // the stated minimum
	rng := rand.New(rand.NewSource(1313))

	blockedGuidance := ui.AccessMessage(model.StatusAccessBlocked)

	for iter := 0; iter < iterations; iter++ {
		fake := fsx.NewFakeFS(t.TempDir())

		// Seed exactly ONE hashed folder (with a file so SizeKB > 0) under the
		// container suffix. The first scan therefore returns StatusOK with a
		// single folder, loading the session.
		hash := prop13Hash(rng)
		rel := filepath.Join(containerSuffix, hash, "data.bin")
		if _, err := fake.WriteFile(rel, make([]byte, rng.Intn(4000)+512)); err != nil {
			t.Fatalf("iter %d: seeding %s: %v", iter, hash, err)
		}

		base := scanner.BasePath(fake.Root())

		// Delete the single folder, then quit from the all-clear screen.
		term := NewFakeTerminal(KeyDelete, KeyConfirmYes, KeyQuit)

		code := Run(term, fake, base, io.Discard, io.Discard)
		if code != 0 {
			t.Fatalf("iter %d: Run returned %d, want 0", iter, code)
		}

		out := term.Output()
		if !strings.Contains(out, allClearMsg) {
			t.Fatalf("iter %d: Run output missing All_Clear_Message %q:\n%s", iter, allClearMsg, out)
		}
		if !strings.Contains(out, listLegend) {
			t.Fatalf("iter %d: Run output missing Key_Legend %q:\n%s", iter, listLegend, out)
		}
		if strings.Contains(out, blockedGuidance) {
			t.Fatalf("iter %d: Run output must not contain ui.AccessMessage guidance in the all-clear flow:\n%s", iter, out)
		}
	}
}

// prop13RandomFolders returns a random, non-empty slice of placeholder
// ScannedFolders for the pure-layer prior model. Contents are only used to
// confirm their rows do NOT leak into the all-clear frame.
func prop13RandomFolders(rng *rand.Rand) []model.ScannedFolder {
	n := rng.Intn(15) + 1 // 1..15
	folders := make([]model.ScannedFolder, n)
	for i := range folders {
		h := prop13Hash(rng)
		folders[i] = model.ScannedFolder{
			Hash:    h,
			AbsPath: "/base/" + h,
			SizeKB:  int64(rng.Intn(10000) + 1),
			Domain:  "example-" + h + ".com",
		}
	}
	return folders
}

// prop13Hash returns a random lowercase-hex-ish folder name usable both as a
// map-safe identifier and as a directory name under the container suffix.
func prop13Hash(rng *rand.Rand) string {
	const alphabet = "0123456789abcdef"
	var b strings.Builder
	for i := 0; i < 16; i++ {
		b.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return b.String()
}
