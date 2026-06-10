// Code generated from protocol/handshake.yaml. DO NOT EDIT.
package com.kvn.client.protocol

// @sk-task kvn-android#T1.1: protocol handshake constants (AC-004)
const val FLAG_IPV6: Byte = 0x01.toByte()
const val FLAG_MTU: Byte = 0x02.toByte()
const val DEFAULT_MTU: Int = 1500
const val CRYPTO_TAG: Byte = 0x09.toByte()
const val GATEWAY_TAG: Byte = 0x0A.toByte()
const val TRANSPORT_TAG: Byte = 0x0B.toByte()
const val PROTO_VERSION: Byte = 0x02.toByte()
const val SESSION_ID_LEN: Int = 16

// @sk-task kvn-android#T1.1: Kotlin ClientHello data class (AC-004)
data class ClientHello(
	val protoVersion: Byte,
	val ipv6: Boolean,
	val token: String,
	val mtu: Int,
	val transport: String
)

// @sk-task kvn-android#T1.1: Kotlin ServerHello data class (AC-004)
data class ServerHello(
	val sessionId: String,
	val assignedIp: String,
	val assignedIpv6: String,
	val mtu: Int,
	val cryptoSalt: ByteArray,
	val gatewayIp: String,
	val transport: String
)

// @sk-task kvn-android#T1.1: Kotlin AuthError data class (AC-004)
data class AuthError(
	val reason: String
)

