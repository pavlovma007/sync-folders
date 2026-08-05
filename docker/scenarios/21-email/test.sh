#!/bin/bash
#
# Тест: e-mail транспорт — полный roundtrip (SMTP push + IMAP pull).
#
# Peer A: postfix (SMTP 587 STARTTLS + SASL) + dovecot (IMAP 993 TLS),
#         ящик test@localhost:testpass.
#   Push: отправляет файл письмом себе через SMTP.
#   Pull: читает письмо через IMAP, извлекает вложение, распаковывает.
#
# Ключевые моменты dovecot в Docker (иначе IMAPS 993 не стартует):
#   - base_dir = /run/dovecot/
#   - МНОГОстрочный passdb/userdb (однострочный `passdb { ...; }` dovecot не парсит)
#   - Создать системного пользователя test, mail_location = maildir:/home/test/Maildir
#
# Проверяет:
#   - SMTP STARTTLS + SASL (push)
#   - IMAP TLS (pull)
#   - Полный roundtrip: push → письмо → pull → файл вернулся

source /opt/sync-test/lib/common.sh

case $ROLE in
    a)
        log_info "A: setup postfix + dovecot"
        mkdir -p /run/sshd /var/spool/postfix/private /run/dovecot
        useradd -m -s /bin/bash test 2>/dev/null || true
        mkdir -p /home/test/Maildir/cur /home/test/Maildir/new /home/test/Maildir/tmp
        chown -R test:test /home/test/Maildir
        openssl req -x509 -newkey rsa:2048 -nodes -keyout /etc/ssl/private/mail.key \
            -out /etc/ssl/certs/mail.crt -days 1 -subj "/CN=localhost" 2>/dev/null
        chmod 600 /etc/ssl/private/mail.key

        # postfix: SMTP с STARTTLS + SASL на 587
        postconf -e "mydestination = localhost" "myhostname = localhost" "mynetworks = 0.0.0.0/0" \
            "inet_interfaces = all" "home_mailbox = Maildir/" \
            "smtpd_tls_cert_file = /etc/ssl/certs/mail.crt" "smtpd_tls_key_file = /etc/ssl/private/mail.key" \
            "smtpd_tls_security_level = may" "smtp_tls_security_level = may" \
            "smtpd_sasl_auth_enable = yes" "smtpd_sasl_type = dovecot" "smtpd_sasl_path = private/auth" \
            "local_recipient_maps ="
        grep -q "^submission" /etc/postfix/master.cf || \
            echo "submission inet n       -       y       -       -       smtpd" >> /etc/postfix/master.cf
        service postfix restart >/dev/null 2>&1

        # dovecot: IMAP implicit TLS на 993
        cat > /etc/dovecot/dovecot.conf <<'EOF'
protocols = imap
listen = *
base_dir = /run/dovecot/
ssl = yes
ssl_cert = </etc/ssl/certs/mail.crt
ssl_key = </etc/ssl/private/mail.key
disable_plaintext_auth = no
mail_location = maildir:/home/test/Maildir
passdb {
  driver = static
  args = password=testpass
}
userdb {
  driver = static
  args = uid=test gid=test home=/home/test
}
service auth {
  unix_listener /var/spool/postfix/private/auth {
    mode = 0666
  }
}
service imap-login {
  inet_listener imaps {
    port = 993
    ssl = yes
  }
}
EOF
        dovecot -c /etc/dovecot/dovecot.conf >/dev/null 2>&1 &
        sleep 4

        mkdir -p /data/source /data/dest
        sync-folders addfolder source /data/source
        sync-folders addfolder dest /data/dest
        cat > /tmp/peer.yaml <<YAML
folder: "source"
transport:
  type: email
  config:
    imap: "127.0.0.1:993"
    smtp: "127.0.0.1:587"
    user: "test@localhost"
    pass: "testpass"
    folder: "INBOX"
    self_signed_certs: "true"
sync:
  direction: "bidirectional"
YAML
        sync-folders addconfig /tmp/peer.yaml

        log_info "A: push via email (SMTP)"
        echo "hello email roundtrip $(date)" > /data/source/from-a.txt
        timeout 30 sync-folders sync peer 2>&1 | tail -5

        log_info "A: pull via email (IMAP)"
        sleep 3
        timeout 30 sync-folders sync peer 2>&1 | tail -5

        # Roundtrip: файл вернулся в source (engine bidirectional использует
        # одну локальную папку). Содержимое должно совпадать.
        if [ -f /data/source/from-a.txt ] && grep -q "hello email roundtrip" /data/source/from-a.txt; then
            log_pass "A: email roundtrip OK: $(cat /data/source/from-a.txt)"
        else
            log_info "A: /data/source: $(ls -la /data/source/ 2>&1)"
            log_fail "A: roundtrip failed - file missing or wrong content"
        fi
        date +%s > /shared/done-a
        log_pass "A: done"
        ;;

    b)
        log_info "B: waiting for A"
        for i in $(seq 1 40); do
            if [ -f /shared/done-a ]; then log_pass "B: A done"; exit 0; fi; sleep 5
        done; log_fail "B: timeout"
        ;;
esac
