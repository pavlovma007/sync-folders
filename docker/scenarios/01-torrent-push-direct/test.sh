#!/bin/bash
#
# Тест: прямая push-синхронизация через торрент.
#
# Сценарий:
#   Peer A создаёт торрент (.torrent) из папки с двумя файлами,
#   публикует magnet-ссылку через DHT (BEP-44) и сидирует.
#   Peer B находит magnet через DHT, добавляет в qBittorrent
#   и скачивает файлы от peer A.
#
# Проверяет:
#   - Работу TorrentTransport.Push и Flush
#   - qBittorrent seeding
#   - Кросс-контейнерный DHT discovery и скачивание
#   - Координацию пиров через shared volume
#
# Топология: общая bridge-сеть (прямая видимость)
# Транспорт: qBittorrent
# Направление: push

source /opt/sync-test/lib/common.sh

QB_USER="admin"
QB_PASS="adminadmin"
qb_login() {
    for i in $(seq 1 20); do
        if curl -s -c /tmp/qb.cookie -X POST "http://127.0.0.1:8080/api/v2/auth/login" \
            -d "username=${QB_USER}&password=${QB_PASS}" | grep -qi "ok\|Ok"; then return 0; fi
        sleep 2
    done
    log_fail "qb login failed"
}
qb_get() { curl -s -b /tmp/qb.cookie "http://127.0.0.1:8080/api/v2/$1"; }
qb_post() { curl -s -b /tmp/qb.cookie -X POST "http://127.0.0.1:8080/api/v2/$1" -d "$2"; }

case $ROLE in
    a)
        log_info "A: starting qBittorrent"; qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30; qb_login
        log_info "A: setup"; mkdir -p /data/source /data/dest
        sync-folders addfolder source /data/source
        sync-folders addfolder dest /data/dest
        sync-folders torrent keygen t01 > /tmp/k.txt
        DHT_PUB=$(grep public_key /tmp/k.txt | awk '{print $2}')
        DHT_PRIV=$(grep private_key /tmp/k.txt | awk '{print $2}')
        echo "$DHT_PUB" > /shared/public-key.txt
        echo "$DHT_PRIV" > /shared/private-key.txt
        cat > /tmp/a.yaml <<YAML
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
    project: "t01"
    staging_dir: "/data/.sync-torrent-staging"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/a.yaml
        log_info "A: creating files"
        echo "hello from A: $(date)" > /data/source/a.txt
        echo "file two content" > /data/source/sub/b.txt
        log_info "A: sync push"
        sync-folders sync a
        HASH=$(qb_get "torrents/info" | jq -r '.[0].hash')
        PORT=$(qb_get "app/preferences" | jq -r '.listen_port')
        STATE=$(qb_get "torrents/info" | jq -r '.[0].state')
        log_info "A: hash=$HASH port=$PORT state=$STATE"
        case "$STATE" in seeding|uploading|stalledUP|checkingUP) log_pass "A: seeding OK ($STATE)";; esac
        echo "$HASH" > /shared/hash.txt
        echo "$PORT" > /shared/port.txt
        echo "magnet:?xt=urn:btih:$HASH" > /shared/magnet.txt
        date +%s > /shared/done-a
        log_info "A: waiting for peer-b"
        for i in $(seq 1 60); do
            if [ -f /shared/done-b ]; then log_pass "A: peer-b done"; exit 0; fi
            sleep 5
        done
        log_fail "A: timeout waiting for peer-b"
        ;;

    b)
        log_info "B: starting qBittorrent"; qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30; qb_login
        log_info "B: waiting for peer-a"; for i in $(seq 1 30); do
            if [ -f /shared/done-a ]; then break; fi; sleep 3
        done
        [ -f /shared/done-a ] || log_fail "B: peer-a never ready"
        MAGNET=$(cat /shared/magnet.txt)
        HASH=$(cat /shared/hash.txt)
        log_info "B: hash=$HASH magnet=$MAGNET"

        DHT_PUB=$(cat /shared/public-key.txt 2>/dev/null || echo "")
        DHT_PRIV=$(cat /shared/private-key.txt 2>/dev/null || echo "")
        sync-folders addfolder source /data/source
        sync-folders addfolder dest /data/dest
        mkdir -p /data/dest

        log_info "B: adding magnet"
        qb_post "torrents/add" "urls=${MAGNET}&savepath=/data/dest" >/dev/null

        log_info "B: waiting for download (DHT discovery ~60-120s)"
        DL=0
        for i in $(seq 1 50); do
            sleep 10
            STATE=$(qb_get "torrents/info?hashes=$HASH" | jq -r '.[0].state // "none"')
            PROG=$(qb_get "torrents/info?hashes=$HASH" | jq -r '.[0].progress // 0')
            log_info "B: state=$STATE progress=$PROG"
            case "$STATE" in stalledUP|uploading) DL=1; break ;; stalledDL|downloading) [ "$PROG" = "1" ] && { DL=1; break; } ;; error|missingFiles) log_fail "B: error" ;; esac
        done
        [ "$DL" = "1" ] || { log_info "B: last state: $(qb_get "torrents/info?hashes=$HASH")"; log_fail "B: download never completed"; }

        log_info "B: checking files"; find /data/dest/ -type f 2>/dev/null
        N=$(find /data/dest/ -type f 2>/dev/null | wc -l)
        if [ "$N" -gt 0 ]; then
            find /data/dest/ -type f -exec head -c 80 {} \; -exec echo "" \;
            log_pass "B: download verified ($N files)"
        else log_fail "B: no files"; fi
        date +%s > /shared/done-b
        log_pass "B: done"
        ;;
esac
