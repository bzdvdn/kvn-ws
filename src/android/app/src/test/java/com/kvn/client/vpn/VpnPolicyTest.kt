package com.kvn.client.vpn

import java.util.concurrent.CountDownLatch
import java.util.concurrent.atomic.AtomicInteger
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NetworkTrackerTest {

    private fun tracker(): NetworkTracker = NetworkTracker().apply { beginRegistration() }

    @Test
    fun firstCallbackIsBaseline() {
        val t = tracker()
        val first = t.onAvailable(1L)
        assertFalse(first.changed)
        assertEquals(1L, first.activeHandle)
    }

    @Test
    fun duplicateCallbackIsNotAChange() {
        val t = tracker()
        t.onAvailable(1L)
        val duplicate = t.onAvailable(1L)
        assertFalse(duplicate.changed)
    }

    @Test
    fun switchingDefaultNetworkRestartsOnce() {
        val t = tracker()
        t.onAvailable(1L)
        val moved = t.onAvailable(2L)
        assertTrue(moved.changed)
        assertEquals(2L, moved.activeHandle)
        assertFalse(t.onAvailable(2L).changed)
    }

    @Test
    fun losingActiveNetworkMarksLostAll() {
        val t = tracker()
        t.onAvailable(1L)
        val lost = t.onLost(1L)
        assertTrue(lost.lostAll)
        assertEquals(null, lost.activeHandle)
    }

    @Test
    fun losingStaleNetworkIsIgnored() {
        val t = tracker()
        t.onAvailable(1L)
        val stale = t.onLost(99L)
        assertFalse(stale.changed)
        assertEquals(1L, stale.activeHandle)
    }

    @Test
    fun networkBackAfterLossIsAChange() {
        val t = tracker()
        t.onAvailable(1L)
        t.onLost(1L)
        val regained = t.onAvailable(1L)
        assertTrue(regained.changed)
        assertEquals(1L, regained.activeHandle)
    }

    @Test
    fun reregistrationStartsWithBaseline() {
        val t = tracker()
        t.onAvailable(1L)
        t.beginRegistration()
        val afterRegister = t.onAvailable(2L)
        assertFalse(afterRegister.changed)
        assertEquals(2L, afterRegister.activeHandle)
    }

    @Test
    fun resetClearsState() {
        val t = tracker()
        t.onAvailable(1L)
        t.reset()
        t.beginRegistration()
        val first = t.onAvailable(3L)
        assertFalse(first.changed)
        assertEquals(3L, first.activeHandle)
    }
}

class SessionRecoveryTest {
    @Test
    fun healthySessionIsNotRestartedOnUnlock() {
        assertFalse(SessionRecovery.shouldRestartOnUnlock(healthy = true))
    }

    @Test
    fun unhealthySessionIsRestartedOnUnlock() {
        assertTrue(SessionRecovery.shouldRestartOnUnlock(healthy = false))
    }
}

class RestartGateTest {
    @Test
    fun concurrentEntrantsProduceSingleWinner() {
        val gate = RestartGate()
        val winners = AtomicInteger(0)
        val start = CountDownLatch(1)
        val threads = (1..16).map {
            Thread {
                start.await()
                if (gate.tryEnter()) winners.incrementAndGet()
            }
        }
        threads.forEach { it.start() }
        start.countDown()
        threads.forEach { it.join() }
        assertEquals(1, winners.get())
        assertTrue(gate.isEntered)
    }

    @Test
    fun exitAllowsReentry() {
        val gate = RestartGate()
        assertTrue(gate.tryEnter())
        assertFalse(gate.tryEnter())
        gate.exit()
        assertTrue(gate.tryEnter())
    }
}

class VpnConfigParserTest {
    @Test
    fun extractsHostsFromHostPortUpstreams() {
        val json = """{"dns_proxy":{"upstreams":["9.9.9.9:53","[2606:4700:4700::1111]:53"]}}"""
        assertEquals(listOf("9.9.9.9", "2606:4700:4700::1111"), VpnConfigParser.dnsServers(json))
    }

    @Test
    fun fallsBackWhenUpstreamsMissing() {
        assertEquals(listOf("1.1.1.1"), VpnConfigParser.dnsServers("{}"))
    }

    @Test
    fun keepsBareAddresses() {
        assertEquals("1.1.1.1", VpnConfigParser.hostOf("1.1.1.1"))
        assertEquals("1.1.1.1", VpnConfigParser.hostOf("1.1.1.1:53"))
        assertEquals("", VpnConfigParser.hostOf(""))
    }
}
