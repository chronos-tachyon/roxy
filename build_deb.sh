#!/bin/bash
set -euo pipefail

export LC_ALL="C"
export TZ="Etc/UTC"

cd "$(dirname "$0")"
umask 022

FULL_VERSION="$(./scripts/next_build_version.sh roxy-deb)"

at_exit() {
  local item
  for item in "${DELETE_ON_EXIT[@]}"; do
    if [ -e "$item" ]; then
      sudo chmod -R u+w -- "$item"
      sudo rm -rf -- "$item"
    fi
  done
}

declare -a DELETE_ON_EXIT
trap 'at_exit' EXIT

SRCDIR="$(mktemp -td "roxy.build_deb.source.$$.XXXXXXXXXX")"
GOPATH="$(mktemp -td "roxy.build_deb.gopath.$$.XXXXXXXXXX")"
INSTALL_DIR_LINUX_AMD64="$(mktemp -td "roxy.build_deb.linux.amd64.$$.XXXXXXXXXX")"
INSTALL_DIR_LINUX_ARM64="$(mktemp -td "roxy.build_deb.linux.arm64.$$.XXXXXXXXXX")"
DELETE_ON_EXIT=( "$SRCDIR" "$GOPATH" "$INSTALL_DIR_LINUX_AMD64" "$INSTALL_DIR_LINUX_ARM64" )
export GOPATH
export TEMPORARY_GOPATH="false"

echo '> cp -at SRCDIR .'
cp -at "$SRCDIR" ./*

"${SRCDIR}/scripts/build_in_tree.sh" "$INSTALL_DIR_LINUX_AMD64" "$FULL_VERSION" linux amd64

"${SRCDIR}/scripts/build_in_tree.sh" "$INSTALL_DIR_LINUX_ARM64" "$FULL_VERSION" linux arm64

DEBFILE_LINUX_AMD64="roxy_${FULL_VERSION}_amd64.deb"
echo "> dpkg-deb -b ... ${DEBFILE_LINUX_AMD64}"
dpkg-deb -b "$INSTALL_DIR_LINUX_AMD64" "$DEBFILE_LINUX_AMD64"

DEBFILE_LINUX_ARM64="roxy_${FULL_VERSION}_arm64.deb"
echo "> dpkg-deb -b ... ${DEBFILE_LINUX_ARM64}"
dpkg-deb -b "$INSTALL_DIR_LINUX_ARM64" "$DEBFILE_LINUX_ARM64"
