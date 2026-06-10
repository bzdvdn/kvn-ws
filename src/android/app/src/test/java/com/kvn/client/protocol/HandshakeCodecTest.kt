package com.kvn.client.protocol

import org.junit.Assert.*
import org.junit.Test

// @sk-test kvn-android#T4.1: TestHandshakeRoundTrip (AC-001, AC-004)
class HandshakeCodecTest {

    @Test
    fun testClientHelloEncodeDecode() {
        val hello = ClientHello(
            protoVersion = PROTO_VERSION,
            token = "test-token-123",
            mtu = 1400,
            ipv6 = true,
            transport = "tcp"
        )
        val frame = HandshakeCodec.encodeClientHello(hello)

        assertEquals(FrameTypes.FRAME_TYPE_HELLO, frame.type)
        assertTrue(frame.payload.size > 4) // header sized payload
    }

    // @sk-test kvn-android#T4.1: TestServerHelloDecode (AC-001)
    @Test
    fun testServerHelloDecode() {
        // Build a minimal ServerHello payload matching Go encoding
        val sessionId = "abcdef1234567890abcdef1234567890" // 32 hex chars = 16 bytes
        val sessionBytes = hexToBytes(sessionId)
        val ip4 = byteArrayOf(10, 0, 0, 1)

        val payload = sessionBytes +
                byteArrayOf(1) + // count = 1
                byteArrayOf(4) + // family = IPv4
                byteArrayOf(4) + // length = 4
                ip4 +
                byteArrayOf(0x05, 0xDC.toByte()) // MTU = 1500

        val frame = Frame(FrameTypes.FRAME_TYPE_HELLO, FrameFlags.FRAME_FLAG_NONE, payload)
        val serverHello = HandshakeCodec.decodeServerHello(frame)

        assertEquals(sessionId, serverHello.sessionId)
        assertEquals("10.0.0.1", serverHello.assignedIp)
        assertEquals(1500, serverHello.mtu)
    }

    // @sk-test kvn-android#T4.1: TestAuthError (AC-001)
    @Test
    fun testAuthErrorEncodeDecode() {
        val frame = HandshakeCodec.encodeAuthError("invalid token")
        val decoded = HandshakeCodec.decodeAuthError(frame)
        assertEquals("invalid token", decoded.reason)
    }

    private fun hexToBytes(hex: String): ByteArray {
        return hex.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
    }
}
