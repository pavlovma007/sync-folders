#!/bin/bash
set -e

# Build script for sync-folders — multi-architecture cross-compilation
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NAME="sync-folders"
OUT_DIR="$SCRIPT_DIR/build"

# Используем Go из PATH (go.mod управляет toolchain автоматически)
GO_CMD="go"
if ! command -v go &>/dev/null; then
    echo "Go not found in PATH. Install Go 1.26+ from https://go.dev/dl/"
    exit 1
fi

info()  { echo -e "\033[0;32m[INFO]\033[0m $1"; }

mkdir -p "$OUT_DIR"

# Платформы и архитектуры для сборки
BUILDS=(
    "linux/amd64"
    "linux/arm64"
    "linux/arm"      # GOARM=5,6,7 — самые старые ARM (ARMv5, Raspberry Pi Zero)
    "linux/386"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "windows/386"
)

for build in "${BUILDS[@]}"; do
    GOOS="${build%%/*}"
    GOARCH="${build##*/}"
    EXT=""

    if [ "$GOOS" = "windows" ]; then
        EXT=".exe"
    fi

    OUT="$OUT_DIR/${NAME}-${GOOS}-${GOARCH}${EXT}"
    info "Building $GOOS/$GOARCH -> $OUT"
    cd "$SCRIPT_DIR"
    if [ "$GOARCH" = "arm" ]; then
        GOARM=5 GOOS=$GOOS GOARCH=$GOARCH $GO_CMD build -o "$OUT" .
    else
        GOOS=$GOOS GOARCH=$GOARCH $GO_CMD build -o "$OUT" .
    fi
done

info "All cross-builds complete:"
ls -lh "$OUT_DIR/"

# Запуск тестов (текущая архитектура)
echo ""
info "Running tests for native architecture..."
cd "$SCRIPT_DIR"
$GO_CMD test -count=1 -timeout 120s ./...
if [ $? -eq 0 ]; then
    info "All tests PASS"
else
    error "Some tests FAIL"
    exit 1
fi
