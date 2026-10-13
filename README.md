# safari-data-tool

A small macOS command-line tool for inspecting and cleaning up Safari's WebKit
website-data storage. Safari keeps per-site caches and storage under its
container, where some web apps accumulate gigabytes that are never cleaned up.
This tool lists the largest per-domain data folders, resolves each hashed folder
to a readable domain, and lets you permanently delete a specific site's data.

> Status: early development. The CLI is being implemented from the spec in
> `.kiro/specs/safari-data-tool/`.

## Requirements

- macOS (Safari-specific paths)
- Go 1.22+ to build
- **Full Disk Access** granted to your terminal application. Without it, the
  Safari container reads as empty and the tool cannot see any data. Grant it in
  System Settings > Privacy & Security > Full Disk Access.

## Build

```sh
CGO_ENABLED=0 go build -o safari-data-tool .
```

## Usage

```sh
# List the 15 largest folders (default)
safari-data-tool

# List the 30 largest
safari-data-tool -n 30

# Delete a specific domain's data (prompts for confirmation)
safari-data-tool delete --domain unifi.ui.com

# Delete by hash without prompting
safari-data-tool delete --hash <hash> --force
```

## ⚠️ Deletion is permanent

Deletion permanently and irreversibly removes the selected folder. There is no
undo and nothing is moved to the Trash. Double-check the domain or hash before
confirming.

## License

MIT. See [LICENSE](LICENSE).
