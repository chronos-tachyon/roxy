#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

umask 022

export LC_ALL="C"
export TZ="Etc/UTC"

readonly PACKAGE="chronostachyon/roxy"

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

build_for_arch() {
  declare DOCKERFILE
  declare ARCH
  declare -a FLAGS
  declare tag

  case "$1" in
    (amd64)
      DOCKERFILE="Dockerfile"
      ARCH="amd64"
      ;;
    (arm64v8)
      DOCKERFILE="Dockerfile.arm64v8"
      ARCH="arm64v8"
      ;;
    (*)
      echo "ERROR: unknown arch: ${1}" >&2
      exit 1
      ;;
  esac

  FLAGS=( --file="$DOCKERFILE" --build-arg=version="$FULL_VERSION" )
  for tag in "${TAGS[@]}"; do
    FLAGS=( "${FLAGS[@]}" --tag="${PACKAGE}:${ARCH}-${tag}" )
  done

  echo "> docker build ${FLAGS[*]} ."
  docker build "${FLAGS[@]}" .

  for tag in "${TAGS[@]}"; do
    echo "> docker push ${PACKAGE}:${ARCH}-${tag}"
    docker push "${PACKAGE}:${ARCH}-${tag}"
  done
}

build_for_arch amd64
build_for_arch arm64v8

for tag in "${TAGS[@]}"; do
  docker manifest create "${PACKAGE}:${tag}" \
    --amend "${PACKAGE}:amd64-${tag}" \
    --amend "${PACKAGE}:arm64v8-${tag}"
  docker manifest push "${PACKAGE}:${tag}"
done
