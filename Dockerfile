FROM golang:1.16.3-alpine3.13 AS builder
RUN ["apk", "add", "--no-cache", "libcap", "ca-certificates"]
RUN ["/bin/sh", "-c", "update-ca-certificates 2>/dev/null || true"]
RUN ["/bin/sh", "-c", "echo 'hosts: files dns' > /etc/nsswitch.conf"]
WORKDIR /build
COPY ./ ./
RUN ["go", "get", "-d", "-v", "./..."]
RUN ["go", "install", "-v", "./..."]
RUN ["setcap", "cap_net_bind_service=+ep", "/go/bin/roxy"]
RUN ["addgroup", "-S", "roxy", "-g", "400"]
RUN ["adduser", "-S", "roxy", "-u", "400", "-G", "roxy", "-h", "/var/opt/roxy/lib", "-H", "-D"]
RUN ["mkdir", "-p", "/etc/roxy", "/opt/roxy/share/misc", "/var/opt/roxy/lib/acme"]
RUN ["cp", "/build/config.json.example", "/etc/roxy/config.json"]
RUN ["cp", "/build/mime.json.example", "/opt/roxy/share/misc/mime.json"]
RUN ["chown", "roxy:roxy", "/var/opt/roxy/lib", "/var/opt/roxy/lib/acme"]
RUN ["chmod", "0750", "/var/opt/roxy/lib", "/var/opt/roxy/lib/acme"]

FROM alpine:3.13 AS final
COPY --from=builder /opt/roxy/ /opt/roxy/
COPY --from=builder /go/bin/ /opt/roxy/bin/
COPY --from=builder /var/opt/roxy/ /var/opt/roxy/
COPY --from=builder /etc/passwd /etc/group /etc/nsswitch.conf /etc/
COPY --from=builder /etc/roxy/ /etc/roxy/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENV PATH /opt/roxy/bin:$PATH
USER roxy:roxy
WORKDIR /var/opt/roxy/lib
EXPOSE 80/tcp 443/tcp
CMD ["roxy", "-S", "-c", "/etc/roxy/config.json"]
