# sync-folders

Утилита синхронизации файлов между вашим компьютером и удалённым хранилищем.
Всё в одном бинарнике — никаких Node.js, Python или Docker не нужно.
Работает на Linux, macOS, Windows и Termux (Android).

**Оглавление:**
- [Быстрый старт за 3 минуты](#быстрый-старт-за-3-минуты)
- [SSH/SCP — для своего сервера](#sshscp--для-своего-сервера)
- [FTP — для старого хостинга](#ftp--для-старого-хостинга)
- [WebDAV — для Nextcloud](#webdav--для-nextcloud)
- [S3 — для облачного хранилища](#s3--для-облачного-хранилища)
- [HTTP (PHP) — для любого хостинга](#http-php--для-любого-хостинга)
- [Email — почтовый ящик как хранилище](#email--почтовый-ящик-как-хранилище)
- [MySQL (PHP) — хостинг с БД](#mysql-php--хостинг-с-бд)
- [IPFS — децентрализованное хранилище](#ipfs--децентрализованное-хранилище)
- [Tor — через анонимную сеть](#tor--через-анонимную-сеть)
- [Torrent / P2P — децентрализованный](#torrent--p2p--децентрализованный)
- [JS-фильтры — что и когда синхронизировать](#js-фильтры--что-и-когда-синхронизировать)
- [Команды](#команды)
- [Что делать если...](#что-делать-если)

---

## Быстрый старт за 3 минуты

Допустим, у вас есть папка `~/Projects` с важными файлами, и вы хотите
автоматически копировать их на сервер по SSH каждые 30 минут.

```bash
# 1. Установить (собрать из исходников)
cd sync
./build.sh

# 2. Зарегистрировать папку
sync-folders addfolder my-projects ~/Projects

# 3. Создать файл конфига (backup.yaml):
cat > backup.yaml << 'ENDCONFIG'
folder: "my-projects"
transport:
  type: ssh
  config:
    host: "myserver.com"
    port: "22"
    user: "root"
    key: "~/.ssh/id_ed25519"
    remote_path: "/backups/projects"
sync:
  period: "30m"
  direction: "push"
ENDCONFIG

# 4. Добавить конфиг в программу
sync-folders addconfig backup.yaml

# 5. Запустить синхронизацию
sync-folders sync backup          # один раз
sync-folders daemon               # или в фоне (каждые 30 мин)
```

Готово. Ваши файлы теперь автоматически бэкапятся на сервер.

## Содержание

- [Быстрый старт за 3 минуты](#быстрый-старт-за-3-минуты)
- [Пошаговые инструкции для каждого транспорта](#пошаговые-инструкции-для-каждого-транспорта)
  - [SSH/SCP — для своего сервера](#sshscp--для-своего-сервера)
  - [FTP — для старого хостинга](#ftp--для-старого-хостинга)
  - [WebDAV — для Nextcloud](#webdav--для-nextcloud)
  - [S3 — для облачного хранилища](#s3--для-облачного-хранилища)
  - [HTTP (PHP) — для любого хостинга](#http-php--для-любого-хостинга)
  - [Email — почтовый ящик как хранилище](#email--почтовый-ящик-как-хранилище)
  - [MySQL (PHP) — хостинг с БД](#mysql-php--хостинг-с-бд)
  - [IPFS — децентрализованное хранилище](#ipfs--децентрализованное-хранилище)
  - [Tor — через анонимную сеть](#tor--через-анонимную-сеть)
- [JS-фильтры — что и когда синхронизировать](#js-фильтры--что-и-когда-синхронизировать)
- [Команды](#команды)
- [Что делать если...](#что-делать-если)

---

## Пошаговые инструкции для каждого транспорта

### SSH/SCP — для своего сервера

**Когда нужно:** У вас есть VPS или выделенный сервер с доступом по SSH.
**Как работает:** Программа подключается к серверу по SSH и копирует файлы через SCP.

**Шаг 1. Подготовить SSH-ключ**
```bash
# Если ключа ещё нет — создать
ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519

# Скопировать ключ на сервер
ssh-copy-id user@myserver.com

# Проверить что заходит без пароля
ssh user@myserver.com "echo ok"
# → ok
```

**Шаг 2. Зарегистрировать папку**
```bash
sync-folders addfolder projects ~/Projects
```

**Шаг 3. Создать конфиг (`vps-backup.yaml`)**
```yaml
folder: "projects"                 # имя папки (как при addfolder)
description: "Backup to VPS"       # что это за конфиг

transport:
  type: ssh                        # тип транспорта
  config:
    host: "myserver.com"           # адрес сервера
    port: "22"                     # порт SSH (обычно 22)
    user: "root"                   # ваш логин на сервере
    key: "~/.ssh/id_ed25519"       # путь к SSH-ключу
    remote_path: "/backups"        # куда класть файлы на сервере

sync:
  period: "1h"                     # проверять каждый час (в daemon-режиме)
  direction: "push"                # только на сервер (бэкап)
```

**Шаг 4. Добавить конфиг и запустить**
```bash
sync-folders addconfig vps-backup.yaml
sync-folders sync vps-backup       # запустить один раз
sync-folders daemon                # или запустить в фоне
```

---

### FTP — для старого хостинга

**Когда нужно:** У вас есть FTP-доступ к хостингу (например, Timeweb, Beget).
**Как работает:** Программа подключается к FTP-серверу как обычный FTP-клиент.

**Шаг 1. Создать конфиг (`ftp-shared.yaml`)**
```yaml
folder: "docs"
transport:
  type: ftp
  config:
    host: "ftp.example.com"        # FTP-сервер
    port: "21"                     # порт (обычно 21)
    user: "team"                   # логин
    password: "${FTP_PASS}"        # пароль (через переменную окружения!)
    remote_path: "/shared"         # папка на FTP
sync:
  period: "5m"                     # каждые 5 минут
  direction: "bidirectional"       # в обе стороны
```

**Важно:** Пароль пишите через `${FTP_PASS}`, а сам пароль храните в переменной окружения:
```bash
export FTP_PASS="your-secret-password"
```

**Шаг 2. Запустить**
```bash
sync-folders addconfig ftp-shared.yaml
sync-folders sync ftp-shared
```

---

### WebDAV — для Nextcloud

**Когда нужно:** У вас есть Nextcloud, Owncloud или любой WebDAV-сервер.
**Как работает:** Программа работает с файлами через WebDAV-протокол (PUT/GET/PROPFIND).

**Где взять URL:**

| Сервис | Как получить URL |
|--------|----------------|
| Nextcloud | `https://your-nc.com/remote.php/dav/files/username/` |
| Owncloud | `https://your-oc.com/remote.php/dav/files/username/` |
| Другой WebDAV | URL, который дал администратор |

**Конфиг (`nextcloud-sync.yaml`):**
```yaml
folder: "docs"
transport:
  type: webdav
  config:
    url: "https://nextcloud.example.com/remote.php/dav/files/user/"
    user: "user"
    password: "${NC_PASS}"
    remote_path: "sync"
sync:
  period: "10m"
  direction: "push"
```

**Запуск:**
```bash
export NC_PASS="your-password"
sync-folders addconfig nextcloud-sync.yaml
sync-folders sync nextcloud-sync
```

---

### S3 — для облачного хранилища

**Когда нужно:** У вас есть S3-совместимое хранилище (AWS S3, Yandex Object Storage, Minio, DigitalOcean Spaces).

**Где взять данные:**

| Параметр | Где взять |
|----------|-----------|
| endpoint | `storage.yandexcloud.net` для Яндекса, `s3.amazonaws.com` для AWS |
| access_key | В личном кабинете облачного провайдера |
| secret_key | Там же |
| bucket | Название созданного bucket'а |

**Конфиг (`s3-backup.yaml`):**
```yaml
folder: "backup"
transport:
  type: s3
  config:
    endpoint: "storage.yandexcloud.net"
    access_key: "${AWS_KEY}"
    secret_key: "${AWS_SECRET}"
    bucket: "my-backup"
    prefix: "data/"
sync:
  period: "24h"
  direction: "push"
```

**Запуск:**
```bash
export AWS_KEY="your-access-key"
export AWS_SECRET="your-secret-key"
sync-folders addconfig s3-backup.yaml
sync-folders sync s3-backup
```

---

### HTTP (PHP) — для любого хостинга

**Когда нужно:** У вас есть любой хостинг с поддержкой PHP (почти все).
**Как работает:** Закидываете один PHP-файл на хостинг, и он превращается в файловое хранилище.

**Шаг 1. Загрузить PHP-скрипт на хостинг**
```bash
# Скопировать php_storage.php на ваш хостинг
scp transport/php_storage.php user@myserver.com:~/public_html/
```
Или загрузите [`transport/php_storage.php`](transport/php_storage.php) через FTP/SFTP в любую папку на хостинге.

**Шаг 2. Проверить что скрипт работает**
```bash
curl https://myserver.com/php_storage.php
# → []   (пустой JSON — значит работает)
```

**Шаг 3. Создать конфиг (`http-backup.yaml`):**
```yaml
folder: "backup"
transport:
  type: http
  config:
    url: "https://myserver.com/php_storage.php"   # URL PHP-скрипта
    base_url: "https://myserver.com"               # базовый URL хостинга
    auth: "${HTTP_AUTH}"                           # опционально: user:password
    self_signed_certs: "true"                      # если HTTPS с самоподписанным сертификатом
sync:
  period: "1h"
  direction: "push"
```

**Запуск:**
```bash
sync-folders addconfig http-backup.yaml
sync-folders sync http-backup
```

---

### Email — почтовый ящик как хранилище

**Когда нужно:** Нет доступа к серверу, но есть email (Gmail, Yandex, корпоративная почта).
**Как работает:** Программа отправляет файлы вам на почту (как вложение) и читает их оттуда же.

**Конфиг (`email-sync.yaml`):**
```yaml
folder: "docs"
transport:
  type: email
  config:
    imap: "imap.example.com:993"        # IMAP-сервер (TLS)
    smtp: "smtp.example.com:587"        # SMTP-сервер (STARTTLS)
    user: "user@example.com"            # ваш email
    pass: "${EMAIL_PASS}"               # пароль от почты
    folder: "INBOX"                     # папка (обычно INBOX)
    compress: "true"                    # сжимать файлы gzip
    self_signed_certs: "true"           # если сертификат самоподписанный
sync:
  period: "1h"
  direction: "bidirectional"
```

**Где взять настройки для популярных почтовых сервисов:**

| Сервис | IMAP | SMTP |
|--------|------|------|
| Gmail | `imap.gmail.com:993` | `smtp.gmail.com:587` (нужен пароль приложения) |
| Yandex | `imap.yandex.ru:993` | `smtp.yandex.ru:587` |
| Mail.ru | `imap.mail.ru:993` | `smtp.mail.ru:587` |
| Яндекс 360 | `imap.yandex.ru:993` | `smtp.yandex.ru:587` |

**Запуск:**
```bash
export EMAIL_PASS="your-email-password"
sync-folders addconfig email-sync.yaml
sync-folders sync email-sync
```

---

### MySQL (PHP) — хостинг с БД

**Когда нужно:** У вас есть хостинг с PHP и MySQL. Хостинги часто дают 30-100 ГБ под БД.
**Как работает:** PHP-скрипт сохраняет файлы в MySQL (LONGBLOB), группирует по проектам.

**Шаг 1. Загрузить PHP-скрипт**
```bash
scp transport/mysql_storage.php user@myserver.com:~/public_html/
```

**Шаг 2. Настроить подключение к БД на сервере**
Создайте или отредактируйте файл `.env` рядом с `mysql_storage.php`:
```bash
# На сервере, рядом с mysql_storage.php
export MYSQL_HOST="localhost"
export MYSQL_DB="file_storage"
export MYSQL_USER="root"
export MYSQL_PASS="your-db-password"
```

Или задайте переменные через панель хостинга (если есть).

**Шаг 3. Проверить**
```bash
curl https://myserver.com/mysql_storage.php
# → []
```

**Шаг 4. Конфиг (`mysql-backup.yaml`):**
```yaml
folder: "backup"
transport:
  type: mysql
  config:
    url: "https://myserver.com/mysql_storage.php"
    group: "my-sync"                            # группа (для нескольких проектов)
    auth: "${MYSQL_AUTH}"                       # опционально
    self_signed_certs: "true"
sync:
  period: "1h"
  direction: "push"
```

---

### IPFS — децентрализованное хранилище

**Когда нужно:** Хотите хранить файлы децентрализованно, без привязки к одному серверу.
**Как работает:** Использует HTTP API IPFS-демона. Три режима: MFS, PubSub, Гибрид.

**Шаг 1. Установить и запустить IPFS**
```bash
# Скачать IPFS
wget https://dist.ipfs.tech/kubo/v0.27.0/kubo_v0.27.0_linux-amd64.tar.gz
tar -xzf kubo_*.tar.gz
cd kubo && sudo bash install.sh

# Инициализировать и запустить
ipfs init
ipfs daemon &

# Проверить что API работает
curl http://127.0.0.1:5001/api/v0/version
```

**Режим MFS (общий узел):**
Если у вас есть сервер с IPFS, к которому подключаются все участники:
```yaml
folder: "shared"
transport:
  type: ipfs
  config:
    api: "http://192.168.1.100:5001"     # общий IPFS-узел
    mfs_root: "/sync/my-project"         # виртуальная папка в MFS
```

**Режим PubSub (каждый со своим IPFS):**
Если у каждого участника свой IPFS-демон:
```yaml
folder: "shared"
transport:
  type: ipfs
  config:
    api: "http://127.0.0.1:5001"         # локальный IPFS
    pubsub_topic: "/sync/a1b2c3d4"       # одинаковый канал для всех
    pin: "true"                          # закреплять файлы
```

**Полная инструкция по подготовке MFS:**
```bash
# На сервере (один раз):
ipfs files mkdir -p /sync/my-project
ipfs files ls /sync/my-project
```

**Полная инструкция по подготовке PubSub:**
```bash
# На каждом компьютере:
ipfs config --bool Pubsub.Enabled true
ipfs daemon &
```

---

### Tor — через анонимную сеть

**Когда нужно:** Спрятать трафик синхронизации, подключиться к .onion-сайтам.
**Как работает:** Все запросы транспорта направляются через SOCKS5-прокси Tor.

**Шаг 1. Установить и запустить Tor**
```bash
# Ubuntu/Debian
sudo apt install tor
sudo systemctl start tor

# Arch
sudo pacman -S tor
sudo systemctl start tor

# macOS
brew install tor
tor &

# Проверить что Tor работает
curl --socks5-hostname 127.0.0.1:9050 https://check.torproject.org/
# → Congratulations. This browser is configured to use Tor.
```

**Шаг 2. Создать конфиг с прокси**

Вариант 1: Через `WrapWithProxy` (на лету, без отдельного конфига).
Вариант 2: Через TorProxy (всё в одном конфиге).

**Способ 1: Просто добавить proxy к любому транспорту**
```yaml
folder: "docs"
transport:
  type: http                                         # любой транспорт
  config:
    url: "https://xyz.onion/php_storage.php"         # .onion адрес
    auth: "${HTTP_AUTH}"
    self_signed_certs: "true"
    proxy: "socks5://127.0.0.1:9050"                 # Tor SOCKS5
sync:
  period: "1h"
  direction: "push"
```

**Способ 2: Явный Tor транспорт (обёртка)**
```yaml
folder: "docs"
transport:
  type: tor
  config:
    proxy: "socks5://127.0.0.1:9050"                # адрес Tor
    inner_type: "ssh"                                # внутренний транспорт
    host: "xyz.onion"                                # .onion адрес сервера
    port: "22"
    user: "admin"
    key: "~/.ssh/id_ed25519"
    remote_path: "/backup"
sync:
  period: "1h"
  direction: "push"
```

**Запуск:**
```bash
# Убедиться что Tor запущен
systemctl status tor

# Запустить синхронизацию
sync-folders addconfig tor-backup.yaml
sync-folders sync tor-backup
```

---

### Torrent / P2P — децентрализованный

**Когда нужно:** Синхронизация между компьютерами без общего сервера, через торренты и DHT.
**Как работает:** sync-folders публикует magnet-ссылку папки в DHT (BEP-44, `anacrolix/dht`),
а файлы передаются через торрент-клиент (qBittorrent/Deluge/Transmission).

**Шаг 1. Сгенерировать ключи**
```bash
sync-folders torrent keygen my-project
# → public_key:  dC8xX2...
# → private_key: pK3mR9...
```

**Шаг 2. Конфиг (`torrent-sync.yaml`)**
```yaml
folder: "shared"
transport:
  type: torrent
  config:
    client: "qbittorrent"              # qbittorrent | deluge | transmission
    api_url: "http://127.0.0.1:8080"
    api_user: "admin"
    api_password: "${QB_PASS}"
    download_dir: "/tmp/sync-torrents"
    dht_public_key: "dC8xX2..."        # hex, 32 байта
    dht_private_key: "${DHT_KEY}"      # hex, 64 байта
    project: "my-project"              # salt = "sync-folders:" + project
    keep_seeds: "3"                    # хранить последние N раздач
sync:
  period: "5m"
  direction: "push"                    # push | pull | bidirectional
```

**Проверка DHT:**
```bash
sync-folders dht put <ключ> <priv> <salt> <seq> '<json>'
sync-folders dht get <ключ> <salt>
```

---

## JS-фильтры — что и когда синхронизировать

Фильтры позволяют исключить ненужные файлы до отправки или получения.
Например: не синхронизировать файлы больше 10 МБ, или только документы PDF.

Фильтр — это JavaScript-функция, которая получает список файлов и возвращает
только те, которые нужно синхронизировать.

**Где писать фильтры:** Прямо в YAML-конфиге, в полях `send_filter` (перед отправкой)
и `receive_filter` (перед получением).

### Пример 1: Только маленькие файлы

Не хотите забивать канал большими файлами — исключите всё, что больше 10 МБ:

```yaml
sync:
  send_filter: |
    function filter(files, ctx) {
      // files — массив файлов в папке
      // ctx — контекст (имя папки, направление)
      // ctx.folder — путь к папке на диске
      // ctx.direction — "send" или "receive"

      return files.filter(function(f) {
        // f.size — размер файла в байтах
        // 10 МБ = 10 * 1024 * 1024 байт
        // Оставляем только файлы меньше 10 МБ
        return f.size < 10 * 1024 * 1024;
      });
    }
```

### Пример 2: Только изображения и документы

Нужны только фото и PDF — всё остальное (архивы, exe, видео) исключаем:

```yaml
sync:
  send_filter: |
    function filter(files, ctx) {
      // Список расширений, которые РАЗРЕШЕНЫ к синхронизации
      var allowedExtensions = ['.jpg', '.png', '.pdf'];

      return files.filter(function(f) {
        // Проверяем, заканчивается ли имя файла на .jpg, .png или .pdf
        // f.name — имя файла (например "photo.jpg")
        // .some() проверяет, есть ли хотя бы одно совпадение в массиве
        // .toLowerCase() — чтобы "Photo.JPG" тоже подходило
        return allowedExtensions.some(function(ext) {
          return f.name.toLowerCase().endsWith(ext);
        });
      });
    }
```

### Пример 3: Только свежие файлы (за последние 7 дней)

Не трогаем старые файлы — экономим трафик:

```yaml
sync:
  send_filter: |
    function filter(files, ctx) {
      // Вычисляем время "7 дней назад"
      // Date.now() —当前 время в миллисекундах
      // Делим на 1000 — переводим в секунды (как в f.mod_time)
      // 86400 — количество секунд в одном дне
      var sevenDaysAgo = Date.now() / 1000 - 7 * 86400;

      return files.filter(function(f) {
        // f.mod_time — дата последнего изменения файла
        // Это Unix-время в секундах (например 1700000000)
        // Оставляем только те, что новее 7 дней
        return f.mod_time > sevenDaysAgo;
      });
    }
```

### Пример 4: Исключить временные файлы

Редакторы (VS Code, Vim) создают временные файлы — их синхронизировать не нужно:

```yaml
sync:
  send_filter: |
    function filter(files, ctx) {
      return files.filter(function(f) {
        // Исключаем файлы, которые заканчиваются на:
        return !f.name.endsWith('.tmp') &&   // временные файлы
               !f.name.endsWith('.swp') &&   // файлы восстановления Vim
               !f.name.endsWith('.bak');     // резервные копии
      });
    }
```

### Пример 5: Разные фильтры для разных папок

У вас несколько папок, и для каждой свои правила:

```yaml
sync:
  send_filter: |
    function filter(files, ctx) {
      // ctx.folder содержит путь к папке
      // Например: "/home/user/Photos" или "/home/user/Documents"

      // Если это папка с фото — разрешаем только изображения
      if (ctx.folder.indexOf('Photos') !== -1) {
        return files.filter(function(f) {
          return f.name.endsWith('.jpg') || f.name.endsWith('.png');
        });
      }

      // Если это папка с документами — максимум 50 МБ
      if (ctx.folder.indexOf('Documents') !== -1) {
        return files.filter(function(f) {
          return f.size < 50 * 1024 * 1024;
        });
      }

      // Для всех остальных папок — пропускаем всё
      return files;
    }
```

### Поля каждого файла (что можно проверять в фильтре)

```javascript
// Каждый элемент в массиве files выглядит так:
{
  name:     "photo.jpg",         // имя файла
  path:     "sub/photo.jpg",     // путь относительно папки
  size:     102400,              // размер в байтах
  mod_time: 1700000000,          // дата изменения (Unix timestamp в секундах)
  is_dir:   false                // true если это папка
}

// Контекст ctx:
ctx.folder       // "/home/user/sync" — полный путь к папке
ctx.direction    // "send" | "receive" — направление синхронизации
```

---

## Команды

```bash
# Веб-интерфейс (откроется браузер)
sync-folders

# Текстовое меню
sync-folders tui

# Управление папками
sync-folders addfolder <имя> <путь>         # добавить папку
sync-folders removefolder <имя>             # удалить папку
sync-folders folders                        # список папок

# Управление конфигами
sync-folders addconfig <файл.yaml>          # добавить конфиг
sync-folders removeconfig <имя>             # удалить конфиг
sync-folders configs                        # список конфигов
sync-folders config template                # показать шаблон конфига

# Синхронизация
sync-folders sync <имя>                     # запустить один раз
sync-folders sync --all                     # запустить все конфиги
sync-folders daemon                         # запустить в фоне
sync-folders dry <имя>                      # пробный прогон (без изменений)

# Torrent / DHT
sync-folders torrent keygen <проект>        # сгенерировать Ed25519 ключи
sync-folders dht put <ключ> <priv> <salt> <seq> '<json>'   # публикация в DHT
sync-folders dht get <ключ> <salt>          # чтение из DHT
sync-folders dht watch <ключ> <salt>        # слежение за обновлениями DHT

# Помощь
sync-folders --help
```

---

## Что делать если...

**Не заходит по SSH**
```bash
# Проверьте ключ
ssh -i ~/.ssh/id_ed25519 user@server.com

# Если ошибка "permissions", исправьте:
chmod 600 ~/.ssh/id_ed25519
```

**Не работает PHP-скрипт**
```bash
# Проверьте что PHP установлен
php -v

# Проверьте скрипт напрямую
curl https://your-site.com/php_storage.php
# Должен вернуть [] или JSON
```

**Не подключается к IMAP/SMTP**
```bash
# Проверьте через openssl
openssl s_client -connect imap.gmail.com:993 -crlf
# Должен показать сертификат и приглашение "* OK"
```

**Не видит файлы после синхронизации**
```bash
# Проверьте что папка зарегистрирована
sync-folders folders

# Проверьте что конфиг добавлен
sync-folders configs

# Проверьте фильтры — возможно они исключают все файлы
```

**Ошибка "self_signed_certs" при подключении**
Добавьте в конфиг:
```yaml
config:
  self_signed_certs: "true"
```

**Хочу, чтобы пароль не светился в конфиге**
Используйте переменные окружения:
```yaml
config:
  password: "${MY_PASSWORD}"
```
А в терминале:
```bash
export MY_PASSWORD="secret"
sync-folders sync my-config
```

---

## Сборка из исходников

```bash
cd sync
./build.sh                  # сборка под 8 платформ + тесты

# Или через Makefile:
make build     # собрать для текущей платформы
make test      # запустить все тесты
make test-v    # тесты подробно
make check     # go vet + go fmt
make clean     # удалить бинарник
```

## Архитектура проекта

```
cmd/root.go          — CLI: разбор аргументов, запуск команд
cmd/tui.go           — текстовое меню (TUI)
cmd/webgui.go        — веб-интерфейс (встроенный HTTP-сервер)

core/config.go       — управление папками и конфигами (JSON)
core/types.go        — типы: SyncConfig, FileInfo, Direction
core/engine.go       — движок синхронизации: SyncEngine, Daemon

transport/
  interface.go       — интерфейс Transport и фабрика Factory()
  ssh.go / ftp.go / webdav.go / s3.go / http.go
  email.go / mysql.go / ipfs.go / tor.go
  torrent*.go        — TorrentTransport (staging, diff, .torrent)
  torrent_qb.go / torrent_deluge.go / torrent_transmission.go

dht/
  manifest.go        — манифест + Ed25519 подпись
  client.go          — BEP-44 клиент (anacrolix/dht)

filter/
  engine.go          — JS-движок (goja) для фильтрации файлов

db/journal.go        — SQLite-журнал операций синхронизации

docker/
  run.sh             — оркестратор Docker-интеграционных тестов
  scenarios/         — 17 сценариев (торренты, IPFS, SSH, FTP, WebDAV, S3, MySQL, email, Tor)
```

**Как работает SyncEngine:**

```
SyncEngine.RunOnce()
  ├── push()          сканировать локальную папку
  │   → применить send_filter (JS)
  │   → для каждого файла: transport.Push()
  │
  ├── pull()          запросить список удалённых файлов
  │   → применить receive_filter (JS)
  │   → для каждого файла: transport.Pull()
  │
  └── bidirectional() push() + pull()

Daemon(interval)
  └── каждые N минут: для каждого конфига → RunOnce()
```

---

## Тесты

```bash
cd sync
make test     # go test -count=1 -timeout 120s ./...
make test-v   # подробный вывод
make check    # go vet + go fmt

# Или напрямую:
/home/mp/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64/bin/go test -v ./...

# Пакет транспортов (~90 тестов):
#   SSH, FTP, WebDAV, S3, HTTP, Email, MySQL, IPFS, Tor
#
# Пакет core (3 теста):
#   TestSyncNewFiles, TestSyncDirectionPull, TestSyncConflictNewer
#
# Пакет filter (4 теста):
#   TestFilterJS, TestFilterEmptyJS, TestFilterByName, TestFilterSyntaxError

# Docker-интеграционные тесты (17 сценариев, 2 узла):
make test-docker
# или вручную: docker/run.sh <сценарий>
docker/run.sh 01-torrent-push-direct
docker/run.sh 11-ssh
# подробнее: docs/blog-docker-integration-tests.md
```

---

## Что уже сделано и что планируется

### ✅ Уже реализовано

| Транспорт | Как работает |
|-----------|-------------|
| **SSH/SCP** | Синхронизация через SCP по SSH-ключу. Любой VDS/VPS с доступом по SSH. |
| **FTP** | Синхронизация через FTP(S). Общие папки на FTP-серверах. |
| **WebDAV** | Синхронизация через WebDAV. Nextcloud, Owncloud и любые WebDAV-серверы. |
| **S3** | Синхронизация с S3-совместимыми хранилищами. Minio, Yandex Object Storage, AWS S3. |
| **HTTP (PHP-хостинг)** | Один PHP-файл на любом хостинге. Файлы загружаются с уникальным суффиксом (защита от перезаписи). |
| **E-mail** | Почтовый ящик как файловое хранилище. IMAP для чтения, SMTP для отправки. Файлы сжимаются gzip. |
| **MySQL (PHP+БД)** | Хранение файлов в LONGBLOB на любом хостинге с MySQL. Группировка файлов, Basic Auth. |
| **IPFS** | Глобальная файловая система через HTTP API IPFS-демона. MFS/PubSub/Гибрид режимы. |
| **Tor / Dark Net** | Прокси-слой поверх любого транспорта через SOCKS5 (Tor). Собственный SOCKS5 клиент. |
| **Torrent / P2P** | Торрент-транспорт: DHT-публикация манифеста (BEP-44, anacrolix/dht), qBittorrent/Deluge/Transmission для сидирования. |

### 📋 Планируется к реализации

| Приоритет | Транспорт | Описание |
|-----------|-----------|----------|
| 1 | **Local / USB / флешки** | синхронизация через внешний носитель с авто-запуском при подключении (inotify/udev) |
| 2 | **Git** | синхронизация через git push/pull, поддержка SSH и HTTPS |


---

## Лицензия

MIT
