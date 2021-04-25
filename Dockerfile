FROM golang:1.16.3-alpine3.13 AS builder
RUN ["apk", "add", "--no-cache", "libcap", "ca-certificates"]
RUN ["/bin/sh", "-c", "update-ca-certificates 2>/dev/null || true"]
WORKDIR /build
COPY ./ ./
RUN ["go", "get", "-d", "-v", "./..."]
RUN ["go", "install", "-v", "./..."]
RUN ["setcap", "cap_net_bind_service=+ep", "/go/bin/roxy"]
RUN ["addgroup", "-S", "roxy", "-g", "400"]
RUN ["adduser", "-S", "roxy", "-u", "400", "-G", "roxy", "-h", "/var/lib/roxy", "-H", "-D"]
RUN ["mkdir", "-p", "/etc/roxy", "/var/lib/roxy/acme"]
RUN ["cp", "/build/config.json.example", "/etc/roxy/config.json"]
RUN ["chown", "roxy:roxy", "/etc/roxy", "/etc/roxy/config.json", "/var/lib/roxy", "/var/lib/roxy/acme"]
RUN ["chmod", "0750", "/var/lib/roxy", "/var/lib/roxy/acme"]

FROM alpine:3.13 AS final
COPY --from=builder /go/bin/ /go/bin/
COPY --from=builder /etc/passwd /etc/group /etc/
COPY --from=builder /etc/roxy/ /etc/roxy/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /var/lib/roxy/ /var/lib/roxy/
ENV PATH /go/bin:$PATH
USER roxy:roxy
WORKDIR /var/lib/roxy
EXPOSE 80/tcp 443/tcp
CMD ["roxy", "-S", "-c", "/etc/roxy/config.json"]
