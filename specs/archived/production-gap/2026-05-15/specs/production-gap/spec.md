# Production Gap

## Scope Snapshot

- In scope: закрыть production-блокеры из roadmap, чтобы релизный путь был безопасным, проверяемым и совместимым с SpecKeep.
- Out of scope: добавление новых продуктовых возможностей вне release-readiness, таких как IPv6, performance-polish или новые routing-capabilities.

## Цель

Подготовить текущий клиент-серверный VPN к первому production-релизу, устранив критические разрывы между работающей базовой функциональностью и требованиями безопасной эксплуатации. Фича считается успешной, когда TLS и mTLS реально защищают соединение, примеры не распространяют секреты, а release-gates подтверждены проверяемыми артефактами и операционными smoke-проверками.

## Основной сценарий

1. Стартовая точка: команда готовит production-релиз из текущей кодовой базы, где базовые сборка и тесты проходят, но roadmap фиксирует незакрытые блокеры.
2. Основное действие: система и репозиторий обновляются так, чтобы клиент доверял только валидному серверу, сервер корректно проверял клиентские сертификаты, примеры были безопасны, а SpecKeep verify-path давал формальное proof.
3. Результат: production release может пройти release review без известных security и process-blockers из roadmap.
4. Ошибка/fallback-путь, если он заметно влияет на опыт: недоверенный серверный сертификат, неизвестный client cert, отсутствие verify-артефактов или небезопасные примеры должны явно останавливать релизный путь с наблюдаемой причиной.

## User Stories

- P1 Story: как инженер релиза, я хочу получить production-путь без insecure TLS/mTLS и без закоммиченных секретов, чтобы выпуск не создавал очевидных эксплуатационных и security-рисков.
- P2 Story: как инженер сопровождения, я хочу видеть formal verify artifacts и smoke/quality proof, чтобы решение о релизе опиралось на наблюдаемые проверки, а не на предположения.

## MVP Slice

- Минимальный independently valuable срез: закрыть TLS trust enforcement, корректную mTLS-проверку, secrets hygiene в примерах и восстановить SpecKeep verify-path.
- Этот срез обязан первым закрыть `AC-001`, `AC-002`, `AC-003`, `AC-004`; `AC-005` завершает release-readiness перед выпуском.

## First Deployable Outcome

- После первого implementation pass команда может показать, что insecure TLS fallback больше не нужен, mTLS режимы ведут себя предсказуемо, а `check-verify-ready` и связанные release artifacts больше не ломают процесс.
- Фича не считается independently releasable без операционных smoke и quality proofs, потому что release-readiness здесь является общей целью всего среза.

## Scope

- Клиентская TLS-конфигурация и runtime-проверка доверия к серверному сертификату.
- Серверная mTLS semantics и её тестируемое поведение для trusted/untrusted client cert.
- Безопасность example-конфигураций и compose-сценариев относительно приватных ключей и секретов.
- SpecKeep-артефакты и verify-path, необходимые для production release gate.
- Финальные operational proofs: privileged smoke, lint и иные quality-gates, которые roadmap требует перед первым релизом.
- Исправление pre-existing lint issues (50 errcheck + 1 staticcheck) во всём репозитории для прохождения lint quality gate.
- Production runtime safety: http.Server timeouts, resource cleanup (NAT/BoltDB), rate limiter map leak fix, обновление зависимостей.

## Контекст

- Проект уже имеет working build/test baseline, но roadmap на 2026-05-14 явно фиксирует production blockers как отдельный release-gap.
- По конституции release не считается завершённым без observable proof, verify-артефактов и соблюдения security/quality gates.
- Репозиторий уже использует Speckeep workflow, поэтому release-readiness должна выражаться не только кодом, но и корректной process-traceability внутри `specs/active/<slug>/`.

## Требования

