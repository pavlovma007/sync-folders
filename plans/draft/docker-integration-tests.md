# Docker Integration Tests для sync-folders

## Цель

Изолированное E2E-тестирование sync-folders в Docker-контейнерах с реальными внешними сервисами (qBittorrent, IPFS, PHP) и различными сетевыми топологиями.

## Архитектура

### Образ

Один Docker-образ, включающий всё необходимое:

| Компонента | Назначение |
|-----------|-----------|
| sync-folders (Go) | Собранный бинарник |
| qBittorrent | Торрент-клиент для seed/download |
| IPFS | IPFS-узел |
| PHP + php_storage.php | HTTP-хранилище |
| iptables | NAT/simulation сетевых условий |
| bash, jq, curl | Подготовка и проверки в test.sh |

### Топология

Два контейнера peer-a и peer-b, каждый со своим транспортом и конфигом.

```
         Интернет (Mainline DHT)
                │
        ┌───────┴───────┐
    ┌───┴───┐       ┌───┴───┐
    │ net-a │       │ net-b │
    │ bridge│       │bridge │
    └───┬───┘       └───┬───┘
        │               │
   ┌────┴────┐     ┌────┴────┐
   │ peer-a  │     │ peer-b  │
   │ qBittor.│     │ qBittor.│
   │ sync-f  │     │ sync-f  │
   │ IPFS    │     │ IPFS    │
   │ PHP     │     │         │
   └─────────┘     └─────────┘
```

- У peer-a и peer-b есть интернет (доступ к Mainline DHT, bootstrap-узлам)
- net-a и net-b — изолированные bridge-сети Docker
- При NAT-сценарии: одна из сетей (или обе) дополнительно закрываются iptables
- Транспорт данных: торрент-клиенты находят друг друга через DHT (hole punching)
- Манифест (magnet) передаётся через DHT (BEP-44, `anacrolix/dht`)

---

## Структура файлов

```
docker/
├── Dockerfile                        # Описание образа
├── .dockerignore                     # Что не включать в образ
├── run.sh                            # Оркестратор на хосте
├── lib/
│   ├── common.sh                     # Функции: assert, wait_for, log_pass/fail
│   └── topology.sh                   # Создание сетей, iptables правил для NAT
└── scenarios/
    ├── README.md                     # Шаблон для нового сценария
    ├── 01-push-direct/
    │   ├── peer-a.yaml               # Конфиг sync-folders для A
    │   ├── peer-b.yaml               # Конфиг sync-folders для B
    │   ├── test.sh                   # Общий скрипт теста
    │   └── topology.sh               # Описание топологии
    ├── 02-push-nat/
    │   ├── ...
    ├── 03-push-both-nat/
    │   ├── ...
    ├── 04-pull-direct/
    │   ├── ...
    ├── 05-bidirectional/
    │   ├── ...
    ├── 06-qb-offline/
    │   ├── ...
    ├── 07-ipfs/
    │   ├── ...
    └── 08-http/
        ├── ...
```

---

## Компоненты

### docker/Dockerfile

Многостадийная сборка:

```dockerfile
# Stage 1: сборка sync-folders
FROM golang:1.26 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o sync-folders .

# Stage 2: финальный образ
FROM ubuntu:22.04

# Установка зависимостей
RUN apt-get update && apt-get install -y \
    qbittorrent-nox \         # торрент-клиент
    iptables \                # для NAT симуляции
    jq curl \                 # вспомогательные утилиты
    php-cli \                 # для http-тестов
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/sync-folders /usr/local/bin/sync-folders
COPY transport/php_storage.php /opt/php-storage/php_storage.php

# IPFS (установка бинарника)
RUN curl -sL https://dist.ipfs.tech/kubo/latest/kubo_linux_amd64.tar.gz \
    | tar xz -C /usr/local --strip-components=1

# Точка входа
COPY docker/lib/ /opt/sync-test/lib/
COPY docker/scenarios/ /opt/sync-test/scenarios/

ENTRYPOINT ["/opt/sync-test/lib/entrypoint.sh"]
```

### docker/run.sh — оркестратор

