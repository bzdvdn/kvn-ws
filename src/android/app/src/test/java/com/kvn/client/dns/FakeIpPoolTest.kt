package com.kvn.client.dns

import org.junit.Assert.*
import org.junit.Test
import java.net.InetAddress

// @sk-test android-fakedns-routing#T1.2: FakeIpPool unit tests (DEC-004)
// @sk-test android-fakedns-routing#T4.1: mapping consistency, random access (AC-006)
class FakeIpPoolTest {

    @Test
    // @sk-test android-fakedns-routing#T1.2: allocate returns IP in pool range
    fun testAllocateReturnsIpInRange() {
        val pool = FakeIpPool()
        val ip = pool.allocate("example.com")
        assertNotNull(ip)
        val raw = ip!!.address
        assertEquals(198, raw[0].toInt() and 0xFF)
        assertTrue((raw[1].toInt() and 0xFF) in 0..127) // 198.18.0.0/17
    }

    @Test
    // @sk-test android-fakedns-routing#T1.2: allocate returns distinct IPs
    fun testAllocateReturnsDistinctIps() {
        val pool = FakeIpPool()
        val ip1 = pool.allocate("a.com")
        val ip2 = pool.allocate("b.com")
        assertNotNull(ip1)
        assertNotNull(ip2)
        assertNotEquals(ip1, ip2)
    }

    @Test
    // @sk-test android-fakedns-routing#T1.2: release makes IP reusable
    fun testReleaseMakesIpReusable() {
        val pool = FakeIpPool(poolSize = 2)
        val ip1 = pool.allocate("a.com")
        val ip2 = pool.allocate("b.com")
        assertNotNull(ip1)
        assertNotNull(ip2)
        pool.release(ip1!!)
        val ip3 = pool.allocate("c.com")
        assertNotNull(ip3)
        assertEquals(ip1, ip3)
    }

    @Test
    // @sk-test android-fakedns-routing#T1.2: allocate returns null on exhaustion
    fun testExhaustionReturnsNull() {
        val pool = FakeIpPool(poolSize = 1)
        assertNotNull(pool.allocate("a.com"))
        assertNull(pool.allocate("b.com"))
    }

    @Test
    // @sk-test android-fakedns-routing#T1.2: lookup returns domain for allocated IP
    fun testLookupReturnsDomain() {
        val pool = FakeIpPool()
        val ip = pool.allocate("example.com")
        assertNotNull(ip)
        assertEquals("example.com", pool.lookup(ip!!))
    }

    @Test
    // @sk-test android-fakedns-routing#T1.2: lookup returns null for unknown IP
    fun testLookupReturnsNullForUnknown() {
        val pool = FakeIpPool()
        assertNull(pool.lookup(InetAddress.getByName("8.8.8.8")))
    }

    @Test
    // @sk-test android-fakedns-routing#T1.2: clear resets all state
    fun testClearResetsState() {
        val pool = FakeIpPool()
        val ip = pool.allocate("example.com")
        assertNotNull(ip)
        assertEquals(1, pool.allocatedCount())
        pool.clear()
        assertEquals(0, pool.allocatedCount())
        assertNull(pool.lookup(ip!!))
    }

    @Test
    // @sk-test android-fakedns-routing#T1.2: allocatedCount tracks active allocations
    fun testAllocatedCount() {
        val pool = FakeIpPool()
        assertEquals(0, pool.allocatedCount())
        val ip1 = pool.allocate("a.com")
        assertEquals(1, pool.allocatedCount())
        val ip2 = pool.allocate("b.com")
        assertEquals(2, pool.allocatedCount())
        pool.release(ip1!!)
        assertEquals(1, pool.allocatedCount())
        pool.release(ip2!!)
        assertEquals(0, pool.allocatedCount())
    }

    @Test
    // @sk-test android-fakedns-routing#T1.2: release of IP outside pool is no-op
    fun testReleaseOutsidePoolIsNoOp() {
        val pool = FakeIpPool()
        val outside = InetAddress.getByName("10.0.0.1")
        pool.release(outside) // should not throw
        assertEquals(0, pool.allocatedCount())
    }

    @Test
    // @sk-test android-fakedns-routing#T4.1: same domain gets same IP after release + re-allocate (AC-006)
    fun testSameDomainReusesIpAfterRelease() {
        val pool = FakeIpPool(poolSize = 2)
        val ip1 = pool.allocate("a.com")!!
        pool.release(ip1)
        val ip2 = pool.allocate("a.com")
        assertNotNull(ip2)
        assertEquals(ip1, ip2)
    }

    @Test
    // @sk-test android-fakedns-routing#T4.1: lookup returns correct domain for each mapping (AC-006)
    fun testLookupConsistency() {
        val pool = FakeIpPool()
        val ip1 = pool.allocate("example.com")!!
        val ip2 = pool.allocate("test.org")!!
        assertEquals("example.com", pool.lookup(ip1))
        assertEquals("test.org", pool.lookup(ip2))
        // After release, lookup returns null
        pool.release(ip1)
        assertNull(pool.lookup(ip1))
        assertEquals("test.org", pool.lookup(ip2))
    }
}
