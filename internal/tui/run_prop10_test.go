package tui

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
	"github.com/sesamsys/safari-data-tool/internal/selection"
)

// Feature: interactive-tui-mode, Property 10: y/Y deletes the selected folder's path
//
// For any Model in modeConfirm over a non-empty Folder_List, driving a
// KeyConfirmYes keypress (y or Y) through the event loop invokes
// deleter.Delete with the AbsPath of the folder at the Selection_Index, and
// after the operation that path no longer exists in the filesystem.
//
// Each iteration seeds a Safari-shaped fake tree with a random number (1..6) of
// hashed child folders, each holding a file so its recursive SizeKB > 0. It
// records the AbsPath of the selected (index 0, largest after TopN) folder,
// then drives tui.Run with a scripted KeyDelete -> KeyConfirmYes -> KeyQuit
// sequence. After the run it asserts the recorded path no longer exists and
// that Run returned exit code 0.
//
// Validates: Requirements 5.3
func TestProperty10_ConfirmYesDeletesSelectedPath(t *testing.T) {
	const iterations = 150 // >= 100 iterations
	rng := rand.New(rand.NewSource(0x10DE1E7E))

	// hashNames supplies distinct hashed-folder names; one per seeded folder.
	hashNames := []string{"aaa", "bbb", "ccc", "ddd", "eee", "fff"}

	for iter := 0; iter < iterations; iter++ {
		fake := fsx.NewFakeFS(t.TempDir())

		// Seed a random count of hashed child folders (1..6), each with a file
		// of a distinct size so every folder has SizeKB > 0 and the descending
		// ordering is deterministic.
		count := rng.Intn(len(hashNames)) + 1
		for i := 0; i < count; i++ {
			// Larger byte counts for later indices would be arbitrary; the
			// absolute ordering does not matter for this property as long as
			// each folder is non-empty. Give each a distinct non-zero size.
			size := (i + 1) * 1000
			rel := filepath.Join(containerSuffix, hashNames[i], "data.bin")
			if _, err := fake.WriteFile(rel, make([]byte, size)); err != nil {
				t.Fatalf("iteration %d: seeding %s: %v", iter, hashNames[i], err)
			}
		}

		base := scanner.BasePath(fake.Root())

		// Record the AbsPath of the folder that will be selected at index 0:
		// the largest after TopN ordering, matching what newModel selects.
		folders, _, err := scanner.Scan(fake, base)
		if err != nil {
			t.Fatalf("iteration %d: scan: %v", iter, err)
		}
		top := selection.TopN(folders, listingLimit)
		if len(top) == 0 {
			t.Fatalf("iteration %d: expected a non-empty folder list after seeding %d folders", iter, count)
		}
		selectedPath := top[0].AbsPath

		// Sanity: the selected path exists before the run.
		if _, err := fake.Stat(selectedPath); err != nil {
			t.Fatalf("iteration %d: selected path %q should exist before deletion: %v", iter, selectedPath, err)
		}

		// Drive the event loop: open the confirmation on the selected (index 0)
		// folder, confirm the deletion, then quit.
		term := NewFakeTerminal(KeyDelete, KeyConfirmYes, KeyQuit)
		var stdout, stderr bytes.Buffer

		code := Run(term, fake, base, &stdout, &stderr)

		// Run returned cleanly on quit (Req 7.1).
		if code != 0 {
			t.Fatalf("iteration %d: Run returned %d, want 0 (stderr=%q)", iter, code, stderr.String())
		}

		// The selected folder's AbsPath no longer exists (Req 5.3).
		if _, err := fake.Stat(selectedPath); err == nil {
			t.Fatalf("iteration %d: selected path %q still exists after KeyConfirmYes", iter, selectedPath)
		} else if !os.IsNotExist(err) {
			t.Fatalf("iteration %d: unexpected Stat error for %q: %v (want not-exist)", iter, selectedPath, err)
		}
	}
}
