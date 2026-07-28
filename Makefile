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
#
# Тесты:
#   33 тестов в 3 пакетах: core (3), filter (4), transport (26)

GO_CMD := go
BINARY := sync-folders

.PHONY: help check test test-v test-short build run clean build-all

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
	@echo "Тесты: 33 тестов в 3 пакетах (core, filter, transport)"
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
