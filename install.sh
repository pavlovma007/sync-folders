#!/bin/bash
set -e

# ============================================================
# install.sh — установка sync-folders
# Определяет платформу, скачивает готовый бинарник и
# устанавливает в PATH.
#
# Базовый URL (захардкожен):
BASE_URL="https://github.com/mp-org/sync-folders/releases/latest/download"
# ============================================================

BINARY_NAME="sync-folders"
INSTALL_PATH=""

info()  { echo -e "\033[0;32m[INFO]\033[0m $1"; }
warn()  { echo -e "\033[0;33m[WARN]\033[0m $1"; }
error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

# -------------------------------------------------------
# 1. Определяем ОС
# -------------------------------------------------------
detect_os() {
    local os
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux*)  echo "linux" ;;
        darwin*) echo "darwin" ;;
        cygwin*|mingw*|msys*) echo "windows" ;;
        *)       echo "unknown" ;;
    esac
}

# -------------------------------------------------------
# 2. Определяем архитектуру
# -------------------------------------------------------
detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv7l|armv8l|arm) echo "arm" ;;
        i686|i386)     echo "386"  ;;
        *)             echo "unknown" ;;
    esac
}

# -------------------------------------------------------
# 3. Определяем путь установки (Termux vs обычный Linux)
# -------------------------------------------------------
detect_install_path() {
    # Termux
    if [ -n "$PREFIX" ] && [ -d "$PREFIX/bin" ]; then
        echo "$PREFIX/bin"
        return
    fi
    # Обычная система
    echo "/usr/local/bin"
}

# -------------------------------------------------------
# 4. Выбираем download-утилиту
# -------------------------------------------------------
detect_downloader() {
    if command -v curl &>/dev/null; then
        echo "curl"
    elif command -v wget &>/dev/null; then
        echo "wget"
    else
        echo ""
    fi
}

# -------------------------------------------------------
# MAIN
# -------------------------------------------------------
main() {
    local os arch ext downloader dest_file url suffix

    os=$(detect_os)
    arch=$(detect_arch)
    ext=""

    info "Определение платформы: ОС=$os, ARCH=$arch"

    if [ "$os" = "unknown" ] || [ "$arch" = "unknown" ]; then
        error "Не удалось определить платформу (os=$os, arch=$arch)."
        error "Поддерживаемые: linux {amd64,arm64,arm,386}, darwin {amd64,arm64}, windows {amd64,386}"
        exit 1
    fi

    # Windows — добавляем .exe
    if [ "$os" = "windows" ]; then
        ext=".exe"
    fi

    suffix="${os}-${arch}${ext}"
    url="${BASE_URL}/sync-folders-${suffix}"

    INSTALL_PATH=$(detect_install_path)
    dest_file="${INSTALL_PATH}/${BINARY_NAME}${ext}"

    # Downloader
    downloader=$(detect_downloader)
    if [ -z "$downloader" ]; then
        error "Ни curl, ни wget не найдены. Установите curl и повторите."
        exit 1
    fi

    # Проверка прав на запись в целевой каталог
    if [ ! -w "$INSTALL_PATH" ]; then
        warn "Нет прав на запись в $INSTALL_PATH. Пробуем через sudo..."
        if [ "$downloader" = "curl" ]; then
            sudo curl -fsSL -o "$dest_file" "$url"
        else
            sudo wget -q -O "$dest_file" "$url"
        fi
        sudo chmod 755 "$dest_file"
    else
        info "Скачивание: $url"
        if [ "$downloader" = "curl" ]; then
            curl -fsSL -o "$dest_file" "$url"
        else
            wget -q -O "$dest_file" "$url"
        fi
        chmod 755 "$dest_file"
    fi

    echo ""
    info "Установлено: $dest_file"
    info "Запуск: $(basename "$dest_file") --help"
}

main "$@"