- RQ-001 Клиент ДОЛЖЕН по умолчанию отклонять TLS-соединение с сервером, чей сертификат не подтверждён доверенной CA-цепочкой и/или не соответствует ожидаемой серверной идентичности.
- RQ-002 Клиент ДОЛЖЕН иметь явную конфигурационную поверхность для production TLS trust settings, достаточную для работы без hardcoded insecure fallback.
- RQ-003 Сервер ДОЛЖЕН в production-режиме mTLS отклонять client certificate, который отсутствует в доверенном `client_ca_file`, и не считать наличие любого сертификата достаточным доказательством доверия.
- RQ-004 Система ДОЛЖНА явно различать режимы client certificate handling так, чтобы оператор мог предсказуемо выбрать запрашиваемый, обязательный или проверяемый режим без двусмысленной semantics.
- RQ-005 Репозиторий и example-сценарии ДОЛЖНЫ быть безопасны по умолчанию: tracked production-like private keys отсутствуют, а примеры не требуют закоммиченных секретов для демонстрации или запуска.
- RQ-006 Репозиторий ДОЛЖЕН содержать SpecKeep-артефакты текущего slug в формате, достаточном для прохождения verify readiness и для фиксации observable proof по release-критериям.
- RQ-007 Перед признанием production-gap закрытым команда ДОЛЖНА иметь наблюдаемое подтверждение operational hardening: privileged e2e smoke для TUN/NAT/reconnect и результат обязательных quality-gates, включая lint.
- RQ-008 Весь репозиторий ДОЛЖЕН проходить `golangci-lint run ./src/...` без ошибок перед признанием production-gap закрытым.
- RQ-009 http.Server ДОЛЖЕН иметь ReadTimeout/WriteTimeout/IdleTimeout для защиты от slow loris.
- RQ-010 NAT rules ДОЛЖНЫ удаляться при завершении сервера (Teardown/Teardown6).
- RQ-011 BoltDB ДОЛЖЕН закрываться при завершении сервера.
- RQ-012 Rate limiter map ДОЛЖНА иметь GC для предотвращения утечки памяти. 

## Вне scope

- Добавление новых транспортов, новых auth-механизмов или новых routing-features сверх уже зафиксированного roadmap.
- Расширение функциональности admin API или `/metrics` за пределы минимально необходимой модели контролируемой доступности.
- Полная программа performance-оптимизаций, IPv6/dual-stack и релизная документация v1.0.0, если они не нужны для закрытия текущих production-blockers.
- Исправление pre-existing lint issues (51 issue) теперь в scope (см. AC-006).
- Переписывание исторических roadmap-этапов, не связанных с production-gap, вместо адресной фиксации текущих блокеров.

## Критерии приемки

### AC-001 TLS trust enforcement на клиенте

- Почему это важно: без этого клиент уязвим к MITM и не может считаться production-ready.
- **Given** клиент настроен на подключение к серверу с указанием production TLS trust settings
- **When** он устанавливает соединение с доверенным сертификатом и затем с недоверенным/self-signed сертификатом вне доверенной цепочки
- **Then** доверенное соединение успешно устанавливается, а недоверенное отвергается с наблюдаемой ошибкой проверки сертификата без использования hardcoded `InsecureSkipVerify=true`
- Evidence: конфигурационные поля клиента и e2e/интеграционный тест или иная воспроизводимая проверка, показывающая accept trusted / reject untrusted

### AC-002 Корректная production mTLS-проверка

- Почему это важно: mTLS не должен быть декоративным; режим `require` обязан реально проверять доверие к client cert.
- **Given** сервер настроен на production mTLS с доверенным `client_ca_file`
- **When** клиент подключается сначала с сертификатом из доверенной цепочки, а затем с неизвестным или недоверенным сертификатом
- **Then** сервер принимает доверенного клиента и отклоняет недоверенного с наблюдаемым результатом, а режимы certificate handling описаны и реализованы без двусмысленности
- Evidence: server config surface, automated tests на trusted/untrusted cert flows и наблюдаемое отличие режимов `request` / `require` / `verify`

### AC-003 Secrets hygiene в репозитории и примерах

- Почему это важно: релиз не должен распространять приватные ключи или небезопасные operational defaults.
- **Given** репозиторий, root demo-flow и example-сценарии проверяются перед релизом
- **When** команда просматривает tracked example assets и запускает documented/example compose flow
- **Then** в tracked root и example файлах отсутствуют private keys, а root/example compose flows используют runtime-generated certs или явное безопасное подключение внешних секретов вместо закоммиченных ключей
- Evidence: список изменённых root/example security файлов, проверка tracked files без приватных ключей и обновлённый пример запуска без встроенных секретов

### AC-004 Release governance через SpecKeep

