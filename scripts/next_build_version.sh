#!/bin/bash
set -euo pipefail

BUILD_NAME="${1?}"

: ${XDG_CACHE_HOME:=~/.cache}

if [ "${RELEASE_MODE:-false}" = "true" ]; then
  FULL_VERSION="$GITHUB_REF"
  FULL_VERSION="${FULL_VERSION##*/v}"
else
  VERSION="$(cat .version)"
  DATESTAMP="$(date --utc +%Y.%m.%d)"
  LASTBUILDDIR="${XDG_CACHE_HOME}/last-build"
  LASTBUILDFILE="${LASTBUILDDIR}/${BUILD_NAME}"
  mkdir -p "$LASTBUILDDIR"
  if [ ! -e "$LASTBUILDFILE" ]; then
    echo 0 > "$LASTBUILDFILE"
  fi
  declare -i LAST_COUNTER="$(cat "$LASTBUILDFILE")"
  declare -i NEXT_COUNTER=$((LAST_COUNTER + 1))
  echo "$NEXT_COUNTER" > "$LASTBUILDFILE"
  FULL_VERSION="${VERSION}-r${DATESTAMP}-${NEXT_COUNTER}"
fi

echo "$FULL_VERSION"
