package com.kvn.client.dns

import java.net.InetAddress
import java.nio.ByteBuffer

// @sk-task android-dns-cache#T2.3: DNS wire format parser for A/AAAA records (AC-001, AC-002)
object DnsParser {

    private const val TYPE_A: Short = 1
    private const val TYPE_AAAA: Short = 28

    fun parseResponse(raw: ByteArray): List<InetAddress> {
        if (raw.size < 12) return emptyList()
        val buf = ByteBuffer.wrap(raw)
        // Skip header ID (2), flags (2)
        buf.position(4)
        val qdcount = buf.short.toInt() and 0xFFFF
        val ancount = buf.short.toInt() and 0xFFFF
        // Skip question section
        var offset = 12
        for (i in 0 until qdcount) {
            val nameLen = skipName(raw, offset) ?: return emptyList()
            offset += nameLen + 4 // name + QTYPE(2) + QCLASS(2)
        }
        val ips = mutableListOf<InetAddress>()
        for (i in 0 until ancount) {
            if (offset + 2 > raw.size) break
            // NAME (may be compressed)
            if (raw[offset].toInt() and 0xC0 == 0xC0) {
                offset += 2
            } else {
                val nameLen = skipName(raw, offset) ?: break
                offset += nameLen
            }
            if (offset + 10 > raw.size) break
            val rtype = ((raw[offset].toInt() and 0xFF) shl 8) or (raw[offset + 1].toInt() and 0xFF)
            offset += 8 // TYPE(2) + CLASS(2) + TTL(4)
            val rdlength = ((raw[offset].toInt() and 0xFF) shl 8) or (raw[offset + 1].toInt() and 0xFF)
            offset += 2
            if (offset + rdlength > raw.size) break
            if (rtype == TYPE_A.toInt() && rdlength == 4) {
                val ip = InetAddress.getByAddress(raw.copyOfRange(offset, offset + 4))
                ips.add(ip)
            } else if (rtype == TYPE_AAAA.toInt() && rdlength == 16) {
                val ip = InetAddress.getByAddress(raw.copyOfRange(offset, offset + 16))
                ips.add(ip)
            }
            offset += rdlength
        }
        return ips
    }

    fun extractQName(raw: ByteArray): String? {
        if (raw.size < 12) return null
        val sb = StringBuilder()
        var pos = 12
        while (pos < raw.size) {
            val len = raw[pos].toInt() and 0xFF
            if (len == 0) return sb.toString()
            if (len and 0xC0 == 0xC0) return sb.toString() // compressed pointer — stop
            if (pos + 1 + len > raw.size) return null
            if (sb.isNotEmpty()) sb.append('.')
            for (i in 0 until len) {
                sb.append((raw[pos + 1 + i].toInt() and 0xFF).toChar())
            }
            pos += 1 + len
        }
        return null
    }

