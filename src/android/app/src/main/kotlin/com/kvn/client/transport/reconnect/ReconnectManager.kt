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
    private fun currentDelay(): Long {
        val base = baseDelayMs * 2.0.pow(retryCount.toDouble()).toLong()
        val capped = min(base, maxDelayMs)
        val jitter = Random.nextLong(0, capped / 4)
        return capped + jitter
    }
}
