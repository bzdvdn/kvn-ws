package com.kvn.client.transport

import com.kvn.client.protocol.Frame

typealias OnFrameCallback = (Frame) -> Unit
typealias OnStateChange = (ConnectionState) -> Unit

enum class ConnectionState {
    DISCONNECTED,
    CONNECTING,
    CONNECTED,
    RECONNECTING,
    DISCONNECTING
}

interface TransportClient {
    fun connect()
    fun send(frame: Frame): Boolean
    fun disconnect()
    fun isConnected(): Boolean
}
