FROM golang:1.16.3-alpine
WORKDIR /build
COPY . .
RUN ["go", "get", "-d", "-v", "./..."]
RUN ["go", "install", "-v", "./..."]
RUN ["/bin/sh", "-c", "apk add --no-cache libcap && addgroup -S roxy && adduser -S roxy -G roxy -h /var/lib/roxy -H -D && mkdir -p /etc/roxy /var/lib/roxy/acme && cp /build/config.json.example /etc/roxy/config.json && chown roxy:roxy /etc/roxy /etc/roxy/config.json /var/lib/roxy /var/lib/roxy/acme && chmod 0750 /var/lib/roxy /var/lib/roxy/acme && setcap cap_net_bind_service=+ep /go/bin/roxy"]
USER roxy:roxy
WORKDIR /var/lib/roxy
EXPOSE 80/tcp 443/tcp
CMD ["roxy", "-S", "-c", "/etc/roxy/config.json"]
