// @sk-task app-crypto#T1: test encryption/decryption

package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, KeyLen)
	for i := range key {
		key[i] = byte(i)
	}
	salt := make([]byte, SaltLen)
	for i := range salt {
		salt[i] = byte(0xca)
	}

	sc, err := NewSessionCipher(key, salt, "test-session-001")
	if err != nil {
		t.Fatalf("NewSessionCipher: %v", err)
	}

	plaintext := []byte("hello world! this is a test packet for vpn encryption")
	ciphertext, err := sc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if len(ciphertext) <= NonceLen+TagOverhead {
		t.Fatalf("ciphertext too short: %d", len(ciphertext))
	}

	decrypted, err := sc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptFailsWithWrongKey(t *testing.T) {
	key1 := make([]byte, KeyLen)
	key2 := make([]byte, KeyLen)
	key2[0] = 0x01

	salt := make([]byte, SaltLen)
	for i := range salt {
		salt[i] = 0xab
	}

	sc1, _ := NewSessionCipher(key1, salt, "session-1")
	sc2, _ := NewSessionCipher(key2, salt, "session-1")

	ciphertext, err := sc1.Encrypt([]byte("secret data"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = sc2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected decrypt error with wrong key, got nil")
	}
}

func TestDecryptFailsWithTamperedCiphertext(t *testing.T) {
	key := make([]byte, KeyLen)
	salt := make([]byte, SaltLen)

	sc, _ := NewSessionCipher(key, salt, "session-tamper")

	ciphertext, err := sc.Encrypt([]byte("tamper me"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err = sc.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected decrypt error with tampered data, got nil")
	}
}

func TestDecryptShortInput(t *testing.T) {
	key := make([]byte, KeyLen)
	salt := make([]byte, SaltLen)
	sc, _ := NewSessionCipher(key, salt, "session-short")

	_, err := sc.Decrypt([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error for short input, got nil")
	}
}

func TestDifferentSaltsProduceDifferentCiphertexts(t *testing.T) {
	key := make([]byte, KeyLen)
	salt1 := make([]byte, SaltLen)
	salt2 := make([]byte, SaltLen)
	salt2[0] = 0xff

	sc1, _ := NewSessionCipher(key, salt1, "session-salt")
	sc2, _ := NewSessionCipher(key, salt2, "session-salt")

	plaintext := []byte("same plaintext")
	ct1, _ := sc1.Encrypt(plaintext)
	ct2, _ := sc2.Encrypt(plaintext)

	if bytes.Equal(ct1, ct2) {
		t.Fatal("ciphertexts with different salts should differ")
	}
}

func TestParseMasterKey(t *testing.T) {
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key, err := ParseMasterKey(hexKey)
	if err != nil {
		t.Fatalf("ParseMasterKey: %v", err)
	}
	if len(key) != KeyLen {
		t.Fatalf("key length %d, want %d", len(key), KeyLen)
	}
	expected, _ := hex.DecodeString(hexKey)
	if !bytes.Equal(key, expected) {
		t.Fatal("key mismatch")
	}

	_, err = ParseMasterKey("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}

	_, err = ParseMasterKey("invalid")
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}

	_, err = ParseMasterKey("aa")
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestGenerateSalt(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt: %v", err)
	}
	if len(salt) != SaltLen {
		t.Fatalf("salt length %d, want %d", len(salt), SaltLen)
	}

	salt2, _ := GenerateSalt()
	if bytes.Equal(salt, salt2) {
		t.Fatal("consecutive salts should differ")
	}
}

func TestSessionKeyIsDeterministic(t *testing.T) {
	key := []byte("01234567890123456789012345678901")[:KeyLen]
	salt := make([]byte, SaltLen)

	sc1, err := NewSessionCipher(key, salt, "same-session")
	if err != nil {
		t.Fatalf("sc1: %v", err)
	}
	sc2, err := NewSessionCipher(key, salt, "same-session")
	if err != nil {
		t.Fatalf("sc2: %v", err)
	}

	pt := []byte("deterministic test")
	ct, _ := sc1.Encrypt(pt)
	dec, err := sc2.Decrypt(ct)
	if err != nil || !bytes.Equal(pt, dec) {
		t.Fatal("session keys should be deterministic")
	}
}

func TestEmptyPlaintext(t *testing.T) {
	key := make([]byte, KeyLen)
	salt := make([]byte, SaltLen)
	sc, _ := NewSessionCipher(key, salt, "empty")

	ct, err := sc.Encrypt([]byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	pt, err := sc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(pt) != 0 {
		t.Fatal("empty plaintext roundtrip should be empty")
	}
}

func BenchmarkEncrypt(b *testing.B) {
	key := make([]byte, KeyLen)
	salt := make([]byte, SaltLen)
	sc, _ := NewSessionCipher(key, salt, "bench")
	payload := make([]byte, 1400)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sc.Encrypt(payload)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key := make([]byte, KeyLen)
	salt := make([]byte, SaltLen)
	sc, _ := NewSessionCipher(key, salt, "bench")
	payload := make([]byte, 1400)
	ct, _ := sc.Encrypt(payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sc.Decrypt(ct)
	}
}
