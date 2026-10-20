#!/bin/sh
#
# install.sh — download and install the latest safari-data-tool release.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/sesamsys/safari-data-tool/main/install.sh | sh
#
# or, from a checkout:
#   ./install.sh
#
# What it does:
#   1. Detects your macOS architecture (arm64 / amd64).
#   2. Finds the latest GitHub release and downloads the matching archive.
#   3. Verifies the SHA-256 checksum against the release's checksums.txt.
#   4. Installs the binary to a bin directory on your PATH (see INSTALL_DIR).
#   5. Adds that directory to your PATH in your shell profile if needed.
#
# After installing you still need to grant Full Disk Access to your terminal
# app (System Settings > Privacy & Security > Full Disk Access) for the tool to
# read Safari's container. See the README for details.
#
# Environment overrides:
#   INSTALL_DIR      Target bin directory (default: ~/.local/bin).
#   VERSION          Release tag to install, e.g. v0.2.0 (default: latest).
#   SKIP_PATH_SETUP  If set to a non-empty value, install the binary but do NOT
#                    modify any shell profile. Useful for throwaway test runs
#                    (e.g. installing into a temp INSTALL_DIR).

set -eu

REPO="sesamsys/safari-data-tool"
BINARY="safari-data-tool"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"
SKIP_PATH_SETUP="${SKIP_PATH_SETUP:-}"

# ---- helpers ---------------------------------------------------------------

err() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

info() {
	printf '%s\n' "$1" >&2
}

need() {
	command -v "$1" >/dev/null 2>&1 || err "required command not found: $1"
}

# ---- preflight -------------------------------------------------------------

os="$(uname -s)"
[ "$os" = "Darwin" ] || err "safari-data-tool is macOS only (detected: $os)"

need uname
need curl
need tar
need mktemp

# A checksum tool: prefer shasum (always present on macOS), fall back to sha256sum.
if command -v shasum >/dev/null 2>&1; then
	sha_cmd="shasum -a 256"
elif command -v sha256sum >/dev/null 2>&1; then
	sha_cmd="sha256sum"
else
	err "need shasum or sha256sum to verify the download"
fi

# ---- detect architecture ---------------------------------------------------

arch="$(uname -m)"
case "$arch" in
arm64 | aarch64) goarch="arm64" ;;
x86_64 | amd64) goarch="amd64" ;;
*) err "unsupported architecture: $arch" ;;
esac

# ---- resolve the release tag -----------------------------------------------

if [ "$VERSION" = "latest" ]; then
	info "Resolving latest release..."
	# Follow the redirect from /releases/latest to read the resolved tag from
	# the final URL. This avoids needing the GitHub API (and its rate limits).
	latest_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
		"https://github.com/$REPO/releases/latest")" ||
		err "could not reach GitHub to resolve the latest release"
	tag="${latest_url##*/tag/}"
	[ -n "$tag" ] && [ "$tag" != "$latest_url" ] ||
		err "could not determine the latest release tag"
else
	tag="$VERSION"
fi

# Release tags are "vX.Y.Z"; GoReleaser archive names use the bare "X.Y.Z".
version="${tag#v}"

archive="${BINARY}_${version}_darwin_${goarch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/$tag"

info "Installing $BINARY $tag ($goarch) to $INSTALL_DIR"

# ---- download + verify + extract -------------------------------------------

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

info "Downloading $archive ..."
curl -fsSL -o "$tmp/$archive" "$base_url/$archive" ||
	err "failed to download $base_url/$archive"

info "Verifying checksum ..."
curl -fsSL -o "$tmp/checksums.txt" "$base_url/checksums.txt" ||
	err "failed to download checksums.txt"

expected="$(awk -v f="$archive" '$2 == f {print $1}' "$tmp/checksums.txt")"
[ -n "$expected" ] || err "no checksum entry for $archive in checksums.txt"

actual="$($sha_cmd "$tmp/$archive" | awk '{print $1}')"
[ "$actual" = "$expected" ] ||
	err "checksum mismatch for $archive (expected $expected, got $actual)"

info "Extracting ..."
tar -xzf "$tmp/$archive" -C "$tmp" "$BINARY" ||
	err "failed to extract $BINARY from $archive"

# ---- install ---------------------------------------------------------------

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/$BINARY" "$INSTALL_DIR/$BINARY" ||
	err "failed to install to $INSTALL_DIR (check permissions)"

info "Installed $INSTALL_DIR/$BINARY"

# ---- ensure INSTALL_DIR is on PATH -----------------------------------------

add_path_line="export PATH=\"$INSTALL_DIR:\$PATH\""

if [ -n "$SKIP_PATH_SETUP" ]; then
	# Explicitly asked not to touch any shell profile (e.g. a test run into a
	# temp dir). Just tell the user how to reach the binary.
	on_path=1
	info "SKIP_PATH_SETUP set — not modifying any shell profile."
	case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		info "To run $BINARY, use its full path or add it to PATH:"
		info "    export PATH=\"$INSTALL_DIR:\$PATH\""
		;;
	esac
else
	case ":$PATH:" in
	*":$INSTALL_DIR:"*)
		on_path=1
		;;
	*)
		on_path=0
		;;
	esac
fi

if [ "$on_path" -eq 0 ]; then
	# Pick a profile file for the current login shell.
	shell_name="$(basename "${SHELL:-/bin/zsh}")"
	case "$shell_name" in
	zsh) profile="$HOME/.zshrc" ;;
	bash) profile="$HOME/.bash_profile" ;;
	*) profile="$HOME/.profile" ;;
	esac

	if [ -f "$profile" ] && grep -Fq "$add_path_line" "$profile" 2>/dev/null; then
		info "PATH entry already present in $profile"
	else
		{
			printf '\n# Added by safari-data-tool install.sh\n'
			printf '%s\n' "$add_path_line"
		} >>"$profile"
		info "Added $INSTALL_DIR to PATH in $profile"
	fi

	info ""
	info "To use $BINARY in this shell right now, run:"
	info "    export PATH=\"$INSTALL_DIR:\$PATH\""
	info "New terminal sessions will pick it up automatically."
fi

# ---- done -------------------------------------------------------------------

info ""
info "Done. Verify with:  $BINARY --version"
info ""
info "Remember: grant Full Disk Access to your terminal app in"
info "System Settings > Privacy & Security > Full Disk Access, then"
info "quit and reopen the terminal so the permission takes effect."
