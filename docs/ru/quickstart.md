<!-- @sk-task docs-and-release#T4.2: russian translation of quickstart (AC-004) -->

# Быстрый старт

Запустите сервер и клиент kvn-ws за 5 минут.

## Требования

- [Docker Engine](https://docs.docker.com/engine/install/) 24+ с плагином docker compose
- [OpenSSL](https://www.openssl.org/) (обычно предустановлен в Linux)
- [Git](https://git-scm.com/)

## Пошаговая инструкция

### 1. Клонируйте репозиторий

```bash
git clone https://github.com/bzdvdn/kvn-ws.git
cd kvn-ws
```

### 2. Скопируйте примеры

```bash
cp -r examples/* .
```

Эта команда копирует docker-compose.yml, конфиги и скрипт запуска в текущую директорию.

### 3. Сгенерируйте TLS-сертификат и запустите

```bash
bash examples/run.sh
```

Скрипт:
1. Генерирует самоподписанный TLS-сертификат (cert.pem, key.pem) через OpenSSL
2. Запускает контейнеры сервера и клиента командой `docker compose up -d`

### 4. Проверьте подключение

```bash
docker compose logs client | grep "handshake complete"
```

Ожидаемый вывод:
```
client-1  | ... "msg":"handshake complete" ... "ip":"10.10.0.2" ...
```

### 5. (Опционально) Логи клиента в реальном времени

```bash
docker compose logs -f client
```

## Архитектура

```
┌──────────┐     WebSocket/TLS 1.3     ┌──────────┐
│  клиент  │ ────────────────────────▶ │  сервер  │
│  (TUN)   │                           │(IP пул)  │
└──────────┘                           └──────────┘
```

## Решение проблем

| Проблема | Решение |
|----------|---------|
| `docker compose` не найдена | Установите Docker Engine 24+ с compose |
| `openssl` не найден | Установите openssl: `apt install openssl` (Debian) / `yum install openssl` (RHEL) |
| Клиент показывает `connection refused` | Проверьте, что порт 443 свободен: `ss -tlnp \| grep 443` |
| Клиент показывает `auth failed` | Убедитесь, что токен в server.yaml и client.yaml совпадает |
| Клиент не подключается | Проверьте логи сервера: `docker compose logs server` |

## Дальнейшие шаги

- [Конфигурация](config.md) — все ключи конфигурации
- [Архитектура](architecture.md) — дизайн системы и потоки данных
- [Примеры](../examples/) — docker-compose, конфиги и скрипты
