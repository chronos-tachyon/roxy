# vim:set ft=dockerfile:
ARG VERSION=unset
ARG ARCH

FROM --platform=$BUILDPLATFORM golang:1.17-bullseye AS builder
ARG VERSION
ARG TARGETPLATFORM
ENV TZ Etc/UTC
ENV LC_ALL C
ENV DEBIAN_FRONTEND noninteractive
ENV NO_SUDO_REQUIRED true
COPY ./ /build/
RUN /build/scripts/docker_build_internal.sh "${TARGETPLATFORM}" "${VERSION}"

FROM --platform=$TARGETPLATFORM ${ARCH}/alpine AS final
COPY --from=builder /target/ /
ENV PATH /opt/roxy/bin:$PATH
USER roxy:roxy
WORKDIR /var/opt/roxy/lib
EXPOSE 80/tcp 443/tcp
CMD ["roxy", "-S"]
