#!/bin/bash
#
# Тест: push-синхронизация через торрент с NAT на получателе.
#
# Peer B за NAT (входящие соединения заблокированы),
# Peer A публикует и сидирует.
# Скачивание должно работать через DHT hole punching.
#
# Проверяет:
#   - DHT discovery при NAT
#
# Топология: общая bridge-сеть (NAT отключён для отладки)
# Транспорт: qBittorrent
# Направление: push

source /opt/sync-test/lib/common.sh
source /opt/sync-test/lib/test-torrent.sh
tprj="$(echo 02-push-nat | tr -d ' -')"

case $ROLE in
    a) torrent_seed "${tprj}" "push" ;;
    b) torrent_leech "${tprj}" "pull" ;;
esac
