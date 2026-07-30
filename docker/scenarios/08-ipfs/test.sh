#!/bin/bash
source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "peer-a: starting IPFS"
        ipfs init
        ipfs daemon &
        sleep 3

        mkdir -p /data/source /data/dest
        sync-folders addfolder source /data/source
        sync-folders addfolder dest /data/dest

        cat > /tmp/peer-a.yaml <<YAML
folder: "source"
transport:
  type: ipfs
  config:
    api: "http://127.0.0.1:5001"
    mfs_root: "/sync/test-08-ipfs"
YAML
        sync-folders addconfig /tmp/peer-a.yaml

        echo "hello from peer-a via IPFS" > /data/source/test.txt
        sync-folders sync peer-a
        log_pass "peer-a: IPFS push completed"
        ;;

    b)
        log_info "peer-b: waiting and syncing"
        sleep 5

        mkdir -p /data/source /data/dest
        sync-folders addfolder source /data/source
        sync-folders addfolder dest /data/dest

        cat > /tmp/peer-b.yaml <<YAML
folder: "dest"
transport:
  type: ipfs
  config:
    api: "http://127.0.0.1:5001"
    mfs_root: "/sync/test-08-ipfs"
YAML
        sync-folders addconfig /tmp/peer-b.yaml

        sync-folders sync peer-b

        assert_file_exists "/data/dest/test.txt" "peer-b: test.txt not found"
        log_pass "peer-b: IPFS pull completed"
        ;;
esac