    // @sk-task android-fakedns-routing#T2.1: build DNS query for a given domain (RQ-012)
    fun buildQuery(domain: String): ByteArray {
        val labels = domain.split(".")
        val qnameLen = labels.sumOf { it.length + 1 } + 1
        val buf = ByteBuffer.allocate(12 + qnameLen + 4)
        buf.putShort(0x1234.toShort()) // ID
        buf.putShort(0x0100.toShort()) // RD=1
        buf.putShort(1)                // QDCOUNT
        buf.putShort(0)                // ANCOUNT
        buf.putShort(0)                // NSCOUNT
        buf.putShort(0)                // ARCOUNT
        for (label in labels) {
            buf.put(label.length.toByte())
            for (c in label) buf.put(c.code.toByte())
        }
        buf.put(0)                     // terminator
        buf.putShort(1)                // QTYPE A
        buf.putShort(1)                // QCLASS IN
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    // @sk-task android-fakedns-routing#T2.1: build DNS response from query + answer IP (RQ-012)
    fun buildResponse(query: ByteArray, answerIp: InetAddress, ttlSeconds: Int = 60): ByteArray? {
        if (query.size < 12) return null
        val domain = extractQName(query) ?: return null
        val labels = domain.split(".")
        val qnameLen = labels.sumOf { it.length + 1 } + 1
        val qEnd = 12 + qnameLen + 4
        if (qEnd > query.size) return null
        val addr = answerIp.address
        val buf = ByteBuffer.allocate(qEnd + 2 + 14)
        // Header
        buf.putShort((((query[0].toInt() and 0xFF) shl 8) or (query[1].toInt() and 0xFF)).toShort()) // ID from query
        val rd = query[2].toInt() and 0x01
        buf.putShort((0x8080 or rd).toShort()) // QR+RA+RD
        buf.putShort(1) // QDCOUNT
        buf.putShort(1) // ANCOUNT
        buf.putShort(0) // NSCOUNT
        buf.putShort(0) // ARCOUNT
        // Question section (copy from query)
        buf.put(query.copyOfRange(12, qEnd))
        // Answer: NAME (compression pointer to question at offset 12)
        buf.put(0xC0.toByte())
        buf.put(0x0C.toByte())
        buf.putShort(1)     // TYPE A
        buf.putShort(1)     // CLASS IN
        buf.putInt(ttlSeconds)
        buf.putShort(addr.size.toShort())
        buf.put(addr)
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    // @sk-task android-fakedns-routing#T3.2: extract QTYPE from DNS query (AC-005, AC-007)
    fun extractQType(raw: ByteArray): Int? {
        if (raw.size < 14) return null
        var pos = 12
        while (pos < raw.size) {
            val len = raw[pos].toInt() and 0xFF
            if (len == 0) {
                pos += 1
                break
            }
            if (len and 0xC0 == 0xC0) {
                pos += 2
                break
            }
            if (pos + 1 + len > raw.size) return null
            pos += 1 + len
        }
        if (pos + 4 > raw.size) return null
        return ((raw[pos].toInt() and 0xFF) shl 8) or (raw[pos + 1].toInt() and 0xFF)
    }

    // @sk-task android-fakedns-routing#T3.2: build empty DNS response (no answers) for unsupported QTYPE (AC-005, AC-007)
    fun buildEmptyResponse(query: ByteArray): ByteArray? {
        if (query.size < 12) return null
        val qnameLen = getQnameLength(query) ?: return null
        val qEnd = 12 + qnameLen + 4
        if (qEnd > query.size) return null
        val buf = ByteBuffer.allocate(qEnd)
        buf.putShort(((query[0].toInt() and 0xFF) shl 8 or (query[1].toInt() and 0xFF)).toShort()) // ID
        val rd = query[2].toInt() and 0x01
        buf.putShort((0x8080 or rd).toShort()) // QR+RA+RD
        buf.putShort(1)  // QDCOUNT
        buf.putShort(0)  // ANCOUNT
        buf.putShort(0)  // NSCOUNT
        buf.putShort(0)  // ARCOUNT
        buf.put(query.copyOfRange(12, qEnd)) // question section
        val out = ByteArray(buf.position())
        buf.flip()
        buf.get(out)
        return out
    }

    private fun getQnameLength(raw: ByteArray): Int? {
        var pos = 12
        val start = pos
        while (pos < raw.size) {
            val len = raw[pos].toInt() and 0xFF
            if (len == 0) return pos - start + 1
            if (len and 0xC0 == 0xC0) return pos - start + 2
            if (pos + 1 + len > raw.size) return null
            pos += 1 + len
        }
        return null
    }

    private fun skipName(raw: ByteArray, start: Int): Int? {
        var pos = start
        while (pos < raw.size) {
            val len = raw[pos].toInt() and 0xFF
            if (len == 0) return pos - start + 1
            if (len and 0xC0 == 0xC0) return pos - start + 2
            if (pos + 1 + len > raw.size) return null
            pos += 1 + len
        }
        return null
    }
}
