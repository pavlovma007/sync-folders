#!/bin/bash
source /opt/sync-test/lib/common.sh

QB_USER="admin"
QB_PASS="adminadmin"
QB_API="http://127.0.0.1:8080"
# qBittorrent webui credentials: admin/adminadmin
# Логинимся один раз, сохраняем cookie
qb_login() {
    curl -s -c /tmp/qb.cookie -b /tmp/qb.cookie \
        -X POST "${QB_API}/api/v2/auth/login" \
        -d "username=${QB_USER}&password=${QB_PASS}" > /dev/null
}
qb_api() {
    curl -s -b /tmp/qb.cookie "${QB_API}$1"
}
qb_api_post() {
    curl -s -b /tmp/qb.cookie -X POST "${QB_API}$1" -d "$2"
}

case $ROLE in
    a)
        log_info "peer-a: starting qBittorrent"
        qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30

        log_info "peer-a: generating DHT keys"
        mkdir -p /data/source /data/dest
        sync-folders torrent keygen "test-push-direct" > /tmp/keys.txt
        DHT_PUB=$(grep public_key /tmp/keys.txt | awk '{print $2}')
        DHT_PRIV=$(grep private_key /tmp/keys.txt | awk '{print $2}')

        log_info "peer-a: saving keys for peer-b"
        echo "$DHT_PUB" > /shared/public-key.txt

        log_info "peer-a: registering folder and config"
        sync-folders addfolder source /data/source
        sync-folders addfolder dest /data/dest

        cat > /tmp/peer-a.yaml <<YAML
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
    project: "test-push-direct"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer-a.yaml

        log_info "peer-a: creating test file"
        echo "hello from peer-a at $(date)" > /data/source/test.txt

        log_info "peer-a: running sync push"
        sync-folders sync peer-a

        # Извлекаем info_hash из qBittorrent (последний добавленный торрент)
        log_info "peer-a: logging into qBittorrent"
        qb_login
        sleep 1
        log_info "peer-a: extracting magnet from qBittorrent"
        INFO_JSON=$(qb_api "/api/v2/torrents/info?sort=added_on&reverse=true&limit=1")
        INFO_HASH=$(echo "$INFO_JSON" | jq -r '.[0].hash // "ERROR"' 2>/dev/null)

        if [ "$INFO_HASH" = "ERROR" ] || [ -z "$INFO_HASH" ]; then
            log_info "peer-a: torrents in qBittorrent: $INFO_JSON"
            log_fail "peer-a: could not extract torrent hash"
        fi
        MAGNET="magnet:?xt=urn:btih:${INFO_HASH}&dn=test-push-direct"
        echo "$MAGNET" > /shared/magnet.txt
        log_pass "peer-a: push completed (magnet: $MAGNET)"
        ;;

    b)
        log_info "peer-b: starting qBittorrent"
        qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30

        # Ждём magnet от peer-a
        log_info "peer-b: waiting for magnet from peer-a"
        for i in $(seq 1 30); do
            if [ -f /shared/magnet.txt ]; then
                MAGNET=$(cat /shared/magnet.txt)
                log_info "peer-b: got magnet: $MAGNET"
                break
            fi
            sleep 2
        done

        if [ -z "$MAGNET" ]; then
            log_fail "peer-b: no magnet after 60s"
        fi

        # Добавляем magnet в qBittorrent B
        log_info "peer-b: adding magnet to qBittorrent"
        		qb_api_post "/api/v2/torrents/add" "urls=${MAGNET}&savepath=/data/dest" > /dev/null

        # Ждём завершения загрузки
        INFO_HASH=$(echo "$MAGNET" | sed 's/.*btih://;s/&.*//')
        log_info "peer-b: waiting for download (hash=$INFO_HASH)"

        for i in $(seq 1 30); do
            STATUS=$(qb_api "/api/v2/torrents/info?hashes=$INFO_HASH")
            PROGRESS=$(echo "$STATUS" | jq -r '.[0].progress // 0' 2>/dev/null)
            STATE=$(echo "$STATUS" | jq -r '.[0].state // "unknown"' 2>/dev/null)
            log_info "peer-b: progress=$PROGRESS state=$STATE"

            if [ "$(echo "$PROGRESS >= 1.0" | bc 2>/dev/null)" = "1" ] || \
               [ "$STATE" = "seeding" ] || [ "$STATE" = "uploading" ]; then
                log_pass "peer-b: download complete!"
                break
            fi
            if [ "$STATE" = "error" ]; then
                log_fail "peer-b: torrent error"
            fi
            sleep 5
        done

        # Проверяем файл
        log_info "peer-b: /data/dest/ contents:"
        find /data/dest/ -type f 2>/dev/null
        ls -la /data/dest/ 2>&1

        if [ -f /data/dest/test.txt ]; then
            content=$(cat /data/dest/test.txt)
            log_pass "peer-b: file received: $content"
        else
            log_fail "peer-b: test.txt not found in /data/dest/"
        fi
        ;;
esac
