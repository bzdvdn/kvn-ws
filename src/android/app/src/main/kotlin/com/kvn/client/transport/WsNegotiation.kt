package com.kvn.client.transport

// @sk-task stealth-surface-hardening#T5.2: no-pad/subprotocol alias negotiation (AC-012, AC-013)
object WsNegotiation {
    const val LEGACY_NO_PAD_HEADER = "X-KVN-NoPad"
    const val LEGACY_SUBPROTOCOL = "kvn-ws-mux"

    fun noPadHeaders(configured: List<String>): List<String> =
        configured.ifEmpty { listOf(LEGACY_NO_PAD_HEADER) }

    fun noPadConfirmed(get: (String) -> String?, configured: List<String>): Boolean =
        noPadHeaders(configured).any { get(it) == "1" }

    fun subprotocolHeader(enabled: Boolean, configured: List<String>): String? {
        if (!enabled) return null
        return configured.ifEmpty { listOf(LEGACY_SUBPROTOCOL) }.joinToString(", ")
    }
}
