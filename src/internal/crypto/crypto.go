// @sk-task app-crypto#T1: real app-layer encryption implementation

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	KeyLen      = 32
	SaltLen     = 32
	NonceLen    = 12
	TagOverhead = 16

	MinPacketOverhead = NonceLen + TagOverhead
)

type SessionCipher struct {
	aead cipher.AEAD
}

func NewSessionCipher(masterKey, salt []byte, sessionID string) (*SessionCipher, error) {
	if len(masterKey) != KeyLen {
		return nil, fmt.Errorf("crypto: master key must be %d bytes", KeyLen)
	}
	if len(salt) != SaltLen {
		return nil, fmt.Errorf("crypto: salt must be %d bytes", SaltLen)
	}
	sessionKey := deriveKey(masterKey, salt, sessionID)
	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: aes init: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: gcm init: %w", err)
	}
	return &SessionCipher{aead: aead}, nil
}

func deriveKey(master, salt []byte, sessionID string) []byte {
	mac := hmac.New(sha256.New, master)
	mac.Write(salt)
	mac.Write([]byte(sessionID))
	return mac.Sum(nil)
}

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("crypto: generate salt: %w", err)
	}
	return salt, nil
}

func ParseMasterKey(hexKey string) ([]byte, error) {
	if hexKey == "" {
		return nil, errors.New("crypto: key is empty")
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: key is not valid hex: %w", err)
	}
	if len(key) != KeyLen {
		return nil, fmt.Errorf("crypto: key must be %d hex chars (%d bytes)", KeyLen*2, KeyLen)
	}
	return key, nil
}

func (sc *SessionCipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, NonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	out := make([]byte, NonceLen+len(plaintext)+TagOverhead)
	copy(out, nonce)
	sc.aead.Seal(out[:NonceLen], nonce, plaintext, nil)
	return out, nil
}

func (sc *SessionCipher) Decrypt(data []byte) ([]byte, error) {
	if len(data) < NonceLen+TagOverhead {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce := data[:NonceLen]
	ciphertext := data[NonceLen:]
	plaintext, err := sc.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}
