#!/usr/bin/env bash
# Build the English ARTEX: web UI (static export) -> embedded Go binary -> Docker image.
#
# Everything runs inside containers, so the host only needs Docker (no Go/Node install).
# Produces the image `artex-en:latest`; start it with:
#   cp docker-compose.override.yml.example docker-compose.override.yml   (once)
#   cp .env.example .env && edit POSTGRES_PASSWORD                        (once)
#   docker compose up -d
set -euo pipefail
cd "$(dirname "$0")/.."

ARCH="${ARCH:-amd64}"                      # amd64 or arm64
NODE_IMAGE="${NODE_IMAGE:-node:22-bookworm}"
GO_IMAGE="${GO_IMAGE:-golang:1.26}"
RUNAS="--user $(id -u):$(id -g)"
# Bind-mounted caches (named volumes would be root-owned and unwritable as --user).
CACHE_DIR="$PWD/.build-cache"
mkdir -p "$CACHE_DIR/npm" "$CACHE_DIR/go-build" "$CACHE_DIR/go-mod"

echo "==> [1/4] Building web UI (static export) in ${NODE_IMAGE} — this can take several minutes"
docker run --rm ${RUNAS} -e HOME=/tmp \
  -e npm_config_cache=/cache/npm \
  -e NODE_OPTIONS="--max-old-space-size=2048" \
  -v "$PWD/web:/web" -v "$CACHE_DIR/npm:/cache/npm" -w /web \
  "$NODE_IMAGE" bash -c "npm ci --no-audit --no-fund && npm run build:static"

echo "==> [2/4] Staging embedded UI (web/out -> server/webui/dist)"
# server/webui is a build-artifact dir (gitignored), so its parent must exist first.
rm -rf server/webui/dist
mkdir -p server/webui
cp -r web/out server/webui/dist

echo "==> [3/4] Building Go binary (linux/${ARCH}) in ${GO_IMAGE}"
mkdir -p "dist/${ARCH}"
docker run --rm ${RUNAS} -e HOME=/tmp \
  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH="$ARCH" \
  -e GOCACHE=/cache/go-build -e GOPATH=/cache/go-mod -e GOMODCACHE=/cache/go-mod/pkg/mod \
  -v "$PWD:/src" -v "$CACHE_DIR/go-build:/cache/go-build" -v "$CACHE_DIR/go-mod:/cache/go-mod" \
  -w /src "$GO_IMAGE" go build -tags embedui -o "dist/${ARCH}/artex" ./cmd/artex

echo "==> [4/4] Building Docker image artex-en:latest"
docker build -t artex-en:latest .

echo
echo "Build complete: artex-en:latest"
echo "Run it with:  docker compose up -d   (then open http://localhost:8787)"
