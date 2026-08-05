#!/bin/bash
#
# Тест: MySQL-транспорт (PHP + MariaDB).
#
# Peer A: запускает MariaDB и PHP-скрипт mysql_storage.php.
# Peer B: подключается к PHP-хранилищу, пушит файл.
#
# Проверяет:
#   - MySQL-транспорт sync-folders
#   - PHP + MariaDB хранилище
#   - Push файла через mysql_storage.php

source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "A: start MariaDB"
        mkdir -p /var/run/mysqld && chown mysql:mysql /var/run/mysqld
        mysqld_safe --skip-grant-tables >/dev/null 2>&1 &
        sleep 6
        # Создаём БД и таблицу
        mysql -uroot <<'SQL' >/dev/null 2>&1
CREATE DATABASE IF NOT EXISTS sync;
USE sync;
CREATE TABLE IF NOT EXISTS files (
    id INT AUTO_INCREMENT PRIMARY KEY,
    file_name VARCHAR(255) NOT NULL,
    file_data LONGBLOB NOT NULL,
    file_group VARCHAR(50) DEFAULT 'default_group',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
SQL

        # Настройка подключения к MySQL (env ДО старта PHP)
        export MYSQL_HOST=127.0.0.1 MYSQL_PORT=3306 MYSQL_USER=root MYSQL_PASS= MYSQL_DB=sync
        log_info "A: start PHP mysql_storage"
        php -S 0.0.0.0:8080 -t /opt/php-storage /opt/php-storage/mysql_storage.php &
        sleep 2

        sync-folders addfolder dest /data/dest
        log_info "A: waiting for B"
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then
                # Проверяем что файл в БД
                CNT=$(mysql -uroot -N -e "SELECT COUNT(*) FROM sync.files WHERE file_name LIKE 'from-b%';" 2>/dev/null)
                if [ "$CNT" -gt 0 ]; then
                    log_pass "A: MySQL file received (count=$CNT)"
                else
                    log_info "A: MySQL files: $(mysql -uroot -N -e 'SELECT file_name FROM sync.files;' 2>/dev/null)"
                    log_fail "A: file not found in MySQL"
                fi
                log_pass "A: done"; exit 0
            fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: setup MySQL transport"
        sleep 3
        PEER_A="${PEER_HOST:-sync-test-19-mysql-a}"
        mkdir -p /data/source
        sync-folders addfolder source /data/source
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: mysql
  config:
    url: "http://${PEER_A}:8080"
    group: "sync"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer.yaml

        log_info "B: push via MySQL to $PEER_A"
        echo "hello from B via MySQL $(date)" > /data/source/from-b.txt
        sync-folders sync peer
        log_pass "B: push done"
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
