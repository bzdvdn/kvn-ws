package com.kvn.client.dns

import com.kvn.client.config.ConnectionConfig
import org.junit.Assert.*
import org.junit.Test
import java.net.InetAddress
import java.nio.ByteBuffer

// @sk-test android-fakedns-routing#T2.2: FakeDnsResolver unit tests (RQ-002, RQ-003, RQ-004, RQ-012)
// @sk-test android-fakedns-routing#T4.1: include matching, fake IP allocation, AAAA (AC-002, AC-008)
class FakeDnsResolverTest {

    private fun buildDnsQuery(domain: String): ByteArray {
        val labels = domain.split(".")
        val nameLen = labels.sumOf { it.length + 1 } + 1
        val buf = ByteBuffer.allocate(12 + nameLen + 4)
        buf.putShort(0x1234.toShort())
        buf.putShort(0x0100.toShort())
        buf.putShort(1)
        buf.putShort(0)
        buf.putShort(0)
        buf.putShort(0)
        for (label in labels) {
            buf.put(label.length.toByte())
            for (c in label) buf.put(c.code.toByte())
        }
        buf.put(0)
        buf.putShort(1)
        buf.putShort(1)
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: routingDomainsEnabled=false returns null
    fun testRoutingDomainsEnabledFalse() {
        val config = ConnectionConfig(routingDomainsEnabled = false,
            routingExcludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, DnsCache())
        val query = buildDnsQuery("example.com")
        assertNull(resolver.resolve(query))
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: exclude suffix match with dot-barrier resolved via cache
    fun testExcludeSuffixMatch() {
        val cache = DnsCache()
        val ip = InetAddress.getByName("93.184.216.34")
        cache.set("example.com", listOf(ip), 60)
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, cache)
        val query = buildDnsQuery("example.com")
        val response = resolver.resolve(query)
        assertNotNull(response)
        assertEquals(1, resolver.excludedSize())
        assertTrue(resolver.isExcluded(ip))
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: suffix without dot-barrier (e.g. "ru" in "prudent.ruhr") no match
    fun testSuffixDotBarrierNoMatch() {
        val cache = DnsCache()
        cache.set("prudent.ruhr", listOf(InetAddress.getByName("10.0.0.1")), 60)
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf("ru"))
        val resolver = FakeDnsResolver(config, cache)
        val query = buildDnsQuery("prudent.ruhr")
        // endsWith("ru") is true but dot-barrier fails — should return null without resolving
        assertNull(resolver.resolve(query))
        assertEquals(0, resolver.excludedSize())
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: suffix with dot-barrier (e.g. "ru" in "example.ru") matches
    fun testSuffixDotBarrierMatch() {
        val cache = DnsCache()
        val ip = InetAddress.getByName("198.51.100.1")
        cache.set("example.ru", listOf(ip), 60)
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf("ru"))
        val resolver = FakeDnsResolver(config, cache)
        val query = buildDnsQuery("example.ru")
        val response = resolver.resolve(query)
        assertNotNull(response)
        assertTrue(resolver.isExcluded(ip))
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: clearExcluded removes all tracked IPs
    fun testClearExcluded() {
        val cache = DnsCache()
        val ip = InetAddress.getByName("93.184.216.34")
        cache.set("example.com", listOf(ip), 60)
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, cache)
        val query = buildDnsQuery("example.com")
        resolver.resolve(query)
        assertEquals(1, resolver.excludedSize())
        resolver.clearExcluded()
        assertEquals(0, resolver.excludedSize())
        assertFalse(resolver.isExcluded(ip))
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: isExcluded returns false for untracked IP
    fun testIsExcludedFalseForUntracked() {
        val config = ConnectionConfig(routingDomainsEnabled = true)
        val resolver = FakeDnsResolver(config, DnsCache())
        assertFalse(resolver.isExcluded(InetAddress.getByName("8.8.8.8")))
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: resolve returns null for bad query
    fun testResolveBadQueryReturnsNull() {
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, DnsCache())
        assertNull(resolver.resolve(byteArrayOf(0, 0)))
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: resolve returns null when no suffix matches
    fun testNoSuffixMatchReturnsNull() {
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf(".org"))
        val resolver = FakeDnsResolver(config, DnsCache())
        val query = buildDnsQuery("example.com")
        assertNull(resolver.resolve(query))
        assertEquals(0, resolver.excludedSize())
    }

    @Test
    // @sk-test android-fakedns-routing#T2.2: resolve returns null when cache empty and DNS fails
    fun testCacheMissReturnsNull() {
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, DnsCache())
        val query = buildDnsQuery("this-domain-should-not-resolve.invalid")
        assertNull(resolver.resolve(query))
        assertEquals(0, resolver.excludedSize())
    }

    @Test
    // @sk-test android-fakedns-routing#T4.1: include suffix matching returns fake IP from pool (AC-002, AC-008)
    fun testIncludeSuffixMatch() {
        val cache = DnsCache()
        val realIp = InetAddress.getByName("93.184.216.34")
        cache.set("example.com", listOf(realIp), 60)
        val pool = FakeIpPool()
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingIncludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, cache, fakeIpPool = pool)
        val query = buildDnsQuery("example.com")
        val response = resolver.resolve(query)
        assertNotNull(response)
        val parsed = DnsParser.parseResponse(response!!)
        assertEquals(1, parsed.size)
        val fakeIp = parsed[0]
        val raw = fakeIp.address
        assertEquals(198, raw[0].toInt() and 0xFF)
        assertEquals("example.com", pool.lookup(fakeIp))
        // Excluded set should not contain this IP (include, not exclude)
        assertEquals(0, resolver.excludedSize())
    }

