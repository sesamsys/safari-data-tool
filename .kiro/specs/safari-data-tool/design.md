# Design Document

## Overview

Safari Data Tool is a standalone, pure-Go CLI for macOS that inspects and manages Safari's WebKit WebsiteData storage. It lists the largest per-domain data folders under the Safari container, resolves each hashed folder to a readable domain by parsing WebKit `origin` files, reports recursive folder sizes, and permanently deletes a chosen domain's data by domain name or hash with a confirmation safeguard. It replaces an existing `safari-data` zsh function and ships from a public GitHub repository with build and usage docs.

The design favors a small, dependency-free core. All filesystem access is funneled through a narrow interface so the live, TCC-protected container (which cannot be read in unit tests) is mockable, while the parsing, sizing, selection, matching, and path-safety logic stays pure and table-testable.

**Platform scope:** macOS only, compiled without CGO (Requirements 7.1, 7.2). All code paths parse `origin` files with `encoding/binary` from the standard library — no SQLite driver, no CGO.

## Architecture

The tool is a single binary with two subcommands dispatched from `main`:

- `list` (default): scan, size, resolve, sort, print the top N (Requirement 1).
- `delete`: match a target by `--domain` and/or `--hash`, validate path-safety, confirm, remove (Requirements 4, 5).

Data flows one direction for listing:

```
main -> cli (parse flags, dispatch)
     -> scanner (enumerate hashed folders via FS interface)
        -> sizer   (recursive du -sk per folder)
        -> originparse (find + parse origin file -> domain)
     -> selection (sort desc, take top N)
     -> ui (format + print lines)
```

For deletion:

```
main -> cli -> scanner (enumerate + resolve) 
            -> deleter.Match (by domain/hash)
            -> deleter.ValidatePath (path-safety gate)
            -> ui.Confirm (unless --force/--yes)
            -> deleter.Delete (recursive remove via FS interface)
```

### Filesystem abstraction

Live reads of the Safari container require Full Disk Access and therefore cannot run in unit tests. All disk interaction goes through one interface so the core is testable against an in-memory or temp-dir implementation:

```go
// FileSystem abstracts the disk operations the tool performs so the
// TCC-protected live container can be mocked in tests.
type FileSystem interface {
    Stat(path string) (os.FileInfo, error)
    ReadDir(path string) ([]os.DirEntry, error)
    Open(path string) (io.ReadCloser, error)
    WalkDir(root string, fn fs.WalkDirFunc) error
    RemoveAll(path string) error
}
```

The production implementation (`osFS`) delegates to `os`/`filepath`. Tests use a temp-directory implementation or a fake. The `du`-equivalent sizer and the origin finder both operate through this interface, so size calc and origin discovery are exercised against real temp trees without touching the Safari container.

### CLI library choice

