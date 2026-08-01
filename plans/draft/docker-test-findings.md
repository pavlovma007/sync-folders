# Docker Integration Tests — Находки

Дата: 2026-07-31

> 🐛 = баг утилиты sync-folders (в план исправления)
> 🧪 = проблема тест-системы / Docker-инфраструктуры (отдельный трекинг)
> ✅ = проверено, работает

---

## Часть 1. Баги утилиты sync-folders (в план исправления)

### B1: Flush удалял staging до того, как qBittorrent получил данные — данные терялись ✅ ИСПРАВЛЕНО

**Файл:** `transport/torrent.go` (`Flush`), `transport/torrent_staging.go`

**Симптом:** peer-a создавал .torrent, добавлял в qBittorrent (savepath=""), но qBittorrent видел state=`stalledDL`, progress=0. Данные отсутствовали — staging удалялся до сидирования.

**Причина:** `tt.staging.Clear()` удалял единственную копию данных (`staging/latest/`), а qBittorrent не знал где файлы (savepath пустой).

**Фикс:** `Staging.PromoteToSeed()` копирует staging/latest/ → seed-директорию, savepath передаётся qBittorrent.

**Проверка:** ✅ После Flush qBittorrent в `stalledUP`, progress=1 (сидирует).

---

### B2: DHT-клиент не публикует манифест в сеть — только в локальную память 🐛 ОТКРЫТ

**Файл:** `dht/client.go`

**Симптом:** `sync-folders dht get <key>` в другом процессе не находит манифест, опубликованный через Flush.

**Причина:** `dht.Client` использует `bep44.NewMemory()` — in-memory Store. Данные не попадают в Mainline DHT.

**Исправление:** Реализовать распределённый put/get — iterative lookup к k ближайшим нодам (traversal + `Server.Put`/`Server.Get`).

---

### B3: `dht.NewClient()` не задаёт bootstrap-ноды 🐛 ОТКРЫТ

**Файл:** `dht/client.go`

**Причина:** `ServerConfig` без `StartingNodes`. Есть готовый `dht.GlobalBootstrapAddrs`.

**Исправление:** `StartingNodes: dht.GlobalBootstrapAddrs` в конфиге сервера.

---

### B4: Hex-декодирование ключей маскирует ошибки 🐛 ОТКРЫТ

**Файл:** `transport/torrent.go` (`newTorrentFromConfig`)

**Причина:** Fallback `[]byte(pk)` при ошибке hex-decoding молча приводит к `ed25519: bad private key length`.

**Исправление:** Валидировать длину (pub=32, priv=64) и возвращать явную ошибку.

---

## Часть 2. Проверено и работает ✅

- [x] `dht.Manifest` sign/verify, keygen
- [x] `dht.Client` put/get (в пределах одного процесса, in-memory store)
- [x] `TorrentTransport.Push` → staging → diff → `.torrent` creation
- [x] qBittorrent `AddTorrentFile` с cookie-auth
- [x] qBittorrent SEEDING после фикса B1 (`stalledUP`, progress=1)
- [x] magnet извлечение из qBittorrent
- [x] `Flush()` вызывается engine после Push
- [x] `DHTClient` interface (Mock + реальный) в TorrentConfig

---

## Часть 3. Проблемы тест-системы (не баги утилиты) 🧪

### T1: qBittorrent metadata exchange между контейнерами нестабилен

**Симптом:** peer-b добавляет magnet, но застревает в `metaDL` (не получает metadata от peer-a), даже в одной сети и с addPeers.

**Анализ:** Известная особенность BitTorrent-клиентов в Docker (NAT, порты, multicast). peer-a сидирует корректно (`stalledUP`), metadata доступна, но peer-b не может установить соединение для BEP-9 exchange.

**Один раз сработало** (ручной тест, ~60 сек), но нестабильно.

**Действия:**
- [ ] Тестировать `--network host` или shared network namespace
- [ ] Или: тестировать torrent download вне Docker (реальные хосты)
- [ ] Или: в тесте проверять только seeding на peer-a (доказательство работы утилиты), не полное P2P

### T2: Файлы скачиваются во вложенную папку `<savepath>/<torrent-name>/`

**Симптом:** qBittorrent сохраняет торрент в `<savepath>/<name>/<file>`, а тест ищет `<savepath>/<file>`.

**Действие:** Исправить пути в test.sh (проверять вложенную папку).

### T3: Оба пира должны быть в одной сети для direct-сценариев

**Симптом:** run.sh всегда создавал разные сети → даже «direct» сценарий без прямой видимости.

**Действие:** ✅ Добавлен `SHARED_NETWORK` в topology.sh (01,04,05,06 = true; 02,03 = false).

---

## Окружение

- Docker: 29.1.3
- Образ: sync-folders-test (ubuntu:22.04 + Go 1.26.5 + qbittorrent-nox + kubo 0.42.0 + php-cli)
