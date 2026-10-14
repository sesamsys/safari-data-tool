# Implementation Plan: Interactive TUI Mode

## Overview

This plan builds the interactive TUI from the inside out, following the design's three-layer split (pure model, pure renderer, thin terminal driver) and the project's established `fsx.FileSystem` + `fsx.Fake` testing pattern.

Work proceeds in dependency order so every step compiles and is testable the moment it lands:

1. Create the `internal/tui` package skeleton with its core types (`Model`, `mode`, `Key`, `action`, the `Terminal` interface) so later files have something to reference.
2. Add the pure key decoder and the `FakeTerminal` test driver.
3. Build the pure `Model` and its transitions (`newModel`, `moveUp`/`moveDown`, `handleKey`), then the impure `applyDelete` that re-scans.
4. Build the pure renderer.
5. Add the real `termios`-backed `osTerminal` and the `Run` event loop that wires everything together.
6. Wire the TUI into the `cli` package via the pure `classify` dispatch helper and the new `runTUI` entry point.

Property tests (minimum 100 iterations each, tagged `Feature: interactive-tui-mode, Property N: <text>`) are written as close to the code they validate as possible; example, edge-case, and the stdlib-only smoke check round out coverage. Property tests reuse `fsx.NewFakeFS(t.TempDir())` for Safari-shaped trees and the new `tui.FakeTerminal` for the loop.

## Tasks

- [x] 1. Create the `internal/tui` package skeleton and core types
  - Create `internal/tui/model.go` with the package declaration and the state/transition type declarations only: `mode` (`modeList`, `modeConfirm`, `modeAccess`, `modeAllClear`), `Key` (`KeyNone`, `KeyUp`, `KeyDown`, `KeyDelete`, `KeyConfirmYes`, `KeyConfirmNo`, `KeyQuit`, `KeyOther`), `action` (`actNone`, `actDelete`, `actQuit`), the `Model` struct (`folders`, `index`, `mode`, `status`, `errMsg`, `quit`, `sessionHasLoaded`), and the `listingLimit = 15` constant
  - Add `modeAllClear` to the `mode` enum and the `sessionHasLoaded bool` field to the `Model` struct so later all-clear logic has the state to reference (Req 9.1)
  - Create `internal/tui/terminal.go` with the `Terminal` interface (`MakeRaw() (func() error, error)`, `ReadKey() (Key, error)`, `Write([]byte) (int, error)`)
  - Add required imports (`github.com/sesamsys/safari-data-tool/internal/model`) and ensure the package compiles with `go build ./...`
  - _Requirements: 1.4, 2.3, 9.1_

- [x] 2. Implement pure key decoding
  - [x] 2.1 Implement `decodeKey([]byte) Key` in `internal/tui/keys.go`
    - Map single bytes `d`/`D` → `KeyDelete`, `y`/`Y` → `KeyConfirmYes`, `n`/`N` → `KeyConfirmNo`, `q`/`Q` → `KeyQuit`
    - Map the three-byte escape sequences `ESC [ A` → `KeyUp` and `ESC [ B` → `KeyDown`
    - Return `KeyOther` for anything else and `KeyNone` for an empty buffer
    - Keep it pure and table-driven so it is unit-testable directly
    - _Requirements: 3.2, 3.3, 5.1, 5.3, 5.4, 7.1_

  - [x] 2.2 Write unit/example tests for `decodeKey`
    - Cover both cases of each command key (`d`/`D`, `y`/`Y`, `n`/`N`, `q`/`Q`), both arrow sequences, and an unrecognized byte mapping to `KeyOther`
    - _Requirements: 3.2, 3.3, 5.1, 5.3, 5.4, 7.1_

