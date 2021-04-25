#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
VERSION="$(cat .version)"
DATESTAMP="$(date --utc +%Y.%m.%d)"
LASTBUILDDIR="${HOME}/.cache/last-build"
LASTBUILDFILE="${LASTBUILDDIR}/roxy-deb"
mkdir -p "$LASTBUILDDIR"
if [ ! -e "$LASTBUILDFILE" ]; then
  echo 0 > "$LASTBUILDFILE"
fi
LAST_COUNTER="$(cat "$LASTBUILDFILE")"
NEXT_COUNTER=$((LAST_COUNTER + 1))
echo "$NEXT_COUNTER" > "$LASTBUILDFILE"
FULL_VERSION="${VERSION}-r${DATESTAMP}-${NEXT_COUNTER}"

umask 022

export LC_ALL="C"
export TZ="Etc/UTC"

build_for_os_arch() {
  export GOOS="${1?}"
  export GOARCH="${2?}"

  DEBARCH="${GOARCH}"

  echo "> Building for ${GOOS}/${GOARCH}..."

  BUILDDIR="$(mktemp -td "roxy.build_deb.${GOARCH}.$$.XXXXXXXXXX")"
  trap 'sudo rm -rf "$BUILDDIR"' EXIT

  export GOPATH="${BUILDDIR}/opt/roxy"

  mkdir -p \
    "${BUILDDIR}/DEBIAN" \
    "${BUILDDIR}/opt/roxy/bin" \
    "${BUILDDIR}/opt/roxy/share/misc" \
    "${BUILDDIR}/etc/opt/roxy" \
    "${BUILDDIR}/var/opt/roxy"

  TOPLEVELDIRS=( opt etc var )

  echo '> go get -d ./...'
  go get -d ./...
  echo '> go install ./...'
  go install ./...
  echo '> rm -rf .../opt/roxy/pkg'
  chmod -R u+w "${BUILDDIR}/opt/roxy/pkg"
  rm -rf "${BUILDDIR}/opt/roxy/pkg"
  echo '> cp (additional files) .../etc/opt/roxy/'
  cp config.json.example "${BUILDDIR}/etc/opt/roxy/config.json.example"
  cp config.json.example "${BUILDDIR}/etc/opt/roxy/config.json"

  echo '> tar -cf .../data.tar'
  tar \
    --create \
    --file="${BUILDDIR}/data.tar" \
    --xattrs \
    --mtime='1980-01-01 00:00:00' \
    --mode='a+rX,u+w,go-w' \
    --owner=root \
    --group=root \
    --sort=name \
    --directory="$BUILDDIR" \
    "${TOPLEVELDIRS[@]}"
  INSTALLED_SIZE_BYTES="$(stat -c %s "${BUILDDIR}/data.tar")"
  INSTALLED_SIZE_KB=$(( (INSTALLED_SIZE_BYTES + 1023) / 1024 ))
  echo '> rm -f .../data.tar'
  rm -f "${BUILDDIR}/data.tar"

  CONTROLFILES=( control md5sums )
  echo '> md5sum (installed files) > .../DEBIAN/md5sums'
  ( cd "$BUILDDIR" && find "${TOPLEVELDIRS[@]}" -type f -print0 | sort -z | xargs -0 md5sum ) > "${BUILDDIR}/DEBIAN/md5sums"
  echo '> cp (additional files) .../DEBIAN/'
  sed \
    -e "s|%VERSION%|${FULL_VERSION}|g" \
    -e "s|%ARCH%|${DEBARCH}|g" \
    -e "s|%SIZE%|${INSTALLED_SIZE_KB}|g" \
    < debian/control.template > "${BUILDDIR}/DEBIAN/control"
  for f in conffiles preinst postinst prerm postrm; do
    if [ -e "debian/${f}" ]; then
      CONTROLFILES=( "${CONTROLFILES[@]}" "$f" )
      cp "debian/${f}" "${BUILDDIR}/DEBIAN/${f}"
    fi
  done

  echo '> chmod -R a+rX,u+w,go-w ...'
  chmod -R a+rX,u+w,go-w "$BUILDDIR"
  echo '> sudo chown -Rh root:root ...'
  sudo chown -Rh root:root "$BUILDDIR"

  DEBFILE="roxy_${FULL_VERSION}_${DEBARCH}.deb"
  echo "> dpkg-deb -b ... ${DEBFILE}"
  dpkg-deb -b "$BUILDDIR" "$DEBFILE"

  trap '' EXIT
  sudo rm -rf "$BUILDDIR"
}

build_for_os_arch linux amd64
build_for_os_arch linux arm64
