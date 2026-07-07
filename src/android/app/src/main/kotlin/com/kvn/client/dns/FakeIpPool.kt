package com.kvn.client.dns

import java.net.InetAddress
import java.util.BitSet

// @sk-task android-fakedns-routing#T1.2: fake IP pool for domain routing (DEC-004)
class FakeIpPool(
    private val poolSize: Int = 32768
) {
    private val baseAddress = 0xC6120000.toInt() // 198.18.0.0
    private val bitmap = BitSet(poolSize)
    private val forwardMap = HashMap<Long, String>()

    @Synchronized
    fun allocate(domain: String): InetAddress? {
        val index = bitmap.nextClearBit(0)
        if (index >= poolSize) return null
        bitmap.set(index)
        val ipLong = (baseAddress.toLong() and 0xFFFFFFFFL) + index
        forwardMap[ipLong] = domain
        return intToInetAddress(ipLong.toInt())
    }

    @Synchronized
    fun release(ip: InetAddress) {
        val ipInt = inetAddressToInt(ip)
        val ipLong = ipInt.toLong() and 0xFFFFFFFFL
        val index = ipInt - baseAddress
        if (index in 0 until poolSize) {
            bitmap.clear(index)
            forwardMap.remove(ipLong)
        }
    }

    @Synchronized
    fun lookup(ip: InetAddress): String? {
        val ipLong = inetAddressToInt(ip).toLong() and 0xFFFFFFFFL
        return forwardMap[ipLong]
    }

    @Synchronized
    fun clear() {
        bitmap.clear()
        forwardMap.clear()
    }

    @Synchronized
    fun allocatedCount(): Int = bitmap.cardinality()

    private fun intToInetAddress(value: Int): InetAddress =
        InetAddress.getByAddress(
            byteArrayOf(
                (value shr 24).toByte(),
                (value shr 16).toByte(),
                (value shr 8).toByte(),
                value.toByte()
            )
        )

    private fun inetAddressToInt(ip: InetAddress): Int {
        val raw = ip.address
        return ((raw[0].toInt() and 0xFF) shl 24) or
                ((raw[1].toInt() and 0xFF) shl 16) or
                ((raw[2].toInt() and 0xFF) shl 8) or
                (raw[3].toInt() and 0xFF)
    }
}
