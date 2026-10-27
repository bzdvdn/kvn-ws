package com.kvn.client.transport

import okhttp3.OkHttpClient
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

// @sk-test fallback-auth-stealth#T4.3: Android sends Bearer and falls back to legacy path (AC-009)
class WebSocketFallbackTest {
    @Test
    fun testStealthBearerAndLegacyFallback() {
        val connected = CountDownLatch(1)
        val authSeen = java.util.concurrent.atomic.AtomicReference<String>()
        val server = MockWebServer()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when (request.path) {
                    "/stealth" -> {
                        authSeen.set(request.getHeader("Authorization"))
                        MockResponse().setResponseCode(200).setBody("<html>fallback</html>")
                    }
                    "/legacy" -> MockResponse().withWebSocketUpgrade(object : WebSocketListener() {
                        override fun onOpen(webSocket: WebSocket, response: Response) {}
                    })
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        server.start()
        try {
            val client = WebSocketClient(
                okHttpClient = OkHttpClient(),
                url = server.url("/stealth").toString(),
                onFrame = {},
                onStateChange = { if (it == com.kvn.client.transport.ConnectionState.CONNECTED) connected.countDown() },
                authorization = "Bearer secret",
                fallbackUrl = server.url("/legacy").toString()
            )
            client.connect()
            assertTrue("client did not connect via legacy fallback", connected.await(5, TimeUnit.SECONDS))
            assertEquals("Bearer secret", authSeen.get())
            client.disconnect()
        } finally {
            server.shutdown()
        }
    }
}
