# Design Document

## Overview

This feature adds an interactive, full-screen terminal UI (TUI) to `safari-data-tool`. Running the tool with no subcommand (a Bare_Invocation) now launches the TUI instead of printing the non-interactive listing; the `list` and `delete` subcommands keep their current behavior untouched.

The TUI lets the operator browse the fifteen largest Safari website-data folders (largest first), move the selection with the arrow keys (wrap-around at both ends), and delete the selected folder after an in-screen confirmation that shows its Domain, Hash, and Size. After a successful deletion the list re-scans and rebuilds; if the operator has cleared every folder, the TUI shows an all-clear empty state rather than the Full Disk Access guidance. `q`/`Q` quits and restores the terminal to its prior mode.

The implementation is pure Go, standard library only, macOS-only. Raw terminal mode is established by driving `termios` through `golang.org/x/sys`-free `syscall`/`unsafe` `ioctl` calls, and all input decoding (arrow keys, command keys) and all drawing (reverse-video selection, redraws, legend) use hand-written ANSI escape sequences.

### Design Principles

The central design choice mirrors the project's existing `fsx.FileSystem` + `fsx.Fake` pattern: **all decision logic is pure and separated from I/O**. The TUI is split into three layers:

1. A **pure model** (`tui.Model`) that holds state (folder list, selection index, screen mode, pending error) and exposes pure transition functions. It never touches the terminal or the clock.
2. A **pure renderer** that turns a `Model` into a byte frame of ANSI escapes and text. It writes to an `io.Writer`, so tests render into a buffer.
3. A thin **terminal driver** (`tui.Terminal`) behind an interface, with a real `termios`-backed implementation and a fake for tests, exactly like `fsx.OS()` vs `fsx.NewFakeFS`.

The event loop wires these together but contains no branching logic of its own worth testing — every meaningful decision lives in a pure function that is driven directly in tests with generated inputs.

The TUI reuses existing building blocks rather than reimplementing them: `scanner.Scan` for enumeration and `AccessStatus`, `selection.TopN` for the size-ranked top-15, `ui.FormatLine` for row text, `ui.FormatSize` for the confirmation size, `ui.AccessMessage` for access guidance, and `deleter.Delete` for removal.

## Architecture

```
main.go
  └── cli.Run(os.Args)
        └── cli.run(args, stdout, stderr, stdin, fsx, home)
              ├── args[1]=="list"   → runList   (unchanged)
              ├── args[1]=="delete" → runDelete (unchanged)
              └── otherwise         → runTUI    (NEW)
                                        │
                                        ▼
                             tui.Run(term, fsx, home, out)
                               │
         ┌─────────────────────┼───────────────────────────┐
         ▼                     ▼                             ▼
  tui.Terminal          tui.Model (pure)              render (pure)
  (raw mode, read,      - folders []ScannedFolder     Model → ANSI frame
   restore)             - index int                   → io.Writer
                        - mode  (List/Confirm/Access/AllClear)
                        - status AccessStatus
                        - transition functions
                              │
                              ├── scanner.Scan + selection.TopN  (rebuild)
                              └── deleter.Delete                 (remove)
```

### Dispatch

`cli.run` currently decides between `runList` and `runDelete`. The only change in the `cli` package is the default branch: a Bare_Invocation (and any first argument that is not `list` or `delete`) routes to the new `runTUI` instead of `runList`.

To keep the dispatch decision testable without launching a terminal, the branch selection is extracted into a pure helper:

```go
// command classifies the invocation from its argument slice.
type command int

const (
	cmdTUI command = iota // bare invocation / default
	cmdList
	cmdDelete
)

// classify picks the command from os.Args-style arguments. args[0] is the
// program name. A first argument of "list" or "delete" selects that
// subcommand; anything else (including no argument) selects the TUI.
func classify(args []string) command {
	if len(args) > 1 {
		switch args[1] {
		case "list":
			return cmdList
		case "delete":
			return cmdDelete
		}
	}
	return cmdTUI
}
```

`run` becomes a switch over `classify(args)`. `runList` and `runDelete` are called with the same signatures as today; `runTUI` is new.

### TUI entry point