```bash
#!/bin/bash
# Использование: ./run.sh <scenario-name> [--keep]
set -euo pipefail

SCENARIO="${1:?Usage: run.sh <scenario-name>}"
SCENARIO_DIR="$(dirname "$0")/scenarios/$SCENARIO"
KEEP="${2:-}"

if [ ! -d "$SCENARIO_DIR" ]; then
    echo "Scenario not found: $SCENARIO"
    exit 1
fi

source "$SCENARIO_DIR/topology.sh"
source "$(dirname "$0")/lib/common.sh"

# 1. Создать сети
SCENARIO_NET_A="sync-test-${SCENARIO}-a"
SCENARIO_NET_B="sync-test-${SCENARIO}-b"
docker network create --subnet 10.${SCENARIO_ID}.1.0/24 "$SCENARIO_NET_A"
docker network create --subnet 10.${SCENARIO_ID}.2.0/24 "$SCENARIO_NET_B"

# 2. Собрать образ (если не собран)
docker build -t sync-folders-test -f "$(dirname "$0")/Dockerfile" .

# 3. Запустить peer-a с монтированием сценария
docker run -d --name "peer-a-$SCENARIO" \
    --net "$SCENARIO_NET_A" \
    -e ROLE=peer-a \
    -e SCENARIO="$SCENARIO" \
    -v "$SCENARIO_DIR:/scenario:ro" \
    --volume "sync-vol-$SCENARIO:/shared" \
    sync-folders-test \
    /opt/sync-test/lib/scenario-entrypoint.sh

# 4. Запустить peer-b
docker run -d --name "peer-b-$SCENARIO" \
    --net "$SCENARIO_NET_B" \
    -e ROLE=peer-b \
    -e SCENARIO="$SCENARIO" \
    -v "$SCENARIO_DIR:/scenario:ro" \
    --volume "sync-vol-$SCENARIO:/shared" \
    sync-folders-test \
    /opt/sync-test/lib/scenario-entrypoint.sh

# 5. Применить NAT-правила (если topology.sh указала)
apply_nat "$SCENARIO_NET_A" "$SCENARIO_NET_B"

# 6. Ждать завершения (или timeout)
wait_for_containers "peer-a-$SCENARIO" "peer-b-$SCENARIO" 300

# 7. Проверить exit codes
check_results "peer-a-$SCENARIO" "peer-b-$SCENARIO"

# 8. Cleanup (если не --keep)
if [ -z "$KEEP" ]; then
    docker rm -f "peer-a-$SCENARIO" "peer-b-$SCENARIO"
    docker volume rm "sync-vol-$SCENARIO" 2>/dev/null
    docker network rm "$SCENARIO_NET_A" "$SCENARIO_NET_B"
fi
```

### docker/lib/topology.sh — описание топологии

```bash
# Параметры сетевой топологии для каждого сценария
SCENARIO_ID=1          # ID для подсетей (10.{ID}.1.0/24)

# NAT настройки
NAT_A_SRC=""           # CIDR, для которого NAT_BLOCKED (пусто = нет NAT)
NAT_B_SRC=""           # Если указан — iptables блокирует входящие

# Пример для 02-push-nat (peer-b за NAT):
# NAT_B_SRC="10.2.1.0/24"  # блокируем входящие на net-b из net-a
```

### docker/lib/common.sh — вспомогательные функции

```bash
# assert_eq <expected> <actual> <message>
# wait_for <condition_cmd> <timeout_sec> <interval_sec>
# log_pass <message>
# log_fail <message>

# Следит за логами контейнера до появления строки
wait_for_log() {
    local container="$1" pattern="$2" timeout="${3:-60}"
    timeout "$timeout" bash -c "
        docker logs --tail=10 -f \"$container\" 2>&1 \
        | head -n 1000 \
        | grep -q \"$pattern\"
    " 2>/dev/null
}

# Проверяет что файл существует в контейнере
assert_file_exists() {
    local container="$1" path="$2"
    docker exec "$container" test -f "$path"
}
```

### test.sh (внутри сценария)

