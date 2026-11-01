package com.kvn.client.vpn

import org.json.JSONObject
import java.util.concurrent.atomic.AtomicBoolean

/** Result of applying one ConnectivityManager callback. */
data class NetworkSelection(
    val activeHandle: Long?,
    val changed: Boolean,
    val lostAll: Boolean,
)

/**
 * Tracks the single default network. The first callback of each registration is
 * a baseline and is not treated as a change, which prevents restart loops when
 * callbacks are re-registered after a restart.
 */
class NetworkTracker {
    private var active: Long? = null
    private var hasBaseline = false

    @Synchronized
    fun beginRegistration() {
        hasBaseline = false
    }

    @Synchronized
    fun onAvailable(handle: Long): NetworkSelection {
        if (!hasBaseline) {
            active = handle
            hasBaseline = true
            return NetworkSelection(active, changed = false, lostAll = false)
        }
        if (handle == active) {
            return NetworkSelection(active, changed = false, lostAll = false)
        }
        active = handle
        return NetworkSelection(active, changed = true, lostAll = false)
    }

    @Synchronized
    fun onLost(handle: Long): NetworkSelection {
        if (handle != active) {
            return NetworkSelection(active, changed = false, lostAll = false)
        }
        active = null
        return NetworkSelection(null, changed = true, lostAll = true)
    }

    @Synchronized
    fun reset() {
        active = null
        hasBaseline = false
    }
}

/** Decides whether an unlock event should refresh the session. */
object SessionRecovery {
    fun shouldRestartOnUnlock(healthy: Boolean): Boolean = !healthy
}

/** Single-flight gate for session restarts. */
class RestartGate {
    private val entered = AtomicBoolean(false)

    fun tryEnter(): Boolean = entered.compareAndSet(false, true)

    fun exit() {
        entered.set(false)
    }

    val isEntered: Boolean
        get() = entered.get()
}

/** Extracts VpnService DNS servers from the v2 client JSON. */
object VpnConfigParser {
    private const val DEFAULT_DNS = "1.1.1.1"

    fun dnsServers(configJson: String): List<String> {
        val upstreams = runCatching { upstreamHosts(configJson) }.getOrDefault(emptyList())
        return upstreams.ifEmpty { listOf(DEFAULT_DNS) }
    }

    fun upstreamHosts(configJson: String): List<String> {
        val dns = JSONObject(configJson).optJSONObject("dns_proxy") ?: return emptyList()
        val upstreams = dns.optJSONArray("upstreams") ?: return emptyList()
        val hosts = LinkedHashSet<String>()
        for (i in 0 until upstreams.length()) {
            val host = hostOf(upstreams.optString(i, ""))
            if (host.isNotEmpty()) {
                hosts.add(host)
            }
        }
        return hosts.toList()
    }

    fun hostOf(raw: String): String {
        val value = raw.trim()
        if (value.isEmpty()) return ""
        if (value.startsWith("[")) {
            val end = value.indexOf(']')
            return if (end > 1) value.substring(1, end) else ""
        }
        return if (value.count { it == ':' } == 1) value.substringBefore(':') else value
    }
}
