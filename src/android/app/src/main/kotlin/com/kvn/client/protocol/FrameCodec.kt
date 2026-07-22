package com.kvn.client.protocol

const val FRAME_HEADER_SIZE = 4
const val FRAME_MAX_PAYLOAD = 65535

// @sk-task kvn-android#T2.2: Kotlin binary frame encode (AC-001, AC-004)
// @sk-task android-latency-power-fix#T2.2: zero-copy encode without ByteBuffer (AC-004)
fun Frame.encode(): ByteArray {
    require(payload.size <= FRAME_MAX_PAYLOAD) { "payload exceeds max frame size" }
    val result = ByteArray(FRAME_HEADER_SIZE + payload.size)
    result[0] = type
    result[1] = flags
    result[2] = ((payload.size shr 8) and 0xFF).toByte()
    result[3] = (payload.size and 0xFF).toByte()
    System.arraycopy(payload, 0, result, FRAME_HEADER_SIZE, payload.size)
    return result
}

// @sk-task kvn-android#T2.2: Kotlin binary frame decode (AC-001, AC-004)
// @sk-task android-latency-power-fix#T2.2: zero-copy decode without ByteBuffer.wrap (AC-004)
fun ByteArray.toFrame(): Frame {
    require(this.size >= FRAME_HEADER_SIZE) { "frame too short" }
    val type = this[0]
    val flags = this[1]
    val length = ((this[2].toInt() and 0xFF) shl 8) or (this[3].toInt() and 0xFF)
    require(length <= this.size - FRAME_HEADER_SIZE) { "frame length exceeds data" }
    val payload = ByteArray(length)
    System.arraycopy(this, FRAME_HEADER_SIZE, payload, 0, length)
    return Frame(type, flags, payload)
}
