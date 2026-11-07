#!/usr/bin/env sh
# Cross-compiles iseall into dist/ for every Raspberry Pi generation (and x86-64).
# Needs only Go ≥ 1.24 — no cgo, no other dependencies.
set -eu
cd "$(dirname "$0")"
VERSION="${VERSION:-1.0.0}"
mkdir -p dist
build() { # name GOARCH [GOARM]
  echo "→ dist/iseall-$1"
  CGO_ENABLED=0 GOOS=linux GOARCH="$2" GOARM="${3:-}" \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "dist/iseall-$1" .
}
build linux-arm64 arm64       # Pi 3/4/5/Zero 2 W on a 64-bit OS (the usual case)
build linux-armv7 arm 7       # Pi 2/3/4 on a 32-bit OS
build linux-armv6 arm 6       # Pi 1 / Zero / Zero W
build linux-amd64 amd64       # any x86-64 server
ls -lh dist/
