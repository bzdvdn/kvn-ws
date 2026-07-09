# Doze-устойчивость VPN-соединения

## Scope Snapshot

- In scope: обеспечение стабильной работы VPN-туннеля на Android при выключенном экране (Doze mode) за счёт WakeLock, TCP keepalive и адаптивных таймаутов сервера.
- Out of scope: iOS клиент, FCM push-пробуждение, глубокая оптимизация энергопотребления (battery drain analysis), desktop/relay клиенты.

## Цель

Пользователь Redmi 15 (и других Android-устройств с агрессивным Doze/MIUI-оптимизациями) получает стабильное VPN-соединение с выключенным экраном: push-уведомления, фоновые данные и нотификации от приложений за VPN доставляются без разрывов. Успех фичи измеряется отсутствием сообщений `WS: send ping but didn't receive pong` при screen-off и восстановлением соединения <5s после screen-on.

## Основной сценарий

1. Пользователь подключается к VPN на Android, выключает экран.
2. WakeLock типа `PARTIAL_WAKE_LOCK` удерживает CPU awake — OkHttp ping-таймеры и TCP keepalive продолжают работать.
3. TCP keepalive на транспортном сокете (kernel-level) служит резервным механизмом детекта обрыва.
4. Сервер использует увеличенный PongTimeout (120s) — Doze-задержки до 90с не приводят к разрыву.
5. При включении экрана `SCREEN_ON` BroadcastReceiver немедленно проверяет `isConnected()`. Если соединение потеряно — запускает reconnect без экспоненциального backoff.
6. После успешного reconnect или подтверждения живучести — TUN-интерфейс продолжает работу, трафик идёт.

## User Stories

- P1: Пользователь выключает экран на 10 минут — получает уведомления от мессенджеров за VPN, соединение не рвётся.
- P2: Пользователь возвращается к устройству через 3 часа — соединение быстро восстанавливается (<5s после screen-on).

## MVP Slice

WakeLock + TCP keepalive + UI toggle + серверный PongTimeout=120s. Закрывает AC-001, AC-002, AC-005, AC-006, AC-007. Screen-on listener — P2.

## First Deployable Outcome

APK с WakeLock и TCP keepalive + сервер с PongTimeout=120s. Проверка: screen-off на 2 минуты, после screen-on — трафик продолжается без переподключения.

## Scope

- Android: `PowerManager.WakeLock` acquired/release
- Android: `Socket.setKeepAlive(true)` в SmartSocketFactory
- Android: BroadcastReceiver на `SCREEN_ON`/`SCREEN_OFF` с проверкой соединения
- Android: уменьшение OkHttp `pingInterval` с 15s до 10s
- Android: UI toggle "Удерживать соединение при выкл. экране" (по умолчанию off), включает WakeLock + TCP keepalive
- Go server: увеличение `DefaultPongTimeout` с 45s до 120s
- Go server: конфигурация `pong_timeout` в `server.yaml` (в `[transport.ws]` секции)
- Документация: changelog, обновление конфигурационных примеров

## Контекст

- Xiaomi/Redmi (MIUI) — агрессивное управление фоном: даже foreground service c `specialUse` может терять сеть при screen-off.
- OkHttp `pingInterval` не срабатывает в глубоком сне CPU — полагаться только на него нельзя.
- TCP keepalive (kernel) — единственный механизм, гарантированно работающий при любой Java-платформе.
- Существующий `SmartSocketFactory` уже проксирует `setKeepAlive` (не вызывается) — достаточно включить.
- PongTimeout=45s недостаточен:实测 Doze latency на Redmi 15 достигает 60-90с.

## Зависимости

- `go: gorilla/websocket` — server-side pong handler deadline
- `android: PowerManager`, `android.permission.WAKE_LOCK`
- `android: ConnectivityManager.NetworkCallback` — уже используется
- `none` — внешние сервисы не требуются

## Требования

- RQ-001 Android клиент ДОЛЖЕН удерживать `PARTIAL_WAKE_LOCK` на всё время VPN-соединения (от `onStartCommand` до `onDestroy`).
- RQ-002 Android клиент ДОЛЖЕН включать `Socket.setKeepAlive(true)` на транспортном сокете WebSocket-соединения.
- RQ-003 Android клиент ДОЛЖЕН детектить `SCREEN_ON` и при отсутствии `isConnected()` инициировать reconnect без backoff-задержки.
- RQ-004 Сервер ДОЛЖЕН использовать `DefaultPongTimeout` не менее 120 секунд.
- RQ-005 Сервер ДОЛЖЕН предоставлять конфигурацию `pong_timeout` (duration) в `server.yaml`.
- RQ-006 Android UI ДОЛЖЕН предоставлять toggle "Удерживать соединение при выкл. экране" (по умолчанию выключен), при включении — acquire WakeLock + TCP keepalive.

## Вне scope

- Анализ энергопотребления WakeLock (battery drain) — ответственность пользователя и штатных средств ОС.
- Механизмы пробуждения через FCM/WebPush — отдельная фича при необходимости.
- Desktop/relay/QUIC-клиенты — Doze специфичен для Android.
- iOS клиент — отдельная платформа.
- Изменение логики `ReconnectManager` — только bypass backoff при screen-on.

