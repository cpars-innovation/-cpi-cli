#!/usr/bin/env bash
# Prints the next release version (vMAJOR.MINOR.PATCH) from the latest
# release tag reachable in this repository.
#
#   scripts/next-version.sh patch|minor|major
#
# Without any release tag the first version is v0.1.0. Pre-release tags
# (v1.2.3-rc1) are ignored when looking for the latest release.
set -euo pipefail

bump="${1:-patch}"
case "$bump" in
  patch|minor|major) ;;
  *) echo "usage: $0 patch|minor|major" >&2; exit 2 ;;
esac

latest="$(git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n 1 || true)"
if [ -z "$latest" ]; then
  echo "v0.1.0"
  exit 0
fi

IFS=. read -r major minor patch <<<"${latest#v}"
case "$bump" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
esac
echo "v${major}.${minor}.${patch}"
