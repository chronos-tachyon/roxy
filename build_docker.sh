#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
VERSION="$(cat .version)"
DATESTAMP="$(date --utc +%Y.%m.%d)"
LASTBUILDDIR="${HOME}/.cache/last-build"
LASTBUILDFILE="${LASTBUILDDIR}/roxy-docker"
mkdir -p "$LASTBUILDDIR"
if [ ! -e "$LASTBUILDFILE" ]; then
  echo 0 > "$LASTBUILDFILE"
fi
LAST_COUNTER="$(cat "$LASTBUILDFILE")"
NEXT_COUNTER=$((LAST_COUNTER + 1))
echo "$NEXT_COUNTER" > "$LASTBUILDFILE"
FULL_VERSION="${VERSION}-r${DATESTAMP}-${NEXT_COUNTER}"
docker build -t "roxy:${FULL_VERSION}" -t "roxy:latest" .
