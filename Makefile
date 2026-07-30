# sync-folders Makefile
# ======================
# Команды:
#   make help       — показать это сообщение
#   make check      — go vet + go fmt (lint)
#   make test       — запустить все тесты
#   make test-v     — запустить все тесты подробно
#   make test-short — только unit-тесты (без интеграционных)
#   make build      — собрать для текущей платформы
#   make run        — собрать и запустить --help
#   make clean      — удалить бинарник
#   make build-all  — кросс-компиляция под 8 платформ (через build.sh)
#   make test-docker — запустить Docker-интеграционные тесты
#
# Тесты:
#   41 тестов в 3 пакетах: core (8), filter (4), transport (29)

# Авто-определение Go toolchain
# Приоритет: GOROOT из .env → go в PATH → toolchain в модульном кеше
ifneq ($(wildcard .env),)
    include .env
    export
endif

ifdef GOROOT
    GO_CMD := $(GOROOT)/bin/go
else
    GO_CMD := $(shell command -v go 2>/dev/null || echo "")
    ifeq ($(GO_CMD),)
        $(error Go not found. Install Go 1.26+ or set GOROOT in .env)
    endif
endif

BINARY := sync-folders

.PHONY: help check test test-v test-short build run clean build-all test-docker

help:
	@echo "sync-folders — Makefile"
	@echo ""
	@echo "  make help        Показать это сообщение"
	@echo "  make check       Проверка кода (go vet + go fmt)"
	@echo "  make test        Запустить все тесты"
	@echo "  make test-v      Запустить все тесты подробно (-v)"
	@echo "  make build       Собрать для текущей платформы"
	@echo "  make run         Собрать и запустить"
	@echo "  make clean       Удалить собранный бинарник"
	@echo "  make build-all   Кросс-компиляция под 8 платформ"
	@echo ""
	@echo "Тесты: 41 тестов в 3 пакетах (core, filter, transport)"
	@echo ""

check:
	@echo "=== go vet ==="
	$(GO_CMD) vet ./...
	@echo ""
	@echo "=== go fmt ==="
	$(GO_CMD) fmt ./...
	@echo "OK"

test:
	$(GO_CMD) test -count=1 -timeout 120s ./...

test-v:
	$(GO_CMD) test -count=1 -timeout 120s -v ./...

build:
	$(GO_CMD) build -o $(BINARY) .

run: build
	./$(BINARY) --help

clean:
	rm -f $(BINARY)

build-all:
	./build.sh

test-docker:
	@echo "=== Docker Integration Tests ==="
	@for scenario in $$(ls docker/scenarios/ | sort); do \
		echo ""; \
		echo "━━━ Running: $$scenario ━━━"; \
		bash docker/run.sh "$$scenario" || exit 1; \
	done
	@echo ""
	@echo "All Docker tests passed"
