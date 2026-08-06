#!/bin/bash
set -e

# ============================================================
# install.sh — установка sync-folders
#
# Использование (user-friendly):
#   curl -fsSL https://raw.githubusercontent.com/pavlovma007/sync-folders/refs/heads/main/install.sh | sh
#
#   # С конкретной версией:
#   VERSION=0.1 sh -c "$(curl -fsSL https://raw.githubusercontent.com/pavlovma007/sync-folders/refs/heads/main/install.sh)"
#
# Скрипт сам определит ОС и архитектуру, скачает нужный
# бинарник из GitHub Releases и установит в /usr/local/bin.
# ============================================================

# Настройки (можно переопределить через окружение)
REPO="${REPO:-pavlovma007/sync-folders}"
# По умолчанию — последняя версия.
# Если latest не работает, укажите явно: VERSION=0.1
VERSION="${VERSION:-0.1}"
INSTALL_DIR="${INSTALL_DIR:-}"

BINARY_NAME="sync-folders"

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
# 3. Определяем путь установки
# -------------------------------------------------------
detect_install_path() {
    # Если пользователь явно указал
    if [ -n "$INSTALL_DIR" ]; then
        echo "$INSTALL_DIR"
        return
    fi
    # Termux
    if [ -n "$PREFIX" ] && [ -d "$PREFIX/bin" ]; then
        echo "$PREFIX/bin"
        return
    fi
    # macOS — ~/bin или /usr/local/bin
    if [ "$(uname -s)" = "Darwin" ]; then
        if [ -d "$HOME/bin" ] && [[ ":$PATH:" == *":$HOME/bin:"* ]]; then
            echo "$HOME/bin"
            return
        fi
        echo "/usr/local/bin"
        return
    fi
    # Linux
    echo "/usr/local/bin"
}

# -------------------------------------------------------
# 4. Выбираем download-утилиту
# -------------------------------------------------------
detect_downloader() {
    if command -v curl >/dev/null 2>&1; then
        printf "curl"
    elif command -v wget >/dev/null 2>&1; then
        printf "wget"
    else
        printf ""
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

    # Формируем URL для скачивания
    suffix="${os}-${arch}${ext}"
    if [ "$VERSION" = "latest" ]; then
        BASE_URL="https://github.com/${REPO}/releases/latest/download"
    else
        BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
    fi
    url="${BASE_URL}/sync-folders-${suffix}"

    INSTALL_PATH=$(detect_install_path)
    dest_file="${INSTALL_PATH}/${BINARY_NAME}${ext}"

    # Downloader
    downloader=$(detect_downloader)
    if [ -z "$downloader" ]; then
        error "Ни curl, ни wget не найдены. Установите curl и повторите."
        exit 1
    fi

    # Создаём целевую директорию, если её нет
    if [ ! -d "$INSTALL_PATH" ]; then
        mkdir -p "$INSTALL_PATH" 2>/dev/null || true
    fi

    # Скачивание
    info "Скачивание: $url"
    info "Установка в: $dest_file"

    if [ ! -w "$INSTALL_PATH" ]; then
        warn "Нет прав на запись в $INSTALL_PATH. Пробуем через sudo..."
        SUDO="sudo"
    else
        SUDO=""
    fi

    if [ "$downloader" = "curl" ]; then
        $SUDO curl -fsSL -o "$dest_file" "$url"
    else
        $SUDO wget -q -O "$dest_file" "$url"
    fi

    # Verify checksum
    info "Verifying checksum..."
    checksum_url="${url}.sha256"
    checksum_file=$(mktemp)
    if [ "$downloader" = "curl" ]; then
        $SUDO curl -fsSL -o "$checksum_file" "$checksum_url" 2>/dev/null || true
    else
        $SUDO wget -q -O "$checksum_file" "$checksum_url" 2>/dev/null || true
    fi

    if [ -s "$checksum_file" ]; then
        if ! echo "$(cat "$checksum_file")  $dest_file" | sha256sum -c --status; then
            warn "Checksum verification FAILED"
            warn "  Downloaded: $(sha256sum "$dest_file" | cut -d' ' -f1)"
            warn "  Expected:   $(cat "$checksum_file")"
            rm -f "$checksum_file"
            exit 1
        fi
        info "Checksum OK"
        rm -f "$checksum_file"
    else
        warn "No checksum file found at $checksum_url (skipping verification)"
    fi

    # Проверка: файл должен быть ELF-бинарником
    if file "$dest_file" | grep -qi "ELF\|executable\|Mach-O"; then
        :
    else
        warn "Скачанный файл не похож на бинарник. Возможно, неверный URL."
        warn "  URL: $url"
        warn "  Размер: $(wc -c < "$dest_file") байт"
        head -c 200 "$dest_file" 2>/dev/null || true
        echo ""
        error "Установка прервана. Проверьте релиз:"
        error "  https://github.com/${REPO}/releases"
        exit 1
    fi
    $SUDO chmod 755 "$dest_file"

    echo ""
    info "Установлено: $dest_file"
    info "Запуск: $(basename "$dest_file") --help"

    # Проверяем, что директория установки в PATH
    case ":$PATH:" in
        *:"$INSTALL_PATH":*) ;;
        *) warn "$INSTALL_PATH не в PATH. Добавьте в ~/.bashrc:"
           warn "  export PATH=\"\$PATH:$INSTALL_PATH\"" ;;
    esac
}

main "$@"
