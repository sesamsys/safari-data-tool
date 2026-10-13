# Implementation Plan: Safari Data Tool

## Overview

Build the tool bottom-up in Go (pure-Go, no CGO) following the design's package layout. Start with the filesystem abstraction and data model so every higher layer can be unit-tested against temp-dir/fake implementations without touching the TCC-protected Safari container. Then implement the pure logic cores (origin parser, sizer, deleter, scanner, ui, selection) each with its own tests, and finally wire everything together through the `cli` dispatcher and `main`. Documentation closes it out.

Each task builds on the previous ones and ends by integrating into a layer already in place, so no code is left orphaned.

## Tasks

- [x] 1. Scaffold the project and filesystem abstraction
  - [x] 1.1 Initialize the module and entry point
    - Create `go.mod` with module path `github.com/<owner>/safari-data-tool` and a current Go version; no third-party dependencies for the core
    - Create a thin `main.go` that calls `cli.Run(os.Args)` and exits with its returned code (stub `cli.Run` to return 0 for now so it compiles)
    - Create the `internal/` package directory skeleton (`cli`, `fsx`, `model`, `scanner`, `originparse`, `sizer`, `deleter`, `ui`)
    - _Requirements: 7.2_

  - [x] 1.2 Define the FileSystem interface and osFS implementation
    - In `internal/fsx/fsx.go` define the `FileSystem` interface (`Stat`, `ReadDir`, `Open`, `WalkDir`, `RemoveAll`) and the production `osFS` implementation delegating to `os`/`filepath`
    - _Requirements: 7.1, 7.2_

  - [x] 1.3 Add the test/fake filesystem implementation
    - In `internal/fsx/fake.go` provide a temp-dir-backed or in-memory implementation usable by all package tests so the live container is never read in tests
    - _Requirements: 7.2_

- [x] 2. Define the data model
  - [x] 2.1 Implement model types
    - In `internal/model/model.go` define `ScannedFolder` (Hash, AbsPath, SizeKB, Domain, Hosts) and `AccessStatus` with constants `StatusOK`, `StatusNotFound`, `StatusAccessBlocked`
    - _Requirements: 1.3, 6.1, 6.3_

- [x] 3. Implement the origin-file parser
  - [x] 3.1 Implement record parsing and domain resolution
    - In `internal/originparse/originparse.go` implement `ParseRecords` (read `[uint32 LE length][1 flag byte][N UTF-8 bytes]` repeatedly, stop + return `ErrMalformed` on truncation, guard `N` against absurd sizes, never panic), `ResolveDomain` (host of first scheme/host pair = text of second record, else `UnknownDomain`), `FindOriginFile` (walk the folder tree via `fsx.FileSystem` for the first file named `origin`), and `Resolve` (find + read + decode, any failure yields `UnknownDomain`)
    - Define the `UnknownDomain` constant and the `ErrMalformed` sentinel
    - _Requirements: 2.1, 2.2, 2.3, 2.4_

  - [x] 3.2 Write table-driven unit tests for the parser
    - Include the real `unifi.ui.com` hex fixture (`[05 00 00 00][01]"https"[0B 00 00 00][01]"unifi.ui.com"`) asserting resolution to `unifi.ui.com`
    - Add malformed/truncated cases (fewer than two records, truncated length-prefixed record, empty input) asserting `Unknown Domain` and no panic
    - Add a `FindOriginFile`/`Resolve` case over a fake tree with a nested `origin` file
    - _Requirements: 2.1, 2.4_

  - [x] 3.3 Write property test for origin record round-trip
    - **Property 4: Origin record round-trip**
    - **Validates: Requirements 2.2**

  - [x] 3.4 Write property test for first-pair host resolution
    - **Property 5: First-pair host resolution**
    - **Validates: Requirements 2.1, 2.3**

  - [x] 3.5 Write property test for malformed origin robustness
    - **Property 6: Malformed origin robustness**
    - **Validates: Requirements 2.4**

- [x] 4. Implement the recursive size calculator
  - [x] 4.1 Implement SizeKB
    - In `internal/sizer/sizer.go` implement `SizeKB(f fsx.FileSystem, root string) (int64, error)` using `f.WalkDir`, summing regular-file sizes with `du -sk` KB-block semantics (ceiling-per-file 1024 rounding), not following symlinks
    - _Requirements: 3.1_

  - [x] 4.2 Write property test for recursive size additivity
    - **Property 7: Recursive size is additive**
    - **Validates: Requirements 3.1**
    - Build temp trees with random files/sizes; assert the total equals an independently summed total and that subtree sizes sum to the whole

