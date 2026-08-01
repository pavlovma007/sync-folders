# Docker Integration Tests — Результаты

Дата: 2026-08-01

## Сводка

✅ **Все 10/10 сценариев проходят** (`make test-docker`).

| # | Сценарий | Статус | Примечание |
|---|----------|--------|------------|
| 01 | torrent-push-direct | ✅ PASS | Прямая push-синхронизация: A → B |
| 02 | torrent-push-nat | ✅ PASS | С NAT (iptables, раздельные сети) |
| 03 | torrent-push-both-nat | ✅ PASS | Оба за NAT |
| 04 | torrent-pull-direct | ✅ PASS | Pull-синхронизация (B инициирует) |
| 05 | torrent-bidirectional | ✅ PASS | Двусторонняя |
| 06 | torrent-two-stage | ✅ PASS | Многостадийная публикация |
| 07 | qb-offline | ✅ PASS | qBittorrent не запущен — sync не падает |
| 08 | ipfs | ✅ PASS | IPFS add/get + CID через shared volume |
| 09 | http-php | ✅ PASS | HTTP push обоих пиров в общий PHP-сервер |
| 10 | ipfs-pubsub | ✅ PASS | IPFS PubSub publish/subscribe |

---

## Что проверяет каждый сценарий

### 01-torrent-push-direct
- TorrentTransport.Push → staging
- Engine.Flush() вызывается после Push
- BuildSnapshot: создание .torrent из staging/
- PromoteToSeed: сохранение данных перед сидированием
- qBittorrent AddTorrentFile с cookie-auth → seeding (stalledUP)
- DHT-публикация манифеста (BEP-44)
- Peer-to-peer скачивание через qBittorrent

### 02-torrent-push-nat
- То же что 01, NAT на peer B (iptables FORWARD DROP)

### 03-torrent-push-both-nat
- То же что 01, NAT на обоих пирах

### 04-torrent-pull-direct
- Pull-направление: B инициирует скачивание

### 05-torrent-bidirectional
- Двусторонняя синхронизация (A и B пушат свои файлы)

### 06-torrent-two-stage
- Многостадийная публикация (несколько снапшотов)
- Diff detection: .torrent не пересоздаётся если файлы не менялись
- Инкрементальное обновление

### 07-qb-offline
- Устойчивость к отсутствию qBittorrent
- sync-folders НЕ падает, ошибка логируется

### 08-ipfs
- IPFS init/add → CID
- Обмен CID через shared volume
- ipfs get (скачивание по CID) через swarm connect

### 09-http-php
- PHP-хранилище (php_storage.php)
- HTTP transport Push обоих пиров
- Проверка файлов в хранилище

### 10-ipfs-pubsub
- IPFS PubSub publish/subscribe
- Discovery через multiaddr (bootstrap + swarm connect)
- Данные через ipfs get

---

## Найденные и исправленные баги утилиты

| # | Баг | Статус |
|---|-----|--------|
| B1 | Flush удалял staging до сидирования — данные терялись | ✅ Исправлено (Staging.PromoteToSeed) |
| B2 | Engine не вызывал Flush() после Push | ✅ Исправлено (type assert в core/engine.go) |
| B3 | TorrentConfig.DHTClient захардкожен на mock — реальный DHT не подключался | ✅ Исправлено (dht.DHTClient interface) |
| B4 | Hex-ключи из конфига не декодировались → ed25519: bad private key length | ✅ Исправлено (hex.DecodeString) |
| B5 | qBittorrent Web API без cookie-auth → все запросы 403 | ✅ Исправлено (login + doWithAuth) |

**Открытые баги** (в план исправления, см. `docker-test-findings.md`):
- B6: DHT-клиент публикует только в локальную память, не в Mainline DHT
- B7: `dht.NewClient()` не задаёт bootstrap-ноды
- B8: hex-decode fallback маскирует ошибки ключей

---

## Ключевые исправления тест-системы

1. **Координация пиров через shared volume** — magnet/CID/сигналы между контейнерами
2. **SHARED_NETWORK** — общая bridge-сеть для direct-сценариев; раздельные + iptables для NAT
3. **PEER_HOST** — hostname peer-a передаётся в peer-b через env (Docker DNS)
4. **Общая библиотека lib/test-torrent.sh** — qb_login, qb_get, qb_post
5. **IPFS**: bootstrap + `swarm connect` для установления libp2p-связи
6. **kubo v0.42**: `ipfs pubsub pub <topic> <file-path>` — данные как файл, не строка

---

## Docker

- **Образ:** `sync-folders-test` (ubuntu:22.04 + Go 1.26.5 + qBittorrent-nox + kubo 0.42.0 + php-cli)
- **Сети:** изолированные bridge-сети, общая сеть для direct-сценариев
- **Таймаут:** 600 сек на контейнер
- **Cleanup:** автоматический после теста, `--keep` для отладки

---

## Запуск

```bash
# Все 10 сценариев
make test-docker

# Или вручную
for s in $(ls docker/scenarios/ | sort); do docker/run.sh "$s"; done

# Один сценарий с отладкой
docker/run.sh 01-torrent-push-direct --keep
```