- [x] 3. Implement the `FakeTerminal` test driver
  - Create `internal/tui/fake.go` (a non-`_test.go` file so it is importable like `fsx.FakeFS`) with a `FakeTerminal` that: records whether `MakeRaw` was called and whether its returned `restore` ran, replays a scripted `[]Key` from successive `ReadKey` calls, and captures all `Write` output into a buffer for frame assertions
  - Add a compile-time assertion that `*FakeTerminal` satisfies `tui.Terminal`
  - Expose fields/accessors the tests need (recorded calls, captured buffer, script cursor) and define `ReadKey` behavior when the script is exhausted (e.g. return an error so the loop terminates deterministically)
  - _Requirements: 1.4, 7.2_

- [x] 4. Implement the pure Model and its transitions
  - [x] 4.1 Implement `newModel(folders []model.ScannedFolder, status model.AccessStatus) Model` in `internal/tui/model.go`
    - For a non-OK status, return `Model{mode: modeAccess, status: status}` with no folders (Req 8.1, 8.2)
    - For `StatusOK`, set `folders` to `selection.TopN(folders, listingLimit)`, `index` to 0, and `mode` to `modeList` (Req 2.2, 2.3, 2.4, 3.1)
    - Set `sessionHasLoaded = len(top) > 0` so a non-empty `StatusOK` scan marks the session as loaded (Req 2.7, 9.1)
    - _Requirements: 2.2, 2.3, 2.4, 2.7, 3.1, 8.1, 8.2, 9.1_

  - [x] 4.2 Implement `moveUp` and `moveDown` on `Model`
    - `moveDown` sets `index = (index + 1) % n`; `moveUp` sets `index = (index + n - 1) % n`; both are no-ops on an empty list
    - _Requirements: 3.2, 3.3, 3.4, 3.5, 3.6_

  - [x] 4.3 Implement `handleKey(k Key) (Model, action)` on `Model`
    - In `modeList`: `KeyUp`→`moveUp`, `KeyDown`→`moveDown`, `KeyDelete`→enter `modeConfirm` only when the list is non-empty (Req 5.1), `KeyQuit`→set `quit` and return `actQuit` (Req 7.1)
    - In `modeConfirm`: `KeyConfirmYes`→return `actDelete` (Req 5.3), `KeyConfirmNo`→return to `modeList` with `index` unchanged (Req 5.4)
    - In `modeAllClear`: `KeyQuit`→set `quit` and return `actQuit`, every other key is ignored returning `actNone` (Req 9.5, 9.6)
    - All other keys/modes are no-ops returning `actNone`
    - _Requirements: 5.1, 5.3, 5.4, 7.1, 9.5, 9.6_

  - [x] 4.4 Write property test for the folder list shape
    - **Property 2: Folder list is always the descending top-15 of the scan** — for any folder set with `StatusOK`, `newModel(...).folders` equals `selection.TopN(folders, 15)`
    - **Validates: Requirements 2.2, 2.3, 2.4, 6.1, 6.2**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 2`

  - [x] 4.5 Write property test for wrap-around navigation
    - **Property 5: Navigation wraps around and is modular** — for any non-empty list of length `n` and index `i` in `[0, n)`, `moveDown` yields `(i+1) mod n` and `moveUp` yields `(i+n-1) mod n`
    - **Validates: Requirements 3.2, 3.3, 3.4, 3.5**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 5`

  - [x] 4.6 Write property test for selection index staying in range
    - **Property 6: Selection index stays in range** — for any sequence of navigation keypresses on a non-empty list, `0 <= index < len(folders)` always holds
    - **Validates: Requirements 3.1, 3.6, 6.3**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 6`

  - [x] 4.7 Write property test for `d`/`D` opening confirmation
    - **Property 8: `d`/`D` opens confirmation for the selected item** — for any `Model` in `modeList` with a non-empty list, `handleKey(KeyDelete)` yields a `Model` in `modeConfirm` targeting the folder at the current index
    - **Validates: Requirements 5.1**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 8`

  - [x] 4.8 Write property test for `n`/`N` cancelling without deleting
    - **Property 11: `n`/`N` cancels without deleting and preserves the index** — for any `modeConfirm` `Model` at any index, `handleKey(KeyConfirmNo)` returns to `modeList` with the same index and reports `actNone` (never `actDelete`)
    - **Validates: Requirements 5.4**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 11`

  - [x] 4.9 Write example test for the initial selection index
    - Assert `newModel` with a non-empty `StatusOK` scan sets `index` to 0 (topmost folder)
    - _Requirements: 3.1_

- [x] 5. Implement the impure delete-and-rebuild transition
  - [x] 5.1 Implement `applyDelete(m Model, f fsx.FileSystem, base string) Model` and the pure `rebuildAfterDelete(prev Model, folders []model.ScannedFolder, status model.AccessStatus) Model` helper in `internal/tui/model.go`
    - In `applyDelete`, call `deleter.Delete(f, base, m.folders[m.index].AbsPath)`; on failure, set `errMsg`, return to `modeList`, leave the folder set unchanged (Req 5.5)
    - On success, re-scan with `scanner.Scan` and delegate the rebuild to `rebuildAfterDelete`
    - In `rebuildAfterDelete`, rebuild via `newModel`, then OR the prior `sessionHasLoaded` into the rebuilt model so it is sticky across the rebuild (Req 9.1); when the rebuilt model is `modeList` with folders, clamp the previous index into range (Req 6.1, 6.2, 6.3)
    - When the re-scan yields no folders (an empty / `StatusAccessBlocked` result) and `sessionHasLoaded` is true, promote the model to `modeAllClear` instead of `modeAccess` (Req 6.4, 8.4, 9.2); when `sessionHasLoaded` is false, leave it as `modeAccess`
    - Add a small `clamp(v, lo, hi int) int` helper
    - _Requirements: 5.3, 5.5, 6.1, 6.2, 6.3, 6.4, 8.4, 9.2_

  - [x] 5.2 Write example test for deletion-failure handling
    - Use a `fsx.FileSystem` whose `RemoveAll` returns an error; assert `applyDelete` records the error, returns to `modeList`, and leaves the folder set unchanged
    - _Requirements: 5.5_

  - [x] 5.3 Write example test for re-scan after successful deletion
    - Seed a Safari-shaped tree with `fsx.NewFakeFS(t.TempDir())`, delete a folder via `applyDelete`, and assert the rebuilt list reflects the removed folder, applies the top-15 descending ordering, and has an in-range index
    - _Requirements: 6.1, 6.2, 6.3_

  - [x] 5.4 Write example test for the all-clear disambiguation in `rebuildAfterDelete`
    - For an empty / `StatusAccessBlocked` re-scan with a prior model whose `sessionHasLoaded` is true, assert the rebuilt model is in `modeAllClear`; with `sessionHasLoaded` false, assert it stays in `modeAccess`
    - _Requirements: 6.4, 8.4, 9.2_

- [x] 6. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Implement the pure renderer
  - [x] 7.1 Implement `render(m Model) []byte` and `renderConfirm(folder model.ScannedFolder) string` in `internal/tui/render.go`
    - Define the escape/legend constants (`escClear`, `escInvert`, `escReset`, `legend`, `allClearMsg`)
    - Start every frame with `escClear`; in `modeAccess` emit `ui.AccessMessage(m.status)` only, no folder rows (Req 8.1, 8.2)
    - In `modeAllClear` emit the `allClearMsg` (All_Clear_Message) only, no folder rows and no `ui.AccessMessage` guidance (Req 9.3)
    - Otherwise render each folder via `ui.FormatLine`, wrapping only the row at `index` in `escInvert … escReset` (Req 2.5, 4.1, 4.2); append the confirmation block in `modeConfirm` (Req 5.2) and any `errMsg`; append the legend on every frame, including the all-clear frame (Req 2.6, 9.4)
    - `renderConfirm` shows Domain, Hash, and `ui.FormatSize(SizeKB)` matching `ui.Confirm`'s detail set
    - _Note: superseded by task 13 — the single `legend` constant was later split into `listLegend`/`confirmLegend` and line endings switched to CRLF._
    - _Requirements: 2.5, 2.6, 4.1, 4.2, 5.2, 8.1, 8.2, 9.3, 9.4_

  - [x] 7.2 Write property test for row format reuse
    - **Property 3: Rendered rows reuse the existing line format** — for any folder in a displayed list, the frame contains that folder's `ui.FormatLine(folder)` text
    - **Validates: Requirements 2.5**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 3`

  - [x] 7.3 Write property test for the always-present legend
    - **Property 4: The legend is present on every frame** — for any `Model`, `render` output contains the legend text regardless of mode (list, confirm, access, or all-clear)
    - _Note: refined by task 13.2 to assert the context-appropriate legend (`confirmLegend` in confirm mode, `listLegend` otherwise)._
    - **Validates: Requirements 2.6, 9.4**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 4`

  - [x] 7.4 Write property test for the single inverted row
    - **Property 7: Exactly one inverted row, at the selection index** — for any `modeList`/`modeConfirm` `Model` with a non-empty list, the frame wraps exactly one row in reverse video, and it is the row at `index`
    - **Validates: Requirements 4.1, 4.2**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 7`

  - [x] 7.5 Write property test for the confirmation detail set
    - **Property 9: Confirmation shows Domain, Hash, and Size** — for any folder, the confirmation frame contains its `Domain`, its `Hash`, and `ui.FormatSize(SizeKB)`
    - **Validates: Requirements 5.2**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 9`

  - [x] 7.6 Write example tests for access-status rendering content
    - Assert the `modeAccess` frame contains `ui.AccessMessage(model.StatusAccessBlocked)` and `ui.AccessMessage(model.StatusNotFound)` respectively, and renders no folder rows
    - _Requirements: 8.1, 8.2_

- [x] 8. Implement the real terminal driver
  - [x] 8.1 Implement `osTerminal` and `NewOSTerminal()` in `internal/tui/terminal.go`
    - Capture the current `termios` via a `TIOCGETA` `ioctl`, build a raw variant (clear `ECHO`, `ICANON`, `ISIG`, `IEXTEN`, input/output post-processing flags; set `VMIN=1`, `VTIME=0`), and apply it with `TIOCSETA`; `restore` re-applies the captured original (Req 7.2)
    - Implement `ReadKey` by reading one byte and, on `ESC` (0x1B), reading up to two more to complete an arrow sequence, then delegating to `decodeKey`
    - Implement `Write` as a passthrough to stdout; issue all `ioctl`s through `syscall.Syscall(syscall.SYS_IOCTL, ...)` with an `unsafe.Pointer` to a Darwin-ABI `termios` struct — standard library only (Req 1.4)
    - Add a compile-time assertion that `*osTerminal` satisfies `tui.Terminal`
    - _Requirements: 1.4, 7.2_

- [x] 9. Implement the event loop and wire the layers together
  - [x] 9.1 Implement `Run(term Terminal, f fsx.FileSystem, base string, stdout, stderr io.Writer) int` in `internal/tui/run.go`
    - Scan before rendering (Req 2.1); on scan error print to stderr and return 1
    - On a non-OK status, print `ui.AccessMessage(status)` to stderr and return 1 without entering raw mode (Req 8.1, 8.2, 8.3)
    - Call `term.MakeRaw`, install `restore` via `defer` so it runs on quit, error, or panic (Req 7.2)
    - Loop: `term.Write(render(m))`, `term.ReadKey`, `m, act = m.handleKey(key)`; on `actQuit` return 0 (Req 7.1); on `actDelete` call `applyDelete`
    - _Requirements: 2.1, 7.1, 7.2, 8.1, 8.2, 8.3_

  - [x] 9.2 Write property test for the access-status exit code
    - **Property 12: Non-OK access status yields a non-zero exit code** — for any non-OK `AccessStatus`, `Run` returns non-zero without rendering the list; for `StatusOK` with at least one folder, a scripted quit keypress makes `Run` return 0
    - **Validates: Requirements 8.1, 8.2, 8.3**
    - Drive via `FakeTerminal` + `fsx.NewFakeFS`; minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 12`

  - [x] 9.3 Write property test for `y`/`Y` deleting the selected path
    - **Property 10: `y`/`Y` deletes the selected folder's path** — for any `modeConfirm` `Model` over a non-empty list, driving `KeyConfirmYes` through the loop invokes `deleter.Delete` with the selected folder's `AbsPath`, and that path no longer exists afterward
    - **Validates: Requirements 5.3**
    - Seed with `fsx.NewFakeFS(t.TempDir())`; minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 10`

  - [x] 9.4 Write example tests for scan-before-render and terminal restore
    - Assert the scan happens before any `Write` (e.g. a non-OK status produces no frame output)
    - Using `FakeTerminal`, assert `restore` runs on a normal quit path and on an error/panic path (`ReadKey`/`Write` returning an error)
    - _Requirements: 2.1, 7.2_

  - [x] 9.5 Write property test for the post-deletion all-clear state
    - **Property 13: Post-deletion empty re-scan in a loaded session yields the all-clear state** — for any `Model` with `sessionHasLoaded` true whose post-deletion re-scan returns an empty / `StatusAccessBlocked` result, the resulting `Model` is in `modeAllClear`, its frame contains the All_Clear_Message and the legend with no folder rows and no `ui.AccessMessage` guidance, and a subsequent quit makes the loop return 0; when `sessionHasLoaded` is false the same empty result stays `modeAccess`
    - **Validates: Requirements 6.4, 8.4, 9.2, 9.3, 9.4, 9.7**
    - Drive via `FakeTerminal` + `fsx.NewFakeFS`; minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 13`

  - [x] 9.6 Write example tests for the all-clear loop and key handling
    - Seed a non-empty list with `fsx.NewFakeFS(t.TempDir())`, delete the last remaining folder through the loop, assert the TUI reaches `modeAllClear` and that a quit keypress makes `Run` return 0 (Req 8.4, 9.7)
    - Assert `handleKey` in `modeAllClear` returns `actQuit` for `KeyQuit` and `actNone` (ignored) for every non-quit key (Req 9.5, 9.6)
    - _Requirements: 8.4, 9.5, 9.6, 9.7_

