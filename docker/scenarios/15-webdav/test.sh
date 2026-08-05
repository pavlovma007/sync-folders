#!/bin/bash
#
# Тест: WebDAV-транспорт (push A → B) через Apache dav.
#
# Peer A: запускает Apache с WebDAV-модулем.
# Peer B: подключается по WebDAV, пушит файл.
#
# Проверяет:
#   - WebDAV-транспорт sync-folders
#   - Кросс-контейнерный WebDAV-доступ
#   - Push файла в WebDAV-папку

source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "A: configure Apache WebDAV"
        mkdir -p /var/www/webdav /var/www/davlock /data/source /data/dest
        chown -R www-data:www-data /var/www/webdav /var/www/davlock
        # Включение модулей
        a2enmod dav dav_fs >/dev/null 2>&1
        # Конфиг WebDAV
        cat > /etc/apache2/sites-available/webdav.conf <<'EOF'
<VirtualHost *:80>
    ServerName localhost
    DavLockDB /var/www/davlock/DavLock
    Alias /webdav /var/www/webdav
    <Directory /var/www/webdav>
        Dav On
        Options Indexes
        AllowOverride None
        Require all granted
    </Directory>
</VirtualHost>
EOF
        a2ensite webdav >/dev/null 2>&1
        a2dissite 000-default >/dev/null 2>&1
        # Старт Apache
        service apache2 start 2>&1 | tail -1
        sleep 2

        sync-folders addfolder dest /data/dest
        log_info "A: waiting for B"
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then
                if ls /var/www/webdav/ | grep -q "from-b"; then
                    log_pass "A: WebDAV file received"
                else
                    log_info "A: /var/www/webdav: $(ls -la /var/www/webdav/ 2>&1)"
                    log_fail "A: file not found"
                fi
                log_pass "A: done"; exit 0
            fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: setup WebDAV"
        sleep 3
        PEER_A="${PEER_HOST:-sync-test-15-webdav-a}"
        mkdir -p /data/source
        sync-folders addfolder source /data/source
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: webdav
  config:
    url: "http://${PEER_A}/webdav"
    user: ""
    password: ""
    remote_path: ""
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer.yaml

        log_info "B: push via WebDAV to $PEER_A"
        echo "hello from B via WebDAV $(date)" > /data/source/from-b.txt
        sync-folders sync peer
        log_pass "B: push done"
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
