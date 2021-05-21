#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

umask 022

export LC_ALL="C"
export TZ="Etc/UTC"

readonly PACKAGE="chronostachyon/roxy"
readonly PLATFORM_LIST=( linux/amd64 linux/arm64v8 )

if [ "${RELEASE_MODE:-false}" = "true" ]; then
  FULL_VERSION="$GITHUB_REF"
  FULL_VERSION="${FULL_VERSION##*/v}"
  TAGS=( "$FULL_VERSION" "latest" )
else
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
  TAGS=( "$FULL_VERSION" "devel" )
fi

run() {
  echo "> $*"
  "$@" || return $?
}

build_for_platform() {
  local platform="$1"
  local platform_tag="${platform//\//-}"
  local arch="${platform#*/}"
  local -i rc=0

  declare -a args

  run \
    docker buildx use "$platform_tag" \
    || rc=$?

  if [ $rc -ne 0 ]; then
    run \
      docker buildx create \
        --name="$platform_tag" \
        --driver="docker-container" \
        --platform="$platform" \
        --use
  fi

  args=( \
    docker \
    buildx \
    build \
    --file="Dockerfile" \
    --platform="$platform" \
    --build-arg=VERSION="$FULL_VERSION" \
    --build-arg=ARCH="$arch" \
  )
  for tag in "${TAGS[@]}"; do
    args=( \
      "${args[@]}" \
      --tag="${PACKAGE}:${platform_tag}-${tag}" \
    )
  done
  args=( \
    "${args[@]}" \
    --push \
    . \
  )

  run "${args[@]}"
}

for platform in "${PLATFORM_LIST[@]}"; do
  build_for_platform "$platform"
done

for tag in "${TAGS[@]}"; do
  run \
    docker \
    manifest \
    rm \
    "${PACKAGE}:${tag}" \
    || true

  args=( \
    docker \
    manifest \
    create \
    "${PACKAGE}:${tag}" \
  )
  for platform in "${PLATFORM_LIST[@]}"; do
    platform_tag="${platform//\//-}"
    args=( "${args[@]}" "${PACKAGE}:${platform_tag}-${tag}" )
  done
  run "${args[@]}"

  run \
    docker \
    manifest \
    push \
    "${PACKAGE}:${tag}"
done
