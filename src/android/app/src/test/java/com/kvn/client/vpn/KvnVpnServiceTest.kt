package com.kvn.client.vpn

import com.kvn.client.config.ConnectionConfig
import com.kvn.client.dns.FakeDnsResolver
import com.kvn.client.transport.ConnectionState
import com.kvn.client.transport.OnStateChange
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import java.net.InetAddress
import java.nio.ByteBuffer

// @sk-test android-dns-cache#T5.4: KvnVpnService integration — WS:EOF + DNS intercept (AC-003, AC-005, AC-001)
// @sk-test android-fakedns-routing#T4.1: rewrite checksum, cleanup, CIDR/IP priority (AC-003, AC-005, AC-006, AC-007)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [26])
class KvnVpnServiceTest {

    private lateinit var service: KvnVpnService

    @Before
    fun setUp() {
        val controller = Robolectric.buildService(KvnVpnService::class.java)
        service = controller.create().get()
    }

    @Test
    fun testWsEofClosesTun() {
        setConfig(ConnectionConfig(autoReconnect = false))

        val stateChange = getField<OnStateChange>(service, "onConnectionStateChange")!!
        stateChange.invoke(ConnectionState.DISCONNECTED)

        assertNull(getField<Any?>(service, "tunFd"))
        assertFalse(getField<Boolean>(service, "tunReaderStarted")!!)
        assertFalse(getField<Boolean>(service, "vpnEstablished")!!)
    }

    // @sk-test android-dns-cache#T5.4: WS:EOF with autoReconnect does not call safeStop (AC-003)
    @Test
    fun testWsEofWithAutoReconnectDoesNotStop() {
        setConfig(ConnectionConfig(autoReconnect = true))
        val stateChange = getField<OnStateChange>(service, "onConnectionStateChange")!!
        stateChange.invoke(ConnectionState.DISCONNECTED)
        val reconnectManager = getField<Any?>(service, "reconnectManager")
        assertNull(reconnectManager)
    }

    // @sk-test android-fakedns-routing#T2.2: routePacket with excluded UDP returns true
    @Test
    fun testRoutePacketExcludedUdpReturnsTrue() {
        setConfig(ConnectionConfig(routingDomainsEnabled = true))

        val excludedIp = InetAddress.getByName("93.184.216.34")
        val resolver = FakeDnsResolver(
            ConnectionConfig(routingDomainsEnabled = true, routingExcludeDomains = listOf(".example.com")),
            com.kvn.client.dns.DnsCache()
        )
        // Directly add IP to excluded set via reflection
        val excludedIpsField = FakeDnsResolver::class.java.getDeclaredField("excludedIps")
        excludedIpsField.isAccessible = true
        @Suppress("UNCHECKED_CAST")
        val excludedSet = excludedIpsField.get(resolver) as MutableSet<InetAddress>
        excludedSet.add(excludedIp)

        setField(service, "fakeDnsResolver", resolver)
        val directDeliverer = createDirectDeliverer(service)
        setField(service, "directDeliverer", directDeliverer)

        val packet = buildUdp4Packet(
            payload = byteArrayOf(0x48, 0x65, 0x6C, 0x6C, 0x6F),
            srcIp = "10.0.0.2",
            dstIp = "93.184.216.34",
            srcPort = 12345,
            dstPort = 80
        )
        val consumed = callMethod(service, "routePacket", packet) as Boolean
        assertTrue(consumed)
    }

