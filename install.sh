#!/usr/bin/env bash
# Installs the dg command line from a GitHub release. On Windows, run it in Git Bash.
#
#   curl -fsSL https://raw.githubusercontent.com/nvcnvn/dg/main/install.sh | bash
#
#   DG_VERSION       the release to install, such as v0.1.2 (default: the latest)
#   DG_INSTALL_DIR   where to put dg (default: ~/.local/bin)
#   DG_RELEASES_URL  the releases to download from (default: this repository's on GitHub)

set -euo pipefail

source_docs=https://github.com/nvcnvn/dg/blob/main/docs/getting-started.md#build-from-source

die() {
  printf 'dg install: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null || die "this script needs $1"
}

detect_target() {
  local os arch
  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    MINGW* | MSYS* | CYGWIN*) os=windows ;;
    *) die "no release for $(uname -s): build dg from source, $source_docs" ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "no release for $(uname -m): build dg from source, $source_docs" ;;
  esac
  # A shell under Rosetta reports x86_64 on Apple silicon.
  if [ "$os" = darwin ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null)" = 1 ]; then
    arch=arm64
  fi
  echo "$os-$arch"
}

latest_tag() {
  local url
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$1/latest") || die "cannot reach $1"
  echo "${url##*/}"
}

sha256() {
  if command -v sha256sum >/dev/null; then
    sha256sum -b "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null; then
    shasum -a 256 -b "$1" | cut -d' ' -f1
  else
    die "this script needs sha256sum or shasum"
  fi
}

report_path() {
  local dir=$1 exe=$2 found
  found=$(command -v "$exe" || true)
  if [ -n "$found" ] && [ "$found" -ef "$dir/$exe" ]; then
    return
  fi
  if [ -n "$found" ]; then
    echo "Another dg comes first on your PATH: $found"
  elif command -v cygpath >/dev/null; then
    echo "Add $(cygpath -w "$dir") to your user Path, then open a new terminal."
  else
    echo "Add it to your PATH, in your shell's profile:"
    echo "  export PATH=\"$dir:\$PATH\""
  fi
}

main() {
  local releases=${DG_RELEASES_URL:-https://github.com/nvcnvn/dg/releases}
  local dir=${DG_INSTALL_DIR:-$HOME/.local/bin}
  local target tag stem archive exe=dg want got

  need curl
  target=$(detect_target)
  tag=${DG_VERSION:-$(latest_tag "$releases")}
  tag=v${tag#v}
  [[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "not a release: $tag"

  stem=dg-$tag-$target
  if [[ $target == windows-* ]]; then
    need unzip
    archive=$stem.zip
    exe=dg.exe
  else
    need tar
    archive=$stem.tar.gz
  fi

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  echo "Downloading dg $tag for $target"
  curl -fsSL --retry 3 -o "$tmp/SHA256SUMS" "$releases/download/$tag/SHA256SUMS" ||
    die "no release $tag at $releases"
  curl -fsSL --retry 3 -o "$tmp/$archive" "$releases/download/$tag/$archive" ||
    die "release $tag has no $archive"

  want=$(awk -v name="$archive" '$2 == name { print $1 }' "$tmp/SHA256SUMS")
  [ -n "$want" ] || die "SHA256SUMS does not list $archive"
  got=$(sha256 "$tmp/$archive")
  [ "$got" = "$want" ] || die "checksum mismatch for $archive: expected $want, got $got"

  if [[ $archive == *.zip ]]; then
    unzip -q "$tmp/$archive" -d "$tmp"
  else
    tar -xzf "$tmp/$archive" -C "$tmp"
  fi

  mkdir -p "$dir"
  # A new file renamed into place: macOS kills a signed binary that is overwritten in place.
  cp "$tmp/$stem/$exe" "$dir/.$exe.new"
  chmod 755 "$dir/.$exe.new"
  mv -f "$dir/.$exe.new" "$dir/$exe" || {
    rm -f "$dir/.$exe.new"
    die "cannot replace $dir/$exe: stop any dg still running from it and try again"
  }

  echo "Installed dg $tag to $dir/$exe"
  report_path "$dir" "$exe"
}

# Last, so that a download cut short runs nothing.
main "$@"
