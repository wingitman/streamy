#!/usr/bin/env sh
set -eu
name="streamy"
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
resolved=$(command -v "$name" 2>/dev/null || true)
if [ -n "${INSTALL_DIR:-}" ]; then dir=$INSTALL_DIR
elif [ -n "$resolved" ]; then dir=$(dirname "$resolved")
else dir="${HOME}/.local/bin"; fi
mkdir -p "$dir"
tmp="$dir/.$name.install.$$"
trap 'rm -f "$tmp"' EXIT HUP INT TERM
if command -v go >/dev/null 2>&1; then
  go build -o "$tmp" "$root/cmd/streamy"
else
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) artifact="$root/releases/$name-linux-amd64";;
    Linux-aarch64|Linux-arm64) artifact="$root/releases/$name-linux-arm64";;
    Darwin-x86_64) artifact="$root/releases/$name-darwin-amd64";;
    Darwin-arm64) artifact="$root/releases/$name-darwin-arm64";;
    *) echo "Go unavailable and no matching release artifact exists" >&2; exit 1;;
  esac
  test -f "$artifact" || { echo "Matching release artifact not found: $artifact" >&2; exit 1; }
  cp "$artifact" "$tmp"
fi
chmod 0755 "$tmp"
mv -f "$tmp" "$dir/$name"
test -x "$dir/$name" || { echo "Installation verification failed" >&2; exit 1; }
resolved=$(command -v "$name" 2>/dev/null || true)
if [ -z "$resolved" ]; then echo "Warning: add $dir to PATH" >&2
elif [ "$resolved" != "$dir/$name" ]; then echo "Warning: PATH resolves $resolved, not $dir/$name" >&2; fi
echo "Installed $name to $dir/$name"