    @Test
    // @sk-test android-fakedns-routing#T4.1: include domain with no pool returns null (AC-008)
    fun testIncludeSuffixNoPool() {
        val cache = DnsCache()
        cache.set("example.com", listOf(InetAddress.getByName("93.184.216.34")), 60)
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingIncludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, cache)
        val query = buildDnsQuery("example.com")
        assertNull(resolver.resolve(query))
    }

    @Test
    // @sk-test android-fakedns-routing#T4.1: AAAA query returns empty response (AC-007)
    fun testAaaaQueryReturnsEmptyResponse() {
        val cache = DnsCache()
        cache.set("example.com", listOf(InetAddress.getByName("93.184.216.34")), 60)
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, cache)
        // Build AAAA query (QTYPE=28)
        val labels = "example.com".split(".")
        val nameLen = labels.sumOf { it.length + 1 } + 1
        val buf = java.nio.ByteBuffer.allocate(12 + nameLen + 4)
        buf.putShort(0x1234.toShort())
        buf.putShort(0x0100.toShort())
        buf.putShort(1)
        buf.putShort(0)
        buf.putShort(0)
        buf.putShort(0)
        for (label in labels) {
            buf.put(label.length.toByte())
            for (c in label) buf.put(c.code.toByte())
        }
        buf.put(0)
        buf.putShort(28) // QTYPE AAAA
        buf.putShort(1)
        val query = ByteArray(buf.position()).also { buf.flip(); buf.get(it) }
        val response = resolver.resolve(query)
        assertNotNull(response)
        // Should have no answers
        val ips = DnsParser.parseResponse(response!!)
        assertEquals(0, ips.size)
        // ID should match query
        assertEquals(0x1234, ((response[0].toInt() and 0xFF) shl 8) or (response[1].toInt() and 0xFF))
    }

    @Test
    // @sk-test android-fakedns-routing#T4.1: exclude takes priority over include (AC-002, AC-008)
    fun testExcludePriorityOverInclude() {
        val cache = DnsCache()
        val realIp = InetAddress.getByName("93.184.216.34")
        cache.set("example.com", listOf(realIp), 60)
        val pool = FakeIpPool()
        val config = ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf(".com"),
            routingIncludeDomains = listOf(".com"))
        val resolver = FakeDnsResolver(config, cache, fakeIpPool = pool)
        val query = buildDnsQuery("example.com")
        val response = resolver.resolve(query)
        assertNotNull(response)
        // Should be excluded — real IP in response
        assertTrue(resolver.excludedSize() > 0)
        assertTrue(resolver.isExcluded(realIp))
        // Pool should not have allocated anything (exclude wins)
        assertEquals(0, pool.allocatedCount())
    }
}
