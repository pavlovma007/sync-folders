#!/bin/bash
# Общие функции для тестовых сценариев.

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    exit 1
}

log_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

assert_file_exists() {
    local path="$1" msg="${2:-file not found: $1}"
    if [ ! -f "$path" ]; then
        log_fail "$msg"
    fi
}

assert_eq() {
    local expected="$1" actual="$2" msg="${3:-value mismatch}"
    if [ "$expected" != "$actual" ]; then
        log_fail "$msg: expected '$expected', got '$actual'"
    fi
}

wait_for_port() {
    local port="$1" timeout="${2:-30}"
    for i in $(seq 1 "$timeout"); do
        if ss -tlnp "sport = :$port" 2>/dev/null | grep -q LISTEN; then
            return 0
        fi
        sleep 1
    done
    log_fail "port $port not ready after ${timeout}s"
}

wait_for_dht_key() {
    local pub="$1" salt="$2" timeout="${3:-60}"
    for i in $(seq 1 "$timeout"); do
        if sync-folders dht get "$pub" "$salt" >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
    done
    log_fail "DHT key $salt not found after ${timeout}s"
}
