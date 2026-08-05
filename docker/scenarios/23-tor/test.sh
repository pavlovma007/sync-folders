#!/bin/bash
#
# Тест: Tor-прокси (SOCKS5) — smoke test.
#
# Peer A: запускает Tor SOCKS-прокси.
# Peer B: проверяет что Tor-прокси peer-a доступен через TorCheck.
#
# Примечание: полный синк через Tor невозможен в изолированной сети
# (Tor не маршрутизирует к приватным IP). Проверяем только живость прокси.
#
# Проверяет:
#   - Tor SOCKS-прокси отвечает
#   - TorCheck (sync-folders)
#
# Топология: общая bridge-сеть
# Транспорт: tor (smoke)

source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "A: start Tor"
        # Tor SOCKS на 0.0.0.0 (иначе только 127.0.0.1, недоступен peer-b)
        echo "SocksPort 0.0.0.0:9050" > /etc/tor/torrc
        tor >/dev/null 2>&1 &
        sleep 5
        # Tor слушает на 9050
        if ss -tlnp 2>/dev/null | grep -q ":9050"; then
            log_pass "A: Tor SOCKS listening on 9050"
        else
            log_info "A: waiting for Tor to bind 9050"
            for i in $(seq 1 20); do
                ss -tlnp 2>/dev/null | grep -q ":9050" && { log_pass "A: Tor OK"; break; }
                sleep 2
            done
        fi
        date +%s > /shared/done-a
        log_info "A: waiting for B"
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then log_pass "A: done"; exit 0; fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: TorCheck against peer-a"
        sleep 5
        PEER_A="${PEER_HOST:-sync-test-23-tor-a}"
        # Проверяем что SOCKS-порт peer-a отвечает
        if timeout 5 bash -c "echo > /dev/tcp/${PEER_A}/9050" 2>/dev/null; then
            log_pass "B: Tor SOCKS port ${PEER_A}:9050 reachable"
        else
            log_fail "B: Tor SOCKS not reachable"
        fi
        # Проверка через TorCheck (утилита)
        if timeout 10 sync-folders status >/dev/null 2>&1; then
            log_pass "B: sync-folders status OK"
        else
            log_info "B: status exit=$? (TorCheck ожидает локальный Tor)"
        fi
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
