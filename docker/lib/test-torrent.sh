#!/bin/bash
# Общие функции для торрент-сценариев.
# Источнится внутри test.sh: source /opt/sync-test/lib/test-torrent.sh

QB_USER="admin"; QB_PASS="adminadmin"

qb_login() { for i in $(seq 1 20); do
    if curl -s -c /tmp/qb.cookie -X POST "http://127.0.0.1:8080/api/v2/auth/login" \
        -d "username=${QB_USER}&password=${QB_PASS}" | grep -qi "ok\|Ok"; then return 0; fi
    sleep 2
done; log_fail "qb login"; }
qb_get() { curl -s -b /tmp/qb.cookie "http://127.0.0.1:8080/api/v2/$1"; }
qb_post() { curl -s -b /tmp/qb.cookie -X POST "http://127.0.0.1:8080/api/v2/$1" -d "$2"; }

# torrent_seed <project> <direction> — peer-a push & wait
torrent_seed() {
    local prj="${1:-test}" dir="${2:-push}"
    log_info "A: qBittorrent"; qbittorrent-nox -d --webui-port=8080
    wait_for_port 8080 30; qb_login
    mkdir -p /data/source /data/dest
    sync-folders addfolder source /data/source
    sync-folders addfolder dest /data/dest
    sync-folders torrent keygen "$prj" > /tmp/k.txt
    DHT_PUB=$(grep public_key /tmp/k.txt | awk '{print $2}')
    DHT_PRIV=$(grep private_key /tmp/k.txt | awk '{print $2}')
    echo "$DHT_PUB" > /shared/public-key.txt
    echo "$DHT_PRIV" > /shared/private-key.txt
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
    project: "${prj}"
    staging_dir: "/data/.sync-torrent-staging"
sync:
  direction: "${dir}"
YAML
    sync-folders addconfig /tmp/peer.yaml
    mkdir -p /data/source/sub
    echo "hello A $(date)" > /data/source/a.txt
    echo "extra $(date)" > /data/source/sub/b.txt
    sync-folders sync peer
    local HASH=$(qb_get "torrents/info" | jq -r '.[0].hash')
    local STATE=$(qb_get "torrents/info" | jq -r '.[0].state')
    log_info "A: hash=$HASH state=$STATE"
    echo "$HASH" > /shared/hash.txt
    echo "magnet:?xt=urn:btih:$HASH" > /shared/magnet.txt
    date +%s > /shared/done-a
    log_info "A: waiting for B..."; for i in $(seq 1 60); do
        if [ -f /shared/done-b ]; then log_pass "A: B done"; return 0; fi
        sleep 5
    done; log_fail "A: timeout"
}

# torrent_leech <project> <dir> — peer-b wait & download
torrent_leech() {
    local prj="${1:-test}" dir="${2:-pull}"
    log_info "B: qBittorrent"; qbittorrent-nox -d --webui-port=8080
    wait_for_port 8080 30; qb_login
    log_info "B: waiting for A"; for i in $(seq 1 30); do
        if [ -f /shared/done-a ]; then break; fi; sleep 3
    done; [ -f /shared/done-a ] || log_fail "B: A never ready"
    local MAGNET=$(cat /shared/magnet.txt) HASH=$(cat /shared/hash.txt)
    log_info "B: H=$HASH"

    DHT_PUB=$(cat /shared/public-key.txt 2>/dev/null || echo ignore) DHT_PRIV=$(cat /shared/private-key.txt 2>/dev/null || echo ignore)
    sync-folders addfolder source /data/source; sync-folders addfolder dest /data/dest; mkdir -p /data/dest

    cat > /tmp/peer.yaml <<YAML
folder: "dest"
transport:
  type: torrent
  config:
    client: "qbittorrent"
    api_url: "http://127.0.0.1:8080"
    api_user: "${QB_USER}"
    api_password: "${QB_PASS}"
    dht_public_key: "${DHT_PUB}"
    dht_private_key: "${DHT_PRIV}"
    project: "${prj}"
    staging_dir: "/data/.sync-torrent-staging"
sync:
  direction: "${dir}"
YAML
    sync-folders addconfig /tmp/peer.yaml

    log_info "B: add magnet"; qb_post "torrents/add" "urls=${MAGNET}&savepath=/data/dest" >/dev/null
    log_info "B: wait download (DHT ~60-120s)"
    local DL=0
    for i in $(seq 1 50); do
        sleep 10
        local STATE=$(qb_get "torrents/info?hashes=$HASH" | jq -r '.[0].state // "none"')
        local PROG=$(qb_get "torrents/info?hashes=$HASH" | jq -r '.[0].progress // 0')
        log_info "B: s=$STATE p=$PROG"
        case "$STATE" in stalledUP|uploading) DL=1; break ;; stalledDL|downloading) [ "$PROG" = "1" ] && { DL=1; break; } ;; error|missingFiles) log_info "B: state=$STATE"; sleep 30 ;; esac
    done
    [ "$DL" = "1" ] || { log_info "B: final=$(qb_get "torrents/info?hashes=$HASH")"; log_fail "B: never downloaded"; }
    log_info "B: files:"; find /data/dest/ -type f 2>/dev/null
    local N=$(find /data/dest/ -type f 2>/dev/null | wc -l)
    [ "$N" -gt 0 ] || log_fail "B: 0 files"
    find /data/dest/ -type f -exec head -c 60 {} \; -exec echo "" \;
    log_pass "B: $N files"
    date +%s > /shared/done-b
}
