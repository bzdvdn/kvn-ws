// Code generated from protocol/frames.yaml. DO NOT EDIT.
package com.kvn.client.protocol

// @sk-task kvn-android#T1.1: Kotlin frame type constants (AC-004)
object FrameTypes {
	const val FRAME_TYPE_AUTH: Byte = 3.toByte()
	const val FRAME_TYPE_CLOSE: Byte = 4.toByte()
	const val FRAME_TYPE_DNS: Byte = 6.toByte()
	const val FRAME_TYPE_DATA: Byte = 1.toByte()
	const val FRAME_TYPE_HELLO: Byte = 2.toByte()
	const val FRAME_TYPE_PROXY: Byte = 5.toByte()
}

// @sk-task kvn-android#T1.1: Kotlin frame flag constants (AC-004)
object FrameFlags {
	const val FRAME_FLAG_NONE: Byte = 0.toByte()
	const val FRAME_FLAG_SEGMENT: Byte = 64.toByte()
	const val FRAME_FLAG_SEGMENT_LAST: Byte = 128.toByte()
}

// @sk-task kvn-android#T1.1: Kotlin Frame data class (AC-004)
data class Frame(
	val type: Byte,
	val flags: Byte,
	val payload: ByteArray
) {
	val length: Int get() = payload.size
}