```bash
#!/bin/bash
# Выполняется ВНУТРИ контейнера. ROLE = peer-a | peer-b

set -euo pipefail
source /opt/sync-test/lib/common.sh

case $ROLE in
    peer-a)
        # 1. Стартуем qBittorrent
        qbittorrent-nox -d --webui-port=8080
        
        # 2. Ждём пока qBittorrent запустится
        wait_for_port 8080 30
        
        # 3. Генерируем DHT-ключи (один раз)
        sync-folders torrent keygen "test-$SCENARIO" > /tmp/dht-keys.txt
        DHT_PUB=$(grep public_key /tmp/dht-keys.txt | awk '{print $2}')
        DHT_PRIV=$(grep private_key /tmp/dht-keys.txt | awk '{print $2}')
        
        # 4. Создаём тестовый файл
        mkdir -p /data/source /data/dest
        echo "hello from peer-a at $(date)" > /data/source/test.txt
        
        # 5. Запускаем sync-folders push
        sync-folders sync --config /scenario/peer-a.yaml \
            --dht-key "$DHT_PUB" --dht-priv "$DHT_PRIV"
        
        # 6. Ждём завершения (проверяем через DHT что манифест опубликован)
        sync-folders dht get "$DHT_PUB" "sync-folders:test-$SCENARIO"
        log_pass "peer-a: manifest published"
        ;;
        
    peer-b)
        # 1. Стартуем qBittorrent
        qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30
        
        # 2. Ждём DHT-манифест от peer-a (с retry)
        for i in $(seq 1 10); do
            if sync-folders dht get "$DHT_PUB" "sync-folders:test-$SCENARIO" 2>/dev/null; then
                log_pass "peer-b: found manifest from peer-a"
                break
            fi
            sleep 5
        done
        
        # 3. Проверяем что файл скачался
        assert_file_exists "/data/dest/test.txt" "peer-b: file not found"
        
        # 4. Проверяем содержимое
        content=$(cat /data/dest/test.txt)
        if echo "$content" | grep -q "hello from peer-a"; then
            log_pass "peer-b: file content verified"
        else
            log_fail "peer-b: content mismatch"
        fi
        ;;
esac
```

---

## Сценарии

### 01-push-direct — прямая синхронизация

| Параметр | Значение |
|----------|----------|
| Сети | net-a (10.1.1.0/24), net-b (10.1.2.0/24) |
| NAT | Нет |
| Транспорт | торрент (qBittorrent) |
| Направление | push (A → B) |
| Данные | 1 текстовый файл |

**topology.sh:**
```bash
SCENARIO_ID=1
NAT_A_SRC=""  # без NAT
NAT_B_SRC=""
```

### 02-push-nat — получатель за NAT

| Параметр | Значение |
|----------|----------|
| Сети | net-a (10.2.1.0/24), net-b (10.2.2.0/24) |
| NAT | B невидим для A (блокируем входящие на net-b) |
| Транспорт | торрент (qBittorrent) |
| Направление | push (A → B) |
| Данные | 3 файла, вложенная структура |

**topology.sh:**
```bash
SCENARIO_ID=2
NAT_B_SRC="10.2.1.0/24"  # A не может инициировать соединение к B
```

### 03-push-both-nat — оба за NAT

| Параметр | Значение |
|----------|----------|
| NAT | Оба через iptables (MASQUERADE) + блокировка входящих |
| Транспорт | торрент (qBittorrent, hole punching) |
| Направление | push (A → B) |

### 04-pull-direct — инициатор — получатель

| Параметр | Значение |
|----------|----------|
| Транспорт | торрент (qBittorrent) |
| Направление | pull (B инициирует, A сидирует) |

### 05-bidirectional — двусторонняя синхронизация

| Параметр | Значение |
|----------|----------|
| Транспорт | торрент (qBittorrent) |
| Направление | bidirectional |
| Данные | A: file-a.txt, B: file-b.txt, оба: common.txt |

### 06-two-stage — синхронизация в 2 стадии

| Параметр | Значение |
|----------|----------|
| Транспорт | торрент (qBittorrent) |
| Направление | push (A → B) |

Сценарий: файлы добавляются в папку A поэтапно. B получает их после каждой стадии.

