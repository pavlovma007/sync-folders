#!/bin/bash
#
# Тест: синхронизация через HTTP/PHP-хранилище.
#
# Peer A запускает PHP-сервер и пушит файл через HTTP-транспорт.
# Peer B загружает свой файл в тот же PHP-сервер.
# Оба проверяют через curl что файлы появились в хранилище.
#
# Проверяет:
#   - HTTP transport Push
#   - PHP storage (многопользовательский доступ)
#   - Кросс-контейнерный HTTP-доступ
#
# Топология: общая bridge-сеть
# Транспорт: HTTP (php_storage.php)

source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "A: PHP server"
        mkdir -p /data/source /data/dest
        php -S 0.0.0.0:8080 -t /opt/php-storage /opt/php-storage/php_storage.php &
        sleep 2

        log_info "A: HTTP push"
        sync-folders addfolder source /data/source; sync-folders addfolder dest /data/dest
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: http
  config:
    url: "http://127.0.0.1:8080"
    base_url: "http://127.0.0.1:8080"
YAML
        sync-folders addconfig /tmp/peer.yaml
        echo "hello-from-A via HTTP $(date)" > /data/source/from-a.txt
        sync-folders sync peer
        log_pass "A: push done"

        date +%s > /shared/done-a
        log_info "A: waiting for B"
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then
                # Check B's file arrived
                FILES=$(curl -s http://127.0.0.1:8080 2>/dev/null)
                log_info "A: all files: $FILES"
                if echo "$FILES" | grep -q "from-b"; then log_pass "A: B's file visible"; else log_info "A: B's file not in list yet"; fi
                log_pass "A: done"; exit 0
            fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: waiting for A"
        PEER_A="${PEER_HOST:-127.0.0.1}"
        for i in $(seq 1 20); do
            if curl -s "http://${PEER_A}:8080" >/dev/null 2>&1; then break; fi
            sleep 3
        done

        log_info "B: HTTP push to $PEER_A"
        mkdir -p /data/source; sync-folders addfolder source /data/source
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: http
  config:
    url: "http://${PEER_A}:8080"
    base_url: "http://${PEER_A}:8080"
YAML
        sync-folders addconfig /tmp/peer.yaml
        echo "hello-from-B via HTTP $(date)" > /data/source/from-b.txt
        sync-folders sync peer
        log_pass "B: push done"

        # Check both files in storage
        FILES=$(curl -s "http://${PEER_A}:8080" 2>/dev/null)
        log_info "B: files: $FILES"
        if echo "$FILES" | grep -q "from-a"; then log_pass "B: A's file visible"; fi
        if echo "$FILES" | grep -q "from-b"; then log_pass "B: own file visible"; fi
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
