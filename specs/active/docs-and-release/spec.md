# Docs & Release — kvn-ws v1.0.0

## Scope Snapshot

- In scope: Создание полной двуязычной документации (EN/RU), рабочих примеров, CHANGELOG, корневого README и публикация GitHub Release v1.0.0.
- Out of scope: Любые изменения Go-кода, новые функциональные возможности, CI/CD пайплайны.

## Цель

Разработчики и пользователи могут быстро понять, развернуть и использовать kvn-ws благодаря comprehensive документации на английском и русском, готовым к запуску примерам (docker compose), опубликованному релизу v1.0.0 на GitHub. Gate-критерий: пользователь, никогда не видевший проект, за 5 минут поднимает сервер через `docker compose up` и подключает клиента, следуя шагам из `docs/en/quickstart.md`.

## Основной сценарий

1. Пользователь заходит на GitHub-репозиторий kvn-ws, видит README с бейджами (build, license, release) и кратким описанием.
2. Пользователь переходит в `docs/en/quickstart.md`, где за 5 шагов описана настройка и запуск через Docker.
3. Пользователь копирует `examples/docker-compose.yml`, `examples/client.yaml`, `examples/server.yaml` и запускает `docker compose up`.
4. Сервер и клиент стартуют, устанавливается WebSocket-соединение, клиент получает IP из пула.
5. При необходимости пользователь обращается к `docs/en/config.md` для тонкой настройки или `docs/en/architecture.md` для понимания внутреннего устройства.
6. Русскоязычные пользователи читают те же материалы в `docs/ru/`.
7. При обновлениях пользователь смотрит CHANGELOG.md для списка изменений.

## User Stories

- P1 Story (MVP): Пользователь может за 5 минут запустить сервер и клиент через `docker compose up`, следуя quickstart — без чтения исходников.
- P2 Story: Пользователь может полностью настроить клиент и сервер через config reference, понимая каждый параметр.
- P3 Story: Русскоязычный пользователь получает полную документацию на родном языке.

## MVP Slice

AC-001 (quickstart) + AC-005 (examples) + AC-006 (root README) + AC-007 (CHANGELOG) + AC-008 (GitHub Release). Этот срез уже даёт пользователю работающий путь от клонирования до запуска.

## First Deployable Outcome

После первого implementation pass можно продемонстрировать: README.md с бейджами, заполненные `docs/en/quickstart.md` и `docs/en/config.md`, `examples/` с готовыми файлами, CHANGELOG.md с v1.0.0, и git tag v1.0.0.

## Scope

- `docs/en/` — quickstart.md, config.md, architecture.md
- `docs/ru/` — полный перевод docs/en/
- `examples/` — docker-compose.yml, client.yaml, server.yaml, run.sh
- `CHANGELOG.md` — формат Keep a Changelog, разделы Added/Changed/Fixed
- `README.md` — корневой, с бейджами, быстрым стартом, ссылками на docs
- GitHub Release v1.0.0 — tag + release notes из CHANGELOG

## Контекст

- Проект — Go-монолит с двумя entry points (client, server) в одном Dockerfile через multi-stage build.
- Текущий docker-compose.yml в корне — для разработки; `examples/` получает упрощённую копию для демонстрации.
- Документация пишется с нуля (директории `docs/en/` и `docs/ru/` существуют, но пусты).
- Английский — язык кода; русский — язык документации для основной аудитории.
- Конфигурация описана в `configs/server.yaml` и `configs/client.yaml` — это source of truth для config reference.

## Требования

