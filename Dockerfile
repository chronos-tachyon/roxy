# vim:set ft=dockerfile:
ARG ARCH=amd64

FROM golang:1.16-alpine3.13 AS builder
ARG VERSION=unset
ARG GOOS=linux
ARG GOARCH=amd64
RUN ["apk", "add", "--no-cache", "libcap", "ca-certificates"]
RUN ["/bin/sh", "-c", "update-ca-certificates 2>/dev/null || true"]
RUN ["/bin/sh", "-c", "echo 'hosts: files dns' > /etc/nsswitch.conf"]
WORKDIR /build
COPY ./ ./
RUN set -euo pipefail; \
    umask 022; \
    export GOPATH=/go GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0; \
    if [ "${VERSION}" = "unset" ]; then \
      cat .version > lib/mainutil/version.txt; \
    else \
      echo "${VERSION}" > lib/mainutil/version.txt; \
    fi; \
    go get -d ./...; \
    go install ./...; \
    mv /go/pkg /junk; \
    chmod -R a+rX,u+w,go-w /build /go/bin; \
    if [ -d /go/bin/${GOOS}_${GOARCH} ]; then \
      mv /go/bin/${GOOS}_${GOARCH}/* /go/bin; \
      rmdir /go/bin/${GOOS}_${GOARCH}; \
    fi; \
    setcap cap_net_bind_service=+ep /go/bin/roxy; \
    addgroup -S roxy -g 400; \
    adduser -S roxy -u 400 -G roxy -h /var/opt/roxy/lib -H -D; \
    mkdir -p /etc/opt/roxy /opt/roxy/share/misc /opt/roxy/share/templates /var/opt/roxy/lib/acme /var/opt/roxy/log /srv/www; \
    cp /build/templates/* /opt/roxy/share/templates/; \
    cp /build/dist/config.json /opt/roxy/share/misc/config.json.example; \
    cp /build/dist/config.json /etc/opt/roxy/config.json.example; \
    cp /build/dist/config.json /etc/opt/roxy/config.json; \
    cp /build/dist/mime.json /opt/roxy/share/misc/mime.json.example; \
    cp /build/dist/mime.json /etc/opt/roxy/mime.json.example; \
    cp /build/dist/mime.json /etc/opt/roxy/mime.json; \
    chown root:roxy /etc/opt/roxy/config.json /etc/opt/roxy/config.json.example; \
    chown root:roxy /var/opt/roxy; \
    chown roxy:roxy /var/opt/roxy/lib /var/opt/roxy/lib/acme; \
    chown roxy:adm  /var/opt/roxy/log; \
    chmod 0640 /etc/opt/roxy/config.json /etc/opt/roxy/config.json.example; \
    chmod 0750 /var/opt/roxy/lib /var/opt/roxy/lib/acme; \
    chmod 2750 /var/opt/roxy/log

FROM ${ARCH}/alpine:3.13 AS final
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
