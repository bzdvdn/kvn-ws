# Context — src/internal/transport/framing

## Purpose

Бинарный формат кадра KVN: 4-байтовый заголовок + payload, с типами/флагами, сегментацией крупных сообщений и пулом буферов, чтобы не аллоцировать на каждый пакет.

## Responsibilities

- Владеет: структурой `Frame`, кодами типов/флагов (сгенерированы из `protocol/frames.yaml`), `Encode`/`Decode`, `EncodeSegmented`, пулами `sync.Pool` и `GetBuffer`/`ReturnBuffer`.
- Не владеет: семантикой типов кадров (её задаёт `protocol/frames.yaml` и потребители в `tunnel`/`handshake`), транспортом и шифрованием.

## How it works

- Заголовок (4 байта): `Type` (1), `Flags` (1), `Length` (uint16 big-endian) — payload максимум 65535 байт.
- `Encode` берёт буфер из пула, пишет заголовок и копирует payload; возвращённый срез **принадлежит вызывающему** и должен быть отдан в `ReturnBuffer`.
- `Decode` проверяет длину, копирует payload в буфер из пула; после обработки кадр нужно освободить через `Release()` (или `ReturnBuffer(f.Payload)`).
- `EncodeSegmented(mtu)` режет payload на куски по `mtu` (по умолчанию 1500) и ставит `FrameFlagSegment`; последний кусок получает `FrameFlagSegmentLast`. Если payload ≤ mtu — один обычный кадр без флагов.
- Типы: `DATA=0x01`, `HELLO=0x02`, `AUTH=0x03`, `CLOSE=0x04`, `PROXY=0x05`, `DNS=0x06`; флаги `0x40`/`0x80`.
- `types_gen.go` генерируется из `protocol/frames.yaml` — единый источник для Go и Kotlin.

## Key files

- `src/internal/transport/framing/framing.go` — `Frame`, encode/decode, пулы, сегментация.
- `src/internal/transport/framing/types_gen.go` — сгенерированные константы (не править руками).
- `protocol/frames.yaml` — источник истины для типов/флагов.

## Public contracts

- `Frame{Type, Flags, Length, Payload}`; `Encode() ([]byte, error)`, `Decode([]byte) error`, `Release()`, `EncodeSegmented(mtu int) ([][]byte, error)`, `IsSegment()`, `IsLastSegment()`.
- Константы `FrameType*`, `FrameFlag*`, `FrameHeaderSize=4`, `FrameMaxPayloadSize=65535`, `DefaultPMTU=1500`, ошибки `ErrPayloadTooLarge`, `ErrFrameTooShort`.
- `GetBuffer(size)` / `ReturnBuffer(buf)` — общий пул буферов.
- `protocol/frames.yaml` — внешний контракт совместимости с Android-клиентом.

## Dependencies

- Только стандартная библиотека.

## Consumed by

- `src/internal/protocol/handshake`, `src/internal/tunnel`, `src/internal/bootstrap/{client,relay}`, `src/internal/dnsproxy`, `src/internal/proxy`, тесты интеграции и gate-программы.

## Gotchas

- Пара «Encode → ReturnBuffer» и «Decode → Release» обязательна: иначе буферы уходят в GC, пул теряет смысл; повторный `Release` безопасен (проверка `nil`), но `ReturnBuffer` дважды — нет.
- `Decode` **копирует** payload в новый буфер; `Frame.Payload` не ссылается на входные данные.
- `EncodeSegmented` режет по `mtu` только payload — итоговый кадр на проводе больше на `FrameHeaderSize` (4 байта); учитывайте это при расчёте реального MTU.
- Сегментные флаги — единственный признак сборки; отдельного reassembly-API в пакете нет (сборка — на стороне потребителя).
- `Decode` на коротких данных возвращает новый `errors.New("frame too short")`, а не объявленный `ErrFrameTooShort` — `errors.Is(err, ErrFrameTooShort)` **не сработает**. Ошибка обычная, не паника: повреждённый/частичный кадр надо трактовать как разрыв соединения.
- Правка констант вручную в `types_gen.go` будет перезаписана кодогенерацией.
