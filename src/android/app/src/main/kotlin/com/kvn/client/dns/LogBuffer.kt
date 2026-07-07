package com.kvn.client.dns

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

object LogBuffer {
    private const val MAX_ENTRIES = 500
    private val entries = ArrayDeque<String>(MAX_ENTRIES)
    private val fmt = DateTimeFormatter.ofPattern("HH:mm:ss.SSS").withZone(ZoneId.systemDefault())

    @Synchronized
    fun log(tag: String, msg: String) {
        val ts = fmt.format(Instant.now())
        if (entries.size >= MAX_ENTRIES) entries.removeFirst()
        entries.addLast("[$ts][$tag] $msg")
    }

    @Synchronized
    fun dump(): List<String> = entries.toList()

    @Synchronized
    fun clear() { entries.clear() }
}