- [x] 10. Wire the TUI into the cli dispatch
  - [x] 10.1 Add the pure `classify` helper and `command` type to `internal/cli/cli.go`
    - Define `command` (`cmdTUI`, `cmdList`, `cmdDelete`) and `classify(args []string) command`: a first argument of `list`/`delete` selects that subcommand, anything else (including none) selects `cmdTUI`
    - _Requirements: 1.1, 1.2, 1.3_

  - [x] 10.2 Add `runTUI` and switch `run` over `classify`
    - Implement `runTUI(stdout, stderr io.Writer, f fsx.FileSystem, home string) int` that constructs `tui.NewOSTerminal()` and delegates to `tui.Run(term, f, scanner.BasePath(home), stdout, stderr)`, returning 1 on terminal-construction error (Req 8.3)
    - Change `run` to switch on `classify(args)`: `cmdList`→`runList`, `cmdDelete`→`runDelete`, `cmdTUI`→`runTUI`, preserving the existing `runList`/`runDelete` signatures and behavior
    - _Requirements: 1.1, 1.2, 1.3_

  - [x] 10.3 Write property test for dispatch classification
    - **Property 1: Bare invocation routes to the TUI** — for any arg slice whose first argument is neither `list` nor `delete` (including none), `classify` returns `cmdTUI`; `list`/`delete` return `cmdList`/`cmdDelete`
    - **Validates: Requirements 1.1, 1.2, 1.3**
    - Minimum 100 iterations; tagged `Feature: interactive-tui-mode, Property 1`

  - [x] 10.4 Write example tests for the `list`/`delete` dispatch preservation
    - Assert `run` with `list` and with `delete` still produces the existing non-interactive behavior and does not launch the TUI
    - _Requirements: 1.2, 1.3_

