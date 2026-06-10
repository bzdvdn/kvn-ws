package com.kvn.client.protocol

import org.junit.Assert.*
import org.junit.Test

// @sk-test kvn-android#T4.1: TestFrameEncodeDecode (AC-001, AC-004)
class FrameCodecTest {

    @Test
    fun testEncodeDecodeDataFrame() {
        val payload = byteArrayOf(0x45, 0x00, 0x00, 0x3c)
        val frame = Frame(FrameTypes.FRAME_TYPE_DATA, FrameFlags.FRAME_FLAG_NONE, payload)
        val encoded = frame.encode()
        val decoded = encoded.toFrame()

        assertEquals(FrameTypes.FRAME_TYPE_DATA, decoded.type)
        assertEquals(FrameFlags.FRAME_FLAG_NONE, decoded.flags)
        assertArrayEquals(payload, decoded.payload)
    }

    // @sk-test kvn-android#T4.1: TestFrameEncodeDecodeAllTypes (AC-004)
    @Test
    fun testEncodeDecodeAllTypes() {
        for (type in byteArrayOf(
            FrameTypes.FRAME_TYPE_DATA, FrameTypes.FRAME_TYPE_HELLO, FrameTypes.FRAME_TYPE_AUTH,
            FrameTypes.FRAME_TYPE_CLOSE, FrameTypes.FRAME_TYPE_PROXY, FrameTypes.FRAME_TYPE_DNS
        )) {
            val frame = Frame(type, FrameFlags.FRAME_FLAG_NONE, byteArrayOf(0x01, 0x02))
            val decoded = frame.encode().toFrame()
            assertEquals(type, decoded.type)
            assertArrayEquals(byteArrayOf(0x01, 0x02), decoded.payload)
        }
    }

    // @sk-test kvn-android#T4.1: TestFrameTruncated (AC-001)
    @Test(expected = IllegalArgumentException::class)
    fun testDecodeTooShort() {
        byteArrayOf(0x01, 0x02).toFrame()
    }

    // @sk-test kvn-android#T4.1: TestFrameEmptyPayload (AC-001)
    @Test
    fun testEncodeDecodeEmptyPayload() {
        val frame = Frame(FrameTypes.FRAME_TYPE_CLOSE, FrameFlags.FRAME_FLAG_NONE, ByteArray(0))
        val decoded = frame.encode().toFrame()
        assertEquals(0, decoded.payload.size)
    }
}