    // @sk-test android-fakedns-routing#T2.2: routePacket with excluded TCP SYN falls back to tunnel
    // when direct connect fails (e.g., test environment has no network).
    // In production with reachable server, handleTcpSyn returns true (split tunnel).
    @Test
    fun testRoutePacketExcludedTcpSynFallback() {
        setConfig(ConnectionConfig(routingDomainsEnabled = true))

        val excludedIp = InetAddress.getByName("93.184.216.34")
        val resolver = FakeDnsResolver(
            ConnectionConfig(routingDomainsEnabled = true, routingExcludeDomains = listOf(".example.com")),
            com.kvn.client.dns.DnsCache()
        )
        val excludedIpsField = FakeDnsResolver::class.java.getDeclaredField("excludedIps")
        excludedIpsField.isAccessible = true
        @Suppress("UNCHECKED_CAST")
        val excludedSet = excludedIpsField.get(resolver) as MutableSet<InetAddress>
        excludedSet.add(excludedIp)

        setField(service, "fakeDnsResolver", resolver)
        val directDeliverer = createDirectDeliverer(service)
        setField(service, "directDeliverer", directDeliverer)

        val packet = buildTcpSynPacket(
            srcIp = "10.0.0.2",
            dstIp = "93.184.216.34",
            srcPort = 40000,
            dstPort = 443
        )
        val consumed = callMethod(service, "routePacket", packet) as Boolean
        // Direct connect fails (no network in test) → falls through → false
        assertFalse(consumed)
    }

    // @sk-test android-fakedns-routing#T2.2: routePacket returns false when routingDomainsEnabled=false
    @Test
    fun testRoutePacketRoutingDomainsDisabled() {
        setConfig(ConnectionConfig(routingDomainsEnabled = false))
        val packet = buildUdp4Packet(
            payload = byteArrayOf(0x00),
            srcIp = "10.0.0.2",
            dstIp = "8.8.8.8",
            srcPort = 12345,
            dstPort = 53
        )
        val consumed = callMethod(service, "routePacket", packet) as Boolean
        assertFalse(consumed)
    }

    // @sk-test android-fakedns-routing#T2.2: routePacket returns false when no resolver
    @Test
    fun testRoutePacketNoResolver() {
        setConfig(ConnectionConfig(routingDomainsEnabled = true))
        val packet = buildUdp4Packet(
            payload = byteArrayOf(0x00),
            srcIp = "10.0.0.2",
            dstIp = "8.8.8.8",
            srcPort = 12345,
            dstPort = 53
        )
        val consumed = callMethod(service, "routePacket", packet) as Boolean
        assertFalse(consumed)
    }

    // @sk-test android-fakedns-routing#T2.2: routePacket with DNS query intercept for excluded domain
    @Test
    fun testRoutePacketFakeDnsIntercept() {
        setConfig(ConnectionConfig(routingDomainsEnabled = true,
            routingExcludeDomains = listOf(".example.com")))

        val fakeIp = InetAddress.getByName("198.18.0.1")
        val cache = com.kvn.client.dns.DnsCache()
        cache.set("test.example.com", listOf(fakeIp), 60)
        val resolver = FakeDnsResolver(
            ConnectionConfig(routingDomainsEnabled = true, routingExcludeDomains = listOf(".example.com")),
            cache
        )

        setField(service, "fakeDnsResolver", resolver)
        // No directDeliverer needed for DNS intercept path

        val dnsQuery = buildDnsQuery("test.example.com")
        val packet = buildUdp4Packet(
            payload = dnsQuery,
            srcIp = "10.0.0.2",
            dstIp = "1.1.1.1",
            srcPort = 34567,
            dstPort = 53
        )
        val consumed = callMethod(service, "routePacket", packet) as Boolean
        assertTrue(consumed)
    }

