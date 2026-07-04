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
