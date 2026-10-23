# Context — src/internal/crypto

## Purpose

App-layer шифрование полезной нагрузки туннеля: вывод сессионного ключа из master-key + серверной соли + `sessionID` и симметричное шифрование AES-256-GCM с nonce в начале пакета.

## Responsibilities

- Владеет: `SessionCipher` (Encrypt/Decrypt), выводом ключа (`deriveKey`), генерацией соли, парсингом master-key из hex.
- Не владеет: TLS, хранением/передачей master-key (`config`), выбором, какие кадры шифровать (`tunnel`), обфускацией (`transport/quic`).

## How it works

- `ParseMasterKey(hexKey)`: строка из 64 hex-символов → 32 байта; пустая строка или неверная длина — ошибка.
- `GenerateSalt()`: 32 случайных байта (`crypto/rand`); сервер генерирует соль на каждую сессию и отдаёт её в `ServerHello.CryptoSalt`.
- `NewSessionCipher(masterKey, salt, sessionID)`: проверяет `len(masterKey)==32`, `len(salt)==32`; ключ = `HMAC-SHA256(masterKey, salt || sessionID)`; инициализирует `aes.NewCipher` + `cipher.NewGCM` (стандартный 12-байтовый nonce, 16-байтовый tag).
- `Encrypt`: генерирует случайный 12-байтовый nonce, возвращает `nonce || ciphertext || tag`; AAD не используется.
- `Decrypt`: требует минимум `NonceLen+TagOverhead = 28` байт, режет первые 12 как nonce, остальное отдаёт `aead.Open`.
- Тот же вывод ключа повторён на Android (`AesGcmCipher.deriveKey`) — совместимость с сервером зависит от точного совпадения `salt || sessionId` (UTF-8) и HMAC-SHA256.

## Key files

- `src/internal/crypto/crypto.go` — константы, `SessionCipher`, `deriveKey`, `GenerateSalt`, `ParseMasterKey`.

## Public contracts

- `KeyLen=32`, `SaltLen=32`, `NonceLen=12`, `TagOverhead=16`, `MinPacketOverhead=28`.
- `NewSessionCipher(masterKey, salt []byte, sessionID string) (*SessionCipher, error)`.
- `(*SessionCipher).Encrypt(plaintext []byte) ([]byte, error)`, `(*SessionCipher).Decrypt(data []byte) ([]byte, error)`.
- `GenerateSalt() ([]byte, error)`, `ParseMasterKey(hexKey string) ([]byte, error)`.
- Wire-формат зашифрованного payload: `nonce(12) | AES-GCM(ciphertext+tag)`.

## Dependencies

- Только стандартная библиотека (`crypto/aes`, `crypto/cipher`, `crypto/hmac`, `crypto/sha256`, `crypto/rand`).

## Consumed by

- `src/internal/bootstrap/{client,server}` — создание `SessionCipher` из конфига и `ServerHello.CryptoSalt`.
- `src/internal/tunnel/session.go` — шифрование/дешифрование `FrameTypeData` и датаграмм.
- Android `AesGcmCipher` — независимая реализация того же контракта.

## Gotchas

- Вывод ключа — **HMAC-SHA256**, а не HKDF, несмотря на формулировки в комментариях/задачах; менять алгоритм нельзя без синхронного изменения Android и совместимости.
- `salt` обязан быть ровно 32 байта; соль другой длины (в т.ч. пустая) — ошибка конструктора.
- Nonce случайный на каждый вызов; счётчика/повторов нет — безопасность держится на вероятности, а не на детерминированной последовательности.
- AAD пустой: метаданные (тип кадра, stream ID, session ID) не аутентифицируются.
- `Decrypt` не отличает «обрезанный пакет» от «битого тега» по типу ошибки — вызывающие обычно просто дропают пакет.
- `SessionCipher` не поддерживает ротацию ключа: при реконнекте создаётся новый экземпляр с новой солью/сессией.
- `Encrypt` аллоцирует новый буфер на каждый пакет (nonce + ciphertext); пула нет.
- Master-key в конфиге клиента без ключа молча отключает шифрование (см. `config`), а сервер при `crypto.enabled` без ключа падает.
