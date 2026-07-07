package com.kvn.client.logger

import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import java.time.Instant

// @sk-task android-log-tag#T1.2: AppLogger ring buffer + SharedFlow (AC-001, AC-013)
object AppLogger {
    private var maxSize = 2000
    private val entries = ArrayDeque<LogEntry>(maxSize)
    private val _logFlow = MutableSharedFlow<LogEntry>(replay = 0, extraBufferCapacity = 2048)

    val logFlow: SharedFlow<LogEntry> = _logFlow

    fun configure(maxSize: Int) {
        this.maxSize = maxSize
    }

    @Synchronized
    fun d(tag: String, msg: String) {
        emit(LogLevel.DEBUG, tag, msg)
    }

    @Synchronized
    fun i(tag: String, msg: String) {
        emit(LogLevel.INFO, tag, msg)
    }

    @Synchronized
    fun w(tag: String, msg: String) {
        emit(LogLevel.WARN, tag, msg)
    }

    @Synchronized
    fun e(tag: String, msg: String, throwable: Throwable? = null) {
        val full = if (throwable != null) "$msg: ${throwable.message}" else msg
        emit(LogLevel.ERROR, tag, full)
    }

    private fun emit(level: LogLevel, tag: String, msg: String) {
        val entry = LogEntry(
            level = level,
            tag = tag,
            message = msg,
            timestamp = Instant.now(),
            thread = Thread.currentThread().name
        )
        if (entries.size >= maxSize) entries.removeFirst()
        entries.addLast(entry)
        _logFlow.tryEmit(entry)
    }

    @Synchronized
    fun snapshot(): List<LogEntry> = entries.toList()

    @Synchronized
    fun clear() {
        entries.clear()
    }
}
