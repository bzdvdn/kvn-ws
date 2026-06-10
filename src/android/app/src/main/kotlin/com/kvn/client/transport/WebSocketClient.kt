package com.kvn.client.transport

import com.kvn.client.protocol.Frame
import com.kvn.client.protocol.encode
import com.kvn.client.protocol.toFrame
import okhttp3.*
import okio.Buffer
import okio.ByteString
import java.nio.ByteBuffer
import java.security.SecureRandom
import java.util.concurrent.atomic.AtomicBoolean

// @sk-task kvn-android#T2.2: WebSocket client with binary framing (AC-001)
// @sk-task kvn-android#T5.8: configurable OkHttpClient for TLS (AC-009)
// @sk-task kvn-android#T5.20: obfuscation padding wrapping matching server WSConn (AC-006)
class WebSocketClient(
    private val okHttpClient: OkHttpClient,
    private val url: String,
    private val onFrame: OnFrameCallback,
    private val onStateChange: OnStateChange,
    private val onFailure: ((Throwable) -> Unit)? = null,
    private val paddingEnabled: Boolean = false,
    private val paddingSize: Int = 512
) : TransportClient {
    private var webSocket: WebSocket? = null
    private val connected = AtomicBoolean(false)
    private val rng = SecureRandom()

    // @sk-task kvn-android#T2.2: connect WebSocket (AC-001)
    override fun connect() {
        onStateChange(ConnectionState.CONNECTING)
        val request = Request.Builder()
            .url(url)
            .build()

        webSocket = okHttpClient.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(ws: WebSocket, response: Response) {
                connected.set(true)
                onStateChange(ConnectionState.CONNECTED)
            }

            override fun onMessage(ws: WebSocket, bytes: ByteString) {
                val data = bytes.toByteArray()
                val frameData = if (paddingEnabled) unwrapPadding(data) else data
                val frame = frameData.toFrame()
                onFrame(frame)
            }

            override fun onClosing(ws: WebSocket, code: Int, reason: String) {
                connected.set(false)
                onStateChange(ConnectionState.DISCONNECTING)
            }

            override fun onClosed(ws: WebSocket, code: Int, reason: String) {
                connected.set(false)
                onStateChange(ConnectionState.DISCONNECTED)
            }

            override fun onFailure(ws: WebSocket, t: Throwable, response: Response?) {
                connected.set(false)
                onFailure?.invoke(t)
                onStateChange(ConnectionState.DISCONNECTED)
            }
        })
    }

    // @sk-task kvn-android#T2.2: send a binary frame (AC-001)
    // @sk-task kvn-android#T5.20: wrap with obfuscation padding when enabled (AC-006)
    override fun send(frame: Frame): Boolean {
        if (!connected.get()) return false
        val data = frame.encode()
        val wireData = if (paddingEnabled) wrapPadding(data) else data
        val b = Buffer()
        b.write(wireData)
        return webSocket?.send(b.readByteString()) ?: false
    }

    // @sk-task kvn-android#T5.20: match server WSConn WriteMessage padding (AC-006)
    private fun wrapPadding(data: ByteArray): ByteArray {
        val padSize = if (paddingSize <= 0) 512 else paddingSize
        val totalLen = 4 + data.size
        val padding = (padSize - totalLen % padSize) % padSize
        val buf = ByteBuffer.allocate(totalLen + padding)
        buf.putInt(data.size)
        buf.put(data)
        if (padding > 0) {
            val pad = ByteArray(padding)
            rng.nextBytes(pad)
            buf.put(pad)
        }
        return buf.array()
    }

    // @sk-task kvn-android#T5.20: match server WSConn ReadMessage padding unwrap (AC-006)
    private fun unwrapPadding(wireData: ByteArray): ByteArray {
        if (wireData.size < 4) {
            throw IllegalArgumentException("padding frame too short: ${wireData.size}")
        }
        val payloadLen = ByteBuffer.wrap(wireData, 0, 4).getInt()
        if (payloadLen < 0 || payloadLen > wireData.size - 4) {
            throw IllegalArgumentException("invalid padding payload length: $payloadLen (max ${wireData.size - 4})")
        }
        return wireData.copyOfRange(4, 4 + payloadLen)
    }

    // @sk-task kvn-android#T2.2: graceful disconnect (AC-006)
    override fun disconnect() {
        webSocket?.close(1000, "client disconnect")
        webSocket = null
        connected.set(false)
        onStateChange(ConnectionState.DISCONNECTED)
    }

    override fun isConnected(): Boolean = connected.get()
}
