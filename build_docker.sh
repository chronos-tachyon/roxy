#!/bin/bash
set -euo pipefail
VERSION=0.0.0
DATESTAMP="$(date --utc +%Y.%m.%d)"
mkdir -p ~/.cache/last-build
if [ ! -e ~/.cache/last-build/roxy ]; then
  echo 0 > ~/.cache/last-build/roxy
fi
LAST_COUNTER="$(cat ~/.cache/last-build/roxy)"
NEXT_COUNTER=$((LAST_COUNTER + 1))
echo "$NEXT_COUNTER" > ~/.cache/last-build/roxy
FULL_VERSION="${VERSION}-${DATESTAMP}-${NEXT_COUNTER}"
docker build -t "roxy:${FULL_VERSION}" -t "roxy:latest" .