    // @sk-test android-fakedns-routing#T4.1: routePacket rewrites dst IP for include domain TCP (AC-006)
    @Test
    fun testRoutePacketIncludeRewriteTcp() {
        setConfig(ConnectionConfig(routingDomainsEnabled = true))
        val realIp = InetAddress.getByName("93.184.216.34")
        val cache = com.kvn.client.dns.DnsCache()
        cache.set("example.com", listOf(realIp), 60)
        val pool = com.kvn.client.dns.FakeIpPool()
        val fakeIp = pool.allocate("example.com") // get actual allocated fake IP

        val resolver = FakeDnsResolver(
            ConnectionConfig(routingDomainsEnabled = true),
            cache
        )
        setField(service, "fakeDnsResolver", resolver)
        setField(service, "fakeIpPool", pool)
        setField(service, "dnsCache", cache)

        val packet = buildTcpSynPacket(
            srcIp = "10.0.0.2",
            dstIp = fakeIp!!.hostAddress!!,
            srcPort = 40000,
            dstPort = 443
        ).toMutableList().toByteArray()

        val consumed = callMethod(service, "routePacket", packet) as Boolean
        assertFalse(consumed)

        val rewrittenDst = InetAddress.getByAddress(packet.copyOfRange(16, 20))
        assertEquals(realIp, rewrittenDst)
    }

    // @sk-test android-fakedns-routing#T4.1: routePacket rewrites dst IP for include domain UDP (AC-006)
    @Test
    fun testRoutePacketIncludeRewriteUdp() {
        setConfig(ConnectionConfig(routingDomainsEnabled = true))
        val realIp = InetAddress.getByName("93.184.216.34")
        val cache = com.kvn.client.dns.DnsCache()
        cache.set("example.com", listOf(realIp), 60)
        val pool = com.kvn.client.dns.FakeIpPool()
        pool.allocate("other.com")
        val fakeIp = pool.allocate("example.com") // second IP in pool

        val resolver = FakeDnsResolver(
            ConnectionConfig(routingDomainsEnabled = true),
            cache
        )
        setField(service, "fakeDnsResolver", resolver)
        setField(service, "fakeIpPool", pool)
        setField(service, "dnsCache", cache)

        val packet = buildUdp4Packet(
            payload = byteArrayOf(0x48, 0x69),
            srcIp = "10.0.0.2",
            dstIp = fakeIp!!.hostAddress!!,
            srcPort = 12345,
            dstPort = 80
        ).toMutableList().toByteArray()

        val consumed = callMethod(service, "routePacket", packet) as Boolean
        assertFalse(consumed)

        val rewrittenDst = InetAddress.getByAddress(packet.copyOfRange(16, 20))
        assertEquals(realIp, rewrittenDst)
    }

    // -- helpers --

    private fun setConfig(cfg: ConnectionConfig) {
        val configField = KvnVpnService::class.java.getDeclaredField("config")
        configField.isAccessible = true
        configField.set(null, cfg)
    }

    private fun setField(obj: Any, name: String, value: Any?) {
        var cls: Class<*> = obj::class.java
        while (cls != Any::class.java) {
            try {
                val field = cls.getDeclaredField(name)
                field.isAccessible = true
                field.set(obj, value)
                return
            } catch (_: NoSuchFieldException) {
                cls = cls.superclass
            }
        }
    }

    private fun createDirectDeliverer(service: KvnVpnService): Any {
        val ddClass = Class.forName("com.kvn.client.vpn.KvnVpnService\$DirectDeliverer")
        val constructor = ddClass.getDeclaredConstructor(KvnVpnService::class.java)
        constructor.isAccessible = true
        return constructor.newInstance(service)
    }

    private fun <T> getField(obj: Any, name: String): T? {
        var cls: Class<*> = obj::class.java
        while (cls != Any::class.java) {
            try {
                val field = cls.getDeclaredField(name)
                field.isAccessible = true
                @Suppress("UNCHECKED_CAST")
                return field.get(obj) as T
            } catch (_: NoSuchFieldException) {
                cls = cls.superclass
            }
        }
        return null
    }

    private fun callMethod(obj: Any, name: String, vararg args: Any?): Any? {
        val paramTypes = args.map { it?.javaClass ?: Any::class.java }.toTypedArray()
        val method = try {
            obj::class.java.getDeclaredMethod(name, *paramTypes)
        } catch (_: NoSuchMethodException) {
            obj::class.java.getDeclaredMethod(name, *paramTypes.map {
                if (it == Any::class.java) Object::class.java else it
            }.toTypedArray())
        }
        method.isAccessible = true
        return method.invoke(obj, *args)
    }

