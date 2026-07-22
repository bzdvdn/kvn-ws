package com.kvn.client.crypto

import org.junit.Assert.*
import org.junit.Test
import javax.crypto.spec.SecretKeySpec

// @sk-test kvn-android#T4.1: TestAesGcmEncryptDecrypt (AC-001)
class AesGcmCipherTest {

    @Test
    fun testEncryptDecrypt() {
        val key = SecretKeySpec(ByteArray(32) { it.toByte() }, "AES")
        val cipher = AesGcmCipher()
        cipher.init(key)

        val plaintext = "Hello KVN!".toByteArray()
        val encrypted = cipher.encrypt(plaintext)
        val decrypted = cipher.decrypt(encrypted)

        assertArrayEquals(plaintext, decrypted)
    }

    // @sk-test kvn-android#T4.1: TestAesGcmDifferentKeys (AC-001)
    @Test
    fun testDecryptWithWrongKeyFails() {
        val key1 = SecretKeySpec(ByteArray(32) { 0x01 }, "AES")
        val key2 = SecretKeySpec(ByteArray(32) { 0x02 }, "AES")

        val cipher1 = AesGcmCipher()
        cipher1.init(key1)
        val cipher2 = AesGcmCipher()
        cipher2.init(key2)

        val plaintext = "secret data".toByteArray()
        val encrypted = cipher1.encrypt(plaintext)

        try {
            cipher2.decrypt(encrypted)
            fail("expected AEADBadTagException")
        } catch (_: Exception) {
            // expected
        }
    }

    // @sk-test kvn-android#T4.1: TestKeyDerivation (AC-001)
    @Test
    fun testKeyDerivation() {
        val masterKey = ByteArray(32) { 0xAA.toByte() }
        val salt = ByteArray(16) { 0xAB.toByte() }
        val sessionId = "test-session-id"
        val key = AesGcmCipher.deriveKey(masterKey, salt, sessionId)

        assertEquals(32, key.encoded.size)
        assertNotNull(key)

        // Deterministic: same inputs = same key
        val key2 = AesGcmCipher.deriveKey(masterKey, salt, sessionId)
        assertArrayEquals(key.encoded, key2.encoded)
    }
}
