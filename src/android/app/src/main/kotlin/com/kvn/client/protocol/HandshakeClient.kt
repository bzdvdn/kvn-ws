package com.kvn.client.protocol

import java.io.ByteArrayOutputStream
import java.net.InetAddress

// @sk-task kvn-android#T2.2: Kotlin handshake encode/decode (AC-004)
object HandshakeCodec {

    fun encodeClientHello(hello: ClientHello): Frame {
        val tokenBytes = hello.token.toByteArray()
        val flags: Byte = (if (hello.ipv6) FLAG_IPV6 else 0).toByte()
        val mtuFlags: Byte = (if (hello.mtu > 0) FLAG_MTU else 0).toByte()
        val transportBytes = hello.transport.toByteArray()

        val baos = ByteArrayOutputStream()
        baos.write(hello.protoVersion.toInt())
        baos.write((flags.toInt() or mtuFlags.toInt()))
        baos.writeShort(tokenBytes.size)

        if (tokenBytes.isNotEmpty()) {
            baos.write(tokenBytes)
        }
        if (hello.mtu > 0) {
            baos.writeShort(hello.mtu)
        }
        if (transportBytes.isNotEmpty()) {
            baos.write(TRANSPORT_TAG.toInt())
            baos.write(transportBytes.size)
            baos.write(transportBytes)
        }

        return Frame(FrameTypes.FRAME_TYPE_HELLO, FrameFlags.FRAME_FLAG_NONE, baos.toByteArray())
    }

    fun decodeServerHello(frame: Frame): ServerHello {
        require(frame.type == FrameTypes.FRAME_TYPE_HELLO) { "unexpected frame type ${frame.type}" }
        val data = frame.payload
        check(data.size >= SESSION_ID_LEN + 1 + 1 + 1 + 4) { "server hello too short" }

        var pos = 0
        val sidBytes = data.copyOfRange(pos, pos + SESSION_ID_LEN)
        pos += SESSION_ID_LEN
        val sid = sidBytes.joinToString("") { "%02x".format(it) }

        val count = data[pos].toInt() and 0xFF
        pos++

        var assignedIp = ""
        var assignedIpv6 = ""

        for (i in 0 until count) {
            val family = data[pos]
            val ipLen = data[pos + 1].toInt() and 0xFF
            pos += 2
            val ipBytes = data.copyOfRange(pos, pos + ipLen)
            pos += ipLen
            when (family) {
                4.toByte() -> assignedIp = InetAddress.getByAddress(ipBytes).hostAddress ?: ""
                6.toByte() -> assignedIpv6 = InetAddress.getByAddress(ipBytes).hostAddress ?: ""
            }
        }

        var mtu = DEFAULT_MTU
        if (pos + 2 <= data.size) {
            mtu = ((data[pos].toInt() and 0xFF) shl 8) or (data[pos + 1].toInt() and 0xFF)
            pos += 2
        }

        var cryptoSalt = ByteArray(0)
        var gatewayIp = ""
        var transport = "tcp"

        while (pos < data.size) {
            if (pos + 2 > data.size) break
            val tag = data[pos]
            val length = data[pos + 1].toInt() and 0xFF
            pos += 2
            if (pos + length > data.size) break
            when (tag) {
                CRYPTO_TAG -> cryptoSalt = data.copyOfRange(pos, pos + length)
                GATEWAY_TAG -> if (length == 4) {
                    gatewayIp = InetAddress.getByAddress(data.copyOfRange(pos, pos + 4)).hostAddress ?: ""
                }
                TRANSPORT_TAG -> transport = String(data.copyOfRange(pos, pos + length))
            }
            pos += length
        }

        return ServerHello(sid, assignedIp, assignedIpv6, mtu, cryptoSalt, gatewayIp, transport)
    }

    fun encodeAuthError(reason: String): Frame {
        return Frame(FrameTypes.FRAME_TYPE_AUTH, FrameFlags.FRAME_FLAG_NONE, reason.toByteArray())
    }

    fun decodeAuthError(frame: Frame): AuthError {
        require(frame.type == FrameTypes.FRAME_TYPE_AUTH) { "unexpected frame type ${frame.type}" }
        return AuthError(String(frame.payload))
    }

    private fun ByteArrayOutputStream.writeShort(value: Int) {
        this.write((value shr 8) and 0xFF)
        this.write(value and 0xFF)
    }
}