- RQ-001 `docs/en/quickstart.md` ДОЛЖЕН содержать пошаговую инструкцию (клон, конфиг, `docker compose up`, проверка) с ожидаемым временем выполнения ≤5 минут.
- RQ-002 `docs/en/config.md` ДОЛЖЕН документировать каждый ключ из `configs/server.yaml` и `configs/client.yaml`: тип, значение по умолчанию, описание, пример.
- RQ-003 `docs/en/architecture.md` ДОЛЖЕН содержать: общую архитектуру, описание компонентов, диаграмму (Mermaid), data flow для handshake и data transfer.
- RQ-004 `docs/ru/` ДОЛЖЕН содержать полный перевод каждого файла из `docs/en/` с сохранением структуры и mermaid-диаграмм.
- RQ-005 `examples/` ДОЛЖЕН содержать: `docker-compose.yml` (упрощённый, самодостаточный), `server.yaml`, `client.yaml`, `run.sh` (генерация TLS-сертификата + `docker compose up`).
- RQ-006 `CHANGELOG.md` ДОЛЖЕН следовать формату [Keep a Changelog](https://keepachangelog.com) с секциями для v1.0.0.
- RQ-007 `README.md` ДОЛЖЕН содержать: бейджи (GitHub Actions, Go version, License, Release), описание проекта за 3 строки, quick start за 30 секунд, ссылки на все doc-файлы.
- RQ-008 GitHub Release v1.0.0 ДОЛЖЕН содержать tag `v1.0.0` на текущем HEAD + release notes, скопированные из CHANGELOG.md.

## Критерии приемки

### AC-001 Quickstart — docker compose up за ≤5 минут

- Почему это важно: Главный onboarding-путь; если quickstart не работает — пользователь уходит.
- **Given** чистый Linux-хост с Docker и git
- **When** пользователь выполняет: `git clone <repo> && cd kvn-ws && cp -r examples/* . && bash examples/run.sh`
- **Then** сервер и клиент запускаются, клиент устанавливает WebSocket-соединение, клиент получает IP-адрес из пула
- Evidence: В логах сервера `session established`, в логах клиента `connected, ip=10.10.0.X`

### AC-002 Config reference — все ключи документированы

- Почему это важно: Без config reference пользователь не может настраивать систему.
- **Given** файл `docs/en/config.md`
- **When** reviewer сравнивает каждый ключ из `configs/server.yaml` и `configs/client.yaml`
- **Then** каждый ключ имеет в docs/en/config.md запись с типом, default, описанием
- Evidence: Чеклист покрытия config keys → docs без пропусков

### AC-003 Architecture docs — система описана

- Почему это важно: Разработчикам и advanced user нужно понимать внутреннее устройство.
- **Given** файл `docs/en/architecture.md`
- **When** reviewer читает документ
- **Then** документ содержит: Mermaid-диаграмму компонентов, описание data flow (handshake + data), перечень компонентов src/internal/*
- Evidence: mermaid-диаграмма отображается на GitHub, data flow описаны текстом

### AC-004 Russian translation — docs/ru/ полный

- Почему это важно: Основная аудитория проекта — русскоязычная.
- **Given** `docs/ru/` директория
- **When** reviewer сравнивает структуру `docs/en/` и `docs/ru/`
- **Then** каждый файл из `docs/en/` имеет соответствующий перевод в `docs/ru/` с той же структурой разделов
- Evidence: Файлы docs/ru/quickstart.md, docs/ru/config.md, docs/ru/architecture.md существуют и являются переводом

### AC-005 Runnable examples — примеры в examples/

- Почему это важно: Пользователь должен иметь готовый набор файлов для запуска, не копаясь в конфигах.
- **Given** директория `examples/`
- **When** пользователь копирует `examples/` в чистую директорию
- **Then** `examples/` содержит docker-compose.yml, server.yaml, client.yaml, run.sh, которые работают без модификации (кроме подстановки токена/сертификата)
- Evidence: `ls examples/` показывает минимум 4 файла, `run.sh` исполняемый

### AC-006 Root README — бейджи, быстрый старт, ссылки

- Почему это важно: README — лицо репозитория на GitHub.
- **Given** корневой README.md
- **When** пользователь открывает репозиторий на GitHub
- **Then** README содержит: бейджи (build status, Go version, license, release), описание проекта (3 строки), quick start (30 секунд, 3 команды), ссылки на docs/en/* и docs/ru/*
- Evidence: README.md рендерится на GitHub со всеми бейджами и ссылками

### AC-007 CHANGELOG — v1.0.0 entry

- Почему это важно: Пользователи и разработчики отслеживают изменения.
- **Given** файл CHANGELOG.md в корне
- **When** reviewer открывает файл
- **Then** CHANGELOG.md следует формату Keep a Changelog, содержит секцию v1.0.0 с подсекциями Added
- Evidence: CHANGELOG.md парсится без ошибок, секция v1.0.0 присутствует

### AC-008 GitHub Release — tag + release notes

- Почему это важно: v1.0.0 — первый стабильный релиз проекта.
- **Given** git tag v1.0.0 и GitHub Release
- **When** `git tag -l v1.0.0` и `gh release view v1.0.0`
- **Then** tag указывает на текущий HEAD, Release Notes содержат тот же текст, что CHANGELOG.md для v1.0.0
- Evidence: `git tag -l v1.0.0` возвращает tag, `gh release view v1.0.0` показывает notes

## Вне scope

- Изменения Go-кода в `src/` (рефакторинг, багфиксы, новые фичи)
- Добавление новых entry points или конфигурационных ключей
- CI/CD pipelines (кроме создания GitHub Release)
- Интеграционные тесты, утилиты для нагрузочного тестирования
- Документация для сторонних SDK, библиотек, API
- Видео-туториалы или скринкасты
- Деплой в cloud (AWS/GCP/Azure)

## Допущения

- Пользователь имеет Docker Engine 24+ и docker compose plugin.
- Пользователь имеет доступ к GitHub для клонирования репозитория.
- TLS-сертификат генерируется самоподписанным (run.sh использует openssl).
- Go-код проекта корректен и не требует изменений для работы примеров.
- Текущий HEAD коммит — корректная точка для v1.0.0 (все foundation-задачи закрыты).
- Аудитория: Go-разработчики и DevOps, знакомые с Docker, VPN, WebSocket.

## Критерии успеха

- SC-001 Время от `git clone` до `docker compose up` с работающим соединением ≤5 минут для нового пользователя.
- SC-002 Покрытие config reference: 100% ключей из `configs/*.yaml` документировано.
- SC-003 README имеет score A на [RepoBeaver](https://repobeaver.com) (или GitHub repo maturity checklist).

## Краевые случаи

- Отсутствие Docker на машине пользователя: quickstart должен явно указать prerequisites.
- Самоподписанный сертификат: run.sh генерирует через openssl, client.yaml указывает `tls_insecure: true`.
- Пустые директории docs/en/ и docs/ru/ — заполняются с нуля.
- Конфиги с `your-token-here`: quickstart и примеры должны инструктировать заменить токен.

## Открытые вопросы

1. Выбор mermaid-диаграммы vs draw.io/png для architecture.md — mermaid принят как стандарт для GitHub.
2. Формат бейджей — shields.io (динамические из GitHub Actions) или статические? Решение: статические на старте, динамические — follow-up.
3. Нужен ли `docs/en/troubleshooting.md` в v1.0.0? Решение: нет, common issues добавить в quickstart как секцию "Troubleshooting".
4. Версионирование CHANGELOG — только v1.0.0 или pre-release истории? Решение: только v1.0.0 как первый entry.