## Критерии приемки

### AC-001 WakeLock acquired на время соединения

- Почему это важно: без WakeLock CPU засыпает, keepalive не работает, соединение рвётся.
- **Given** VPN-соединение установлено (state = `CONNECTED`)
- **When** экран выключается (broadcast `SCREEN_OFF`)
- **Then** `PARTIAL_WAKE_LOCK` удерживается процессом, CPU не засыпает
- Evidence: `adb shell dumpsys power | grep "LOCK.*kvn"` показывает активный WakeLock; `adb shell dumpsys power | grep mWakefulness` показывает `Awake` или `Doze` (но не `Asleep`)

### AC-002 TCP keepalive включён на сокете

- Почему это важно: kernel-level keepalive работает при любой нагрузке на JVM/OkHttp.
- **Given** WebSocket-соединение установлено
- **When** проверяем сокет
- **Then** `Socket.getKeepAlive()` возвращает `true`
- Evidence: `adb shell cat /proc/net/tcp | grep <server_ip>` + strace/log уровня `SO_KEEPALIVE`

### AC-003 Screen-on проверяет и восстанавливает соединение

- Почему это важно: при длительном screen-off соединение может быть потеряно; пользователь ждёт быстрого восстановления.
- **Given** VPN был активен, экран выключен на >2 мин, соединение разорвано сервером
- **When** пользователь включает экран (`SCREEN_ON`)
- **Then** клиент проверяет `isConnected()` и, если false, запускает reconnect (bypass backoff)
- Evidence: лог `AppLogger` содержит запись `"SCREEN_ON: connection dead, reconnecting"`; `ConnectionState.RECONNECTING` отправлен в callback

### AC-004 Быстрое восстановление после screen-on

- Почему это важно: пользователь не должен ждать >5с после включения экрана.
- **Given** screen-on, соединение потеряно
- **When** reconnect триггерится
- **Then** новое WebSocket-соединение устанавливается за <5с
- Evidence: замер по логам между `RECONNECTING` и `CONNECTED` <5000мс

### AC-005 Серверный PongTimeout конфигурируем

- Почему это важно: оператор сервера должен иметь возможность адаптировать timeout под свои условия эксплуатации.
- **Given** существует `server.yaml`
- **When** в секции `[transport.ws]` указано `pong_timeout: 120s`
- **Then** сервер использует 120с для pong deadline
- Evidence: лог сервера при старте: `"ws pong_timeout"=120s`; при превышении deadline соединение закрывается через 120с, а не 45с

### AC-006 Соединение выдерживает 2 минуты screen-off

- Почему это важно: базовый сценарий — короткое отключение экрана (звонок, пауза).
- **Given** VPN активен
- **When** экран выключен ровно на 120 секунд
- **When** экран включается
- **Then** соединение всё ещё в состоянии `CONNECTED` (keepalive сработал)
- Evidence: `ConnectionState.CONNECTED` после screen-on; трафик продолжает идти через TUN

### AC-007 Toggle "Удерживать соединение при выкл. экране"

- Почему это важно: пользователь сам выбирает баланс между стабильностью и батареей; по умолчанию WakeLock выключен.
- **Given** Android UI главного экрана (ConnectScreen/SettingsScreen)
- **When** пользователь открывает экран настроек соединения
- **Then** отображается toggle "Удерживать соединение при выкл. экране" (по умолчанию off)
- Evidence: toggle visible в UI; при включении в логах `AppLogger.i("WakeLock", "acquired")`; при выключении — `AppLogger.i("WakeLock", "released")`

## Допущения

- Устройство имеет достаточный заряд батареи — WakeLock не отключается системой принудительно.
- Пользователь предоставил разрешение `WAKE_LOCK` (manifest) и не отключал его в настройках.
- MIUI оптимизации не блокируют `PARTIAL_WAKE_LOCK` на уровне прошивки (проверено на Redmi 15 HyperOS 2.0).
- Серверная часть обновлена до версии, поддерживающей `pong_timeout` конфиг (иначе client-side только AC-001/AC-002 дают частичный эффект).

## Критерии успеха

- SC-001 После 10 циклов screen-off(120s)/screen-on: не более 1 разрыва соединения (успех >90%).
- SC-002 Время восстановления после screen-on: 90-перцентиль <5с.

## Краевые случаи

- Screen-on во время reconnect: второй reconnect не запускается (already reconnecting).
- Battery Saver активирован: WakeLock может быть revoked системой — корректный shutdown TUN.
- Пользователь вручную выключает VPN (kill switch): WakeLock немедленно release.
- Несколько последовательных screen-on без screen-off: no-op.
- Устройство переходит в Deep Doze (неактивно >30 мин): WakeLock не спасает — требуется reconnect после screen-on (AC-003).

## Открытые вопросы

1. Нужно ли добавить принудительное удержание WiFi-коннекта (`WifiManager.createWifiLock`) для старых Android?
   - Решение: пока нет — Redmi 15 использует WiFi 6, пакеты не теряются на уровне L2.