- [x] 5. Implement the deleter (matching, path-safety, removal)
  - [x] 5.1 Implement Match
    - In `internal/deleter/deleter.go` define `Target{Domain, Hash}` and implement `Match` (both fields when both given, single field otherwise; domain comparison case-insensitive; `ErrNoMatch` when nothing matches)
    - _Requirements: 4.1, 4.2, 4.3_

  - [x] 5.2 Implement ValidatePath and Delete
    - Implement `ValidatePath(base, target)`: clean + absolutize both, reject when target equals base, reject when `filepath.Dir(clean(target)) != clean(base)` (rejects nested descendants and `..` escapes), return `ErrPathUnsafe` otherwise
    - Implement `Delete(f, base, target)`: re-run `ValidatePath` as a defensive final gate, then `f.RemoveAll(target)`
    - _Requirements: 4.6, 5.1, 5.2, 5.3_

  - [x] 5.3 Write property test for deletion target matching
    - **Property 9: Deletion target matching**
    - **Validates: Requirements 4.1, 4.2, 4.3**

  - [x] 5.4 Write property test for path-safety direct-child requirement
    - **Property 10: Path-safety requires a direct child of base**
    - **Validates: Requirements 5.1, 5.2, 5.3**
    - Generate direct children (accept), the base itself, nested descendants, and `..`/traversal paths (reject)

  - [x] 5.5 Write unit test for Delete against a fake base
    - Delete a temp folder under a fake base and assert it is gone; assert an outside-base target is refused before removal
    - _Requirements: 4.6, 5.2_

- [x] 6. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Implement the scanner
  - [x] 7.1 Implement BasePath, Classify, and Scan
    - In `internal/scanner/scanner.go` implement `BasePath(home)` as `filepath.Join(home, containerSuffix)` with the fixed macOS container suffix
    - In `internal/scanner/access.go` implement `Classify(baseExists, childCount)` returning `StatusNotFound` when the base does not exist, `StatusAccessBlocked` when it exists with zero readable children, else `StatusOK`
    - Implement `Scan(f, base)` to enumerate direct child directories, call the sizer and the origin resolver for each, and return the folders plus an `AccessStatus`; skip unreadable folders and continue
    - _Requirements: 2.1, 3.1, 6.1, 6.3, 7.1_

  - [x] 7.2 Write property test for access-status classification
    - **Property 11: Access-status classification**
    - **Validates: Requirements 6.1, 6.3**

  - [x] 7.3 Write property test for base-path derivation
    - **Property 12: Base-path derivation**
    - **Validates: Requirements 7.1**

  - [x] 7.4 Write unit test for Scan over a fake tree
    - Build a fake base with several hashed folders (some with origin files, one unreadable) and assert folders are sized, resolved, and the unreadable one is skipped
    - _Requirements: 2.1, 3.1_

- [x] 8. Implement the ui layer (formatting, confirmation, messaging)
  - [x] 8.1 Implement FormatSize and FormatLine
    - In `internal/ui/ui.go` implement `FormatSize(kb)` using 1024-based units (`B/KB/MB/GB/TB`, largest unit under which value is `>= 1`, no space, integer-or-one-decimal magnitude) and `FormatLine(folder)` producing `fmt.Sprintf("%7s  %-30s  (%s)", size, domain, hash)`
    - _Requirements: 1.3, 3.2_

  - [x] 8.2 Implement Confirm and AccessMessage
    - Implement `Confirm(w, r, folder)` displaying domain, hash, and size and returning true only on an affirmative answer
    - Implement `AccessMessage(status)`: for `StatusAccessBlocked` name Full Disk Access and `System Settings > Privacy & Security > Full Disk Access`; for `StatusNotFound` state the Safari website-data directory was not found
    - _Requirements: 4.4, 4.5, 6.1, 6.2, 6.3_

  - [x] 8.3 Write property test for listing line fields
    - **Property 3: Listing line contains required fields**
    - **Validates: Requirements 1.3**

  - [x] 8.4 Write property test for human-readable size formatting
    - **Property 8: Human-readable size formatting**
    - **Validates: Requirements 3.2**

  - [x] 8.5 Write unit tests for Confirm and AccessMessage
    - Use a mocked `io.Reader` for `Confirm` (yes / no answers); assert `AccessMessage` text for each non-OK status
    - _Requirements: 4.4, 4.5, 6.2_

