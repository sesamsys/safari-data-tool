# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0]

### Added

- Initial release of `safari-data-tool`, a pure-Go, macOS-only CLI for
  inspecting and managing Safari's WebKit `WebsiteData` storage.
- `list` command showing the largest per-domain data folders, with `-n` to
  control how many are shown (default 15). A bare invocation defaults to
  listing.
- `delete` command to permanently remove a site's data by `--domain` and/or
  `--hash`, with an interactive confirmation prompt and `--force` / `--yes` to
  skip it.
- Origin-file parsing that resolves Safari's hashed folder names to
  human-readable domains, falling back to "Unknown Domain" when no origin file
  is present or readable.
- Recursive folder sizing with `du -sk` semantics (per-file rounding to 1024-byte
  blocks) and human-readable size formatting.
- Full Disk Access detection: when Safari's container is unreadable, the tool
  reports that Full Disk Access is likely required instead of silently showing
  an empty list.
- Path-safety guard on deletion that only ever removes a direct child of the
  resolved `WebsiteData` base path, rejecting the base itself, nested
  descendants, and traversal escapes.
- Filesystem abstraction (`fsx.FileSystem`) with a temp-dir-backed fake,
  enabling the full tool to be tested without touching the live Safari container.
- Comprehensive test suite: table-driven and unit tests plus 12 property-based
  tests (100+ iterations each) using only the standard library.
- Documentation (`README.md`) covering build/install, usage, the Full Disk
  Access requirement, and a deletion-permanence warning; MIT `LICENSE`.
- Release automation: GoReleaser config building CGO-free `darwin/amd64` and
  `darwin/arm64` archives with checksums, plus a GitHub Actions workflow that
  releases on `v*` tags.

[Unreleased]: https://github.com/sesamsys/safari-data-tool/commits/main
