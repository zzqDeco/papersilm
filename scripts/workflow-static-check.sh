#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ACTIONLINT_VERSION="${ACTIONLINT_VERSION:-1.7.12}"

shopt -s nullglob
WORKFLOWS=(.github/workflows/*.yml .github/workflows/*.yaml)
shopt -u nullglob

if ((${#WORKFLOWS[@]} == 0)); then
  echo "no GitHub Actions workflows found"
  exit 0
fi

if command -v actionlint >/dev/null 2>&1; then
  exec actionlint "${WORKFLOWS[@]}"
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  linux|darwin|freebsd) ;;
  *)
    echo "unsupported OS for actionlint download: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  armv6l) arch="armv6" ;;
  i386|i686) arch="386" ;;
  *)
    echo "unsupported architecture for actionlint download: $arch" >&2
    exit 1
    ;;
esac

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

asset="actionlint_${ACTIONLINT_VERSION}_${os}_${arch}.tar.gz"
base_url="https://github.com/rhysd/actionlint/releases/download/v${ACTIONLINT_VERSION}"
archive="$tmpdir/$asset"
checksums="$tmpdir/actionlint_${ACTIONLINT_VERSION}_checksums.txt"

curl -fsSL "$base_url/$asset" -o "$archive"
curl -fsSL "$base_url/actionlint_${ACTIONLINT_VERSION}_checksums.txt" -o "$checksums"

expected="$(awk -v asset="$asset" '$2 == asset { print $1 }' "$checksums")"
if [[ -z "$expected" ]]; then
  echo "checksum entry not found for $asset" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$archive" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$archive" | awk '{ print $1 }')"
else
  echo "sha256sum or shasum is required to verify actionlint" >&2
  exit 1
fi

if [[ "$actual" != "$expected" ]]; then
  echo "checksum mismatch for $asset" >&2
  exit 1
fi

tar -xzf "$archive" -C "$tmpdir"
"$tmpdir/actionlint" "${WORKFLOWS[@]}"
