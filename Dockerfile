# syntax=docker/dockerfile:1
# Cross-compiles natively on the build host for every target platform (no QEMU needed).
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS TARGETARCH TARGETVARIANT VERSION=dev
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY web ./web
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/iseall .

# alpine rather than scratch so the web terminal has a shell
FROM alpine:3.20
COPY --from=build /out/iseall /usr/local/bin/iseall
EXPOSE 8765
VOLUME /etc/iseall
ENTRYPOINT ["/usr/local/bin/iseall", "-config", "/etc/iseall/config.json"]
