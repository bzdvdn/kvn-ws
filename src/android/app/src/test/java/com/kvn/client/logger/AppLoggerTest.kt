package com.kvn.client.logger

import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

// @sk-test android-log-tag#T2.5: AppLogger thread safety + flow tests (AC-001, AC-013)
// @sk-test android-log-tag#T5.1: filter + edge case tests (AC-002, AC-003, AC-009)
class AppLoggerTest {

    @Before
    fun setUp() {
        AppLogger.clear()
    }

    @Test
    // @sk-test android-log-tag#T2.5: single-threaded write and overflow (AC-013)
    fun testWriteAndOverflow() {
        AppLogger.configure(maxSize = 100)
        for (i in 1..150) {
            AppLogger.i("TAG", "msg $i")
        }
        val snap = AppLogger.snapshot()
        assertEquals(100, snap.size)
        assertEquals("msg 51", snap.first().message)
        assertEquals("msg 150", snap.last().message)
    }

    @Test
    // @sk-test android-log-tag#T2.5: concurrent writes from multiple threads (AC-013)
    fun testConcurrentWrites() {
        AppLogger.configure(maxSize = 5000)
        val threads = (1..10).map { t ->
            Thread {
                repeat(500) { i ->
                    AppLogger.d("T$t", "log $i")
                }
            }
        }
        threads.forEach { it.start() }
        threads.forEach { it.join() }

        val snap = AppLogger.snapshot()
        assertEquals(5000, snap.size)
    }

    @Test
    // @sk-test android-log-tag#T2.5: SharedFlow delivers all entries (AC-001)
    fun testSharedFlowDeliversAllEntries() {
        AppLogger.configure(maxSize = 1000)
        val collected = mutableListOf<LogEntry>()
        val doneLatch = CountDownLatch(100)

        val job = Thread {
            runBlocking {
                AppLogger.logFlow.collect {
                    collected.add(it)
                    doneLatch.countDown()
                }
            }
        }.apply { isDaemon = true; start() }

        Thread.sleep(200)
        repeat(100) { i ->
            AppLogger.i("TAG", "msg $i")
        }

        assertTrue(doneLatch.await(5, TimeUnit.SECONDS))
        job.interrupt()
        assertEquals(100, collected.size)
    }

    @Test
    // @sk-test android-log-tag#T2.5: clear empties buffer (AC-009)
    fun testClear() {
        AppLogger.i("TAG", "hello")
        assertTrue(AppLogger.snapshot().isNotEmpty())
        AppLogger.clear()
        assertTrue(AppLogger.snapshot().isEmpty())
    }

    @Test
    // @sk-test android-log-tag#T2.5: all log levels create correct entries (AC-002)
    fun testLogLevels() {
        AppLogger.d("TAG", "debug")
        AppLogger.i("TAG", "info")
        AppLogger.w("TAG", "warn")
        AppLogger.e("TAG", "error")

        val snap = AppLogger.snapshot()
        assertEquals(4, snap.size)
        assertEquals(LogLevel.DEBUG, snap[0].level)
        assertEquals(LogLevel.INFO, snap[1].level)
        assertEquals(LogLevel.WARN, snap[2].level)
        assertEquals(LogLevel.ERROR, snap[3].level)
    }

    @Test
    // @sk-test android-log-tag#T5.1: filter snapshot by level (AC-002)
    fun testFilterByLevel() {
        AppLogger.i("DNS", "resolve")
        AppLogger.w("TUN", "timeout")
        AppLogger.i("DNS", "cached")
        AppLogger.e("TUN", "error")

        val warnOrHigher = AppLogger.snapshot().filter { it.level >= LogLevel.WARN }
        assertEquals(2, warnOrHigher.size)
        assertEquals(LogLevel.WARN, warnOrHigher[0].level)
        assertEquals(LogLevel.ERROR, warnOrHigher[1].level)
    }

    @Test
    // @sk-test android-log-tag#T5.1: filter snapshot by tag (AC-003)
    fun testFilterByTag() {
        AppLogger.i("DNS", "resolve")
        AppLogger.i("TUN", "data")
        AppLogger.i("DNS", "cached")

        val dnsOnly = AppLogger.snapshot().filter { it.tag == "DNS" }
        assertEquals(2, dnsOnly.size)
    }

    @Test
    // @sk-test android-log-tag#T5.1: error with throwable overload (AC-013)
    fun testErrorWithThrowable() {
        val cause = RuntimeException("connection lost")
        AppLogger.e("TUN", "failed", cause)

        val snap = AppLogger.snapshot()
        assertEquals(1, snap.size)
        assertEquals(LogLevel.ERROR, snap[0].level)
        assertTrue(snap[0].message.contains("connection lost"))
    }

    @Test
    // @sk-test android-log-tag#T5.1: empty buffer snapshot and clear (AC-009)
    fun testEmptyBuffer() {
        assertTrue(AppLogger.snapshot().isEmpty())
        AppLogger.clear()
        assertTrue(AppLogger.snapshot().isEmpty())
    }
}
