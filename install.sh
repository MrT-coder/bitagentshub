#!/bin/sh
# Installs BitAgentsHub from its GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/MrT-coder/bitagentshub/master/install.sh | sh
#
# Environment:
#   BAH_VERSION      tag to install, e.g. v0.2.0 (default: latest release)
#   BAH_INSTALL_DIR  target directory (default: $HOME/.local/bin)
#   GITHUB_TOKEN     optional, avoids GitHub API rate limits
set -eu

repo="MrT-coder/bitagentshub"
version="${BAH_VERSION:-}"
install_dir="${BAH_INSTALL_DIR:-$HOME/.local/bin}"

fail() { echo "bah: $*" >&2; exit 1; }

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported OS $(uname -s); on Windows use install.ps1" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported architecture $(uname -m)" ;;
esac

command -v curl >/dev/null 2>&1 || fail "curl is required"
if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
  fail "sha256sum or shasum is required"
fi

api() {
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H "User-Agent: bah-installer" "$@"
  else
    curl -fsSL -H "User-Agent: bah-installer" "$@"
  fi
}

if [ -n "$version" ]; then
  release_url="https://api.github.com/repos/$repo/releases/tags/$version"
else
  release_url="https://api.github.com/repos/$repo/releases/latest"
fi
json="$(api -H "Accept: application/vnd.github+json" "$release_url")" ||
  fail "cannot read release $release_url (rate limited? set GITHUB_TOKEN)"
# One key per line, whether the API answered pretty-printed or compact.
release="$(printf '%s\n' "$json" | awk '{ gsub(/"(url|name|tag_name)": */, "\n&"); print }')"

tag="$(printf '%s\n' "$release" | sed -n 's/^"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
[ -n "$tag" ] || fail "cannot read the tag of $release_url"

# asset_url NAME prints the API URL of the release asset called NAME. Each
# asset object lists its "url" before its "name".
asset_url() {
  printf '%s\n' "$release" | awk -v want="$1" '
    /^"url": *"https:\/\/api\.github\.com\/repos\/.*\/releases\/assets\// {
      u = $0; sub(/^"url": *"/, "", u); sub(/".*/, "", u)
    }
    /^"name": *"/ {
      n = $0; sub(/^"name": *"/, "", n); sub(/".*/, "", n)
      if (n == want && u != "") { print u; exit }
    }'
}

archive="bah_${tag#v}_${os}_${arch}.tar.gz"
archive_url="$(asset_url "$archive")"
sums_url="$(asset_url checksums.txt)"
[ -n "$archive_url" ] && [ -n "$sums_url" ] || fail "release $tag has no ${os}_${arch} archive"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading BitAgentsHub $tag ($os/$arch)..."
# Asset API URLs work for public and private repositories; curl does not
# forward the Authorization header to the storage host it redirects to.
api -H "Accept: application/octet-stream" -o "$tmp/$archive" "$archive_url"
api -H "Accept: application/octet-stream" -o "$tmp/checksums.txt" "$sums_url"

expected="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
[ -n "$expected" ] || fail "$archive is missing from checksums.txt"
[ "$expected" = "$(sha256 "$tmp/$archive")" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" bah
mkdir -p "$install_dir"
install -m 0755 "$tmp/bah" "$install_dir/bah"

echo "Installed $("$install_dir/bah" -version) to $install_dir/bah"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to your PATH, e.g. in ~/.profile: export PATH=\"$install_dir:\$PATH\"" ;;
esac
