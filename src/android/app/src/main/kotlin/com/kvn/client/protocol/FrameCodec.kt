package com.kvn.client.protocol

import java.nio.ByteBuffer

const val FRAME_HEADER_SIZE = 4
const val FRAME_MAX_PAYLOAD = 65535

// @sk-task kvn-android#T2.2: Kotlin binary frame encode (AC-001, AC-004)
fun Frame.encode(): ByteArray {
    require(payload.size <= FRAME_MAX_PAYLOAD) { "payload exceeds max frame size" }
    val buf = ByteBuffer.allocate(FRAME_HEADER_SIZE + payload.size)
    buf.put(type)
    buf.put(flags)
    buf.putShort(payload.size.toUShort().toShort())
    buf.put(payload)
    return buf.array()
}

// @sk-task kvn-android#T2.2: Kotlin binary frame decode (AC-001, AC-004)
fun ByteArray.toFrame(): Frame {
    require(this.size >= FRAME_HEADER_SIZE) { "frame too short" }
    val buf = java.nio.ByteBuffer.wrap(this)
    val type = buf.get()
    val flags = buf.get()
    val length = buf.getShort().toInt() and 0xFFFF
    require(length <= this.size - FRAME_HEADER_SIZE) { "frame length exceeds data" }
    val payload = ByteArray(length)
    buf.get(payload)
    return Frame(type, flags, payload)
}
