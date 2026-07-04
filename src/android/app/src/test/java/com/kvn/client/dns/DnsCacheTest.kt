package com.kvn.client.dns

import org.junit.Assert.*
import org.junit.Test
import java.net.InetAddress

// @sk-test android-dns-cache#T5.1: DnsCache unit tests (AC-001, AC-004)
class DnsCacheTest {

    @Test
    fun testSetAndGet() {
        val cache = DnsCache(maxEntries = 1024)
        val ip = InetAddress.getByName("1.2.3.4")
        cache.set("example.com", listOf(ip), 60)
        val result = cache.get("example.com")
        assertNotNull(result)
        assertEquals(1, result!!.size)
        assertEquals(ip, result[0])
    }

    @Test
    fun testGetReturnsNullForMissing() {
        val cache = DnsCache()
        assertNull(cache.get("nonexistent.com"))
    }

    @Test
    fun testGetReturnsNullAfterExpiry() {
        val cache = DnsCache()
        val ip = InetAddress.getByName("1.2.3.4")
        cache.set("example.com", listOf(ip), 1)
        Thread.sleep(1100)
        assertNull(cache.get("example.com"))
    }

    @Test
    fun testLruEviction() {
        val cache = DnsCache(maxEntries = 3)
        val ip = InetAddress.getByName("1.2.3.4")
        cache.set("a.com", listOf(ip), 3600)
        cache.set("b.com", listOf(ip), 3600)
        cache.set("c.com", listOf(ip), 3600)
        // Access a.com to make it most recently used
        assertNotNull(cache.get("a.com"))
        // Add one more — should evict b.com (least recently used)
        cache.set("d.com", listOf(ip), 3600)
        assertNull(cache.get("b.com"))
        assertNotNull(cache.get("a.com"))
        assertNotNull(cache.get("c.com"))
        assertNotNull(cache.get("d.com"))
    }

    @Test
    fun testClear() {
        val cache = DnsCache()
        val ip = InetAddress.getByName("1.2.3.4")
        cache.set("example.com", listOf(ip), 60)
        cache.clear()
        assertNull(cache.get("example.com"))
    }

    @Test
    fun testSetWithZeroTtlCoercesToMin() {
        val cache = DnsCache()
        val ip = InetAddress.getByName("1.2.3.4")
        cache.set("example.com", listOf(ip), 0)
        val result = cache.get("example.com")
        assertNotNull(result)
        assertEquals(ip, result!![0])
    }

    @Test
    fun testMultipleIps() {
        val cache = DnsCache()
        val ips = listOf(
            InetAddress.getByName("1.1.1.1"),
            InetAddress.getByName("2.2.2.2"),
            InetAddress.getByName("3.3.3.3")
        )
        cache.set("example.com", ips, 60)
        val result = cache.get("example.com")
        assertNotNull(result)
        assertEquals(3, result!!.size)
        assertEquals(ips, result)
    }

    @Test
    fun testSize() {
        val cache = DnsCache(maxEntries = 10)
        val ip = InetAddress.getByName("1.2.3.4")
        assertEquals(0, cache.size())
        cache.set("a.com", listOf(ip), 60)
        assertEquals(1, cache.size())
        cache.set("b.com", listOf(ip), 60)
        assertEquals(2, cache.size())
    }
}
