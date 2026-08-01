# Stage 1: сборка sync-folders (на ubuntu:22.04, чтобы libc совпадала)
FROM ubuntu:22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    gcc \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

# Установка Go
RUN curl -fsSL https://go.dev/dl/go1.26.5.linux-amd64.tar.gz \
    | tar xz -C /usr/local
ENV PATH=/usr/local/go/bin:$PATH

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o sync-folders .

# Stage 2: финальный образ
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

RUN apt-get update && apt-get install -y --no-install-recommends \
    qbittorrent-nox \
    iptables \
    jq \
    curl \
    ca-certificates \
    php-cli \
    iproute2 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/sync-folders /usr/local/bin/sync-folders
COPY transport/php_storage.php /opt/php-storage/php_storage.php

RUN curl -fsSL https://dist.ipfs.tech/kubo/v0.42.0/kubo_v0.42.0_linux-amd64.tar.gz \
    | tar xz -C /tmp && mv /tmp/kubo/ipfs /usr/local/bin/ipfs && rm -rf /tmp/kubo

WORKDIR /data
ENTRYPOINT []
