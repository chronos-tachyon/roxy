#!/bin/bash
set -euo pipefail

export LC_ALL="C"
export TZ="Etc/UTC"

INSTALL_DIR="${1?}"
FULL_VERSION="${2?}"
export GOOS="${3?}"
export GOARCH="${4?}"
export CGO_ENABLED="0"

cd "$(dirname $(dirname "$0"))"
umask 022

at_exit() {
  local -i rc=$?
  local item
  for item in "${DELETE_ON_EXIT[@]}"; do
    if [ -e "$item" ]; then
      sudo chmod -R u+w -- "$item"
      sudo rm -rf -- "$item"
    fi
  done
  return $rc
}

declare -a DELETE_ON_EXIT=( : )
trap 'at_exit' EXIT

DEBARCH="${GOARCH}"

if [ "${TEMPORARY_GOPATH:-true}" = "true" ]; then
  GOPATH="$(mktemp -td "roxy.build_deb.${GOOS}.${GOARCH}.$$.XXXXXXXXXX")"
  DELETE_ON_EXIT=( "${DELETE_ON_EXIT[@]}" "$GOPATH" )
fi

if [ "${NO_SUDO_REQUIRED:-false}" = "true" ]; then
  sudo() { "$@"; }
fi

echo "> Building ${FULL_VERSION} for ${GOOS}/${GOARCH}..."
echo "$FULL_VERSION" > lib/mainutil/version.txt

mkdir -p \
  "${INSTALL_DIR}/DEBIAN" \
  "${INSTALL_DIR}/etc/default" \
  "${INSTALL_DIR}/etc/logrotate.d" \
  "${INSTALL_DIR}/etc/opt/roxy" \
  "${INSTALL_DIR}/etc/opt/atc" \
  "${INSTALL_DIR}/etc/systemd/system" \
  "${INSTALL_DIR}/opt/roxy/bin" \
  "${INSTALL_DIR}/opt/roxy/share/examples" \
  "${INSTALL_DIR}/opt/roxy/share/templates" \
  "${INSTALL_DIR}/var/opt/roxy/lib/acme" \
  "${INSTALL_DIR}/var/opt/roxy/lib/state" \
  "${INSTALL_DIR}/var/opt/roxy/log"

echo '> go get -d ./...'
echo '::group::go get'
go get -d ./...
echo '::endgroup::'

echo '> go install ./...'
echo '::group::go install'
go install -v ./...
echo '::endgroup::'

echo '> mv -t .../opt/roxy/bin (binaries)'
SRCBIN="${GOPATH}/bin"
SRCARCHBIN="${SRCBIN}/${GOOS}_${GOARCH}"
DSTBIN="${INSTALL_DIR}/opt/roxy/bin"
if [ -d "$SRCARCHBIN" ]; then
  mv -t "$DSTBIN" "$SRCARCHBIN"/*
  rmdir "$SRCARCHBIN"
else
  mv -t "$DSTBIN" "$SRCBIN"/*
fi

setcap cap_net_bind_service=+ep "${INSTALL_DIR}/opt/roxy/bin/roxy"

echo '> cp -t .../etc/opt/roxy (additional files)'
cp templates/index.html "${INSTALL_DIR}/opt/roxy/share/templates/index.html"
cp templates/redir.html "${INSTALL_DIR}/opt/roxy/share/templates/redir.html"
cp templates/error.html "${INSTALL_DIR}/opt/roxy/share/templates/error.html"
cp dist/roxy.config.json "${INSTALL_DIR}/opt/roxy/share/examples/roxy.config.json"
cp dist/roxy.config.json "${INSTALL_DIR}/etc/opt/roxy/config.json"
cp dist/roxy.mime.json "${INSTALL_DIR}/opt/roxy/share/examples/roxy.mime.json"
cp dist/roxy.mime.json "${INSTALL_DIR}/etc/opt/roxy/mime.json"
cp dist/atc.global.json "${INSTALL_DIR}/opt/roxy/share/examples/atc.global.json"
cp dist/atc.global.json "${INSTALL_DIR}/etc/opt/atc/global.json"
cp dist/atc.peers.json "${INSTALL_DIR}/opt/roxy/share/examples/atc.peers.json"
cp dist/atc.peers.json "${INSTALL_DIR}/etc/opt/atc/peers.json"
cp dist/atc.services.json "${INSTALL_DIR}/opt/roxy/share/examples/atc.services.json"
cp dist/atc.services.json "${INSTALL_DIR}/etc/opt/atc/services.json"
cp dist/atc.cost.json "${INSTALL_DIR}/opt/roxy/share/examples/atc.cost.json"
cp dist/atc.cost.json "${INSTALL_DIR}/etc/opt/atc/cost.json"
cp dist/logrotate.conf "${INSTALL_DIR}/opt/roxy/share/examples/logrotate.conf"
cp dist/logrotate.conf "${INSTALL_DIR}/etc/logrotate.d/roxy"
cp dist/roxy.default "${INSTALL_DIR}/etc/default/roxy"
cp dist/roxy.service "${INSTALL_DIR}/etc/systemd/system/roxy.service"
cp dist/atc.default "${INSTALL_DIR}/etc/default/atc"
cp dist/atc.service "${INSTALL_DIR}/etc/systemd/system/atc.service"

declare -a topleveldirs
topleveldirs=( etc opt var )

echo '> tar -cf .../data.tar'
tar \
  --create \
  --file="${INSTALL_DIR}/data.tar" \
  --xattrs \
  --mtime='2022-01-01 00:00:00' \
  --mode='u=rwX,go=rX' \
  --owner=root \
  --group=root \
  --sort=name \
  --directory="$INSTALL_DIR" \
  "${topleveldirs[@]}"
declare -i INSTALLED_SIZE_BYTES="$(stat -c %s "${INSTALL_DIR}/data.tar")"
declare -i INSTALLED_SIZE_KB=$(( (INSTALLED_SIZE_BYTES + 1023) / 1024 ))
echo '> rm -f .../data.tar'
rm -f "${INSTALL_DIR}/data.tar"

echo '> md5sum (installed files) > .../DEBIAN/md5sums'
( cd "$INSTALL_DIR" && find "${topleveldirs[@]}" -type f -print0 | sort -z | xargs -0 md5sum ) > "${INSTALL_DIR}/DEBIAN/md5sums"
echo '> cp (additional files) .../DEBIAN/'
sed \
  -e "s|%VERSION%|${FULL_VERSION}|g" \
  -e "s|%ARCH%|${DEBARCH}|g" \
  -e "s|%SIZE%|${INSTALLED_SIZE_KB}|g" \
  < debian/control.template > "${INSTALL_DIR}/DEBIAN/control"
for f in conffiles preinst postinst prerm postrm; do
  if [ -e "debian/${f}" ]; then
    cp "debian/${f}" "${INSTALL_DIR}/DEBIAN/${f}"
  fi
done

echo '> chmod -R u=rwX,go=rX ...'
chmod -R u=rwX,go=rX "$INSTALL_DIR"

echo '> sudo chown -Rh root:root ...'
sudo chown -Rh root:root "$INSTALL_DIR"

echo '> find ... | xargs touch -d 2022-01-01'
find "$INSTALL_DIR" -print0 | sudo xargs -0 touch -d '2022-01-01 00:00:00' --