```go
// runTUI launches the interactive browser (Req 1.1). It constructs the real
// termios-backed terminal and delegates to tui.Run. A non-OK access status
// yields a non-zero exit code (Req 8.3).
func runTUI(stdout, stderr io.Writer, f fsx.FileSystem, home string) int {
	term, err := tui.NewOSTerminal() // opens /dev/tty-equivalent (os.Stdin/Stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return tui.Run(term, f, scanner.BasePath(home), stdout, stderr)
}
```

## Components and Interfaces

### Package layout

A new package `internal/tui` holds all TUI code:

- `internal/tui/model.go` — the pure `Model` and its transition functions.
- `internal/tui/render.go` — the pure renderer (`Model` → ANSI frame).
- `internal/tui/keys.go` — input decoding (bytes → `Key`).
- `internal/tui/terminal.go` — the `Terminal` interface and `termios` syscall driver.
- `internal/tui/fake.go` — `FakeTerminal` for tests (non-`_test.go`, importable like `fsx.FakeFS`).
- `internal/tui/run.go` — the event loop that wires the above together.

### The Terminal interface

The driver interface isolates the only non-portable, non-pure part of the feature so the rest can be tested against a fake, mirroring `fsx.FileSystem`.

```go
// Terminal abstracts raw-mode terminal control and byte I/O so the event loop
// can be exercised against a FakeTerminal in tests (Req 1.4, 7.2).
type Terminal interface {
	// MakeRaw puts the terminal into unbuffered, no-echo raw mode and returns
	// a restore function that returns it to the mode captured at this call.
	MakeRaw() (restore func() error, err error)
	// ReadKey blocks for the next decoded keypress.
	ReadKey() (Key, error)
	// Write emits a rendered frame (ANSI escapes + text) to the screen.
	Write(p []byte) (int, error)
}
```

The real implementation (`osTerminal`) captures the current `termios` via a `TIOCGETA` `ioctl`, builds a raw variant (clearing `ECHO`, `ICANON`, `ISIG`, `IEXTEN`, input/output post-processing flags, setting `VMIN=1`/`VTIME=0`), and applies it with `TIOCSETA`. `restore` re-applies the captured original struct with `TIOCSETA`. These `ioctl`s are issued through `syscall.Syscall(syscall.SYS_IOCTL, ...)` with an `unsafe.Pointer` to a `termios` struct whose layout matches the Darwin ABI — all standard library, no third-party dependency (Req 1.4).

`restore` is installed with `defer` the moment raw mode succeeds, so it runs on every exit path: normal quit, error return, or panic (Req 7.2).

