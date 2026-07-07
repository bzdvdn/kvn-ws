package com.kvn.client.dns

import android.net.Network
import com.kvn.client.config.ConnectionConfig
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.util.LinkedHashSet

class FakeDnsResolver(
    private val config: ConnectionConfig,
    private val dnsCache: DnsCache,
    private val dnsServers: List<String> = emptyList(),
    private val fakeIpPool: FakeIpPool? = null,
    private val defaultNetwork: Network? = null
) {

    private val excludedIps = LinkedHashSet<InetAddress>()

    fun isExcluded(ip: InetAddress): Boolean = synchronized(excludedIps) { excludedIps.contains(ip) }

    @Synchronized
    fun clearExcluded() { excludedIps.clear() }

    @Synchronized
    fun excludedSize(): Int = excludedIps.size

    fun resolve(query: ByteArray): ByteArray? {
        if (!config.routingDomainsEnabled) return null
        val domain = DnsParser.extractQName(query) ?: return null

        val qtype = DnsParser.extractQType(query)
        if (qtype == 28) {
            LogBuffer.log("DNS", "AAAA query for $domain → empty response")
            return DnsParser.buildEmptyResponse(query)
        }

        for (suffix in config.routingExcludeDomains) {
            if (domain.endsWith(suffix) && dotBarrier(domain, suffix)) {
                val ips = resolveDomain(domain)
                if (ips.isNotEmpty()) {
                    synchronized(excludedIps) { excludedIps.addAll(ips) }
                    LogBuffer.log("DNS", "exclude match $domain → ${ips[0].hostAddress}")
                    return DnsParser.buildResponse(query, ips[0], 60)
                } else {
                    LogBuffer.log("DNS", "exclude match $domain but resolution failed → fwd")
                }
            }
        }

        for (suffix in config.routingIncludeDomains) {
            if (domain.endsWith(suffix) && dotBarrier(domain, suffix)) {
                val pool = fakeIpPool ?: return null
                val ips = resolveDomain(domain)
                if (ips.isNotEmpty()) {
                    dnsCache.set(domain, ips, 60)
                    val fakeIp = pool.allocate(domain) ?: run {
                        LogBuffer.log("DNS", "include match $domain but pool exhausted")
                        return null
                    }
                    LogBuffer.log("DNS", "include match $domain → fake ${fakeIp.hostAddress}")
                    return DnsParser.buildResponse(query, fakeIp, 60)
                } else {
                    LogBuffer.log("DNS", "include match $domain but resolution failed → fwd")
                }
            }
        }

        LogBuffer.log("DNS", "no match for $domain → forward")
        return null
    }

    private fun dotBarrier(domain: String, suffix: String): Boolean =
        suffix.startsWith(".") || domain.length == suffix.length || domain[domain.length - suffix.length - 1] == '.'

    private fun resolveDomain(domain: String): List<InetAddress> {
        val cached = dnsCache.get(domain)
        if (cached != null) return cached

        val net = defaultNetwork
        if (net == null || dnsServers.isEmpty()) {
            LogBuffer.log("DNS", "client-side resolution skipped for $domain (no network/bind)")
            return emptyList()
        }

        for (dnsServer in dnsServers) {
            try {
                val serverAddr = InetAddress.getByName(dnsServer)
                val socket = DatagramSocket()
                try {
                    net.bindSocket(socket)
                } catch (_: Exception) {
                    LogBuffer.log("DNS", "bindSocket failed for $dnsServer → skip")
                    socket.close()
                    continue
                }
                socket.soTimeout = 3000
                val query = DnsParser.buildQuery(domain)
                socket.send(DatagramPacket(query, query.size, serverAddr, 53))
                val raw = ByteArray(512)
                val pkt = DatagramPacket(raw, raw.size)
                socket.receive(pkt)
                socket.close()
                val ips = DnsParser.parseResponse(raw.copyOf(pkt.length))
                if (ips.isNotEmpty()) {
                    dnsCache.set(domain, ips, 60)
                    LogBuffer.log("DNS", "resolved $domain → ${ips[0].hostAddress}")
                    return ips
                }
            } catch (_: Exception) { }
        }
        LogBuffer.log("DNS", "all DNS servers failed for $domain")
        return emptyList()
    }
}
