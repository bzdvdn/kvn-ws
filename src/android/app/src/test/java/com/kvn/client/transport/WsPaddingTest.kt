package com.kvn.client.transport

import org.junit.Assert.*
import org.junit.Test
import java.util.Random

// @sk-test stealth-surface-hardening#T5.5: padding codec matches Go random/fixed wire format (AC-011)
class WsPaddingTest {
    @Test
    fun testFixedPaddingRoundTrip() {
        val payload = ByteArray(100) { it.toByte() }
        val wire = WsPadding.wrap(payload, true, 512, "fixed", Random(1))
        assertEquals(512, wire.size)
        assertArrayEquals(payload, WsPadding.unwrap(wire))
    }

    @Test
    fun testRandomPaddingVariesSmallFramesAndSkipsLarge() {
        val rng = Random(42)
        val payload = ByteArray(100) { it.toByte() }
        val sizes = mutableSetOf<Int>()
        repeat(20) {
            val wire = WsPadding.wrap(payload, true, 1024, "random", rng)
            sizes.add(wire.size)
            assertTrue(wire.size in (4 + payload.size)..1024)
            assertArrayEquals(payload, WsPadding.unwrap(wire))
        }
        assertTrue("random padding should vary sizes", sizes.size > 1)

        val large = ByteArray(1100) { 0x42 }
        val wire = WsPadding.wrap(large, true, 1024, "random", rng)
        assertEquals(4 + large.size, wire.size)
        assertArrayEquals(large, WsPadding.unwrap(wire))
    }

    @Test
    fun testPaddingDisabled() {
        val payload = ByteArray(10) { 1 }
        assertSame(payload, WsPadding.wrap(payload, false, 512, "random", Random(1)))
    }

    @Test
    fun testNoPadAliases() {
        val headers = WsNegotiation.noPadHeaders(listOf("X-Client-Flags", "X-KVN-NoPad"))
        assertTrue(WsNegotiation.noPadConfirmed({ if (it == "X-Client-Flags") "1" else null }, headers))
        assertFalse(WsNegotiation.noPadConfirmed({ null }, headers))
        assertEquals("X-KVN-NoPad", WsNegotiation.noPadHeaders(emptyList()).first())
    }

    @Test
    fun testSubprotocolAliases() {
        assertEquals("telemetry.v1, kvn-ws-mux", WsNegotiation.subprotocolHeader(true, listOf("telemetry.v1", "kvn-ws-mux")))
        assertEquals("kvn-ws-mux", WsNegotiation.subprotocolHeader(true, emptyList()))
        assertNull(WsNegotiation.subprotocolHeader(false, listOf("telemetry.v1")))
    }
}
