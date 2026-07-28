package cmd

import (
	"fmt"
	"os"
	"sync-folders/core"
	"time"
)

// Run — точка входа для CLI.
func Run() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	switch os.Args[1] {
	case "addfolder":
		if len(os.Args) < 4 {
			fmt.Println("Usage: sync-folders addfolder <name> <path>")
			return
		}
		if err := core.AddFolder(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Folder %q added: %s\n", os.Args[2], os.Args[3])

	case "removefolder":
		if len(os.Args) < 3 {
			fmt.Println("Usage: sync-folders removefolder <name>")
			return
		}
		if err := core.RemoveFolder(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Folder %q removed\n", os.Args[2])

	case "folders":
		folders, err := core.ListFolders()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		for _, f := range folders {
			fmt.Printf("%s\t%s\n", f.Name, f.Path)
		}

	case "addconfig":
		if len(os.Args) < 3 {
			fmt.Println("Usage: sync-folders addconfig <file.yaml>")
			return
		}
		if err := core.AddConfig(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Config %q added\n", os.Args[2])

	case "removeconfig":
		if len(os.Args) < 3 {
			fmt.Println("Usage: sync-folders removeconfig <name>")
			return
		}
		if err := core.RemoveConfig(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Config %q removed\n", os.Args[2])

	case "configs":
		cfgs, err := core.ListConfigs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		for name, sc := range cfgs {
			fmt.Printf("%s\t-> folder %q (%s)\n", name, sc.Folder, sc.Transport.Type)
		}

	case "sync":
		if len(os.Args) < 3 {
			fmt.Println("Usage: sync-folders sync <name|--all>")
			return
		}
		if os.Args[2] == "--all" {
			core.Daemon(0)
		} else {
			cfg, err := core.LoadConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			sc, ok := cfg.Syncs[os.Args[2]]
			if !ok {
				fmt.Fprintf(os.Stderr, "Config %q not found\n", os.Args[2])
				os.Exit(1)
			}
			engine, err := core.NewSyncEngine(os.Args[2], sc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			if err := engine.RunOnce(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

	case "daemon":
		interval := 5 * time.Minute
		if len(os.Args) >= 4 && os.Args[2] == "--interval" {
			d, err := time.ParseDuration(os.Args[3])
			if err == nil {
				interval = d
			}
		}
		fmt.Printf("Daemon started, interval=%v\n", interval)
		if err := core.Daemon(interval); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "dry":
		if len(os.Args) < 3 {
			fmt.Println("Usage: sync-folders dry <name> [--direction send|receive] [--file path]")
			return
		}
		cfg, err := core.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		sc, ok := cfg.Syncs[os.Args[2]]
		if !ok {
			fmt.Fprintf(os.Stderr, "Config %q not found\n", os.Args[2])
			os.Exit(1)
		}
		fmt.Printf("Dry-run for %q (no files will be changed)\n", os.Args[2])
		_ = sc

	case "config":
		if len(os.Args) >= 3 && os.Args[2] == "template" {
			fmt.Println(`# ═══════════════════════════════════════════════════════════════
# sync-folders — полный шаблон конфига
# ═══════════════════════════════════════════════════════════════
# Использование:
#   1. Сохранить как my-sync.yaml
#   2. sync-folders addconfig my-sync.yaml
#   3. sync-folders sync my-sync
#
# Все опции документированы. Удалите ненужные секции.
# ═══════════════════════════════════════════════════════════════

# ─── Обязательные поля ────────────────────────────────────────
folder: "my-data"              # имя зарегистрированной папки (sync-folders addfolder)
description: "Sync my data"    # описание (опционально)

# ─── Транспорт ────────────────────────────────────────────────
# Выберите ОДИН из вариантов ниже, остальные закомментируйте/удалите.

# Вариант 1: SSH/SCP
# transport:
#   type: ssh
#   config:
#     host: "server.com"              # хост
#     port: "22"                      # порт (по умолч. 22)
#     user: "username"                # пользователь
#     key: "~/.ssh/id_ed25519"        # путь к ключу
#     remote_path: "/backup"          # удалённая директория

# Вариант 2: FTP
# transport:
#   type: ftp
#   config:
#     host: "ftp.example.com"         # хост
#     port: "21"                      # порт (по умолч. 21)
#     user: "team"                    # пользователь
#     password: "${FTP_PASS}"         # пароль (или ${VAR})
#     remote_path: "/shared"          # удалённая директория

# Вариант 3: WebDAV (Nextcloud, Owncloud)
# transport:
#   type: webdav
#   config:
#     url: "https://nc.example.com/remote.php/dav/files/user/"  # WebDAV URL
#     user: "user"                    # пользователь
#     password: "${NC_PASS}"          # пароль (или ${VAR})
#     remote_path: "sync"             # подпапка в WebDAV

# Вариант 4: S3 (Minio, Yandex Object Storage, AWS S3)
# transport:
#   type: s3
#   config:
#     endpoint: "storage.yandexcloud.net"   # S3 endpoint
#     access_key: "${AWS_KEY}"              # access key (или ${VAR})
#     secret_key: "${AWS_SECRET}"           # secret key (или ${VAR})
#     bucket: "my-backup"                   # bucket name
#     prefix: "data/"                       # префикс / виртуальная папка

# Вариант 5: HTTP (PHP-хостинг)
# transport:
#   type: http
#   config:
#     url: "https://myserver.com/php_storage.php"   # URL PHP-скрипта
#     base_url: "https://myserver.com"              # базовый URL для скачивания
#     auth: "${HTTP_AUTH}"                          # Basic Auth (опционально)
#     self_signed_certs: "true"                              # самоподписанные сертификаты (опционально)
#
# Серверная часть: transport/php_storage.php
# Разместите php_storage.php на любом PHP-хостинге.
# POST — загрузка файла (unique suffix, защита от перезаписи)
# GET  — JSON-список файлов
# GET  /uploads/{filename} — скачивание

# ─── Настройки синхронизации ──────────────────────────────────
sync:
	 # period: интервал автоматической синхронизации в daemon-режиме.
	 # Формат: 30m, 1h, 24h, 0 — только ручной запуск.
	 period: "0"

	 # direction: направление синхронизации.
	 #   push          — только на удалённое хранилище (бэкап)
	 #   pull          — только с удалённого хранилища (восстановление)
	 #   bidirectional — двусторонняя (новые/изменённые — туда и обратно)
	 direction: "bidirectional"

	 # conflict: разрешение конфликтов.
	 #   newer_wins — файл с более новой датой модификации побеждает
	 #   keep_both  — сохранить обе версии (переименовать старую)
	 conflict: "newer_wins"

	 # send_filter: JS-фильтр для отправляемых файлов (опционально).
	 # Фильтр применяется к списку локальных файлов перед push.
	 send_filter: |
	   function filter(files, ctx) {
	     return files.filter(function(f) {
	       return f.size < 10 * 1024 * 1024;  // только файлы < 10MB
	     });
	   }

	 # receive_filter: JS-фильтр для получаемых файлов (опционально).
	 # Фильтр применяется к списку удалённых файлов перед pull.
	 receive_filter: |
	   function filter(files, ctx) {
	     return files.filter(function(f) {
	       return f.name.endsWith('.pdf') || f.name.endsWith('.jpg');
	     });
	   }

# ─── Справка по JS-фильтрам ───────────────────────────────────
# Контекст (ctx):
#   ctx.folder       — "/home/user/sync" (локальная папка)
#   ctx.direction    — "send" | "receive"
#
# Каждый файл:
#   { name:"photo.jpg", path:"sub/photo.jpg", size:102400,
#     mod_time:1700000000, is_dir:false }
#
# Встроенные функции:
#   korni.ls(dir)     — список файлов в директории
#   korni.stat(path)  — информация о файле
#   korni.read(path)  — прочитать файл
#
# Примеры фильтров:
#   // Только по расширениям
#   var ext = ['.jpg', '.png', '.pdf'];
#   return files.filter(function(f) {
#     return ext.some(function(e) { return f.name.endsWith(e); });
#   });
#
#   // Только новее N дней
#   var cutoff = Date.now()/1000 - 7*86400;
#   return files.filter(function(f) { return f.mod_time > cutoff; });`)
			return
		}
		printHelp()

	case "help":
		printHelp()

	default:
		printHelp()
	}
}

func printHelp() {
	fmt.Println(`sync-folders - File synchronization utility

Commands:
  addfolder <name> <path>       Register a local folder
  removefolder <name>           Remove a folder
  folders                       List registered folders

  addconfig <file.yaml>         Add sync config
  removeconfig <name>           Remove a config
  configs                       List configs

  sync <name|--all>             Run sync once
  daemon [--interval <d>]       Run in background
  dry <name>                    Test run (no changes)

  config template               Show config template
  help                          This help`)
}
