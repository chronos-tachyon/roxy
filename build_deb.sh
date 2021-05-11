#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

umask 022

export LC_ALL="C"
export TZ="Etc/UTC"

if [ "${RELEASE_MODE:-false}" = "true" ]; then
  FULL_VERSION="$GITHUB_REF"
  FULL_VERSION="${FULL_VERSION##*/v}"
else
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
fi

OUTDIR="$(pwd)"
SRCDIR="$(mktemp -td "roxy.build_deb.source.$$.XXXXXXXXXX")"
DELETE_ON_EXIT=( "$SRCDIR" )
trap 'sudo rm -rf "${DELETE_ON_EXIT[@]}"' EXIT

cp -a ./* "${SRCDIR}/"
echo "$FULL_VERSION" > "${SRCDIR}/lib/mainutil/version.txt"
cd "$SRCDIR"

build_for_os_arch() {
  export GOOS="${1?}"
  export GOARCH="${2?}"

  DEBARCH="${GOARCH}"

  echo "> Building for ${GOOS}/${GOARCH}..."

  BUILDDIR="$(mktemp -td "roxy.build_deb.${GOARCH}.$$.XXXXXXXXXX")"
  DELETE_ON_EXIT=( "${DELETE_ON_EXIT[@]}" "$BUILDDIR" )

  export GOPATH="${BUILDDIR}/opt/roxy"
  export CGO_ENABLED="0"

  mkdir -p \
    "${BUILDDIR}/DEBIAN" \
    "${BUILDDIR}/etc/default" \
    "${BUILDDIR}/etc/logrotate.d" \
    "${BUILDDIR}/etc/opt/roxy" \
    "${BUILDDIR}/etc/opt/atc" \
    "${BUILDDIR}/etc/systemd/system" \
    "${BUILDDIR}/opt/roxy/bin" \
    "${BUILDDIR}/opt/roxy/share/misc" \
    "${BUILDDIR}/opt/roxy/share/templates" \
    "${BUILDDIR}/var/opt/roxy/lib/acme" \
    "${BUILDDIR}/var/opt/roxy/log"

  TOPLEVELDIRS=( etc opt var )

  echo '> go get -d ./...'
  echo '::group::go get'
  go get -d ./...
  echo '::endgroup::'

  echo '> go install ./...'
  go install ./...

  echo '> rm -rf .../opt/roxy/pkg'
  chmod -R u+w "${BUILDDIR}/opt/roxy/pkg"
  rm -rf "${BUILDDIR}/opt/roxy/pkg"

  echo '> cp (additional files) .../etc/opt/roxy/'
  cp templates/index.html "${BUILDDIR}/opt/roxy/share/templates/index.html"
  cp templates/redir.html "${BUILDDIR}/opt/roxy/share/templates/redir.html"
  cp templates/error.html "${BUILDDIR}/opt/roxy/share/templates/error.html"
  cp dist/roxy.config.json "${BUILDDIR}/opt/roxy/share/misc/roxy.config.json.example"
  cp dist/roxy.config.json "${BUILDDIR}/etc/opt/roxy/config.json.example"
  cp dist/roxy.config.json "${BUILDDIR}/etc/opt/roxy/config.json"
  cp dist/roxy.mime.json "${BUILDDIR}/opt/roxy/share/misc/roxy.mime.json.example"
  cp dist/roxy.mime.json "${BUILDDIR}/etc/opt/roxy/mime.json.example"
  cp dist/roxy.mime.json "${BUILDDIR}/etc/opt/roxy/mime.json"
  cp dist/atc.config.json "${BUILDDIR}/opt/roxy/share/misc/atc.config.json.example"
  cp dist/atc.config.json "${BUILDDIR}/etc/opt/atc/config.json.example"
  cp dist/atc.config.json "${BUILDDIR}/etc/opt/atc/config.json"
  cp dist/atc.main.json "${BUILDDIR}/opt/roxy/share/misc/atc.main.json.example"
  cp dist/atc.main.json "${BUILDDIR}/etc/opt/atc/main.json.example"
  cp dist/atc.main.json "${BUILDDIR}/etc/opt/atc/main.json"
  cp dist/atc.cost.json "${BUILDDIR}/opt/roxy/share/misc/atc.cost.json.example"
  cp dist/atc.cost.json "${BUILDDIR}/etc/opt/atc/cost.json.example"
  cp dist/atc.cost.json "${BUILDDIR}/etc/opt/atc/cost.json"
  cp dist/logrotate.conf "${BUILDDIR}/opt/roxy/share/misc/logrotate.conf"
  cp dist/logrotate.conf "${BUILDDIR}/etc/logrotate.d/roxy"
  cp dist/roxy.default "${BUILDDIR}/etc/default/roxy"
  cp dist/roxy.service "${BUILDDIR}/etc/systemd/system/roxy.service"
  cp dist/atc.default "${BUILDDIR}/etc/default/atc"
  cp dist/atc.service "${BUILDDIR}/etc/systemd/system/atc.service"

  echo '> tar -cf .../data.tar'
  tar \
    --create \
    --file="${BUILDDIR}/data.tar" \
    --xattrs \
    --mtime='2000-01-01 00:00:00' \
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
  echo '> find ... | xargs touch -d 2000-01-01'
  find "$BUILDDIR" -print0 | sudo xargs -0 touch -d '2000-01-01 00:00:00' --

  DEBFILE="roxy_${FULL_VERSION}_${DEBARCH}.deb"
  echo "> dpkg-deb -b ... ${DEBFILE}"
  dpkg-deb -b "$BUILDDIR" "${OUTDIR}/${DEBFILE}"
}

build_for_os_arch linux amd64
build_for_os_arch linux arm64
