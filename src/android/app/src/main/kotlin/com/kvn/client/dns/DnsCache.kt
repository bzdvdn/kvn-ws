package com.kvn.client.dns

import java.net.InetAddress
import java.time.Instant

// @sk-task android-dns-cache#T2.1: TTL DNS cache with LRU eviction (AC-001, AC-004)
class DnsCache(
    private val maxEntries: Int = 1024
) {
    private data class Entry(
        val ips: List<InetAddress>,
        val deadline: Instant
    )

    private val entries = object : LinkedHashMap<String, Entry>(16, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, Entry>?): Boolean =
            size > maxEntries
    }

    @Synchronized
    fun get(domain: String): List<InetAddress>? {
        val entry = entries[domain] ?: return null
        if (Instant.now().isAfter(entry.deadline)) {
            entries.remove(domain)
            return null
        }
        return entry.ips
    }

    @Synchronized
    fun set(domain: String, ips: List<InetAddress>, ttlSeconds: Int) {
        val ttl = ttlSeconds.coerceIn(1, 86400)
        entries[domain] = Entry(ips, Instant.now().plusSeconds(ttl.toLong()))
    }

    @Synchronized
    fun clear() {
        entries.clear()
    }

    @Synchronized
    fun size(): Int = entries.size
}
