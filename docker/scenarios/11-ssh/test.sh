#!/bin/bash
#
# Тест: SSH-транспорт (push A → B).
#
# Peer A: запускает sshd, генерирует SSH-ключ, делится им.
# Peer B: подключается к peer-a по SSH, пушит файл.
#
# Проверяет:
#   - SSH-транспорт sync-folders
#   - Кросс-контейнерный SSH-доступ
#   - Push файла на удалённый SSH-сервер

source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "A: start sshd"
        mkdir -p /data/remote /data/source /data/dest /run/sshd
        # Host keys для sshd
        ssh-keygen -A >/dev/null 2>&1
        # SSH-ключ для подключения
        ssh-keygen -t ed25519 -f /root/.ssh/id_ed25519 -N "" >/dev/null 2>&1
        cat /root/.ssh/id_ed25519.pub >> /root/.ssh/authorized_keys
        chmod 600 /root/.ssh/authorized_keys
        # Копия ключа для peer-b
        cp /root/.ssh/id_ed25519 /shared/ssh-key
        # Старт sshd
        /usr/sbin/sshd
        sleep 2
        # Проверка что sshd слушает
        for i in $(seq 1 10); do
            ss -tlnp 2>/dev/null | grep -q ":22" && { log_pass "A: sshd listening"; break; }
            sleep 1
        done

        sync-folders addfolder dest /data/dest

        log_info "A: waiting for B"
        for i in $(seq 1 30); do
            if [ -f /shared/done-b ]; then
                # Проверяем что файл появился на SSH-сервере
                if [ -f /data/remote/from-b.txt ]; then
                    log_pass "A: file received via SSH: $(cat /data/remote/from-b.txt)"
                else
                    log_info "A: /data/remote: $(ls -la /data/remote/ 2>&1)"
                    log_fail "A: file not found"
                fi
                log_pass "A: done"; exit 0
            fi; sleep 5
        done; log_fail "A: timeout"
        ;;

    b)
        log_info "B: setup SSH key"
        sleep 3
        # Получаем ключ от peer-a
        for i in $(seq 1 20); do if [ -f /shared/ssh-key ]; then break; fi; sleep 3; done
        [ -f /shared/ssh-key ] || log_fail "B: no ssh key"
        cp /shared/ssh-key /root/.ssh/id_ed25519
        chmod 600 /root/.ssh/id_ed25519

        PEER_A="${PEER_HOST:-sync-test-11-ssh-a}"
        mkdir -p /data/source
        sync-folders addfolder source /data/source
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: ssh
  config:
    host: "${PEER_A}"
    port: "22"
    user: "root"
    key: "/root/.ssh/id_ed25519"
    remote_path: "/data/remote"
sync:
  direction: "push"
YAML
        sync-folders addconfig /tmp/peer.yaml

        log_info "B: push via SSH to $PEER_A"
        echo "hello from B via SSH $(date)" > /data/source/from-b.txt
        sync-folders sync peer
        log_pass "B: push done"
        date +%s > /shared/done-b; log_pass "B: done"
        ;;
esac
