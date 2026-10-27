package com.kvn.client.transport

import com.kvn.client.config.ConnectionConfig
import com.kvn.client.transport.reconnect.ReconnectManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.Assert.assertTrue
import org.junit.Test

// @sk-test stealth-surface-hardening#T5.5: Android reconnect jitter stays bounded (AC-014)
class ReconnectJitterTest {
    private fun manager(pct: Int): ReconnectManager = ReconnectManager(
        scope = CoroutineScope(Dispatchers.Unconfined),
        config = ConnectionConfig(minBackoffSec = 10, maxBackoffSec = 10, reconnectJitterPct = pct),
        onReconnect = {},
        onStateChange = {}
    )

    @Test
    fun testConfiguredJitterBounds() {
        val mgr = manager(20)
        val base = 10_000L
        val delta = base * 20 / 100
        repeat(1000) {
            val d = mgr.delayForAttempt(0)
            assertTrue("delay $d outside bounds", d in (base - delta)..(base + delta))
        }
    }

    @Test
    fun testLegacyJitterIsNonNegative() {
        val mgr = manager(0)
        repeat(1000) {
            assertTrue(mgr.delayForAttempt(0) >= 10_000L)
        }
    }
}
