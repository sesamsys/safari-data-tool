package tui

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/scanner"
)

// seedOKTree writes a single non-empty hashed child folder under the Safari
// container suffix so scanner.Scan yields a StatusOK, non-empty Folder_List.
// It returns the FakeFS and the derived base path.
func seedOKTree(t *testing.T) (fsx.FileSystem, string) {
	t.Helper()
	fake := fsx.NewFakeFS(t.TempDir())
	rel := filepath.Join(containerSuffix, "hash1", "data.bin")
	if _, err := fake.WriteFile(rel, make([]byte, 2000)); err != nil {
		t.Fatalf("seeding ok tree: %v", err)
	}
	return fake, scanner.BasePath(fake.Root())
}

// TestRun_ScanBeforeRender asserts the scan happens before any frame is
// rendered: when the scan returns a non-OK status (here StatusNotFound, since
// the container does not exist), Run exits non-zero without ever entering raw
// mode or writing a frame. Nothing is drawn and MakeRaw is never called.
//
// _Requirements: 2.1_
func TestRun_ScanBeforeRender(t *testing.T) {
	// Empty FakeFS: the container path does not exist, so scanner.Scan
	// classifies it as StatusNotFound.
	fake := fsx.NewFakeFS(t.TempDir())
	base := scanner.BasePath(fake.Root())

	term := NewFakeTerminal() // empty script

	code := Run(term, fake, base, io.Discard, io.Discard)
	if code == 0 {
		t.Fatalf("expected non-zero exit code for non-OK status, got %d", code)
	}

	if got := term.Output(); got != "" {
		t.Fatalf("expected no frame output when status is non-OK, got %q", got)
	}
	if term.MakeRawCalled {
		t.Fatal("MakeRaw was called before/without a successful scan; scan must gate raw mode")
	}
}

// TestRun_RestoreOnNormalQuit asserts that on the normal quit path Run enters
// raw mode and the restore returned by MakeRaw runs via defer before Run
// returns 0.
//
// _Requirements: 7.2_
func TestRun_RestoreOnNormalQuit(t *testing.T) {
	fake, base := seedOKTree(t)

	term := NewFakeTerminal(KeyQuit)

	code := Run(term, fake, base, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("expected exit code 0 on normal quit, got %d", code)
	}

	if !term.MakeRawCalled {
		t.Fatal("expected MakeRaw to be called on the OK path")
	}
	if !term.RestoreCalled {
		t.Fatal("expected restore to run on the normal quit path")
	}
}

// TestRun_RestoreOnErrorPath asserts that when the event loop aborts because
// ReadKey returns an error, the deferred restore still runs before Run returns
// its non-zero code. This exercises the error/panic exit path of the
// restore-on-every-exit contract.
//
// _Requirements: 7.2_
func TestRun_RestoreOnErrorPath(t *testing.T) {
	fake, base := seedOKTree(t)

	term := NewFakeTerminal(KeyDown).FailReadWith(errors.New("read fail"))

	code := Run(term, fake, base, io.Discard, io.Discard)
	if code != 1 {
		t.Fatalf("expected exit code 1 on ReadKey failure, got %d", code)
	}

	if !term.MakeRawCalled {
		t.Fatal("expected MakeRaw to be called on the OK path")
	}
	if !term.RestoreCalled {
		t.Fatal("expected deferred restore to run despite the ReadKey error")
	}
}
