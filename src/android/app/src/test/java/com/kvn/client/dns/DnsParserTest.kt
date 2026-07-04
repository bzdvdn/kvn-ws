package com.kvn.client.dns

import org.junit.Assert.*
import org.junit.Test
import java.net.InetAddress
import java.nio.ByteBuffer

// @sk-test android-dns-cache#T5.1: DnsParser unit tests (AC-001, AC-002)
class DnsParserTest {

    private fun buildQuery(domain: String): ByteArray {
        val labels = domain.split(".")
        val nameLen = labels.sumOf { it.length + 1 } + 1
        val buf = ByteBuffer.allocate(12 + nameLen + 4)
        buf.putShort(0x1234) // ID
        buf.putShort(0x0100) // flags: RD
        buf.putShort(1)      // QDCOUNT
        buf.putShort(0)      // ANCOUNT
        buf.putShort(0)      // NSCOUNT
        buf.putShort(0)      // ARCOUNT
        for (label in labels) {
            buf.put(label.length.toByte())
            for (c in label) buf.put(c.code.toByte())
        }
        buf.put(0)           // terminator
        buf.putShort(1)      // QTYPE A
        buf.putShort(1)      // QCLASS IN
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    private fun buildResponse(
        domain: String,
        ips: List<InetAddress>,
        ttl: Int = 60,
        compression: Boolean = false
    ): ByteArray {
        val labels = domain.split(".")
        val qnameLen = labels.sumOf { it.length + 1 } + 1
        val nameFieldLen = if (compression) 2 else qnameLen
        val answerLen = ips.size * (nameFieldLen + 14)
        val buf = ByteBuffer.allocate(12 + qnameLen + 4 + answerLen)
        buf.putShort(0x1234)                // ID
        buf.putShort(0x8180.toShort())      // flags: QR+RD+RA
        buf.putShort(1)                     // QDCOUNT
        buf.putShort(ips.size.toShort())    // ANCOUNT
        buf.putShort(0)                     // NSCOUNT
        buf.putShort(0)                     // ARCOUNT
        // Question
        for (label in labels) {
            buf.put(label.length.toByte())
            for (c in label) buf.put(c.code.toByte())
        }
        buf.put(0)
        buf.putShort(1) // QTYPE A
        buf.putShort(1) // QCLASS IN
        // Answers
        for (ip in ips) {
            if (compression) {
                buf.put(0xC0.toByte())
                buf.put(0x0C.toByte()) // pointer to name at offset 12
            } else {
                for (label in labels) {
                    buf.put(label.length.toByte())
                    for (c in label) buf.put(c.code.toByte())
                }
                buf.put(0)
            }
            buf.putShort(1)   // TYPE A
            buf.putShort(1)   // CLASS IN
            buf.putInt(ttl)
            val addr = ip.address
            buf.putShort(addr.size.toShort())
            buf.put(addr)
        }
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    @Test
    fun testExtractQNameSimple() {
        val query = buildQuery("example.com")
        assertEquals("example.com", DnsParser.extractQName(query))
    }

    @Test
    fun testExtractQNameSubdomain() {
        val query = buildQuery("www.example.com")
        assertEquals("www.example.com", DnsParser.extractQName(query))
    }

    @Test
    fun testExtractQNameReturnsNullForTruncated() {
        assertEquals(null, DnsParser.extractQName(byteArrayOf(0, 0)))
    }

    @Test
    fun testParseResponseReturnsIps() {
        val ip = InetAddress.getByName("93.184.216.34")
        val response = buildResponse("example.com", listOf(ip))
        val ips = DnsParser.parseResponse(response)
        assertEquals(1, ips.size)
        assertEquals(ip, ips[0])
    }

    @Test
    fun testParseResponseMultipleIps() {
        val ips = listOf(
            InetAddress.getByName("93.184.216.34"),
            InetAddress.getByName("93.184.216.35")
        )
        val response = buildResponse("example.com", ips)
        val result = DnsParser.parseResponse(response)
        assertEquals(2, result.size)
        assertEquals(ips, result)
    }

    @Test
    fun testParseResponseWithNameCompression() {
        val ip = InetAddress.getByName("93.184.216.34")
        val response = buildResponse("example.com", listOf(ip), compression = true)
        val ips = DnsParser.parseResponse(response)
        assertEquals(1, ips.size)
        assertEquals(ip, ips[0])
    }

    @Test
    fun testParseResponseReturnsEmptyForTruncated() {
        assertEquals(emptyList<InetAddress>(), DnsParser.parseResponse(byteArrayOf(0, 0)))
    }

    @Test
    fun testParseResponseReturnsEmptyForNoAnswers() {
        val buf = ByteBuffer.allocate(12)
        buf.putShort(0x1234)
        buf.putShort(0x8180.toShort())
        buf.putShort(0) // QDCOUNT=0
        buf.putShort(0) // ANCOUNT=0
        buf.putShort(0)
        buf.putShort(0)
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        val ips = DnsParser.parseResponse(out)
        assertEquals(emptyList<InetAddress>(), ips)
    }

    @Test
    fun testExtractQNameFromResponse() {
        val ip = InetAddress.getByName("93.184.216.34")
        val response = buildResponse("example.com", listOf(ip))
        assertEquals("example.com", DnsParser.extractQName(response))
    }
}
