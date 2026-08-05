#!/bin/bash
#
# Тест: FTP-транспорт (push A → B).
#
# Peer A: запускает vsftpd (anonymous, write enabled).
# Peer B: подключается по FTP, пушит файл.
#
# Проверяет:
#   - FTP-транспорт sync-folders
#   - Кросс-контейнерный FTP-доступ
#   - Push файла на FTP-сервер

source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "A: configure vsftpd"
        mkdir -p /data/ftp /data/source /data/dest /var/run/vsftpd/empty
        cat > /etc/vsftpd.conf <<'EOF'
listen=YES
anonymous_enable=YES
write_enable=YES
anon_root=/data/ftp
anon_upload_enable=YES
anon_mkdir_write_enable=YES
anon_other_write_enable=YES
pasv_enable=YES
pasv_min_port=40000
pasv_max_port=40050
port_enable=YES
EOF
        # anon_root не-writable (иначе vsftpd отказывается), загрузка в /upload
        chown -R root:root /data/ftp
        mkdir -p /data/ftp/upload
        chmod 777 /data/ftp/upload
        vsftpd /etc/vsftpd.conf &
        sleep 2

        sync-folders addfolder dest /data/dest
        log_info "A: waiting for B"
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then
                if [ -f /data/ftp/upload/from-b.txt ]; then
                    log_pass "A: file received via FTP: $(cat /data/ftp/from-b.txt)"
                else
                    log_info "A: /data/ftp/upload: $(ls -la /data/ftp/upload/ 2>&1)"
                    log_fail "A: file not found"
                fi
                log_pass "A: done"; exit 0
            fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: setup FTP"
        sleep 3
        PEER_A="${PEER_HOST:-sync-test-13-ftp-a}"
        mkdir -p /data/source
        sync-folders addfolder source /data/source
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: ftp
  config:
    host: "${PEER_A}"
    port: "21"
    user: "anonymous"
    password: ""
    remote_path: "/upload"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer.yaml

        log_info "B: push via FTP to $PEER_A"
        echo "hello from B via FTP $(date)" > /data/source/from-b.txt
        sync-folders sync peer
        log_pass "B: push done"
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