- [x] 9. Implement selection and the CLI wiring
  - [x] 9.1 Implement selection helpers
    - Implement `TopN(folders, count)` (sort by descending `SizeKB`, return first `min(count, len)`) and `ValidateCount(count)` (reject non-positive) in `internal/cli` or a small `internal/selection` package
    - _Requirements: 1.1, 1.2, 1.4, 1.5_

  - [x] 9.2 Write property test for Top-N selection
    - **Property 1: Top-N selection**
    - **Validates: Requirements 1.1, 1.2, 1.4**

  - [x] 9.3 Write property test for count validation
    - **Property 2: Count validation rejects non-positive values**
    - **Validates: Requirements 1.5**

  - [x] 9.4 Implement cli.Run dispatch and the list flow
    - In `internal/cli/cli.go` implement `Run(args)`: inspect `args[1]`; route `delete` to the delete flow, else default to listing
    - List flow: `-n COUNT` flag (default 15) via a `flag.FlagSet`, `ValidateCount`, scan via `fsx.osFS`, on non-OK status print only `AccessMessage` and exit non-zero, else `TopN` + `FormatLine` printed to stdout
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 6.1, 6.2, 6.3_

  - [x] 9.5 Implement the delete flow and finalize main wiring
    - Delete flow: `--domain`, `--hash`, `--force`/`--yes` flags via a `flag.FlagSet`; require at least one of domain/hash (else usage error, exit non-zero); scan, `Match` (no match -> message + exit non-zero), `ValidatePath`, `Confirm` unless force (decline -> exit zero without deleting), then `Delete`
    - Confirm the real `main.go` passes `os.Args` to `cli.Run` and propagates the exit code
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 5.1, 5.2, 5.3_

  - [x] 9.6 Write unit tests for cli dispatch and flows
    - Assert `list`/empty/flag args route to listing and `delete` routes to the delete flow; assert `-n` non-positive and `delete` with neither flag produce input errors and non-zero exits; drive the delete flow against a fake filesystem for match / no-match / force / declined paths
    - _Requirements: 1.5, 4.3, 4.5, 4.7_

- [x] 10. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Documentation and distribution
  - [x] 11.1 Write the README and LICENSE
    - Document build & install (`CGO_ENABLED=0 go build -o safari-data-tool .`, `go install .../safari-data-tool@latest`, macOS-only note), usage examples for `list`, `list -n 30`, `delete --domain ...`, `delete --hash ... --force`, the Full Disk Access requirement (with empty-listing symptom), and a prominent warning that deletion is permanent and irreversible; add a `LICENSE` file
    - _Requirements: 8.1, 8.2, 8.3, 8.4_

  - [x] 11.2 Add optional GoReleaser and GitHub Actions release config
    - **OPTIONAL.** Add `.goreleaser.yaml` (build matrix `darwin/amd64` + `darwin/arm64`, `CGO_ENABLED=0`, checksums) and `.github/workflows/release.yml` to publish cross-arch macOS binaries on tag push
    - _Requirements: 8.1_

## Notes

- Tasks marked with `*` are optional (test and optional-distribution subtasks) and can be skipped for a faster MVP.
- Each task references specific requirements for traceability; property-test subtasks additionally reference the design's correctness property.
- Checkpoints (tasks 6 and 10) ensure incremental validation before building the next layer.
- The core stays pure and dependency-free: all disk access goes through `fsx.FileSystem`, so parsing, sizing, matching, and path-safety are table/property-testable without the live Safari container.
- All 12 correctness properties from the design are covered by property-test subtasks (3.3-3.5, 4.2, 5.3-5.4, 7.2-7.3, 8.3-8.4, 9.2-9.3).

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "1.3", "2.1"] },
    { "id": 2, "tasks": ["3.1", "4.1", "5.1", "5.2", "8.1", "8.2", "9.1"] },
    { "id": 3, "tasks": ["3.2", "3.3", "3.4", "3.5", "4.2", "5.3", "5.4", "5.5", "7.1", "8.3", "8.4", "8.5", "9.2", "9.3"] },
    { "id": 4, "tasks": ["7.2", "7.3", "7.4", "9.4"] },
    { "id": 5, "tasks": ["9.5"] },
    { "id": 6, "tasks": ["9.6", "11.1"] },
    { "id": 7, "tasks": ["11.2"] }
  ]
}
```