Use the standard library `flag` package with a tiny hand-rolled subcommand switch (inspect `os.Args[1]`; if it is not a known subcommand, treat the invocation as `list`). Rationale: two subcommands and a handful of flags do not justify a third-party CLI framework. Staying on stdlib keeps the core dependency-free (supporting Requirement 7.2's pure-Go goal and the minimal-dependency objective), simplifies the build, and keeps the binary small. Each subcommand gets its own `flag.FlagSet`.

## Project Structure

```
safari-data-tool/
├── go.mod                      # module path, Go version; no third-party deps for core
├── main.go                     # thin entry point: calls cli.Run(os.Args)
├── README.md                   # build, usage, FDA, deletion-permanence (Req 8)
├── LICENSE
├── internal/
│   ├── cli/
│   │   ├── cli.go              # Run(args), subcommand dispatch, flag parsing
│   │   └── cli_test.go
│   ├── fsx/
│   │   ├── fsx.go              # FileSystem interface + osFS impl
│   │   └── fake.go             # test/in-memory impl (build tag or plain file)
│   ├── model/
│   │   └── model.go            # ScannedFolder, AccessStatus
│   ├── scanner/
│   │   ├── scanner.go          # enumerate hashed folders, orchestrate size+resolve
│   │   ├── access.go           # Full Disk Access / not-found classification
│   │   ├── scanner_test.go
│   │   └── access_test.go
│   ├── originparse/
│   │   ├── originparse.go       # origin finder + binary record parser + resolver
│   │   └── originparse_test.go
│   ├── sizer/
│   │   ├── sizer.go            # recursive du -sk equivalent
│   │   └── sizer_test.go
│   ├── deleter/
│   │   ├── deleter.go          # Match, ValidatePath, Delete
│   │   └── deleter_test.go
│   └── ui/
│       ├── ui.go               # size formatter, line formatter, confirmation prompt, messages
│       └── ui_test.go
├── .github/
│   └── workflows/
│       └── release.yml         # OPTIONAL: GoReleaser release on tag
└── .goreleaser.yaml            # OPTIONAL: cross-arch macOS build config
```

`internal/` keeps everything unexported to external importers; this is a tool, not a library.

## Data Model

```go
package model

// ScannedFolder is one hashed folder discovered directly under the
// WebsiteData base path, enriched with its resolved domain and size.
type ScannedFolder struct {
    Hash     string // the hashed directory name (direct child of base)
    AbsPath  string // absolute path to the folder
    SizeKB   int64  // recursive size in 1024-byte blocks (du -sk semantics)
    Domain   string // resolved primary domain, or "Unknown Domain"
    Hosts    []string // optional: all hosts parsed from the origin file (partition hosts)
}

// AccessStatus classifies the state of the base path for messaging.
type AccessStatus int

const (
    StatusOK            AccessStatus = iota // base exists, >=1 readable folder
    StatusNotFound                          // base path does not exist (Req 6.3)
    StatusAccessBlocked                     // base exists but zero readable folders (Req 6.1)
)
```

`SizeKB` stores kilobytes (1024-byte blocks) to mirror `du -sk`; the human-readable formatter converts for display.

## Components

### 1. Folder scanner (`internal/scanner`)

Enumerates the direct children of the base path, computes each folder's size and resolved domain, and classifies access state.

```go
// BasePath derives the WebsiteData base path from a home directory using the
// fixed macOS Safari container layout (Req 7.1).
func BasePath(home string) string

const containerSuffix = "Library/Containers/com.apple.Safari/Data/Library/WebKit/WebsiteData/Default"

// Scan enumerates hashed folders directly under base, sizing and resolving each.
// It returns the folders and an AccessStatus describing the base-path state.
func Scan(f fsx.FileSystem, base string) ([]model.ScannedFolder, model.AccessStatus, error)

// Classify decides the access status from whether the base exists and how many
// readable child folders were found (Req 6.1, 6.3).
func Classify(baseExists bool, childCount int) model.AccessStatus
```

`BasePath` is `filepath.Join(home, containerSuffix)`. `Scan` lists direct children, keeps only directories, and for each calls the sizer and the origin resolver. `Classify` is pure and the unit of the access-status property.

### 2. Origin-file parser (`internal/originparse`)

Finds an `origin` file within a hashed folder tree and decodes it into the resolved domain.

#### Binary layout

An `origin` file is a sequence of **records**. Each record is:

```
offset  size  field
0       4     length N  (uint32, little-endian)
4       1     flag byte (observed value 0x01)
5       N     text      (N bytes of UTF-8)
```

Records appear in order and **pair up as (scheme, host)**: the first record is a scheme string (e.g. `http`, `https`), the second is a host string (e.g. `www.gstatic.com`). The **host of the first pair** is the `Primary_Domain` (Requirement 2.3). Additional pairs may describe partition/related hosts and are captured in `Hosts` but do not change the primary resolution.

Example (the real `unifi.ui.com` case), bytes shown as the records they decode to:

```
[05 00 00 00][01] "https"           <- scheme (first record)
[0B 00 00 00][01] "unifi.ui.com"    <- host   (first pair host -> Primary_Domain)
```

#### Parsing algorithm

```go
// Record is one decoded origin record.
type Record struct{ Text string }

// ParseRecords decodes the full record sequence from an origin file's bytes.
// It reads [uint32 LE length][1 flag byte][N text bytes] repeatedly until the
// input is exhausted. On a malformed or truncated record it stops and returns
// the records decoded so far plus ErrMalformed, never panicking (Req 2.2, 2.4).
func ParseRecords(data []byte) ([]Record, error)

// ResolveDomain returns the host of the first scheme/host pair: the text of the
// second record. Returns "Unknown Domain" when fewer than two records parse (Req 2.3, 2.4).
func ResolveDomain(data []byte) string

// FindOriginFile walks the hashed folder tree and returns the path of the first
// file named "origin" (Req 2.1), or "" if none is found.
func FindOriginFile(f fsx.FileSystem, folder string) string

// Resolve ties finding + reading + decoding together, returning the resolved
// domain and the full host list. Any failure yields "Unknown Domain" (Req 2.4).
func Resolve(f fsx.FileSystem, folder string) (domain string, hosts []string)

const UnknownDomain = "Unknown Domain"
```

Parsing detail and robustness:

1. Maintain a cursor at offset 0.
2. If fewer than 5 bytes remain, stop (end of data or trailing garbage).
3. Read `N` as `binary.LittleEndian.Uint32(data[cursor:cursor+4])`.
4. Skip the 1 flag byte (not validated beyond presence; recorded if ever needed).
5. If `cursor+5+N` exceeds `len(data)`, the record is truncated — stop and return `ErrMalformed` with records collected so far. Guard `N` against absurd values (e.g. `N > len(data)`) to avoid huge allocations.
6. Append the `N` text bytes as a `Record`, advance the cursor, repeat.
7. `ResolveDomain` returns `records[1].Text` when at least two records decoded; otherwise `UnknownDomain`.

Even/odd record counts are tolerated: an unpaired trailing scheme record is ignored for primary resolution. A completely unreadable or missing file resolves to `UnknownDomain` so one bad folder never aborts a listing.

### 3. Recursive size calculator (`internal/sizer`)

```go
// SizeKB returns the recursive size of root in 1024-byte blocks, equivalent to
// `du -sk root`: it sums file sizes across the whole tree and converts to KB
// using 1024-byte block semantics (Req 3.1).
func SizeKB(f fsx.FileSystem, root string) (int64, error)
```

Implementation: `f.WalkDir(root, ...)` summing `info.Size()` for regular files. Convert the byte total to KB blocks with ceiling-per-file 1024 rounding to approximate `du` block accounting; the primary guarantee the tests assert is additivity (size of a tree equals the sum of its subtree sizes). Symlinks are not followed (consistent with `du` default and avoiding cycles).

### 4. Domain/hash matcher for deletion (`internal/deleter`)

```go
// Target identifies a deletion request by domain and/or hash.
type Target struct {
    Domain string // from --domain; empty if not provided
    Hash   string // from --hash;   empty if not provided
}

// Match selects the folder whose Domain equals Target.Domain and/or whose Hash
// equals Target.Hash. Returns ErrNoMatch when nothing matches (Req 4.1, 4.2, 4.3).
func Match(folders []model.ScannedFolder, t Target) (model.ScannedFolder, error)
```

Matching rules: if both `--domain` and `--hash` are given, a folder must satisfy both; if only one is given, that field alone must match. Exact string equality (domain comparison case-insensitive on host). No match returns `ErrNoMatch` and the caller exits without deleting.

### 5. Path-safety validator (`internal/deleter`)

```go
// ValidatePath verifies that target resolves to an absolute path that is a
// DIRECT CHILD of base (Req 5.1, 5.2, 5.3). It cleans and absolutizes both
// paths and requires filepath.Dir(cleanAbs(target)) == cleanAbs(base).
// Returns ErrPathUnsafe otherwise.
func ValidatePath(base, target string) error
```

Algorithm: compute `b = filepath.Clean(absBase)` and `t = filepath.Clean(absTarget)`. Reject when `t == b` (the base itself), when `filepath.Dir(t) != b` (not a direct child — rejects both nested descendants and `..` escapes once cleaned), and therefore any traversal that resolves outside the base. This runs before any removal, independent of how the target was matched.

### 6. Deleter (`internal/deleter`)

```go
// Delete removes the selected folder recursively after the caller has matched,
// validated, and confirmed it (Req 4.6).
func Delete(f fsx.FileSystem, base, target string) error
```

`Delete` re-runs `ValidatePath(base, target)` as a defensive final gate, then calls `f.RemoveAll(target)`.

### 7. Confirmation prompt + messaging (`internal/ui`)

```go
// Confirm displays the target's domain, hash, and size and reads a yes/no
// answer from r. Returns true only on an affirmative response (Req 4.4, 4.5).
func Confirm(w io.Writer, r io.Reader, folder model.ScannedFolder) bool

// FormatSize renders a byte or KB value in human-readable units (Req 3.2).
func FormatSize(kb int64) string

// FormatLine renders one listing row (Req 1.3). See Output Format.
func FormatLine(folder model.ScannedFolder) string

// AccessMessage returns operator guidance for a non-OK AccessStatus
// (Req 6.1, 6.2, 6.3).
func AccessMessage(status model.AccessStatus) string
```

`Confirm` is bypassed entirely when `--force`/`--yes` is set, so stdin is never read on the force path (Req 4.7). `AccessMessage` for `StatusAccessBlocked` names Full Disk Access and the path *System Settings > Privacy & Security > Full Disk Access*; for `StatusNotFound` it states the Safari website-data directory was not found.

### 8. Selection (`internal/cli` helper or small `internal/selection`)

```go
// TopN sorts folders by descending SizeKB and returns the first min(count, len).
// count defaults to 15 at the call site when the flag is unset (Req 1.1, 1.2, 1.4).
func TopN(folders []model.ScannedFolder, count int) []model.ScannedFolder

// ValidateCount rejects non-positive counts (Req 1.5).
func ValidateCount(count int) error
```

## CLI Command and Flag Structure

```
safari-data-tool [list] [-n COUNT]
safari-data-tool delete [--domain DOMAIN] [--hash HASH] [--force|--yes]
```

- Default/`list` command
  - `-n COUNT` (int, default `15`): number of largest folders to show (Req 1.1, 1.2). Non-positive -> input error, exit non-zero (Req 1.5). A non-integer is rejected by `flag` parsing.
- `delete` command
  - `--domain string`: select by resolved domain (Req 4.1).
  - `--hash string`: select by hashed folder name (Req 4.2).
  - At least one of `--domain`/`--hash` is required; neither -> input error.
  - `--force` / `--yes` (bool): skip confirmation (Req 4.4, 4.7).

Dispatch: `cli.Run` inspects `args[1]`. `"delete"` routes to the delete flow; `"list"` or any flag/empty routes to listing (listing is the default).

## Output Format

Listing rows match the existing zsh function's style:

```
771MB  www.gstatic.com  (hash)
```

`FormatLine` produces three columns separated by two spaces:

1. **Size** — right-aligned in a fixed width of **7** characters (e.g. `771MB`, `  1.2GB`, `  512KB`). `FormatSize` emits an integer-or-one-decimal magnitude plus a unit from `B/KB/MB/GB/TB` chosen by the largest unit under which the value is `>= 1`, no space between number and unit.
2. **Domain** — left-aligned, minimum width **30**, truncated with an ellipsis only if it would break alignment badly (otherwise printed in full and the hash column shifts right).
3. **Hash** — the hashed folder name wrapped in parentheses: `(ABCDEF…)`.

Format string: `fmt.Sprintf("%7s  %-30s  (%s)", size, domain, hash)`. The listing is preceded by nothing extra on success; on a non-OK access status the tool prints only `AccessMessage` and exits.

`FormatSize` uses 1024-based units (consistent with `du -sk`'s KB blocks): `KB = 1024 B`, `MB = 1024 KB`, etc.

## Error Handling

| Condition | Requirement | Behavior |
|-----------|-------------|----------|
| Base path does not exist | 6.3 | Print "Safari website-data directory not found" guidance; exit non-zero. `Classify -> StatusNotFound`. |
| Base exists, zero readable child folders | 6.1, 6.2 | Print Full Disk Access guidance naming System Settings > Privacy & Security; exit non-zero. `Classify -> StatusAccessBlocked`. |
| Base exists, folders present | — | Normal listing. `StatusOK`. |
| `-n` not a positive integer | 1.5 | Print input error; exit non-zero without listing. |
| `delete` with neither `--domain` nor `--hash` | 4 (input) | Print usage error; exit non-zero. |
| Delete target matches no folder | 4.3 | Print "no matching folder found"; exit non-zero without deleting. |
| Resolved target not a direct child of base | 5.2, 5.3 | Print path-safety error; refuse deletion; exit non-zero. |
| Confirmation declined | 4.5 | Exit zero without deleting. |
| Single unparseable `origin` file | 2.4 | Domain becomes `Unknown Domain`; folder still listed; no abort. |
| Single unreadable folder during scan | — | Skip with a logged warning; continue scanning others. |

Errors are returned up to `cli.Run`, which maps them to a message on `stderr` and an exit code. The core packages return typed sentinel errors (`ErrMalformed`, `ErrNoMatch`, `ErrPathUnsafe`) rather than printing, keeping them pure and testable.

## Distribution

### Repository structure

Public GitHub repo rooted at the module path (e.g. `github.com/<owner>/safari-data-tool`). Contents as in the Project Structure tree: `main.go`, `internal/`, `go.mod`, `README.md`, `LICENSE`, and the optional CI/release files.

### README contents (Requirement 8)

1. **Build & install** (8.1): `go build -o safari-data-tool .` and `go install github.com/<owner>/safari-data-tool@latest`; note macOS-only and CGO-free build (`CGO_ENABLED=0 go build`).
2. **Usage** (8.2): examples for `list`, `list -n 30`, `delete --domain www.gstatic.com`, `delete --hash <hash> --force`.
3. **Full Disk Access** (8.3): the host terminal must be granted Full Disk Access in System Settings > Privacy & Security, with the symptom (empty listing) called out.
4. **Deletion permanence** (8.4): an explicit, prominent warning that deletion is permanent and irreversible.

### Build

```
CGO_ENABLED=0 go build -o safari-data-tool .
```

Pure stdlib core means a clean static build satisfying Requirement 7.2.

### Optional: GitHub Actions + GoReleaser (clearly optional)

- **OPTIONAL** `.goreleaser.yaml`: build matrix for `darwin/amd64` and `darwin/arm64`, `CGO_ENABLED=0`, archive naming, checksums.
- **OPTIONAL** `.github/workflows/release.yml`: on git tag push, run GoReleaser to publish cross-arch macOS release binaries.

These are conveniences for distributing prebuilt binaries; the tool builds and runs without them via `go build`/`go install`.

## Testing Strategy

Dual approach — property-based tests for universal logic (minimum 100 iterations each, tagged with the feature name and property number), plus example and edge-case unit tests. Live container reads are **not** unit-tested; they are abstracted behind `fsx.FileSystem` and exercised with temp-dir and fake implementations.

- **Origin parser** (`originparse`): table-driven unit tests with hex fixtures, including the real `unifi.ui.com` example (`[05 00 00 00][01]"https"[0B 00 00 00][01]"unifi.ui.com"`); plus a round-trip property (encode random record sequences, parse, assert equality) and malformed/truncated inputs (assert `Unknown Domain`, no panic).
- **Size calculator** (`sizer`): build temp trees with random files/sizes; assert the total equals an independently summed total and that subtree sizes are additive.
- **Path-safety validator** (`deleter.ValidatePath`): generate direct children (accept), the base itself, nested descendants, and `..`/traversal paths that escape the base (reject). Property plus explicit traversal examples.
- **Selection & matcher**: property tests over random folder slices for `TopN` (length `min(count, n)`, descending order, default 15) and `Match` (present -> found, absent -> `ErrNoMatch`).
- **Access classifier** (`scanner.Classify`): enumerate `(exists, count)` states; assert FDA status iff `exists && count==0`, not-found iff `!exists`.
- **Formatters** (`ui`): line content contains size/domain/hash; `FormatSize` monotonic and unit-suffixed.
- **Confirmation & deletion**: mocked `io.Reader` for `Confirm` (yes/no/force-skips-read); `Delete` against a temp folder under a fake base (gone afterward; refuses outside-base targets).

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system — essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Top-N selection

*For any* set of scanned folders and any positive count, the listing returns exactly `min(count, n)` folders ordered by non-increasing size, and returns all folders when count is at least the number of folders (count defaults to 15 when unspecified).

**Validates: Requirements 1.1, 1.2, 1.4**

### Property 2: Count validation rejects non-positive values

*For any* integer count that is not a positive integer, count validation returns an input error and no listing is produced.

**Validates: Requirements 1.5**

### Property 3: Listing line contains required fields

*For any* scanned folder, the rendered listing line contains the human-readable size, the resolved domain, and the hashed folder name.

**Validates: Requirements 1.3**

### Property 4: Origin record round-trip

*For any* sequence of record texts encoded with the layout `[uint32 LE length][flag byte][N UTF-8 bytes]`, parsing the encoded bytes yields the original sequence of texts.

**Validates: Requirements 2.2**

### Property 5: First-pair host resolution

*For any* origin byte stream whose first two records decode to a scheme followed by a host, the resolved domain equals that host.

**Validates: Requirements 2.1, 2.3**

### Property 6: Malformed origin robustness

*For any* truncated or malformed origin byte stream (fewer than two decodable records or a truncated record), domain resolution yields `Unknown Domain` and never panics.

**Validates: Requirements 2.4**

### Property 7: Recursive size is additive

*For any* directory tree, the computed folder size equals the sum of all contained file sizes, and the size of a tree equals the sum of the sizes of its subtrees.

**Validates: Requirements 3.1**

### Property 8: Human-readable size formatting

*For any* non-negative size value, the formatted output is non-empty, carries a unit suffix, and is non-decreasing in the underlying size.

**Validates: Requirements 3.2**

### Property 9: Deletion target matching

*For any* set of scanned folders and a target by domain and/or hash, the matcher returns the folder satisfying the provided field(s) when one exists, and returns a no-match error when none satisfies the target.

**Validates: Requirements 4.1, 4.2, 4.3**

### Property 10: Path-safety requires a direct child of base

*For any* candidate target path, the path-safety validator accepts it if and only if its cleaned absolute form is a direct child of the cleaned absolute base path; it rejects the base itself, nested descendants, and any traversal path that resolves outside the base.

**Validates: Requirements 5.1, 5.2, 5.3**

### Property 11: Access-status classification

*For any* combination of base-path existence and readable-child-folder count, the classifier reports access-blocked exactly when the base exists with zero readable child folders, and reports not-found exactly when the base does not exist.

**Validates: Requirements 6.1, 6.3**

### Property 12: Base-path derivation

*For any* home directory path, the derived WebsiteData base path equals the home directory joined with the fixed macOS Safari container suffix.

**Validates: Requirements 7.1**
