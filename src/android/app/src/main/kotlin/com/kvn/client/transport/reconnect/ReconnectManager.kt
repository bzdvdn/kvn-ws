package com.kvn.client.transport.reconnect

import com.kvn.client.config.ConnectionConfig
import com.kvn.client.transport.ConnectionState
import kotlinx.coroutines.*
import kotlin.math.min
import kotlin.math.pow
import kotlin.random.Random

const val MAX_RETRIES = 10

// @sk-task kvn-android#T5.18: configurable exponential backoff (AC-005, RQ-010)
// @sk-task kvn-android#T3.1+BUGFIX: onRetriesExhausted callback to avoid safeStop loop (AC-005)
class ReconnectManager(
    private val scope: CoroutineScope,
    private val config: ConnectionConfig,
    private val onReconnect: suspend () -> Unit,
    private val onStateChange: (ConnectionState) -> Unit,
    private val onRetriesExhausted: () -> Unit = {}
) {
    private var retryCount = 0
    private var job: Job? = null

    private val baseDelayMs = (config.minBackoffSec * 1000L).coerceAtLeast(100)
    private val maxDelayMs = (config.maxBackoffSec * 1000L).coerceAtLeast(baseDelayMs)
    private val jitterPct = config.reconnectJitterPct.coerceIn(0, 50)

    // @sk-task kvn-android#T3.1: start reconnecting (AC-005)
    fun start() {
        if (job?.isActive == true) return
        job = scope.launch {
            while (isActive && retryCount < MAX_RETRIES) {
                onStateChange(ConnectionState.RECONNECTING)
                delay(currentDelay())
                retryCount++
                try {
                    onReconnect()
                    retryCount = 0
                    return@launch
                } catch (_: Exception) {
                    // will retry
                }
            }
            onRetriesExhausted()
        }
    }

    // @sk-task kvn-android#T3.1: reset retry state (AC-005)
    fun reset() {
        retryCount = 0
        job?.cancel()
        job = null
    }

    // @sk-task kvn-android#T3.1: stop reconnecting (AC-005)
    fun stop() {
        job?.cancel()
        job = null
        retryCount = 0
    }

    // @sk-task kvn-android#T5.18: exponential backoff with jitter (AC-005)
    // @sk-task stealth-surface-hardening#T5.3: configurable bounded reconnect jitter (AC-014)
    private fun currentDelay(): Long = delayForAttempt(retryCount)

    internal fun delayForAttempt(attempt: Int): Long {
        val base = baseDelayMs * 2.0.pow(attempt.toDouble()).toLong()
        val capped = min(base, maxDelayMs)
        if (jitterPct == 0) {
            // Legacy behavior: keep the original positive-only jitter.
            val legacy = Random.nextLong(0, (capped / 4).coerceAtLeast(1))
            return capped + legacy
        }
        val delta = capped * jitterPct / 100
        if (delta <= 0) return capped
        return (capped - delta + Random.nextLong(0, 2 * delta + 1)).coerceAtLeast(0)
    }
}
