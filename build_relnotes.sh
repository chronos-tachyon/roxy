#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

umask 022

export LC_ALL="C"
export TZ="Etc/UTC"

FULL_VERSION="$GITHUB_REF"
FULL_VERSION="${FULL_VERSION##*/v}"

version_regexp="v${FULL_VERSION/./\\.}"

rm -f release-notes.txt

awk '
  BEGIN { X=0 }
  /^v[0-9.]+$/ { X=0 }
  /^'"${version_regexp}"'$/ { X=1 }
  X == 1 {print}
' < CHANGELOG.txt > release-notes.txt
