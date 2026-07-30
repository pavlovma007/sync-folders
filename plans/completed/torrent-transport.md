# Torrent-транспорт для sync-folders

## Предпосылки

- DHT: 9/10 важности. Kademlia — единственный живой алгоритм.
- 20M узлов в Mainline DHT. BEP-44 позволяет put/get до 1000 байт.
- NAT: ~47% узлов достижимы напрямую, k=20 реплик дают надёжность 99%+.
- Один .torrent на всю папку (snapshot), не на каждый файл.
- Данные передаются через торрент, в DHT кладём только magnet-ссылку.

## Архитектура

Три компонента:

```
sync-folders
  │
  ├── DHT-клиент (anacrolix/dht)
  │     BEP-44 mutable items: put/get манифеста
  │     Mainline DHT, 20M узлов
  │     Встроенный Go, без внешних процессов
  │
  ├── TorrentClient (interface)
  │     qBittorrent Web API — первый
  │     Deluge / Transmission — опционально следом
  │     Сидирование и скачивание торрентов
  │
  └── TorrentTransport (Transport interface)
        Push → staging → diff → snapshot → DHT publish
        Pull → DHT watch → magnet → download → merge
```

### Сторонние компоненты

| Компонента | sync-folders делает | Внешний инструмент |
|-----------|---------------------|-------------------|
| **DHT** (публикация/слежение) | Весь код | `anacrolix/dht` (Go lib) |
| **Торрент-транспорт** (сидирование/скачивание) | HTTP API клиент | qBittorrent (внешний процесс) |

### Форк qBittorrent — ОТЛОЖЕНО

Форк qBittorrent (C++/Qt) с BEP-44 API описан в `docs/blog-dht-in-torrent.md`.
На первом этапе **не нужен** — DHT делаем через `anacrolix/dht` (Go, Mainline-совместим).
Форк может понадобиться в будущем для нативного UI или расширенных DHT-операций.

---

## DHT-слой (dht/)

### dht/client.go — DHT-клиент на anacrolix/dht

```go
package dht

// Client — обёртка над anacrolix/dht для BEP-44 mutable items.
type Client struct {
    server *dht.Server
    timeout time.Duration
}

// Put публикует mutable item.
// key — Ed25519 публичный ключ (32 байта)
// salt — "sync-folders:" + project
// seq  — монотонный номер версии
// value — подписанный JSON манифеста (≤1000 байт)
func (c *Client) Put(ctx context.Context, key [32]byte, salt string, seq int64, value []byte, priv [64]byte) error

// Get получает последнюю версию mutable item.
func (c *Client) Get(ctx context.Context, key [32]byte, salt string) (value []byte, seq int64, err error)

// Close останавливает DHT-сервер.
func (c *Client) Close() error
```

### dht/manifest.go — манифест и подпись

```go
// Manifest — публикуется в DHT (JSON, ≤500 байт).
type Manifest struct {
    Seq       int64  `json:"seq"`
    Magnet    string `json:"magnet"`     // magnet:?xt=urn:btih:...
    Timestamp int64  `json:"ts"`
    FilesHash string `json:"files_hash"` // sha256(files.json) для быстрого сравнения
}

// Sign подписывает манифест Ed25519.
func (m *Manifest) Sign(priv [64]byte) (sig [64]byte, key [32]byte)

// Verify проверяет подпись.
func (m *Manifest) Verify(key [32]byte, sig [64]byte) bool
```

**Формат BEP-44 ключа:**

```
Key  = Ed25519 публичный ключ (32 байта) — "странный ключ" проекта
Salt = "sync-folders:" + project
Seq  = монотонный номер версии

Value (JSON, ≤500 байт):
{
  "seq": 42,
  "magnet": "magnet:?xt=urn:btih:d4e5f6...",
  "ts": 1700000000,
  "files_hash": "sha256 hex"
}
```

---

## TorrentClient interface

### transport/torrent_client.go

```go
package transport

// TorrentClient — абстракция над торрент-клиентом (qBittorrent, Deluge, …).
type TorrentClient interface {
    Name() string
    AddMagnet(magnetURI string, savePath string) (hash string, err error)
    AddTorrentFile(data []byte, savePath string) (hash string, err error)
    GetInfo(hash string) (TorrentInfo, error)
    List() ([]TorrentInfo, error)
    Delete(hash string, deleteFiles bool) error
    Test() error
}

type TorrentInfo struct {
    Hash     string
    Name     string
    Progress float64   // 0.0 – 1.0
    State    string    // downloading, seeding, paused, error
    SavePath string
}
```

### transport/torrent_qb.go — qBittorrent

```
POST /api/v2/torrents/add    — form-data: urls, savepath
GET  /api/v2/torrents/info   — список с хэшами, статусами
POST /api/v2/torrents/delete — form-data: hashes, deleteFiles
```

