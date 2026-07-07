package com.kvn.client.logger

import java.time.Instant

// @sk-task android-log-tag#T1.1: LogLevel enum (AC-002)
enum class LogLevel {
    DEBUG,
    INFO,
    WARN,
    ERROR
}

// @sk-task android-log-tag#T1.1: LogEntry data model (AC-001, AC-013)
data class LogEntry(
    val level: LogLevel,
    val tag: String,
    val message: String,
    val timestamp: Instant,
    val thread: String
)