- Почему это важно: даже при готовом коде релизный путь остаётся заблокированным, если verify artifacts и структура slug не соответствуют процессу.
- **Given** для `production-gap` подготовлены активные SpecKeep-артефакты текущего slug
- **When** команда запускает readiness-проверку verify-path и просматривает release evidence этого slug
- **Then** verify readiness проходит без ошибок о missing `spec.md` / `tasks.md` и release-критерии имеют observable proof в артефактах slug
- Evidence: существующие файлы `spec.md`, `tasks.md`, `verify.md` для slug и успешный результат `./.speckeep/scripts/check-verify-ready.sh .`

### AC-005 Финальные operational и quality gates

- Почему это важно: production readiness должна быть подтверждена в реальном runtime и формальными quality checks, а не только unit tests.
- **Given** security и process-blockers уже закрыты
- **When** команда прогоняет privileged smoke для TUN/NAT/reconnect, ограничение operational endpoints и обязательные quality-gates
- **Then** release review получает наблюдаемое подтверждение, что критичный runtime-сценарий проходит, `/metrics` и admin surfaces имеют осознанную модель доступа, а lint/result quality gates зафиксированы в verify
- Evidence: вывод smoke-команды/сценария, подтверждение модели доступа для operational endpoints и verify artifact с результатами lint и связанных проверок

### AC-006 Lint quality gate без ошибок

- Почему это важно: lint gate блокирует archive; pre-existing issues не относятся к production-gap, но должны быть исправлены для прохождения gate.
- **Given** репозиторий проверяется `golangci-lint run ./src/...` совместимым binary
- **When** команда прогоняет lint через бинарь, собранный под Go 1.25+
- **Then** lint завершается без ошибок (0 issues)
- Evidence: вывод `golangci-lint run ./src/...` с exit code 0

### AC-007 Production runtime safety (resource leaks + timeouts)

- Почему это важно: сервер без таймаутов и cleanup уязвим к DoS и утечкам ресурсов.
- **Given** сервер запущен и завершается (SIGTERM/SIGINT)
- **When** сервер обрабатывает запросы и затем останавливается
- **Then** http.Server имеет ReadTimeout/WriteTimeout/IdleTimeout, NAT rules удаляются, BoltDB закрывается, rate limiter map не растёт бесконечно
- Evidence: кодовая проверка наличия таймаутов, defer Teardown/Close, startCleanup goroutine

## Допущения

- Базовый end-to-end tunnel, routing и текущая конфигурационная модель уже существуют в объёме, достаточном для focused production-hardening без пересборки архитектуры с нуля.
- Production release оценивается по состоянию текущего tree и релизных артефактов; отдельная процедура очистки уже опубликованной git-истории может потребовать внешнего согласования.
- Для этой фичи secrets hygiene ограничивается текущим tree и релизными артефактами; очистка уже опубликованной git-истории не является обязательной частью scope.
- Минимально достаточная модель доступа для `/metrics` и admin surface в рамках acceptance — доступ по токену.
- Privileged smoke и quality-gates доступны в среде verify или могут быть выполнены воспроизводимым способом до release decision.

## Критерии успеха

- SC-001 Клиент отклоняет недоверенный серверный сертификат и принимает доверенный во всех поддержанных production TLS сценариях, покрытых verify.
- SC-002 `./.speckeep/scripts/check-verify-ready.sh .` проходит успешно для slug `production-gap`.
- SC-003 Release review для первого production выпуска не содержит открытых P0-блокеров из блока `Production Gap` roadmap от 2026-05-14.
- SC-004 `golangci-lint run ./src/...` завершается без ошибок.

## Краевые случаи

- Оператор мигрирует со старого insecure client config и получает понятный отказ или явно задокументированный способ перейти на trusted TLS settings.
- Сервер запущен в режиме, где client cert запрашивается, но не должен быть обязательным; semantics режима остаётся наблюдаемой и не совпадает по поведению с `require`.
- Example compose запускается в чистом окружении без заранее подготовленных локальных сертификатов.
- Verify readiness ломается не кодом, а отсутствующим артефактом или несогласованностью между spec/tasks/verify.
- Operational endpoints доступны только при корректном токене и не считаются acceptably protected без token gate.

## Открытые вопросы

- none
