#!/bin/bash
source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "peer-a: starting PHP storage server"
        mkdir -p /data/uploads
        php -S 0.0.0.0:8080 -t /opt/php-storage /opt/php-storage/php_storage.php &
        sleep 2

        mkdir -p /data/source
        sync-folders addfolder source /data/source

        cat > /tmp/peer-a.yaml <<YAML
folder: "source"
transport:
  type: http
  config:
    url: "http://127.0.0.1:8080"
    base_url: "http://127.0.0.1:8080"
YAML
        sync-folders addconfig /tmp/peer-a.yaml

        echo "hello via HTTP storage" > /data/source/test.txt
        sync-folders sync peer-a
        log_pass "peer-a: HTTP push completed"
        ;;

    b)
        log_info "peer-b: syncing from HTTP storage"
        sleep 3

        mkdir -p /data/dest
        sync-folders addfolder dest /data/dest

        cat > /tmp/peer-b.yaml <<YAML
folder: "dest"
transport:
  type: http
  config:
    url: "http://127.0.0.1:8080"
    base_url: "http://127.0.0.1:8080"
YAML
        sync-folders addconfig /tmp/peer-b.yaml

        sync-folders sync peer-b

        assert_file_exists "/data/dest/test.txt" "peer-b: test.txt not found"
        log_pass "peer-b: HTTP pull completed"
        ;;
esac
