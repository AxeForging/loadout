#!/bin/sh
set -eu
REPO="${LOADOUT_REPO:-AxeForging/loadout}"
VERSION="${LOADOUT_VERSION:-latest}"
if [ -n "${LOADOUT_INSTALL_DIR:-}" ]; then INSTALL_DIR="$LOADOUT_INSTALL_DIR"; elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then INSTALL_DIR=/usr/local/bin; else INSTALL_DIR="${HOME:?HOME is required}/.local/bin"; fi
die() { printf 'loadout installer: %s\n' "$*" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"
fetch() { if [ -n "${GITHUB_TOKEN:-}" ]; then curl -H "Authorization: Bearer $GITHUB_TOKEN" "$@"; else curl "$@"; fi; }
case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) die "unsupported operating system: $(uname -s)" ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) die "unsupported architecture: $(uname -m)" ;; esac
if [ "$VERSION" = latest ]; then VERSION="$(fetch -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest" | awk -F/ '{print $NF}')"; [ -n "$VERSION" ] || die "could not resolve latest release"; fi
asset="loadout-${os}-${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${VERSION}"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT HUP INT TERM
fetch -fsSL "$base/$asset" -o "$tmp/$asset"
fetch -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
expected="$(awk -v a="$asset" '$2 == a || $2 == "*" a {print $1; exit}' "$tmp/checksums.txt")"; [ -n "$expected" ] || die "checksum not found"
if command -v sha256sum >/dev/null 2>&1; then actual="$(sha256sum "$tmp/$asset" | awk '{print $1}')"; elif command -v shasum >/dev/null 2>&1; then actual="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"; else die "sha256sum or shasum is required"; fi
[ "$actual" = "$expected" ] || die "checksum verification failed"
tar -xzf "$tmp/$asset" -C "$tmp"
[ -f "$tmp/loadout" ] || die "archive does not contain loadout"
mkdir -p "$INSTALL_DIR"; install -m 0755 "$tmp/loadout" "$INSTALL_DIR/loadout"
printf 'loadout %s installed to %s/loadout\n' "$VERSION" "$INSTALL_DIR"
case ":$PATH:" in *":$INSTALL_DIR:"*) ;; *) printf 'Add %s to PATH to run loadout.\n' "$INSTALL_DIR" ;; esac