### transport/torrent_deluge.go — Deluge (JSON-RPC)

```json
{"method": "core.add_torrent_url", "params": ["magnet:", {"download_location": "/path"}]}
{"method": "core.remove_torrent", "params": ["hash", true]}
```

### transport/torrent_transmission.go — Transmission (JSON-RPC)

```json
{"method": "torrent-add", "arguments": {"filename": "magnet:", "download-dir": "/path"}}
```

---

## TorrentTransport — алгоритмы

### Push: накопление + diff + снапшот

Engine вызывает `Push()` для каждого файла, затем **один раз** `Flush()`.

```
Engine.Push(f1) ──→ TorrentTransport копирует файл в staging/
Engine.Push(f2)       (.sync-torrent-staging/latest/)
Engine.Push(f3)

Engine.Flush() ──→
  1. Сравнить staging/ с .torrent-last-manifest
     (ndjson, по одной строке на файл: путь, размер, mtime, sha256)
  2. Если ни один файл не изменился → return nil (ничего не делаем)
  3. Если изменились:
     a. Создать .torrent из ВСЕХ файлов папки (не только изменённых)
     b. magnet = magnet:?xt=urn:btih:<info_hash>
     c. Переместить staging/latest/ → seq-N/ (история)
     d. Добавить .torrent в qBittorrent на раздачу (seed)
     e. DHT publish(seq++, magnet, files_hash)
     f. Записать .torrent-last-manifest (новый ndjson)
     g. Очистить staging/
     h. Удалить старые раздачи (keep_seeds, max_seed_age)
```

**Push — однократное копирование.** В отличие от debounce-подхода, `Push()` просто копирует файл.
Только `Flush()` — тяжёлая операция (diff, .torrent, DHT, qBittorrent).
Это дешевле: файлы не переписываются повторно при debounce.

### Staging-директория

```
.sync-torrent-staging/
  latest/           — текущая версия (накопление)
    docs/
      readme.md
    images/
      logo.png
  seq-42/           — зафиксированный снапшот
  seq-41/
```

Staging = полная копия папки на момент Push, чтобы .torrent создавался из консистентного снимка.

### Diff через NDJSON-манифест

```ndjson
# .torrent-last-manifest (sorted by path)
{"p":"docs/readme.md","s":1024,"m":1700000001,"h":"abc123..."}
{"p":"images/logo.png","s":2048,"m":1700000002,"h":"def456..."}
```

- Читается построчно (`bufio.Scanner`), без загрузки всего JSON в память
- `p` (path) — первый для сортировки
- `h` (sha256) — для сравнения содержимого
- Если sha256 совпадает у всех файлов — `Flush()` ничего не делает

### Pull: Watch → Magnet → Download → Merge

Фоновая горутина (стартует при `NewTorrentTransport`):

```
1. Каждые 30 сек: DHT get(key, salt)
2. Если seq > last_seen_seq И подпись валидна:
   a. Добавляем magnet в qBittorrent (download_dir = /tmp/sync-torrents/xxx)
   b. Ждём завершения загрузки (прогресс 100%)
   c. Сравниваем загруженное с локальной папкой:
      - Новые файлы → копировать
      - Изменённые (sha256 не совпадает) → копировать
      - Файлы, которых нет в снапшоте, но есть локально → ОСТАВИТЬ
      (не удаляем, консервативно)
   d. Оставляем торрент на сидировании
   e. last_seen_seq = новый seq
```

**Merge-стратегии** (конфигурируются):

| Режим | Поведение |
|-------|-----------|
| `keep_local` (default) | Только дополнять/обновлять. Локальные файлы не трогать. |
| `mirror_remote` | Полная репликация удалённой папки (удаляет лишнее локально). |

### Управление старыми раздачами

| Параметр | По умолчанию | Описание |
|----------|-------------|----------|
| `keep_seeds` | 3 | Хранить последние N версий в сидировании |
| `max_seed_age` | 168h | Удалять раздачи старше N часов |

При `Flush()`: после публикации новой версии проверяем список всех раздач,
удаляем торренты сверх лимита по возрасту.

---

## YAML-конфиг

```yaml
folder: "shared"
transport:
  type: torrent
  config:
    # Торрент-клиент
    client: "qbittorrent"              # qbittorrent | deluge | transmission
    api_url: "http://127.0.0.1:8080"
    api_user: "admin"
    api_password: "${QB_PASS}"         # можно и текстом, и env-переменной
    download_dir: "/tmp/sync-torrents"  # временная папка, не целевая

    # DHT
    dht_public_key: "dC8xX2..."        # base64, 32 байта
    dht_private_key: "${DHT_KEY}"      # base64, 64 байта
    project: "my-project"              # salt = "sync-folders:" + project

    # Управление раздачами
    keep_seeds: "3"
    max_seed_age: "168h"
    snapshot_merge: "keep_local"       # keep_local | mirror_remote
sync:
  period: "5m"
  direction: "push"                    # push | pull | bidirectional
```

