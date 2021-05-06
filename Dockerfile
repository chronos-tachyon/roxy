# vim:set ft=dockerfile:

FROM golang:1.16-alpine3.13 AS builder
ARG version=unset
RUN ["apk", "add", "--no-cache", "libcap", "ca-certificates"]
RUN ["/bin/sh", "-c", "update-ca-certificates 2>/dev/null || true"]
RUN ["/bin/sh", "-c", "echo 'hosts: files dns' > /etc/nsswitch.conf"]
WORKDIR /build
COPY ./ ./
RUN set -euo pipefail; \
    umask 022; \
    export GOPATH=/go GOOS=linux GOARCH=amd64 CGO_ENABLED=0; \
    if [ "${version}" = "unset" ]; then \
      cat .version > lib/mainutil/version.txt; \
    else \
      echo "${version}" > lib/mainutil/version.txt; \
    fi; \
    go get -d ./...; \
    go install ./...; \
    mv /go/pkg /junk; \
    chmod -R a+rX,u+w,go-w /build /go/bin; \
    setcap cap_net_bind_service=+ep /go/bin/roxy; \
    addgroup -S roxy -g 400; \
    adduser -S roxy -u 400 -G roxy -h /var/opt/roxy/lib -H -D; \
    mkdir -p /etc/opt/roxy /opt/roxy/share/misc /opt/roxy/share/templates /var/opt/roxy/lib/acme /var/opt/roxy/log /srv/www; \
    cp /build/templates/* /opt/roxy/share/templates/; \
    cp /build/config.json.example /opt/roxy/share/misc/config.json.example; \
    cp /build/config.json.example /etc/opt/roxy/config.json.example; \
    cp /build/config.json.example /etc/opt/roxy/config.json; \
    cp /build/mime.json.example /opt/roxy/share/misc/mime.json.example; \
    cp /build/mime.json.example /etc/opt/roxy/mime.json.example; \
    cp /build/mime.json.example /etc/opt/roxy/mime.json; \
    chown root:roxy /etc/opt/roxy/config.json /etc/opt/roxy/config.json.example; \
    chown root:roxy /var/opt/roxy; \
    chown roxy:roxy /var/opt/roxy/lib /var/opt/roxy/lib/acme; \
    chown roxy:adm  /var/opt/roxy/log; \
    chmod 0640 /etc/opt/roxy/config.json /etc/opt/roxy/config.json.example; \
    chmod 0750 /var/opt/roxy/lib /var/opt/roxy/lib/acme; \
    chmod 2750 /var/opt/roxy/log

FROM alpine:3.13 AS final
COPY --from=builder /opt/roxy/ /opt/roxy/
COPY --from=builder /go/bin/ /opt/roxy/bin/
COPY --from=builder /var/opt/roxy/ /var/opt/roxy/
COPY --from=builder /etc/opt/roxy/ /etc/opt/roxy/
COPY --from=builder /etc/passwd /etc/group /etc/nsswitch.conf /etc/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /srv/ /srv/
ENV PATH /opt/roxy/bin:$PATH
USER roxy:roxy
WORKDIR /var/opt/roxy/lib
EXPOSE 80/tcp 443/tcp
CMD ["roxy", "-S", "-c", "/etc/opt/roxy/config.json"]
