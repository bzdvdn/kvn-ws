package com.kvn.client.transport

import java.nio.ByteBuffer
import java.util.Random

// @sk-task stealth-surface-hardening#T5.2: shared padding codec matching Go WSConn (AC-011)
object WsPadding {
    const val DEFAULT_SIZE = 512

    fun wrap(
        data: ByteArray,
        enabled: Boolean,
        size: Int,
        mode: String,
        rng: Random
    ): ByteArray {
        if (!enabled) return data
        val padSize = if (size <= 0) DEFAULT_SIZE else size
        val totalLen = 4 + data.size
        val padding = if (mode.equals("random", ignoreCase = true)) {
            if (totalLen < padSize) rng.nextInt(padSize - totalLen + 1) else 0
        } else {
            (padSize - totalLen % padSize) % padSize
        }
        val buf = ByteBuffer.allocate(totalLen + padding)
        buf.putInt(data.size)
        buf.put(data)
        if (padding > 0) {
            val pad = ByteArray(padding)
            rng.nextBytes(pad)
            buf.put(pad)
        }
        return buf.array()
    }

    fun unwrap(wireData: ByteArray): ByteArray {
        require(wireData.size >= 4) { "padding frame too short: ${wireData.size}" }
        val payloadLen = ByteBuffer.wrap(wireData, 0, 4).int
        require(payloadLen >= 0 && payloadLen <= wireData.size - 4) {
            "invalid padding payload length: $payloadLen (max ${wireData.size - 4})"
        }
        return wireData.copyOfRange(4, 4 + payloadLen)
    }
}
