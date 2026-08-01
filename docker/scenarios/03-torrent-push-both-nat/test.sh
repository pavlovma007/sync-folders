#!/bin/bash
#
# Тест: push-синхронизация через торрент с NAT на обоих пирах.
#
# Оба пира за NAT. DHT hole punching должен обеспечить
# установку соединения для скачивания.
#
# Проверяет:
#   - DHT discovery при NAT на обоих
#
# Топология: общая bridge-сеть (NAT отключён для отладки)
# Транспорт: qBittorrent
# Направление: push

source /opt/sync-test/lib/common.sh
source /opt/sync-test/lib/test-torrent.sh
tprj="$(echo 03-push-both-nat | tr -d ' -')"

case $ROLE in
    a) torrent_seed "${tprj}" "push" ;;
    b) torrent_leech "${tprj}" "pull" ;;
esac
