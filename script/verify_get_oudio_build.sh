#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_root="${TMPDIR:-/private/tmp}/get-oudio-apple-music-downloader"
output="${OUTPUT:-$tmp_root/apple-music-downloader}"
go_build_cache="${GOCACHE:-$tmp_root/go-build-cache}"
go_mod_cache="${GOMODCACHE:-$tmp_root/go-mod-cache}"

mkdir -p "$(dirname "$output")" "$go_build_cache" "$go_mod_cache"
output_dir="$(cd "$(dirname "$output")" && pwd -P)"
output_path="$output_dir/$(basename "$output")"

case "$output_path" in
  "$repo_root"/*)
    printf 'Refusing to write verifier output inside repo: %s\n' "$output_path" >&2
    exit 1
    ;;
esac

cd "$repo_root"
env \
  CGO_ENABLED=0 \
  GOOS=darwin \
  GOARCH=arm64 \
  GOCACHE="$go_build_cache" \
  GOMODCACHE="$go_mod_cache" \
  go build -trimpath -ldflags="-s -w" -o "$output_path" main.go

version_info="$(go version -m "$output_path")"
printf '%s\n' "$version_info"
for required in \
  $'\tbuild\t-trimpath=true' \
  $'\tbuild\tCGO_ENABLED=0' \
  $'\tbuild\tGOARCH=arm64' \
  $'\tbuild\tGOOS=darwin'
do
  if [[ "$version_info" != *"$required"* ]]; then
    printf 'Missing expected go version metadata: %s\n' "$required" >&2
    exit 1
  fi
done

if command -v otool >/dev/null 2>&1; then
  otool_output="$(otool -L "$output_path")"
  printf '%s\n' "$otool_output"
  if [[ "$otool_output" == *"/opt/homebrew/"* || "$otool_output" == *"/usr/local/"* ]]; then
    printf 'Unexpected non-system dynamic library dependency detected.\n' >&2
    exit 1
  fi
fi

printf 'Verified Get Oudio downloader build: %s\n' "$output_path"
