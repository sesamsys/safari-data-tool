# safari-data-tool

A pure-Go, macOS-only command-line tool for inspecting and managing Safari's
WebKit `WebsiteData` storage. Safari keeps per-site caches and storage under its
container, where some web apps quietly accumulate gigabytes that are never
cleaned up.

The tool scans the per-domain data folders Safari stores under its container,
resolves each hashed folder name to a human-readable domain by parsing the
plaintext `origin` files WebKit writes, reports accurate recursive folder sizes
(`du`-style), and lets you clear a specific site's data by domain name or hash
with a confirmation safeguard. By default, deletion clears the site's data while
preserving the tiny `origin` metadata sidecar, so the now-small folder stays
identifiable to its domain on the next scan instead of becoming
"Unknown Domain".

## Requirements

- **macOS only.** The tool derives Safari's `WebsiteData` path from the macOS
  Safari container layout and is not meant to run on other platforms.
- **Full Disk Access** granted to your terminal application (see below).

## Install

The quickest way on macOS is the install script. It detects your architecture,
downloads the latest release, verifies its checksum, installs the binary to
`~/.local/bin`, and adds that directory to your `PATH`:

```sh
curl -fsSL https://raw.githubusercontent.com/sesamsys/safari-data-tool/main/install.sh | sh
```

After it finishes, grant **Full Disk Access** to your terminal app (see below),
then quit and reopen the terminal. Verify with:

```sh
safari-data-tool --version
```

The script accepts a couple of environment overrides:

- `INSTALL_DIR` — target bin directory (default `~/.local/bin`; no `sudo`
  needed). Set `INSTALL_DIR=/usr/local/bin` for the system location.
- `VERSION` — a specific release tag to install, e.g. `VERSION=v0.2.0`
  (default: latest).

### Build from source

The binary is built as a pure-Go binary with CGO disabled, so there is no C
toolchain dependency and the result is a single self-contained executable.

Build locally from a checkout:

```sh
CGO_ENABLED=0 go build -o safari-data-tool .
```

Or install the latest version with the Go toolchain:

```sh
go install github.com/sesamsys/safari-data-tool@latest
```

## Usage

Running the tool with no subcommand launches the interactive TUI, a
full-screen browser of the largest Safari website-data folders. The `list` and
`delete` subcommands provide the equivalent non-interactive flows.

```sh
# Launch the interactive TUI (default — bare invocation)
safari-data-tool

# List the 15 largest folders (non-interactive)
safari-data-tool list

# List the 30 largest
safari-data-tool list -n 30

# Clear a specific domain's data (prompts for confirmation)
safari-data-tool delete --domain www.gstatic.com

# Clear by hash without prompting
safari-data-tool delete --hash <hash> --force
```

Notes:

- A bare invocation (`safari-data-tool`, no subcommand) launches the
  interactive TUI. Navigate with the arrow keys and clear the selected folder
  after an in-screen confirmation; press `q` to quit.
- `list` accepts `-n <count>` to control how many of the largest folders to
  show (default 15). If the count exceeds the number of folders available, all
  of them are listed.
- `delete` requires at least one of `--domain` or `--hash`. If both are given,
  the target must match both.
- Pass `--force` (or `--yes`) to skip the interactive confirmation prompt.
- Clearing a folder (via `delete` or the TUI) removes the site's data but keeps
  the tiny `origin` metadata sidecar by default, so the now-small folder stays
  resolvable to its domain. A folder with no `origin` sidecar is removed
  entirely.
- A folder occasionally lists as **Unknown Domain** when WebKit has recreated
  its storage from scratch but not yet written the `origin` sidecar back —
  WebKit writes it lazily, typically after a Safari restart-and-reaccess rather
  than on a page reload. Folders you clear with this tool keep their sidecar and
  stay resolvable. This is inherent WebKit behavior, not an error in the tool.

## Full Disk Access

Safari's container is protected by macOS TCC. The terminal application you run
the tool from must be granted **Full Disk Access** in
**System Settings > Privacy & Security > Full Disk Access**.

Without this permission the Safari container reads as empty: the tool sees zero
folders and reports an empty listing with a message that this likely requires
Full Disk Access. If you see that, grant access to your terminal app, then quit
and reopen the terminal so the new permission takes effect.

## ⚠️ Clearing a site's data is permanent and irreversible

Clearing a folder permanently and irreversibly removes the site's data. There is
**no undo**, and nothing is moved to the Trash. Once a domain's data is cleared
it cannot be recovered. The tiny `origin` metadata marker and the folder itself
remain so the folder stays identifiable, but the data is gone. Always
double-check the domain or hash before confirming — and before using `--force`,
which clears without any prompt.

## License

MIT. See [LICENSE](LICENSE).
