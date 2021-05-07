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

OS_ARCH_LIST=( linux/amd64 linux/arm64 )

arch_from_goos_goarch() {
  local GOOS="$1"
  local GOARCH="$2"
  case "${GOOS}/${GOARCH}" in
    (linux/arm64)
      echo arm64v8
      ;;
    (linux/*)
      echo "$GOARCH"
      ;;
    (*)
      echo "error: ${GOOS}/${GOARCH} not implemented" >&2
      exit 1
      ;;
  esac
}

build_for_os_arch() {
  local goos="$1"
  local goarch="$2"

  local dockerfile="Dockerfile"
  local arch="$(arch_from_goos_goarch "$goos" "$goarch")"

  declare -a flags
  flags=(
    --file="$dockerfile"
    --build-arg=GOOS="$goos"
    --build-arg=GOARCH="$goarch"
    --build-arg=ARCH="$arch"
    --build-arg=VERSION="$FULL_VERSION"
  )
  for tag in "${TAGS[@]}"; do
    flags=( "${flags[@]}" --tag="${PACKAGE}:${arch}-${tag}" )
  done

  echo "> docker build ${flags[*]} ."
  docker build "${flags[@]}" .

  local tag
  for tag in "${TAGS[@]}"; do
    echo "> docker push ${PACKAGE}:${arch}-${tag}"
    docker push "${PACKAGE}:${arch}-${tag}"
  done
}

for OS_ARCH in linux/amd64 linux/arm64; do
  GOOS="${OS_ARCH%/*}"
  GOARCH="${OS_ARCH#*/}"
  build_for_os_arch "$GOOS" "$GOARCH"
done

for tag in "${TAGS[@]}"; do
  ARGS=( "${PACKAGE}:${tag}" )
  for OS_ARCH in linux/amd64 linux/arm64; do
    GOOS="${OS_ARCH%/*}"
    GOARCH="${OS_ARCH#*/}"
    ARCH="$(arch_from_goos_goarch "$GOOS" "$GOARCH")"
    ARGS=( "${ARGS[@]}" --amend "${PACKAGE}:${ARCH}-${tag}" )
  done
  docker manifest create "${ARGS[@]}"
  docker manifest push "${PACKAGE}:${tag}"
done
