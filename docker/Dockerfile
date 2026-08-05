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
RUN go build -o mkbucket ./docker/mkbucket

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
    php-mysql \
    iproute2 \
    openssh-server \
    openssh-client \
    vsftpd \
    apache2 \
    mariadb-server \
    postfix \
    dovecot-imapd \
    tor \
    sshpass \
    mc \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/sync-folders /usr/local/bin/sync-folders
COPY --from=builder /build/mkbucket /usr/local/bin/mkbucket
COPY transport/php_storage.php /opt/php-storage/php_storage.php
COPY transport/mysql_storage.php /opt/php-storage/mysql_storage.php

# IPFS kubo — бинарник из локальной директории (надёжно; curl к dist.ipfs.tech флакает).
# Положить бинарник: curl -fsSL https://dist.ipfs.tech/kubo/v0.42.0/kubo_v0.42.0_linux-amd64.tar.gz \
#   | tar xz -C /tmp && mv /tmp/kubo/ipfs docker/bin/ipfs
COPY docker/bin/ipfs /usr/local/bin/ipfs
RUN chmod +x /usr/local/bin/ipfs

# MinIO (S3)
RUN curl -fsSL https://dl.min.io/server/minio/release/linux-amd64/minio \
    -o /usr/local/bin/minio && chmod +x /usr/local/bin/minio

# SSH: разрешить root вход по ключу
RUN mkdir -p /root/.ssh && chmod 700 /root/.ssh \
    && echo "PermitRootLogin yes" >> /etc/ssh/sshd_config \
    && echo "PasswordAuthentication yes" >> /etc/ssh/sshd_config

WORKDIR /data
ENTRYPOINT []
