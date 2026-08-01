#!/bin/bash
#
# Тест: двусторонняя синхронизация через торрент.
#
# Оба пира в режиме bidirectional:
# Peer A пушит свои файлы, Peer B пуллит их
# и одновременно пушит свои → Peer A пуллит.
#
# Проверяет:
#   - Bidirectional sync
#
# Топология: общая bridge-сеть
# Транспорт: qBittorrent
# Направление: bidirectional

source /opt/sync-test/lib/common.sh
source /opt/sync-test/lib/test-torrent.sh
tprj="$(echo 05-bidirectional | tr -d ' -')"

case $ROLE in
    a) torrent_seed "${tprj}" "bidirectional" ;;
    b) torrent_leech "${tprj}" "bidirectional" ;;
esac
