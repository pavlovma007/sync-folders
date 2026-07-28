#!/bin/bash
set -e

# Build script for sync-folders — multi-architecture cross-compilation
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NAME="sync-folders"
OUT_DIR="$SCRIPT_DIR/build"

# ─── Определение Go toolchain ──────────────────────────────
# Пытаемся найти Go 1.26+ в нескольких местах:
#   1. go в PATH (если установлен глобально)
#   2. GOROOT из .env файла
#   3. toolchain в модульном кеше Go
#   4. GOROOT из go env (если go доступен)
#
# Можно переопределить через переменную окружения GO_CMD.

if [ -z "$GO_CMD" ]; then
    # Пробуем go в PATH
    if command -v go &>/dev/null; then
        # Проверяем, что go может собрать проект (go 1.26+)
        GO_CMD="go"
    fi

    # Если .env существует — загружаем GOROOT
    if [ -f "$SCRIPT_DIR/.env" ]; then
        set -a
        source "$SCRIPT_DIR/.env"
        set +a
    fi

    # Если GOROOT задан — используем его
    if [ -n "$GOROOT" ] && [ -x "$GOROOT/bin/go" ]; then
        GO_CMD="$GOROOT/bin/go"
    fi

    # Ищем toolchain в модульном кеше
    if [ -z "$GO_CMD" ] || ! $GO_CMD version &>/dev/null; then
        TOOLCHAIN_DIRS=(
            "$HOME/go/pkg/mod/golang.org/toolchain@latest/bin/go"
            "$HOME/go/pkg/mod/golang.org/"toolchain@*/bin/go
            "/usr/local/go/bin/go"
            "/usr/lib/go/bin/go"
        )
        for candidate in "${TOOLCHAIN_DIRS[@]}"; do
            # Для glob-паттерна
            for f in $candidate; do
                if [ -x "$f" ]; then
                    GO_CMD="$f"
                    break 2
                fi
            done
        done
    fi
fi

if [ -z "$GO_CMD" ] || ! $GO_CMD version &>/dev/null; then
    echo "ERROR: Go toolchain not found."
    echo ""
    echo "  Установите Go 1.26+: https://go.dev/dl/"
    echo "  Или пропишите GOROOT в .env файле:"
    echo "    echo 'GOROOT=/path/to/go1.26' >> .env"
    exit 1
fi

GO_VERSION=$($GO_CMD version 2>&1)
echo "Using: $GO_CMD — $GO_VERSION"
echo ""

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
