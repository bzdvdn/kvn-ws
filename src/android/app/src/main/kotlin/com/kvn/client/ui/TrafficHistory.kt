package com.kvn.client.ui

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

// @sk-task android-per-server-override-ui#T3.3: ring buffer for traffic graph data (AC-008)
data class TrafficPoint(
    val timestamp: Long,
    val rxBytes: Long,
    val txBytes: Long
)

// @sk-task android-per-server-override-ui#T3.3: ring buffer for traffic graph data (AC-008)
class TrafficHistory(private val maxPoints: Int = 60) {
    private val _points = MutableStateFlow<List<TrafficPoint>>(emptyList())
    val points: StateFlow<List<TrafficPoint>> = _points.asStateFlow()

    private var prevRx = 0L
    private var prevTx = 0L

    fun record(rx: Long, tx: Long) {
        val now = System.currentTimeMillis()
        val current = _points.value.toMutableList()
        current.add(TrafficPoint(now, rx, tx))
        if (current.size > maxPoints) current.removeAt(0)
        _points.value = current
        prevRx = rx
        prevTx = tx
    }

    fun clear() {
        _points.value = emptyList()
        prevRx = 0L
        prevTx = 0L
    }
}