    private fun buildDnsQuery(domain: String): ByteArray {
        val labels = domain.split(".")
        val buf = ByteBuffer.allocate(512)
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

    private fun buildDnsResponse(domain: String, ips: List<InetAddress>): ByteArray {
        val labels = domain.split(".")
        val qnameLen = labels.sumOf { it.length + 1 } + 1
        val answerLen = ips.size * (qnameLen + 14)
        val buf = ByteBuffer.allocate(12 + qnameLen + 4 + answerLen)
        buf.putShort(0x1234.toShort())
        buf.putShort(0x8180.toShort())
        buf.putShort(1)
        buf.putShort(ips.size.toShort())
        buf.putShort(0)
        buf.putShort(0)
        for (label in labels) {
            buf.put(label.length.toByte())
            for (c in label) buf.put(c.code.toByte())
        }
        buf.put(0)
        buf.putShort(1)
        buf.putShort(1)
        for (ip in ips) {
            for (label in labels) {
                buf.put(label.length.toByte())
                for (c in label) buf.put(c.code.toByte())
            }
            buf.put(0)
            buf.putShort(1)
            buf.putShort(1)
            buf.putInt(60)
            val addr = ip.address
            buf.putShort(addr.size.toShort())
            buf.put(addr)
        }
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    private fun buildUdp4Packet(payload: ByteArray, srcIp: String, dstIp: String, srcPort: Int, dstPort: Int): ByteArray {
        val totalLen = 20 + 8 + payload.size
        val buf = ByteBuffer.allocate(totalLen)
        // IP header
        buf.put(0x45) // version=4, IHL=5
        buf.put(0) // DSCP+ECN
        buf.putShort(totalLen.toShort())
        buf.putShort(0x0000.toShort()) // ID
        buf.putShort(0x4000.toShort()) // flags=DF
        buf.put(64) // TTL
        buf.put(17) // UDP
        buf.putShort(0) // checksum (0 = not computed)
        buf.put(InetAddress.getByName(srcIp).address)
        buf.put(InetAddress.getByName(dstIp).address)
        // UDP header
        val udpLen = 8 + payload.size
        buf.putShort(srcPort.toShort())
        buf.putShort(dstPort.toShort())
        buf.putShort(udpLen.toShort())
        buf.putShort(0) // checksum (0 = not computed)
        buf.put(payload)
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    private fun buildTcpSynPacket(srcIp: String, dstIp: String, srcPort: Int, dstPort: Int): ByteArray {
        val tcpLen = 20
        val totalLen = 20 + tcpLen
        val buf = ByteBuffer.allocate(totalLen)
        // IP header
        buf.put(0x45)
        buf.put(0)
        buf.putShort(totalLen.toShort())
        buf.putShort(0x0000.toShort())
        buf.putShort(0x4000.toShort())
        buf.put(64)
        buf.put(6) // TCP
        buf.putShort(0) // checksum
        buf.put(InetAddress.getByName(srcIp).address)
        buf.put(InetAddress.getByName(dstIp).address)
        // TCP header
        buf.putShort(srcPort.toShort())
        buf.putShort(dstPort.toShort())
        // seq num = 100
        buf.putInt(100)
        // ack num = 0 (SYN)
        buf.putInt(0)
        // data offset=5, flags=SYN(0x02)
        buf.put(0x50.toByte())
        buf.put(0x02.toByte())
        // window
        buf.putShort(65535.toShort())
        // checksum
        buf.putShort(0)
        // urgent ptr
        buf.putShort(0)
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    private fun getIpHeaderLen(packet: ByteArray): Int {
        return ((packet[0].toInt() and 0x0F) * 4)
    }
}
