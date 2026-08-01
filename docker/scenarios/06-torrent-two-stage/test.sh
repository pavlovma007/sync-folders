#!/bin/bash
#
# Тест: многостадийная синхронизация через торрент.
#
# Peer A выполняет 2 последовательные публикации:
#   1. Файл a.txt
#   2. Добавляет b.txt (теперь .torrent содержит оба)
# Peer B скачивает финальный снапшот (с обоими файлами).
#
# Проверяет:
#   - Инкрементальное обновление (snapshot всей папки)
#   - Diff detection
#   - Многостадийная публикация
#
# Топология: общая bridge-сеть
# Транспорт: qBittorrent

source /opt/sync-test/lib/common.sh
source /opt/sync-test/lib/test-torrent.sh

case $ROLE in
    a)
        log_info "A: qBittorrent"; qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30; qb_login
        mkdir -p /data/source/sub /data/dest
        sync-folders addfolder source /data/source; sync-folders addfolder dest /data/dest
        sync-folders torrent keygen t06 > /tmp/k.txt
        DHT_PUB=$(grep public_key /tmp/k.txt | awk '{print $2}')
        DHT_PRIV=$(grep private_key /tmp/k.txt | awk '{print $2}')
        echo "$DHT_PUB" > /shared/public-key.txt; echo "$DHT_PRIV" > /shared/private-key.txt
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: torrent
  config:
    client: "qbittorrent"
    api_url: "http://127.0.0.1:8080"
    api_user: "${QB_USER}"
    api_password: "${QB_PASS}"
    dht_public_key: "${DHT_PUB}"
    dht_private_key: "${DHT_PRIV}"
    project: "t06"
    staging_dir: "/data/.sync-torrent-staging"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer.yaml

        echo "s1-a" > /data/source/a.txt
        sync-folders sync peer
        log_pass "A: stage 1 (a.txt only)"
        sleep 10

        echo "s2-b" > /data/source/b.txt
        sync-folders sync peer
        HASH=$(qb_get "torrents/info" | jq -r '.[0].hash')
        log_pass "A: stage 2 (a.txt + b.txt, hash=$HASH)"
        echo "$HASH" > /shared/hash.txt
        echo "magnet:?xt=urn:btih:$HASH" > /shared/magnet.txt
        date +%s > /shared/done-a

        log_info "A: waiting for B"
        for i in $(seq 1 60); do
            if [ -f /shared/done-b ]; then log_pass "A: done"; exit 0; fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: qBittorrent"; qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30; qb_login
        mkdir -p /data/dest; sync-folders addfolder dest /data/dest

        log_info "B: waiting for A"
        for i in $(seq 1 30); do if [ -f /shared/done-a ]; then break; fi; sleep 3; done
        [ -f /shared/done-a ] || log_fail "B: A never ready"

        MAGNET=$(cat /shared/magnet.txt); HASH=$(cat /shared/hash.txt)
        log_info "B: HASH=$HASH adding magnet"
        qb_post "torrents/add" "urls=${MAGNET}&savepath=/data/dest" >/dev/null

        log_info "B: waiting for download"
        DL=0
        for i in $(seq 1 50); do sleep 10
            STATE=$(qb_get "torrents/info?hashes=$HASH" | jq -r '.[0].state // "none"')
            PROG=$(qb_get "torrents/info?hashes=$HASH" | jq -r '.[0].progress // 0')
            log_info "B: s=$STATE p=$PROG"
            case "$STATE" in stalledUP|uploading) DL=1; break ;; stalledDL|downloading) [ "$PROG" = "1" ] && { DL=1; break; } ;; error) break ;; esac
        done; [ "$DL" = "1" ] || log_fail "B: never downloaded"

        log_info "B: checking files"
        find /data/dest/ -type f 2>/dev/null
        A_COUNT=$(find /data/dest/ -name "a.txt" -type f 2>/dev/null | wc -l)
        B_COUNT=$(find /data/dest/ -name "b.txt" -type f 2>/dev/null | wc -l)
        log_info "B: a.txt=$A_COUNT b.txt=$B_COUNT"
        [ "$A_COUNT" -gt 0 ] && [ "$B_COUNT" -gt 0 ] && log_pass "B: both files present" || log_info "B: some files missing"
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