**Примечание:** `download_dir` — временная папка. После скачивания торрента
sync-folders копирует новые/изменённые файлы в целевую папку.
Это гарантирует консистентность: если скачивание прервалось — целевая папка не повреждена.

---

## CLI-утилиты

### DHT операции

```bash
# Публикация манифеста в DHT
sync-folders dht put \
  --key "dC8xX2..." \
  --priv "pK3mR9..." \
  --salt "my-project" \
  --value '{"seq":42,"magnet":"magnet:?xt=urn:btih:d4e5f6...","ts":1700000000,"files_hash":"abc..."}' \
  --seq 42

# Получение манифеста из DHT
sync-folders dht get --key "dC8xX2..." --salt "my-project"
# → {"seq": 42, "magnet": "magnet:?...", "ts": 1700000000, "files_hash": "abc..."}

# Слежение за обновлениями в реальном времени
sync-folders dht watch --key "dC8xX2..." --salt "my-project"
# → seq=42 magnet=magnet:?...
# → seq=43 magnet=magnet:?...  (ждёт и выводит новые)
```

### Управление ключами

```bash
# Генерация Ed25519 пары
sync-folders torrent keygen my-project
# → public_key:  dC8xX2... (base64, 32 байта)
# → private_key: pK3mR9... (base64, 64 байта)
```

### Диагностика

```bash
# Статус торрент-клиента и DHT
sync-folders torrent status
# → Client: qBittorrent (running)
# → DHT: connected (248 nodes)
# → Active torrents: 3
```

---

## Структура файлов

```
dht/
  client.go       — DHT-клиент (anacrolix/dht, BEP-44 put/get)
  manifest.go     — Manifest + Ed25519 sign/verify

transport/
  torrent_client.go        — TorrentClient interface
  torrent.go               — TorrentTransport (Transport + Flush)
  torrent_staging.go       — staging-директория + diff
  torrent_qb.go            — qBittorrent Web API client
  torrent_deluge.go        — Deluge JSON-RPC client (опционально)
  torrent_transmission.go  — Transmission RPC client (опционально)
  torrent_test.go          — тесты

cmd/
  torrent.go               — dht-put, dht-get, dht-watch, keygen
```

---

## Этапы (TDD — тесты вперёд)

Каждый этап включает тесты до или вместе с реализацией.

| # | Шаг | Описание | Файлы |
|---|------|----------|-------|
| 1 | **Тесты моков** | mock DHT-сервер, mock qBittorrent API, mock торрент-клиент | `dht/mock_test.go`, `transport/torrent_mock_test.go` |
| 2 | **DHT-клиент** | anacrolix/dht, BEP-44 put/get, под тесты | `dht/client.go`, `dht/client_test.go` |
| 3 | **Манифест** | Manifest + Ed25519 sign/verify | `dht/manifest.go`, `dht/manifest_test.go` |
| 4 | **TorrentClient + qBittorrent** | interface + qBittorrent реализация | `transport/torrent_client.go`, `transport/torrent_qb.go`, тесты |
| 5 | **Staging + diff + Flush** | ndjson-манифест, сравнение, .torrent creation | `transport/torrent_staging.go`, тесты |
| 6 | **TorrentTransport** | Push/Pull через staging, Factory | `transport/torrent.go`, тесты |
| 7 | **CLI** | dht-put, dht-get, dht-watch, keygen, status | `cmd/torrent.go`, тесты |
| 8 | **Pull-cycle** | watch → magnet → download → merge (фоновая горутина) | `transport/torrent.go`, тесты |
| 9 | **Deluge / Transmission** | опциональные клиенты | `torrent_deluge.go`, `torrent_transmission.go` |
| 10 | **Интеграционные тесты** | реальный qBittorrent + два sync-folders | `transport/torrent_integration_test.go` |

---

## Верификация

```bash
go vet ./... && go build ./...

# Модульные тесты
go test ./dht/... -v
go test ./transport/... -run Torrent -v

# Интеграционные (требуют qBittorrent)
go test ./transport/... -run TorrentIntegration -v -tags=integration
```

Ключевые сценарии тестов:

1. **Push → staging → Flush → DHT publish** (с моками)
2. **Diff без изменений → Flush no-op** (ничего не публикуется)
3. **Diff с изменениями → новый .torrent** (создаётся и публикуется)
4. **Pull → watch → magnet → download → merge** (с моками)
5. **keep_seeds / max_seed_age** — очистка старых раздач
6. **NDJSON манифест** — построчное чтение тысяч записей
7. **qBittorrent client** — AddMagnet, GetInfo, Delete, ошибки
8. **DHT Client** — put/get, подпись, верификация
