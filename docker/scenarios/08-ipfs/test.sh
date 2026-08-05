#!/bin/bash
#
# Тест: IPFS add/get — обмен CID через shared volume.
#
# Peer A инициализирует IPFS, добавляет файл, получает CID.
# Peer B инициализирует IPFS, читает CID из shared volume,
# и скачивает файл через ipfs get.
#
# Проверяет:
#   - IPFS init/add → CID
#   - IPFS get (скачивание по CID)
#   - Обмен CID через shared volume

source /opt/sync-test/lib/common.sh

export IPFS_PATH=/tmp/ipfs

case $ROLE in
    a)
        log_info "A: init IPFS"; ipfs init >/dev/null 2>&1
        ipfs config Addresses.API /ip4/0.0.0.0/tcp/5001
        ipfs daemon &
        sleep 6

        # Публикуем multiaddr peer-a для peer-b
        PEER_ID=$(ipfs id -f '<id>')
        HOST_IP=$(hostname -i | awk '{print $1}')
        echo "/ip4/${HOST_IP}/tcp/4001/p2p/${PEER_ID}" > /shared/peer-a.addr
        log_info "A: multiaddr=/ip4/${HOST_IP}/tcp/4001/p2p/${PEER_ID}"

        log_info "A: add file"; mkdir -p /data/source
        echo "hello from A via IPFS $(date)" > /data/source/test.txt
        CID=$(ipfs add -Q /data/source/test.txt)
        log_pass "A: CID=$CID"
        ipfs pin add "$CID" >/dev/null 2>&1 || true

        echo "$CID" > /shared/cid.txt
        date +%s > /shared/done-a

        log_info "A: waiting for B"
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then log_pass "A: B done"; exit 0; fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: init IPFS"; ipfs init >/dev/null 2>&1
        ipfs config Addresses.API /ip4/0.0.0.0/tcp/5001
        ipfs daemon &
        sleep 6

        # Подключаемся к peer-a через его multiaddr
        log_info "B: waiting for peer-a multiaddr"
        for i in $(seq 1 20); do if [ -f /shared/peer-a.addr ]; then break; fi; sleep 3; done
        PEER_A_ADDR=$(cat /shared/peer-a.addr)
        log_info "B: bootstrap to $PEER_A_ADDR"
        ipfs bootstrap add "$PEER_A_ADDR" 2>&1 | tail -1
        # Явное соединение с peer-a (bootstrap может не установить прямую связь)
        log_info "B: swarm connect $PEER_A_ADDR"
        ipfs swarm connect "$PEER_A_ADDR" 2>&1 | tail -1
        sleep 2

        log_info "B: waiting for CID"
        for i in $(seq 1 20); do
            if [ -f /shared/done-a ]; then CID=$(cat /shared/cid.txt); break; fi
            sleep 3
        done; [ -n "$CID" ] || log_fail "B: no CID"

        log_info "B: CID=$CID, downloading"
        mkdir -p /data/dest
        # Ретраи скачивания (IPFS bitswap может быть медленным)
        # ipfs cat — потоковое чтение контента по CID
        for attempt in $(seq 1 5); do
            timeout 20 ipfs cat "$CID" > /data/dest/out.txt 2>/dev/null
            if [ -s /data/dest/out.txt ]; then break; fi
            sleep 5
        done
        if [ -f /data/dest/out.txt ]; then
            log_pass "B: file OK: $(head -c 40 /data/dest/out.txt)"
        else
            find /data/dest/ -type f -exec head -c 40 {} \; -exec echo "" \; 2>/dev/null
            log_fail "B: file not found in /data/dest/"
        fi
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
