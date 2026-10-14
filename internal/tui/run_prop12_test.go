package tui

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
)

// Feature: interactive-tui-mode, Property 12: Non-OK access status yields a non-zero exit code
//
// Property 12: Non-OK Initial_Scan yields a non-zero exit code — for any non-OK
// AccessStatus returned by the Initial_Scan (while sessionHasLoaded is still
// false), tui.Run returns a non-zero exit code without entering raw mode and
// without rendering the Folder_List; and for an Initial_Scan of StatusOK with
// at least one folder, a scripted quit keypress makes tui.Run return 0.
//
// Validates: Requirements 8.1, 8.2, 8.3
//
// The two scenarios are interleaved across a minimum of 100 iterations, varying
// which non-OK status is produced (StatusNotFound via a missing container,
// StatusAccessBlocked via an empty-but-existing container) and, for the OK case,
// the number of seeded folders.
func TestRun_Property12_NonOKExitCode(t *testing.T) {
	const iterations = 120

	for i := 0; i < iterations; i++ {
		i := i
		t.Run("", func(t *testing.T) {
			switch i % 3 {
			case 0:
				// Scenario 1a: base (container suffix) does not exist -> the
				// scanner returns StatusNotFound. Run must exit non-zero
				// without entering raw mode and without rendering any frame.
				fake := fsx.NewFakeFS(t.TempDir())
				base := scanner.BasePath(fake.Root())
				assertNonOKRun(t, fake, base)

			case 1:
				// Scenario 1b: base exists but is empty (zero child folders) ->
				// the scanner returns StatusAccessBlocked. Same expectations.
				fake := fsx.NewFakeFS(t.TempDir())
				if _, err := fake.Mkdir(containerSuffix); err != nil {
					t.Fatalf("creating empty container suffix: %v", err)
				}
				base := scanner.BasePath(fake.Root())
				assertNonOKRun(t, fake, base)

			default:
				// Scenario 2: StatusOK with at least one folder. A scripted
				// quit keypress makes Run return 0.
				fake := fsx.NewFakeFS(t.TempDir())
				// Vary the folder count across iterations (1..5) so the OK path
				// is exercised with different list sizes.
				count := (i % 5) + 1
				for j := 0; j < count; j++ {
					hash := string(rune('a'+j)) + "hash"
					rel := filepath.Join(containerSuffix, hash, "data.bin")
					// SizeKB > 0 for every folder.
					if _, err := fake.WriteFile(rel, make([]byte, 2000)); err != nil {
						t.Fatalf("seeding folder %q: %v", hash, err)
					}
				}
				base := scanner.BasePath(fake.Root())

				term := NewFakeTerminal(KeyQuit)
				var stdout, stderr bytes.Buffer
				code := Run(term, fake, base, &stdout, &stderr)

				if code != 0 {
					t.Fatalf("StatusOK with %d folders: expected exit code 0, got %d (stderr=%q)",
						count, code, stderr.String())
				}
				if !term.MakeRawCalled {
					t.Fatalf("StatusOK: expected MakeRaw to be called for the interactive loop")
				}
				if !term.RestoreCalled {
					t.Fatalf("StatusOK: expected the terminal to be restored on quit")
				}
				// A frame should have been rendered at least once before the
				// quit keypress was consumed.
				if term.Output() == "" {
					t.Fatalf("StatusOK: expected at least one rendered frame, got none")
				}
			}
		})
	}
}

// assertNonOKRun drives tui.Run against a fake FS whose scan yields a non-OK
// AccessStatus and asserts the startup error contract of Property 12:
//   - Run returns a non-zero exit code,
//   - raw mode is never entered (MakeRaw is not called),
//   - no frame is written to the terminal (the Folder_List is never rendered).
func assertNonOKRun(t *testing.T, fake *fsx.FakeFS, base string) {
	t.Helper()

	term := NewFakeTerminal() // empty script: no keys needed on the non-OK path
	var stdout, stderr bytes.Buffer

	code := Run(term, fake, base, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("non-OK status: expected a non-zero exit code, got 0")
	}
	if term.MakeRawCalled {
		t.Fatalf("non-OK status: expected raw mode NOT to be entered, but MakeRaw was called")
	}
	if term.Output() != "" {
		t.Fatalf("non-OK status: expected no frame to be rendered, got %q", term.Output())
	}
	// stdout must stay empty: guidance goes to stderr, the list is never drawn.
	if stdout.Len() != 0 {
		t.Fatalf("non-OK status: expected empty stdout, got %q", stdout.String())
	}
}
