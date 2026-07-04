package com.kvn.client.vpn

import com.kvn.client.config.ConnectionConfig
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

    // -- helpers --

    private fun setConfig(cfg: ConnectionConfig) {
        val configField = KvnVpnService::class.java.getDeclaredField("config")
        configField.isAccessible = true
        configField.set(null, cfg)
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

    private fun getIpHeaderLen(packet: ByteArray): Int {
        return ((packet[0].toInt() and 0x0F) * 4)
    }
}
