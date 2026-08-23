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
            transport = "tcp",
            channel = "",
            sessionId = ""
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

    // @sk-test android-dual-ws#T4.1: TestClientHelloSecondaryTags (AC-001)
    @Test
    fun testClientHelloSecondaryTags() {
        val hello = ClientHello(
            protoVersion = PROTO_VERSION,
            token = "test-token-123",
            mtu = 1400,
            ipv6 = true,
            transport = "tcp",
            channel = "secondary",
            sessionId = "abcdef1234567890abcdef1234567890"
        )
        val frame = HandshakeCodec.encodeClientHello(hello)
        val data = frame.payload

        val flags = data[1].toInt() and 0xFF
        val tokenLen = ((data[2].toInt() and 0xFF) shl 8) or (data[3].toInt() and 0xFF)
        var pos = 4 + tokenLen
        if (flags and FLAG_MTU.toInt() != 0) pos += 2

        val tags = mutableMapOf<Byte, String>()
        while (pos + 2 <= data.size) {
            val tag = data[pos]
            val len = data[pos + 1].toInt() and 0xFF
            if (pos + 2 + len > data.size) break
            tags[tag] = String(data, pos + 2, len)
            pos += 2 + len
        }

        assertEquals("secondary", tags[CHANNEL_TAG])
        assertEquals("abcdef1234567890abcdef1234567890", tags[SESSION_TAG])
    }

    // @sk-test android-dual-ws#T4.1: TestClientHelloNoTagsBackwardCompat (AC-007)
    @Test
    fun testClientHelloNoTagsBackwardCompat() {
        val hello = ClientHello(
            protoVersion = PROTO_VERSION,
            token = "tok",
            mtu = 0,
            ipv6 = false,
            transport = "tcp",
            channel = "",
            sessionId = ""
        )
        val frame = HandshakeCodec.encodeClientHello(hello)
        // token(2+3) + flags(1) + version(1) + transport tag(2+3) = 12 bytes, no channel/session
        assertEquals(2 + 3 + 1 + 1 + 2 + 3, frame.payload.size)
    }

    private fun hexToBytes(hex: String): ByteArray {
        return hex.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
    }
}
