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

## Build & install

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

If you run the tool with no subcommand, it defaults to listing.

```sh
# List the 15 largest folders (default — bare invocation also lists)
safari-data-tool list

# List the 30 largest
safari-data-tool list -n 30

# Delete a specific domain's data (prompts for confirmation)
safari-data-tool delete --domain www.gstatic.com

# Delete by hash without prompting
safari-data-tool delete --hash <hash> --force
```

Notes:

- A bare invocation (`safari-data-tool`, no subcommand) behaves the same as
  `safari-data-tool list`.
- `list` accepts `-n <count>` to control how many of the largest folders to
  show (default 15). If the count exceeds the number of folders available, all
  of them are listed.
- `delete` requires at least one of `--domain` or `--hash`. If both are given,
  the target must match both.
- Pass `--force` (or `--yes`) to skip the interactive confirmation prompt.

## Full Disk Access

Safari's container is protected by macOS TCC. The terminal application you run
the tool from must be granted **Full Disk Access** in
**System Settings > Privacy & Security > Full Disk Access**.

Without this permission the Safari container reads as empty: the tool sees zero
folders and reports an empty listing with a message that this likely requires
Full Disk Access. If you see that, grant access to your terminal app, then quit
and reopen the terminal so the new permission takes effect.

## "Unknown Domain" after deleting and recreating a folder

The tool resolves a hashed folder's domain solely by reading the plaintext
`origin` sidecar file that WebKit writes inside that folder. By default the tool
preserves this sidecar on delete, so a folder you clear with this tool stays
resolvable to its domain.

WebKit itself writes the sidecar lazily. If a folder's `origin` sidecar is
missing — for example because WebKit recreated the folder's storage from scratch
(reloading a page can repopulate data before the sidecar is written back) — the
tool cannot name the origin and lists it as **Unknown Domain**, even though the
hash is unchanged.

Observed behavior on macOS WebKit: a page reload within a running Safari
session does not restore a missing sidecar, and neither does quitting Safari.
WebKit rewrites the `origin` file on a **restart-and-reaccess** — launching
Safari and visiting the site again. After that the tool resolves the domain
normally. This is inherent WebKit lazy-write behavior, not an error in the tool.

## ⚠️ Clearing a site's data is permanent and irreversible

Clearing a folder permanently and irreversibly removes the site's data. There is
**no undo**, and nothing is moved to the Trash. Once a domain's data is cleared
it cannot be recovered. The tiny `origin` metadata marker and the folder itself
remain so the folder stays identifiable, but the data is gone. Always
double-check the domain or hash before confirming — and before using `--force`,
which clears without any prompt.

## License

MIT. See [LICENSE](LICENSE).