- [x] 11. Add the stdlib-only smoke check
  - [x] 11.1 Write a test asserting the module stays standard-library only
    - Parse `go.mod` and assert there are no third-party `require` lines, so Req 1.4 holds for the build
    - _Requirements: 1.4_

- [x] 12. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 13. Post-implementation refinements (from live-terminal testing)
  - [x] 13.1 Emit CRLF line endings so rows render correctly under raw mode
    - Raw mode clears output post-processing (`OPOST`/`ONLCR`), so a bare `\n` moved the cursor down without returning to column 0 and rows stair-stepped across the screen
    - Introduce a `crlf = "\r\n"` constant in `render.go` and end every rendered line with it (including `renderConfirm`), keeping `render` self-contained rather than relying on terminal newline translation
    - Verified on the built binary in a live terminal
    - _Requirements: 2.5, 2.6_

  - [x] 13.2 Make the Key_Legend context-sensitive and fix its placement on the confirmation screen
    - On the confirmation screen the always-present legend landed on the same line as the `Proceed? [y/N]:` prompt
    - Split the single `legend` constant into `listLegend` (`↑/↓ move   d delete   q quit`) and `confirmLegend` (`y confirm   n cancel`); in `modeConfirm` render `confirmLegend` above the confirmation block and keep the `Proceed? [y/N]:` prompt as the final line (no trailing newline) so the cursor rests after it; keep `listLegend` as the final line in list, access, and all-clear modes
    - Update the Property 4 test for the context-appropriate legend and add a Property 14 test (`render_prop14_test.go`) asserting the confirm legend sits above the prompt and the prompt is the last line
    - Verified on the built binary in a live terminal
    - _Requirements: 2.6, 5.2, 5.6, 5.7_

