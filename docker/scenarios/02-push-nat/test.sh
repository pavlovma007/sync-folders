#!/bin/bash
source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "peer-a: starting qBittorrent"
        qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30

        log_info "peer-a: generating DHT keys"
        sync-folders torrent keygen "test-02-push-nat" > /tmp/keys.txt
        DHT_PUB=$(grep public_key /tmp/keys.txt | awk '{print $2}')
        DHT_PRIV=$(grep private_key /tmp/keys.txt | awk '{print $2}')

        echo "$DHT_PUB" > /shared/public-key.txt
        echo "$DHT_PRIV" > /shared/private-key.txt

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
    project: "test-02-push-nat"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer-a.yaml

        echo "hello from peer-a at $(date)" > /data/source/test.txt

        sync-folders sync peer-a
        log_pass "peer-a: sync completed"
        ;;

    b)
        log_info "peer-b: starting qBittorrent"
        qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30

        log_info "peer-b: reading DHT keys"
        DHT_PUB=$(cat /shared/public-key.txt)
        DHT_PRIV=$(cat /shared/private-key.txt)

        sync-folders addfolder source /data/source
        sync-folders addfolder dest /data/dest

        cat > /tmp/peer-b.yaml <<YAML
folder: "dest"
transport:
  type: torrent
  config:
    client: "qbittorrent"
    api_url: "http://127.0.0.1:8080"
    dht_public_key: "${DHT_PUB}"
    dht_private_key: "${DHT_PRIV}"
    project: "test-02-push-nat"
sync:
  direction: "pull"
YAML
        sync-folders addconfig /tmp/peer-b.yaml

        log_info "peer-b: waiting for DHT manifest"
        wait_for_dht_key "$DHT_PUB" "sync-folders:test-02-push-nat" 120

        sync-folders sync peer-b
        assert_file_exists "/data/dest/test.txt" "peer-b: test.txt not found"
        log_pass "peer-b: sync completed"
        ;;
esac
