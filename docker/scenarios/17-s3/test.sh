#!/bin/bash
#
# Тест: S3-транспорт (push A → B) через MinIO.
#
# Peer A: запускает MinIO, создаёт bucket.
# Peer B: подключается по S3, пушит файл.
#
# Проверяет:
#   - S3-транспорт sync-folders
#   - Кросс-контейнерный S3-доступ
#   - Push файла в S3 bucket

source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "A: start MinIO"
        mkdir -p /data/minio /data/source /data/dest
        export MINIO_ROOT_USER=admin MINIO_ROOT_PASSWORD=adminadmin TERM=xterm
        minio server /data/minio --address 0.0.0.0:9000 &
        sleep 4
        # Создаём bucket через mkbucket (minio-go)
        mkbucket 127.0.0.1:9000 admin adminadmin make sync 2>&1
        log_pass "A: bucket setup"

        sync-folders addfolder dest /data/dest
        log_info "A: waiting for B"
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then
                # Проверяем что объект появился (через minio-go)
                if mkbucket 127.0.0.1:9000 admin adminadmin check sync 2>&1 | tee /tmp/s3check.txt | grep -q "found"; then
                    log_pass "A: S3 object received: $(cat /tmp/s3check.txt | grep object)"
                else
                    log_info "A: S3 check: $(cat /tmp/s3check.txt 2>&1)"
                    log_fail "A: object not found"
                fi
                log_pass "A: done"; exit 0
            fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: setup S3"
        sleep 3
        PEER_A="${PEER_HOST:-sync-test-17-s3-a}"
        mkdir -p /data/source
        sync-folders addfolder source /data/source
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: s3
  config:
    endpoint: "${PEER_A}:9000"
    access_key: "admin"
    secret_key: "adminadmin"
    bucket: "sync"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer.yaml

        log_info "B: push via S3 to $PEER_A"
        echo "hello from B via S3 $(date)" > /data/source/from-b.txt
        sync-folders sync peer
        log_pass "B: push done"
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
