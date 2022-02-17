#!/bin/bash
set -euo pipefail
umask 022

TARGETPLATFORM="${1?}"
VERSION="${2?}"

case "$TARGETPLATFORM" in
  (linux/amd64)   export GOOS=linux GOARCH=amd64 ;;
  (linux/arm64v8) export GOOS=linux GOARCH=arm64 ;;
  (*) echo "error: unknown platform $TARGETPLATFORM" >&1; exit 1 ;;
esac

set -x

cd /

/usr/bin/apt-get update
/usr/bin/apt-get install -yq --no-install-recommends --no-install-suggests ca-certificates libcap2-bin
update-ca-certificates 2>/dev/null || true
echo 'hosts: files dns' > /etc/nsswitch.conf

/build/scripts/build_in_tree.sh /target "$VERSION" "$GOOS" "$GOARCH"

setcap cap_net_bind_service=+ep /target/opt/roxy/bin/roxy

addgroup --system --gid 400 roxy
adduser --system --uid 400 --gid 400 --gecos roxy --home /var/opt/roxy/lib --no-create-home --disabled-login roxy

mkdir -p /target/etc/ssl/certs /target/srv/www
cp -t /target/etc /etc/passwd /etc/group /etc/nsswitch.conf
cp -t /target/etc/ssl/certs /etc/ssl/certs/ca-certificates.crt

chown roxy:roxy /target/var/opt/roxy/lib /target/var/opt/roxy/lib/acme /target/var/opt/roxy/lib/state
chown roxy:adm  /target/var/opt/roxy/log
chmod 0640 /target/etc/opt/roxy/config.json
chmod 0750 /target/var/opt/roxy/lib /target/var/opt/roxy/lib/acme /target/var/opt/roxy/lib/state
chmod 2750 /target/var/opt/roxy/log

find /target -print0 | xargs -0 touch -d '2022-01-01 00:00:00' --