```
Стадия 1: A создаёт a.txt → Push → B получает a.txt
Стадия 2: A создаёт b.txt → Push → B получает a.txt + b.txt
Стадия 3: A меняет a.txt  → Push → B получает новую версию a.txt
```

Это проверяет:
- Debounce/накопление файлов перед .torrent
- Diff detection — .torrent не пересоздаётся если файлы не менялись
- Инкрементальное обновление — B не скачивает всё заново

**test.sh:**
```bash
case $ROLE in
    peer-a)
        qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30
        sync-folders torrent keygen "two-stage" > /tmp/keys.txt
        DHT_PUB=$(grep public_key /tmp/keys.txt | awk '{print $2}')
        DHT_PRIV=$(grep private_key /tmp/keys.txt | awk '{print $2}')
        echo "$DHT_PUB" > /shared/public-key.txt
        
        mkdir -p /data/source /data/dest
        
        # Стадия 1: один файл
        echo "file-a version 1" > /data/source/a.txt
        sync-folders sync --config /scenario/peer-a.yaml \
            --dht-key "$DHT_PUB" --dht-priv "$DHT_PRIV"
        log_pass "peer-a: stage 1 done (a.txt)"
        sleep 2
        
        # Стадия 2: добавляем второй файл
        echo "file-b version 1" > /data/source/b.txt
        sync-folders sync --config /scenario/peer-a.yaml \
            --dht-key "$DHT_PUB" --dht-priv "$DHT_PRIV"
        log_pass "peer-a: stage 2 done (a.txt + b.txt)"
        sleep 2
        
        # Стадия 3: меняем первый файл
        echo "file-a version 2" > /data/source/a.txt
        sync-folders sync --config /scenario/peer-a.yaml \
            --dht-key "$DHT_PUB" --dht-priv "$DHT_PRIV"
        log_pass "peer-a: stage 3 done (a.txt updated)"
        ;;
        
    peer-b)
        qbittorrent-nox -d --webui-port=8080
        wait_for_port 8080 30
        DHT_PUB=$(cat /shared/public-key.txt)
        
        for stage in 1 2 3; do
            sleep 10  # ждём пока peer-a опубликует
            sync-folders dht get "$DHT_PUB" "sync-folders:two-stage"
            
            # Проверяем какие файлы уже скачались
            if [ -f /data/dest/a.txt ]; then
                log_pass "peer-b: stage $stage — a.txt present"
            fi
            if [ -f /data/dest/b.txt ]; then
                log_pass "peer-b: stage $stage — b.txt present"
            fi
        done
        
        # Финальная проверка
        content_a=$(cat /data/dest/a.txt)
        if [ "$content_a" = "file-a version 2" ]; then
            log_pass "peer-b: a.txt is latest version"
        else
            log_fail "peer-b: a.txt wrong version: $content_a"
        fi
        ;;
esac
```

### 06-qb-offline — qBittorrent не запущен

| Параметр | Значение |
|----------|----------|
| Транспорт | торрент (qBittorrent НЕ запущен) |
| Ожидание | sync-folders не падает, ошибка логируется |

### 08-ipfs — IPFS транспорт

| Параметр | Значение |
|----------|----------|
| Транспорт | IPFS (MFS) |
| Подготовка | `ipfs init`, `ipfs daemon` на peer-a |
| Синхронизация | A → B через общий IPFS-узел или PubSub |

### 09-http — PHP-хранилище

| Параметр | Значение |
|----------|----------|
| Транспорт | HTTP |
| Подготовка | php_storage.php запущен на peer-a |
| Синхронизация | A пушит файлы в HTTP-хранилище, B пуллит |

---

## Makefile target

```makefile
.PHONY: test-docker

test-docker:
	@echo "=== Docker Integration Tests ==="
	@for scenario in $$(ls docker/scenarios/); do \
		echo "Running: $$scenario"; \
		docker/run.sh $$scenario || exit 1; \
	done
	@echo "All Docker tests passed"
```

---

## Использование

```bash
# Все тесты
make test-docker

# Один сценарий
docker/run.sh 01-push-direct

# Сценарий с сохранением контейнеров (для отладки)
docker/run.sh 02-push-nat --keep

# Вручную посмотреть логи
docker logs peer-a-01-push-direct
```
