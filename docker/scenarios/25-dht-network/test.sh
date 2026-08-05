#!/bin/bash
#
# Тест: DHT network put/get — обмен данными через Mainline DHT (BEP-44).
#
# Peer A генерирует Ed25519-ключи и публикует значение через
# sync-folders dht put. Peer B получает только публичный ключ через
# shared volume (discovery), а сами данные извлекает из DHT через
# sync-folders dht get. Данные через shared volume НЕ передаются.
#
# Проверяет:
#   - sync-folders torrent keygen (генерация Ed25519-ключей)
#   - sync-folders dht put (публикация mutable item в Mainline DHT)
#   - sync-folders dht get (получение item по pubkey+salt)
#   - DHT-обмен данными между двумя контейнерами без shared volume
#
# Топология: общая bridge-сеть; оба пира ходят в Mainline DHT через интернет.
# Транспорт: BEP-44 mutable item (sync-folders dht CLI)
#
# Требует интернет из контейнеров (UDP в Mainline DHT).

source /opt/sync-test/lib/common.sh

SALT="sync-folders:dht-test"
SEQ=42
VALUE='{"test":"network-dht-put-get"}'

case $ROLE in
    a)
        log_info "A: generate keys"
        sync-folders torrent keygen "dht-test" > /tmp/k.txt
        DHT_PUB=$(grep public_key /tmp/k.txt | awk '{print $2}')
        DHT_PRIV=$(grep private_key /tmp/k.txt | awk '{print $2}')
        [ -n "$DHT_PUB" ] || log_fail "A: no public key generated"
        echo "$DHT_PUB" > /shared/public-key.txt
        log_pass "A: keys generated"

        log_info "A: publish to DHT (put)"
        SYNC_OUT=$(timeout 60 sync-folders dht put "$DHT_PUB" "$DHT_PRIV" "$SALT" "$SEQ" "$VALUE" 2>&1)
        echo "$SYNC_OUT"
        [ -n "$SYNC_OUT" ] || log_fail "A: dht put failed (empty output)"
        log_pass "A: published to DHT"

        date +%s > /shared/done-a
        log_info "A: waiting for B"
        for i in $(seq 1 120); do
            if [ -f /shared/done-b ]; then log_pass "A: B done"; exit 0; fi
            sleep 3
        done; log_fail "A: timeout waiting for B"
        ;;

    b)
        log_info "B: wait for pubkey from A"
        for i in $(seq 1 30); do
            if [ -f /shared/public-key.txt ]; then break; fi
            sleep 2
        done
        DHT_PUB=$(cat /shared/public-key.txt)
        [ -n "$DHT_PUB" ] || log_fail "B: no pubkey from A"

        log_info "B: query DHT (get)"
        SYNC_OUT=""
        for i in $(seq 1 10); do
            SYNC_OUT=$(timeout 60 sync-folders dht get "$DHT_PUB" "$SALT" 2>&1)
            if echo "$SYNC_OUT" | grep -q "network-dht-put-get"; then
                break
            fi
            log_info "B: DHT get attempt $i not found yet, retrying in 5s"
            sleep 5
        done

        if echo "$SYNC_OUT" | grep -q "network-dht-put-get"; then
            log_pass "B: DHT get successful"
            echo "$SYNC_OUT"
        else
            log_info "B: DHT output: $SYNC_OUT"
            log_fail "B: DHT get failed - item not found"
        fi

        date +%s > /shared/done-b
        log_pass "B: done"
        ;;
esac