Clearing the output post-processing flag (`OPOST`) means the terminal no longer translates `\n` into `\r\n`, so the renderer must emit explicit `\r\n` line endings itself (see [Rendering](#rendering)).

### Keys

Input decoding is pure: it maps raw byte sequences to a small `Key` enum. Arrow keys arrive as the three-byte sequence `ESC [ A/B/C/D`; command keys arrive as single bytes.

```go
type Key int

const (
	KeyNone Key = iota
	KeyUp         // ESC [ A
	KeyDown       // ESC [ B
	KeyDelete     // 'd' or 'D'
	KeyConfirmYes // 'y' or 'Y'
	KeyConfirmNo  // 'n' or 'N'
	KeyQuit       // 'q' or 'Q'
	KeyOther      // anything else; ignored by the model
)

// decodeKey maps a raw byte (and, for escape sequences, a 3-byte buffer) to a
// Key. It is pure and table-driven so it can be unit tested directly.
func decodeKey(b []byte) Key
```

`osTerminal.ReadKey` reads one byte; if it is `ESC` (0x1B) it reads up to two more to complete an arrow sequence, then calls `decodeKey`. The `FakeTerminal` replays a scripted sequence of `Key` values, so the loop can be driven deterministically.

### The Model

```go
// mode is the current screen the TUI is showing.
type mode int

const (
	modeList     mode = iota // browsing the folder list
	modeConfirm              // confirmation prompt for the selected folder
	modeAccess               // non-OK access status at startup; guidance only
	modeAllClear             // all folders cleared during this session (Req 9)
)

// Model is the pure, I/O-free state of the TUI. Every transition is a method
// that returns a new Model (or mutates and returns itself); none of them touch
// the terminal, filesystem, or clock directly.
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
```

#### Building the model from a scan

`scanner.Scan` and `selection.TopN` are reused verbatim. The result is wrapped into a `Model`.

```go
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
```

Because `newModel` delegates ordering and limiting to `selection.TopN`, the Folder_List is always the descending-size top-15 by construction (Req 2.2, 2.3, 2.4), and the same function is used for the post-deletion rebuild (Req 6.2), so there is a single source of truth for list shape.

Once set, `sessionHasLoaded` is sticky for the rest of the session (Req 9.1): the rebuild path OR-s the prior value into the newly built model so a later empty re-scan still remembers the session had loaded.

#### Navigation

Wrap-around navigation is modular arithmetic over the list length, which collapses the four directional cases (Req 3.2–3.5) into two one-line transitions.

```go
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
```

Both keep `index` in `[0, len(folders))` whenever the list is non-empty, which is the index-in-range invariant that must also hold after a rebuild (Req 3.6, 6.3).

#### Deletion flow

Key handling in `modeList` and `modeConfirm` is a pure transition that, for the delete/confirm keys, reports an intended side effect to the loop rather than performing it. This keeps the model pure while letting tests assert the decision.

```go
// action is a side effect the event loop must perform after a transition.
type action int

const (
	actNone   action = iota
	actDelete        // delete folders[index], then re-scan and rebuild
	actQuit          // stop the loop
)

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
```

The loop performs `actDelete` by calling `deleter.Delete`, then re-scanning:

```go
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
```

`deleter.Delete` already re-runs `ValidatePath` as a defensive final gate, so the TUI inherits the same path-safety guarantee as the `delete` subcommand without duplicating it.

The disambiguation lives entirely in `rebuildAfterDelete`, which only ever runs after `applyDelete` — that is, after a successful `StatusOK` session reached raw mode. The genuine startup-blocked case is handled earlier in `Run` (before raw mode) where `sessionHasLoaded` is still false, so it can never be misclassified as all-clear.

### Rendering

The renderer is a pure function `render(m Model) []byte` (the loop writes the result through `Terminal.Write`). It builds a full-screen frame: clear screen + home cursor (`ESC [ 2J`, `ESC [ H`), the folder rows (or access guidance), the confirmation block when confirming, any pending error, and the always-visible legend.

**Line endings under raw mode.** Every line in the frame ends with an explicit carriage return + line feed (`\r\n`), not a bare `\n`. The event loop runs the terminal in raw mode, which clears output post-processing (`OPOST`/`ONLCR`); in that mode the terminal no longer translates `\n` into `\r\n`, so a bare line feed moves the cursor down one row **without** returning it to column 0 and the rows stair-step across the screen. Emitting `\r\n` keeps each row left-aligned regardless of terminal mode and keeps `render` self-contained — its output is correct without relying on the terminal translating newlines. This is captured in a single `crlf` constant used for every line ending.

```go
const (
	escClear      = "\x1b[2J\x1b[H"
	escInvert     = "\x1b[7m" // reverse video for the selected row (Req 4.1)
	escReset      = "\x1b[0m"
	listLegend    = "↑/↓ move   d delete   q quit"  // Key_Legend for list / all-clear
	confirmLegend = "y confirm   n cancel"           // Key_Legend for the confirmation prompt
	allClearMsg   = "No Safari website-data folders remain." // All_Clear_Message (Req 9.3)

	// crlf ends every rendered line. Raw mode disables OPOST/ONLCR, so a bare
	// "\n" would not return the cursor to column 0 and rows would stair-step.
	crlf = "\r\n"
)

// render produces the full ANSI frame for the current model. It is pure: given
// the same Model it always yields the same bytes, so tests render into a buffer
// and assert on the output.
//
// The Key_Legend is context-sensitive (Req 2.6). In every mode except
// modeConfirm it is the last line of the frame. In modeConfirm the legend
// shows the confirm/cancel keys and is placed *above* the prompt so the
// "Proceed? [y/N]:" line stays last and the operator's input appears directly
// after it (Req 5.6, 5.7).
func render(m Model) []byte {
	var b strings.Builder
	b.WriteString(escClear)

	switch m.mode {
	case modeAccess:
		// Access guidance only; no folder rows (Req 8.1, 8.2). The list
		// legend is appended below.
		b.WriteString(ui.AccessMessage(m.status))
		b.WriteString(crlf)
		b.WriteString(listLegend) // Key_Legend (Req 2.6)
		b.WriteString(crlf)
	case modeAllClear:
		// Everything was cleared this session: show the all-clear message
		// instead of the ui.AccessMessage guidance, with no folder rows
		// (Req 9.3, 9.4). The list legend is appended below.
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
		"About to permanently delete:%[1]s  Domain: %[2]s%[1]s  Hash:   %[3]s%[1]s  Size:   %[4]s%[1]sProceed? [y/N]: ",
		crlf, folder.Domain, folder.Hash, ui.FormatSize(folder.SizeKB),
	)
}
```

The selected row is the only one wrapped in `escInvert … escReset`, giving the "exactly one inverted row at the selection index" guarantee (Req 4.1, 4.2). A Key_Legend is appended on every frame; it is the generic `listLegend` as the last line in list, access, and all-clear modes, and the `confirmLegend` placed just above the prompt in confirmation mode so the `Proceed? [y/N]:` line stays last (Req 2.6, 5.6, 5.7).

### Event loop

```go
// Run establishes raw mode, performs the initial scan, and drives the
// read-render loop until the operator quits. It restores the terminal on every
// exit path via defer (Req 7.2) and returns a non-zero code for a non-OK access
// status (Req 8.3).
func Run(term Terminal, f fsx.FileSystem, base string, stdout, stderr io.Writer) int {
	folders, status, err := scanner.Scan(f, base) // scan before rendering (Req 2.1)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if status != model.StatusOK {
		fmt.Fprintln(stderr, ui.AccessMessage(status)) // Req 8.1, 8.2
		return 1                                        // Req 8.3
	}

	restore, err := term.MakeRaw()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer restore() // Req 7.2 — runs on quit, error, or panic

	m := newModel(folders, status)
	for {
		if _, err := term.Write(render(m)); err != nil {
			return 1
		}
		key, err := term.ReadKey()
		if err != nil {
			return 1
		}
		var act action
		m, act = m.handleKey(key)
		switch act {
		case actQuit:
			return 0 // Req 7.1
		case actDelete:
			m = applyDelete(m, f, base)
		}
	}
}
```

`Run` returns before entering raw mode when the Initial_Scan status is non-OK, so the startup access path never leaves the terminal in a modified state and still returns a non-zero code (Req 8.1, 8.2, 8.3). `sessionHasLoaded` starts false, so this early return is the only place a non-OK status is treated as a hard error.

The loop itself is mode-agnostic: it renders, reads a key, and dispatches through `handleKey`. `modeAllClear` reuses this exact loop and is fully interactive — `handleKey` returns `actQuit` for `KeyQuit` and `actNone` for everything else (Req 9.5, 9.6). Because `applyDelete` (and the `rebuildAfterDelete` that can produce `modeAllClear`) runs only *after* a successful `StatusOK` session has already entered raw mode, reaching `modeAllClear` never passes through the early non-zero return above. A subsequent quit therefore takes the ordinary `actQuit` path and `Run` returns `0` (Req 9.7), with `defer restore()` still restoring the terminal (Req 7.2).

## Data Models

The feature introduces no new persisted data types. It reuses `model.ScannedFolder` and `model.AccessStatus` unchanged. The only new types are internal to `internal/tui`: `Model`, `mode`, `Key`, `action`, and the `Terminal` interface described above.

| Type | Purpose | Lifetime |
| --- | --- | --- |
| `tui.Model` | Pure TUI state (folders, index, mode, status, error, quit, sessionHasLoaded) | One per `Run` invocation; replaced on rebuild |
| `tui.mode` | Which screen is shown (list/confirm/access/all-clear) | Field of `Model` |
| `tui.Key` | Decoded keypress | Per keystroke |
| `tui.action` | Side effect for the loop (none/delete/quit) | Per keystroke |
| `tui.Terminal` | Raw-mode control + byte I/O | One per `Run` invocation |

## Error Handling

| Condition | Handling | Requirement |
| --- | --- | --- |
| `scanner.Scan` returns an error | Print to stderr, return exit code 1, never enter raw mode | — |
| Non-OK `AccessStatus` | Print `ui.AccessMessage(status)` to stderr, do not render the list, return non-zero | 8.1, 8.2, 8.3 |
| `term.MakeRaw` fails | Print to stderr, return 1 | — |
| `deleter.Delete` fails | Record `errMsg`, return to `modeList` with the folder set unchanged, show the failure on the next frame | 5.5 |
| `term.Write` / `term.ReadKey` fails | Return 1; `defer restore()` still runs | 7.2 |
| Panic in the loop | `defer restore()` restores the terminal before the panic propagates | 7.2 |
| Empty re-scan after a deletion, session already loaded | `rebuildAfterDelete` promotes the empty `StatusAccessBlocked` result to `modeAllClear` (sessionHasLoaded is true); the All_Clear_Message and legend are shown, no rows, no `ui.AccessMessage` guidance, and quit still exits with code 0 | 6.4, 8.4, 9.2, 9.3 |

Deletion failures are deliberately non-fatal: the operator stays in the browser and can retry or pick another folder, consistent with Req 5.5.

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system — essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Bare invocation routes to the TUI

*For any* argument slice whose first argument after the program name is neither `list` nor `delete` (including a slice with no such argument), `classify` SHALL return `cmdTUI`; and *for any* slice whose first argument is `list` or `delete`, `classify` SHALL return `cmdList` or `cmdDelete` respectively.

**Validates: Requirements 1.1, 1.2, 1.3**

### Property 2: Folder list is always the descending top-15 of the scan

*For any* set of scanned folders with status `StatusOK`, the `Model` built by `newModel` SHALL have a `folders` slice equal to `selection.TopN(folders, 15)` — ordered by descending size and limited to fifteen entries — both at initial build and after a post-deletion rebuild.

**Validates: Requirements 2.2, 2.3, 2.4, 6.1, 6.2**

### Property 3: Rendered rows reuse the existing line format

*For any* folder in a displayed Folder_List, the rendered frame SHALL contain that folder's `ui.FormatLine(folder)` text as its row.

**Validates: Requirements 2.5**

### Property 4: A context-appropriate legend is present on every frame

*For any* `Model`, the frame produced by `render` SHALL contain a Key_Legend: the `confirmLegend` text when the mode is `modeConfirm`, and the `listLegend` text in every other mode (list, access, or all-clear).

**Validates: Requirements 2.6, 5.6, 9.4**

### Property 5: Navigation wraps around and is modular

*For any* non-empty Folder_List of length `n` and any starting index `i` in `[0, n)`, `moveDown` SHALL yield `(i+1) mod n` and `moveUp` SHALL yield `(i+n-1) mod n`, so moving down from the last item yields the first and moving up from the first yields the last.

**Validates: Requirements 3.2, 3.3, 3.4, 3.5**

### Property 6: Selection index stays in range

*For any* sequence of navigation keypresses applied to a non-empty Folder_List, and *for any* post-deletion rebuild that leaves at least one folder, the `Selection_Index` SHALL satisfy `0 <= index < len(folders)`.

**Validates: Requirements 3.1, 3.6, 6.3**

### Property 7: Exactly one inverted row, at the selection index

*For any* `Model` in `modeList` or `modeConfirm` with a non-empty Folder_List, the rendered frame SHALL wrap exactly one row in the reverse-video escape, and that row SHALL be the one at the `Selection_Index`.

**Validates: Requirements 4.1, 4.2**

### Property 8: `d`/`D` opens confirmation for the selected item

*For any* `Model` in `modeList` with a non-empty Folder_List, applying a `KeyDelete` keypress (`d` or `D`) SHALL produce a `Model` in `modeConfirm` whose confirmation targets the folder at the current `Selection_Index`.

**Validates: Requirements 5.1**

### Property 9: Confirmation shows Domain, Hash, and Size

*For any* folder, the confirmation frame rendered for it SHALL contain that folder's `Domain`, its `Hash`, and its size as rendered by `ui.FormatSize(SizeKB)`.

**Validates: Requirements 5.2**

### Property 10: `y`/`Y` deletes the selected folder's path

*For any* `Model` in `modeConfirm` over a non-empty Folder_List, applying a `KeyConfirmYes` keypress (`y` or `Y`) SHALL invoke `deleter.Delete` with the `AbsPath` of the folder at the `Selection_Index`, and after the operation that path SHALL no longer exist in the filesystem.

**Validates: Requirements 5.3**

### Property 11: `n`/`N` cancels without deleting and preserves the index

*For any* `Model` in `modeConfirm` at any `Selection_Index`, applying a `KeyConfirmNo` keypress (`n` or `N`) SHALL return the `Model` to `modeList` with the same `Selection_Index` and SHALL NOT invoke `deleter.Delete`.

**Validates: Requirements 5.4**

### Property 12: Non-OK Initial_Scan yields a non-zero exit code

*For any* non-OK `AccessStatus` returned by the Initial_Scan (while `sessionHasLoaded` is still false), `tui.Run` SHALL return a non-zero exit code without entering raw mode and without rendering the Folder_List; and for an Initial_Scan of `StatusOK` with at least one folder, a quit keypress SHALL cause `tui.Run` to return zero. This claim is scoped to the startup scan and does not govern an empty re-scan reached after a loaded session, which Property 13 covers.

**Validates: Requirements 8.1, 8.2, 8.3**

### Property 13: Post-deletion empty re-scan in a loaded session yields the all-clear state

*For any* `Model` with `sessionHasLoaded` true, when `applyDelete`'s re-scan returns an empty / `StatusAccessBlocked` result, the resulting `Model` SHALL be in `modeAllClear`, its rendered frame SHALL contain the All_Clear_Message and the Key_Legend and NO folder rows and SHALL NOT contain the `ui.AccessMessage` guidance, and the loop SHALL still return `0` on a subsequent quit keypress. Conversely, when `sessionHasLoaded` is false the same empty result SHALL remain `modeAccess`.

**Validates: Requirements 6.4, 8.4, 9.2, 9.3, 9.4, 9.7**

### Property 14: The confirmation prompt is the last line, with the legend above it

*For any* `Model` in `modeConfirm` over a non-empty Folder_List, the rendered frame SHALL contain the `confirmLegend` on a line that precedes the `Proceed? [y/N]:` prompt, and the prompt SHALL be the final content of the frame with no trailing line ending after it, so the operator's keypress appears directly after the prompt.

**Validates: Requirements 5.6, 5.7**

## Testing Strategy

### Dual approach

- **Property tests** drive the pure functions (`classify`, `newModel`, `moveUp`/`moveDown`, `handleKey`, `render`, `decodeKey`, `applyDelete`, `rebuildAfterDelete`) with generated inputs — random folder sets, indices, key sequences, access statuses, and prior `sessionHasLoaded` values — asserting the fourteen properties above. Each property test runs a minimum of 100 iterations and is tagged `Feature: interactive-tui-mode, Property N: <text>`.
- **Example and edge-case tests** cover: the `list`/`delete` dispatch examples (Req 1.2, 1.3), scan-before-render sequencing (Req 2.1), initial index at top (Req 3.1), access-status rendering content for each status (Req 8.1, 8.2), deletion-failure handling via a FileSystem whose `RemoveAll` errors (Req 5.5), terminal restore on both normal quit and an error/panic path via a `FakeTerminal` that records `MakeRaw`/restore calls (Req 7.2), that `handleKey` in `modeAllClear` ignores non-quit keys and returns `actQuit` for quit (Req 9.5, 9.6), and an end-to-end loop that scans a non-empty list, deletes the last remaining folder, reaches `modeAllClear`, and quits with exit code 0 (Req 8.4, 9.7).
- **Smoke check**: confirm the module stays stdlib-only (no third-party `require` lines in `go.mod`) so Req 1.4 holds for the build.

### Fakes and dependency injection

Tests use the existing `fsx.NewFakeFS(t.TempDir())` to seed a Safari-shaped tree (hashed child folders under the container suffix) and a new `tui.FakeTerminal` that:

- records whether `MakeRaw` was called and whether its returned restore ran,
- replays a scripted `[]Key` from `ReadKey`,
- captures all `Write` output into a buffer for frame assertions.

This lets the entire `tui.Run` loop be driven end-to-end deterministically — scan, navigate, confirm, delete, re-scan, quit — without a real terminal or the live Safari container, matching the project's established `fsx.Fake` testing pattern.

### What is not property-tested

Raw `termios` control and the real `osTerminal` byte I/O (`MakeRaw`, `ReadKey` syscalls) are not property-tested: they wrap non-portable syscalls whose behavior does not vary with generated input. They are exercised structurally through the `FakeTerminal` contract (restore runs on every exit path) and validated manually on macOS. Likewise `go.mod`'s stdlib-only constraint is a one-time build check, not a property.
