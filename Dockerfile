# vim:set ft=dockerfile:
ARG VERSION=unset
ARG ARCH

FROM --platform=$BUILDPLATFORM golang:1.16-alpine3.13 AS builder
ARG VERSION
ARG TARGETPLATFORM
RUN ["apk", "add", "--no-cache", "libcap", "ca-certificates"]
RUN ["/bin/sh", "-c", "update-ca-certificates 2>/dev/null || true"]
RUN ["/bin/sh", "-c", "echo 'hosts: files dns' > /etc/nsswitch.conf"]
WORKDIR /build
COPY ./ ./
RUN set -euo pipefail; \
    umask 022; \
    export LC_ALL=C TZ=Etc/UTC; \
    export GOPATH=/go CGO_ENABLED=0; \
    case "$TARGETPLATFORM" in \
      (linux/amd64)   export GOOS=linux GOARCH=amd64 ;; \
      (linux/arm64v8) export GOOS=linux GOARCH=arm64 ;; \
      (*) echo "error: unknown platform $TARGETPLATFORM" >&1; exit 1;; \
    esac; \
    if [ "${VERSION}" = "unset" ]; then \
      cat .version > lib/mainutil/version.txt; \
    else \
      echo "${VERSION}" > lib/mainutil/version.txt; \
    fi; \
    echo '::group::go get'; \
    go get -d ./...; \
    echo; echo '::endgroup::'; \
    echo '::group::go install'; \
    go install -v ./...; \
    echo; echo '::endgroup::'; \
    mv /go/pkg /junk; \
    chmod -R a+rX,u+w,go-w /build /go/bin; \
    if [ -d /go/bin/${GOOS}_${GOARCH} ]; then \
      mv /go/bin/${GOOS}_${GOARCH}/* /go/bin; \
      rmdir /go/bin/${GOOS}_${GOARCH}; \
    fi; \
    setcap cap_net_bind_service=+ep /go/bin/roxy; \
    addgroup -S roxy -g 400; \
    adduser -S roxy -u 400 -G roxy -h /var/opt/roxy/lib -H -D; \
    mkdir -p /etc/opt/roxy /etc/opt/atc /opt/roxy/share/misc /opt/roxy/share/templates /var/opt/roxy/lib/acme /var/opt/roxy/lib/state /var/opt/roxy/log /srv/www; \
    cp /build/templates/* /opt/roxy/share/templates/; \
    cp /build/dist/roxy.config.json /opt/roxy/share/misc/roxy.config.json.example; \
    cp /build/dist/roxy.config.json /etc/opt/roxy/config.json.example; \
    cp /build/dist/roxy.config.json /etc/opt/roxy/config.json; \
    cp /build/dist/roxy.mime.json /opt/roxy/share/misc/roxy.mime.json.example; \
    cp /build/dist/roxy.mime.json /etc/opt/roxy/mime.json.example; \
    cp /build/dist/roxy.mime.json /etc/opt/roxy/mime.json; \
    cp /build/dist/atc.global.json /opt/roxy/share/misc/atc.global.json.example; \
    cp /build/dist/atc.global.json /etc/opt/atc/global.json.example; \
    cp /build/dist/atc.global.json /etc/opt/atc/global.json; \
    cp /build/dist/atc.peers.json /opt/roxy/share/misc/atc.peers.json.example; \
    cp /build/dist/atc.peers.json /etc/opt/atc/peers.json.example; \
    cp /build/dist/atc.peers.json /etc/opt/atc/peers.json; \
    cp /build/dist/atc.services.json /opt/roxy/share/misc/atc.services.json.example; \
    cp /build/dist/atc.services.json /etc/opt/atc/services.json.example; \
    cp /build/dist/atc.services.json /etc/opt/atc/services.json; \
    cp /build/dist/atc.cost.json /opt/roxy/share/misc/atc.cost.json.example; \
    cp /build/dist/atc.cost.json /etc/opt/atc/cost.json.example; \
    cp /build/dist/atc.cost.json /etc/opt/atc/cost.json; \
    find /etc/opt/roxy /etc/opt/atc /opt/roxy /var/opt/roxy /srv/www -print0 | xargs -0 touch -d '2000-01-01 00:00:00' --; \
    chown root:roxy /etc/opt/roxy/config.json /etc/opt/roxy/config.json.example; \
    chown root:roxy /var/opt/roxy; \
    chown roxy:roxy /var/opt/roxy/lib /var/opt/roxy/lib/acme /var/opt/roxy/lib/state; \
    chown roxy:adm  /var/opt/roxy/log; \
    chmod 0640 /etc/opt/roxy/config.json /etc/opt/roxy/config.json.example; \
    chmod 0750 /var/opt/roxy/lib /var/opt/roxy/lib/acme /var/opt/roxy/lib/state; \
    chmod 2750 /var/opt/roxy/log

FROM --platform=$TARGETPLATFORM ${ARCH}/alpine:3.13 AS final
COPY --from=builder /srv/ /srv/
COPY --from=builder /etc/passwd /etc/group /etc/nsswitch.conf /etc/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/opt/roxy/ /etc/opt/roxy/
COPY --from=builder /var/opt/roxy/ /var/opt/roxy/
COPY --from=builder /opt/roxy/ /opt/roxy/
COPY --from=builder /go/bin/ /opt/roxy/bin/
ENV PATH /opt/roxy/bin:$PATH
USER roxy:roxy
WORKDIR /var/opt/roxy/lib
EXPOSE 80/tcp 443/tcp
CMD ["roxy", "-S"]