## Notes

- Tasks marked with `*` are optional and can be skipped for a faster MVP; they are all test tasks (property, unit/example, edge-case, and the stdlib smoke check).
- Each property test runs a minimum of 100 iterations and is tagged `Feature: interactive-tui-mode, Property N: <text>`, driving the pure functions directly with generated inputs.
- Property-based tests validate the fourteen universal correctness properties (Property 14 was added during the post-implementation legend refinement); example and edge-case tests cover specific requirements (dispatch examples, scan-before-render, initial index, access-status content, deletion-failure handling, all-clear disambiguation, terminal restore, the all-clear loop and key handling).
- Tests reuse the existing `fsx.NewFakeFS(t.TempDir())` for Safari-shaped trees and the new `tui.FakeTerminal` for end-to-end loop runs, matching the project's established fake-based testing pattern.
- Raw `termios` control and the real `osTerminal` byte I/O are not property-tested (non-portable syscalls); they are exercised structurally through the `FakeTerminal` contract and validated manually on macOS.
- Checkpoints ensure incremental validation at natural breaks.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1"] },
    { "id": 1, "tasks": ["2.1", "3"] },
    { "id": 2, "tasks": ["2.2", "4.1", "4.2", "4.3"] },
    { "id": 3, "tasks": ["4.4", "4.5", "4.6", "4.7", "4.8", "4.9", "5.1", "7.1", "8.1", "10.1"] },
    { "id": 4, "tasks": ["5.2", "5.3", "5.4", "7.2", "7.3", "7.4", "7.5", "7.6", "9.1", "10.2", "11.1"] },
    { "id": 5, "tasks": ["9.2", "9.3", "9.4", "9.5", "9.6", "10.3", "10.4"] }
  ]
}
```
