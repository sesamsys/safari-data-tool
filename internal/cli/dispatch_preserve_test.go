package cli

import (
	"bytes"
	"strings"
	"testing"
)

// tuiClearScreen is the TUI's full-screen clear escape (see render's escClear).
// The non-interactive list/delete flows never emit it, so its absence in the
// captured stdout proves the TUI was not launched.
const tuiClearScreen = "\x1b[2J"

// --- Task 10.4: list/delete dispatch preservation (Req 1.2, 1.3) ---------
//
// These example tests pin the behavior that explicit "list" and "delete"
// subcommands keep running their existing non-interactive flows and do NOT
// launch the TUI, even though a bare invocation now opens the interactive
// browser.

// TestRun_ListPreservesNonInteractiveBehavior asserts that `run` with the
// explicit "list" subcommand still produces the plain listing (a FormatLine
// per folder: domain + (HASH)) and does not launch the TUI (Req 1.2).
func TestRun_ListPreservesNonInteractiveBehavior(t *testing.T) {
	fake := newPopulatedFake(t)
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"prog", "list"},
		&stdout, &stderr, strings.NewReader(""), fake, fake.Root(),
	)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}

	out := stdout.String()
	// Existing non-interactive listing behavior: the seeded domain and its
	// hashed-folder tag appear in the plain output.
	if !strings.Contains(out, "example.com") || !strings.Contains(out, "(AAAA1111)") {
		t.Fatalf("list output missing seeded folder, got:\n%s", out)
	}

	// The TUI was NOT launched: the plain listing never emits the full-screen
	// clear escape that every TUI frame starts with.
	if strings.Contains(out, tuiClearScreen) {
		t.Fatalf("list unexpectedly launched the TUI (found clear-screen escape):\n%q", out)
	}
}

// TestRun_DeletePreservesNonInteractiveBehavior asserts that `run` with the
// explicit "delete" subcommand still performs the non-interactive, forced
// deletion (removing the targeted folder and printing the "Deleted ..."
// confirmation) and does not launch the TUI (Req 1.3).
func TestRun_DeletePreservesNonInteractiveBehavior(t *testing.T) {
	fake := newPopulatedFake(t)
	folder := fake.Path(containerRel + "/AAAA1111")
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"prog", "delete", "--hash", "AAAA1111", "--force"},
		&stdout, &stderr, strings.NewReader(""), fake, fake.Root(),
	)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}

	out := stdout.String()
	// Existing non-interactive deletion behavior: the confirmation line is
	// printed to stdout. Under the new default this reports a clear.
	if !strings.Contains(out, "Cleared") {
		t.Fatalf("expected clear confirmation on stdout, got:\n%s", out)
	}

	// New default: the targeted folder's data is cleared while it remains
	// resolvable (its origin sidecar is preserved).
	assertCleared(t, fake, folder, "example.com")

	// The TUI was NOT launched: the forced-delete flow never emits the
	// full-screen clear escape that every TUI frame starts with.
	if strings.Contains(out, tuiClearScreen) {
		t.Fatalf("delete unexpectedly launched the TUI (found clear-screen escape):\n%q", out)
	}
}
