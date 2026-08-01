#!/bin/bash
#
# Тест: pull-синхронизация через торрент.
#
# Peer A сидирует (push), Peer B инициирует скачивание (pull).
# Отличается от 01 тем, что направление синхронизации — pull.
#
# Проверяет:
#   - Режим pull (B инициирует)
#
# Топология: общая bridge-сеть
# Транспорт: qBittorrent
# Направление: pull

source /opt/sync-test/lib/common.sh
source /opt/sync-test/lib/test-torrent.sh
tprj="$(echo 04-pull-direct | tr -d ' -')"

case $ROLE in
    a) torrent_seed "${tprj}" "push" ;;
    b) torrent_leech "${tprj}" "pull" ;;
esac
