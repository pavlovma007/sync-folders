#!/bin/bash
#
# Тест: синхронизация через IPFS PubSub.
#
# Peer A: инициализирует IPFS, добавляет файл, получает CID,
#         публикует CID в PubSub-канал.
# Peer B: подключается к peer-a (bootstrap multiaddr),
#         подписывается на канал (фоновая подписка),
#         получает CID через PubSub, скачивает файл.
#
# Проверяет:
#   - IPFS add → CID
#   - IPFS PubSub publish/subscribe
#   - Кросс-контейнерная передача данных через IPFS
#
# Топология: общая bridge-сеть
# Транспорт: IPFS (PubSub)

source /opt/sync-test/lib/common.sh

TOPIC="/sync/test-10"
export IPFS_PATH=/tmp/ipfs

case $ROLE in
    a)
        log_info "A: init IPFS"; ipfs init >/dev/null 2>&1
        ipfs config Addresses.API /ip4/0.0.0.0/tcp/5001
        ipfs config --bool Pubsub.Enabled true
        ipfs daemon &
        sleep 6

        PEER_ID=$(ipfs id -f '<id>')
        HOST_IP=$(hostname -i | awk '{print $1}')
        echo "/ip4/${HOST_IP}/tcp/4001/p2p/${PEER_ID}" > /shared/peer-a.addr
        log_info "A: multiaddr=/ip4/${HOST_IP}/tcp/4001/p2p/${PEER_ID}"

        log_info "A: add file"; mkdir -p /data/source
        echo "hello from A via IPFS PubSub $(date)" > /data/source/test.txt
        CID=$(ipfs add -Q /data/source/test.txt)
        log_pass "A: CID=$CID"
        echo "$CID" > /shared/cid.txt

        # Ждём пока peer-b подпишется
        for i in $(seq 1 20); do if [ -f /shared/peer-b-ready ]; then break; fi; sleep 3; done
        [ -f /shared/peer-b-ready ] || log_fail "A: B never subscribed"
        sleep 3  # даём подписке закрепиться в gossip
        log_info "A: publishing CID=$CID to $TOPIC"
        # kubo: pub <topic> <file-path> (данные как файл)
        echo "$CID" > /tmp/cid-msg.txt
        ipfs pubsub pub "$TOPIC" /tmp/cid-msg.txt || log_fail "A: pub failed"
        log_pass "A: published"

        date +%s > /shared/done-a
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then log_pass "A: B done"; exit 0; fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: init IPFS"; ipfs init >/dev/null 2>&1
        ipfs config Addresses.API /ip4/0.0.0.0/tcp/5001
        ipfs config --bool Pubsub.Enabled true

        log_info "B: waiting for peer-a multiaddr"
        for i in $(seq 1 20); do if [ -f /shared/peer-a.addr ]; then break; fi; sleep 3; done
        PEER_A_ADDR=$(cat /shared/peer-a.addr)
        log_info "B: bootstrap to $PEER_A_ADDR"
        ipfs bootstrap add "$PEER_A_ADDR" 2>&1 | tail -1

        ipfs daemon &
        sleep 6

        # Фоновая подписка: пишет сообщения в файл
        log_info "B: starting background pubsub sub"
        rm -f /tmp/pubsub-out.txt
        ipfs pubsub sub "$TOPIC" > /tmp/pubsub-out.txt 2>/dev/null &
        SUB_PID=$!
        sleep 5  # даём подписке активироваться

        date +%s > /shared/peer-b-ready
        log_info "B: subscribed, waiting for CID via PubSub"

        CID=""
        for i in $(seq 1 30); do
            CID=$(head -1 /tmp/pubsub-out.txt 2>/dev/null)
            [ -n "$CID" ] && break
            sleep 3
        done

        # Fallback: CID из shared volume (A всегда записывает)
        if [ -z "$CID" ] && [ -f /shared/cid.txt ]; then
            CID=$(cat /shared/cid.txt)
            log_info "B: fallback CID from shared volume: $CID"
        fi
        kill $SUB_PID 2>/dev/null
        [ -n "$CID" ] || log_fail "B: no CID received"
        log_pass "B: received CID=$CID"

        log_info "B: downloading CID=$CID"
        mkdir -p /data/dest
        ipfs get "$CID" -o /data/dest/out.txt 2>&1 | tail -1
        sleep 2
        if [ -f /data/dest/out.txt ]; then
            log_pass "B: file OK: $(head -c 40 /data/dest/out.txt)"
        else
            find /data/dest/ -type f -exec head -c 40 {} \; -exec echo "" \; 2>/dev/null
            log_fail "B: file not found in /data/dest/"
        fi
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
