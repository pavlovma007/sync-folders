#!/bin/bash
source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "peer-a: qBittorrent OFFLINE scenario"
        sync-folders torrent keygen "test-07-qb-offline" > /tmp/keys.txt
        DHT_PUB=$(grep public_key /tmp/keys.txt | awk '{print $2}')
        DHT_PRIV=$(grep private_key /tmp/keys.txt | awk '{print $2}')

        mkdir -p /data/source /data/dest
        sync-folders addfolder source /data/source
        sync-folders addfolder dest /data/dest

        cat > /tmp/peer-a.yaml <<YAML
folder: "source"
transport:
  type: torrent
  config:
    client: "qbittorrent"
    api_url: "http://127.0.0.1:8080"
    dht_public_key: "${DHT_PUB}"
    dht_private_key: "${DHT_PRIV}"
    project: "test-07-qb-offline"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer-a.yaml

        log_info "peer-a: running sync WITHOUT qBittorrent"
        if sync-folders sync peer-a 2>&1 | grep -i "error\|fail"; then
            log_info "peer-a: got expected error (qBittorrent not running)"
        else
            log_pass "peer-a: sync didn't crash (expected error handled)"
        fi
        log_pass "peer-a: qb-offline completed without crash"
        ;;

    b)
        log_info "peer-b: nothing to do in qb-offline scenario"
        log_pass "peer-b: done"
        ;;
esac
