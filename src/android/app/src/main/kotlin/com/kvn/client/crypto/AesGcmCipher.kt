package com.kvn.client.crypto

import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.Mac
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec
import javax.crypto.SecretKey

const val AES_KEY_SIZE = 32 // AES-256
const val GCM_IV_SIZE = 12
const val GCM_TAG_SIZE = 16

// @sk-task kvn-android#T2.2: AES-256-GCM encrypt/decrypt (AC-001)
// @sk-task android-latency-power-fix#T2.1: cached Cipher instances (AC-003)
class AesGcmCipher {
    private var encryptCipher: Cipher? = null
    private var decryptCipher: Cipher? = null
    private var secretKey: SecretKey? = null

    fun isInitialized(): Boolean = encryptCipher != null

    // @sk-task android-latency-power-fix#T2.1: init with cached Cipher.getInstance (AC-003)
    fun init(key: SecretKey) {
        secretKey = key
        encryptCipher = Cipher.getInstance("AES/GCM/NoPadding")
        decryptCipher = Cipher.getInstance("AES/GCM/NoPadding")
    }

    // @sk-task android-latency-power-fix#T2.1: reset with new key on reconnect (AC-003)
    fun reset(key: SecretKey) {
        secretKey = key
    }

    companion object {
        // @sk-task kvn-android#T2.2: derive AES key via HMAC-SHA256 (AC-001)
        // Matches Go server: HMAC-SHA256(masterKey, salt || sessionId)
        fun deriveKey(masterKey: ByteArray, salt: ByteArray, sessionId: String): SecretKey {
            val mac = Mac.getInstance("HmacSHA256")
            mac.init(SecretKeySpec(masterKey, "HmacSHA256"))
            mac.update(salt)
            mac.update(sessionId.toByteArray())
            return SecretKeySpec(mac.doFinal(), "AES")
        }

        // @sk-task kvn-android#T2.2: create cipher with random IV (AC-001)
        fun randomIv(): ByteArray {
            val iv = ByteArray(GCM_IV_SIZE)
            SecureRandom().nextBytes(iv)
            return iv
        }
    }

    // @sk-task kvn-android#T2.2: encrypt plaintext (AC-001)
    // @sk-task android-latency-power-fix#T2.1: use cached Cipher instance (AC-003)
    fun encrypt(plaintext: ByteArray, iv: ByteArray = randomIv()): ByteArray {
        val cipher = encryptCipher ?: throw IllegalStateException("AesGcmCipher not initialized")
        val spec = GCMParameterSpec(GCM_TAG_SIZE * 8, iv)
        cipher.init(Cipher.ENCRYPT_MODE, secretKey, spec)
        val ciphertext = cipher.doFinal(plaintext)
        return iv + ciphertext
    }

    // @sk-task kvn-android#T2.2: decrypt ciphertext (first 12 bytes are IV) (AC-001)
    // @sk-task android-latency-power-fix#T2.1: use cached Cipher instance (AC-003)
    fun decrypt(data: ByteArray): ByteArray {
        require(data.size >= GCM_IV_SIZE + GCM_TAG_SIZE) { "ciphertext too short" }
        val iv = data.copyOfRange(0, GCM_IV_SIZE)
        val ciphertext = data.copyOfRange(GCM_IV_SIZE, data.size)
        val cipher = decryptCipher ?: throw IllegalStateException("AesGcmCipher not initialized")
        val spec = GCMParameterSpec(GCM_TAG_SIZE * 8, iv)
        cipher.init(Cipher.DECRYPT_MODE, secretKey, spec)
        return cipher.doFinal(ciphertext)
    }

    fun clear() {
        encryptCipher = null
        decryptCipher = null
        secretKey = null
    }
}
