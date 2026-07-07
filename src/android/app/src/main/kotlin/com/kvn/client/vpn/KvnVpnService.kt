package com.kvn.client.vpn

import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.LinkProperties
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelFileDescriptor
import com.kvn.client.config.ConnectionConfig
import com.kvn.client.crypto.AesGcmCipher
import com.kvn.client.dns.DnsCache
import com.kvn.client.dns.DnsParser
import com.kvn.client.dns.FakeDnsResolver
import com.kvn.client.dns.FakeIpPool
import com.kvn.client.logger.AppLogger
import com.kvn.client.protocol.*
import com.kvn.client.transport.ConnectionState
import com.kvn.client.transport.OnStateChange
import com.kvn.client.transport.TransportClient
import com.kvn.client.transport.WebSocketClient
import com.kvn.client.transport.reconnect.ReconnectManager
import com.kvn.client.ui.MainActivity
import kotlinx.coroutines.*
import java.io.InputStream
import java.io.OutputStream
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.Inet4Address
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket
import java.net.SocketAddress
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicLong
import javax.crypto.SecretKey
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager
import javax.net.SocketFactory
import kotlin.math.max
import okhttp3.OkHttpClient
import java.security.SecureRandom
import java.security.cert.X509Certificate

    // @sk-task kvn-android#T2.1: VpnService + TUN read/write (AC-006)
    // @sk-task kvn-android#T5.16: kill switch blocks stop on disconnect (AC-010)
// @sk-task android-log-tag#T3.1: migrated LogBuffer to AppLogger (AC-012)
class KvnVpnService : VpnService() {

    private val serviceScope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private var tunFd: ParcelFileDescriptor? = null
    private var tunInput: InputStream? = null
    private var tunOutput: OutputStream? = null
    private var transportClient: TransportClient? = null
    private var onStateChange: ((ConnectionState) -> Unit)? = null
    private var reconnectManager: ReconnectManager? = null
    private var cipher: AesGcmCipher? = null
    private var cryptoEnabled = false
    private var serverSessionId: String = ""

    // @sk-task kvn-android#T3.2: traffic counters (AC-002)
    private val rxBytes = AtomicLong(0)
    private val txBytes = AtomicLong(0)
    var onTrafficUpdate: ((rx: Long, tx: Long) -> Unit)? = null

    private var notificationUpdateJob: kotlinx.coroutines.Job? = null
    private var networkCallback: ConnectivityManager.NetworkCallback? = null

    // @sk-task android-fakedns-routing#T2.1: fakeDNS resolver for domain routing (DEC-001)
    // @sk-task android-fakedns-routing#T3.1: fakeIpPool for include IP rewrite (AC-002)
    private var defaultNetwork: Network? = null
    private var fakeDnsResolver: FakeDnsResolver? = null
    private var dnsCache: DnsCache = DnsCache()
    private var directDeliverer: DirectDeliverer? = null
    private var fakeIpPool: FakeIpPool? = null

    companion object {
        private const val NOTIFICATION_ID = 1
        private const val CHANNEL_ID = "kvn_vpn"
        private const val ACTION_STOP = "com.kvn.client.action.STOP_VPN"
        private const val HANDSHAKE_TIMEOUT_MS = 30_000L

        private var config = ConnectionConfig()
        private var stateCallback: ((ConnectionState) -> Unit)? = null
        private var trafficCallback: ((rx: Long, tx: Long) -> Unit)? = null
        private var errorCallback: ((String) -> Unit)? = null
        private var killed = false // true when user explicitly disconnects
        private var tunFdRef: ParcelFileDescriptor? = null
        // @sk-task android-per-app-dns#T1.3: app-level settings (AC-003, AC-004, AC-005)
        private var appIncludeList: List<String> = emptyList()
        private var appExcludeList: List<String> = emptyList()
        private var dnsServers: List<String> = listOf("1.1.1.1", "8.8.8.8")

        // @sk-task kvn-android#T2.1: start VPN service (AC-001, AC-006)
        // @sk-task android-per-app-dns#T1.3: pass app-level settings to VpnService (AC-003, AC-004, AC-005)
        fun start(
            context: Context,
            cfg: ConnectionConfig,
            appIncludeList: List<String> = emptyList(),
            appExcludeList: List<String> = emptyList(),
            dnsServers: List<String> = listOf("1.1.1.1", "8.8.8.8"),
            onStateChange: ((ConnectionState) -> Unit)? = null,
            onTrafficUpdate: ((rx: Long, tx: Long) -> Unit)? = null,
            onError: ((String) -> Unit)? = null
        ) {
            config = cfg
            this.appIncludeList = appIncludeList
            this.appExcludeList = appExcludeList
            this.dnsServers = dnsServers
            stateCallback = onStateChange
            trafficCallback = onTrafficUpdate
            errorCallback = onError
            val intent = Intent(context, KvnVpnService::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(intent)
            } else {
                context.startService(intent)
            }
        }

        // @sk-task kvn-android#T2.1: stop VPN service (AC-006)
        // @sk-task kvn-android#T5.16: mark killed for kill switch (AC-010)
        fun stop(context: Context) {
            killed = true
            tunFdRef?.close()
            tunFdRef = null
            context.stopService(Intent(context, KvnVpnService::class.java))
        }
    }

    // @sk-task kvn-android#T5.8: build OkHttp client with TLS config (AC-009)
    // @sk-task kvn-android#T5.19: SocketFactory that protects sockets from VPN routing loop (AC-006)
    private inner class SmartSocketFactory : SocketFactory() {
        private val delegate = SocketFactory.getDefault()

        override fun createSocket(): Socket {
            val raw = delegate.createSocket()
            // Always protect — safe when VPN is down, critical during reconnect races
            this@KvnVpnService.protect(raw)
            return object : Socket() {
                override fun connect(endpoint: SocketAddress, timeout: Int) {
                    raw.connect(endpoint, timeout)
                }
                override fun bind(bindpoint: SocketAddress) { raw.bind(bindpoint) }
                override fun close() { raw.close() }
                override fun connect(endpoint: SocketAddress) { raw.connect(endpoint) }
                override fun getChannel() = raw.channel
                override fun getInetAddress() = raw.inetAddress
                override fun getInputStream() = raw.getInputStream()
                override fun getLocalAddress() = raw.localAddress
                override fun getLocalPort() = raw.localPort
                override fun getLocalSocketAddress() = raw.localSocketAddress
                override fun getOutputStream() = raw.getOutputStream()
                override fun getPort() = raw.port
                override fun getReceiveBufferSize() = raw.receiveBufferSize
                override fun getRemoteSocketAddress() = raw.remoteSocketAddress
                override fun getSendBufferSize() = raw.sendBufferSize
                override fun getSoTimeout() = raw.soTimeout
                override fun getTcpNoDelay() = raw.tcpNoDelay
                override fun getTrafficClass() = raw.trafficClass
                override fun isBound() = raw.isBound
                override fun isClosed() = raw.isClosed
                override fun isConnected() = raw.isConnected
                override fun isInputShutdown() = raw.isInputShutdown
                override fun isOutputShutdown() = raw.isOutputShutdown
                override fun sendUrgentData(data: Int) { raw.sendUrgentData(data) }
                override fun setPerformancePreferences(ct: Int, lt: Int, bw: Int) { raw.setPerformancePreferences(ct, lt, bw) }
                override fun setReceiveBufferSize(size: Int) { raw.receiveBufferSize = size }
                override fun setSendBufferSize(size: Int) { raw.sendBufferSize = size }
                override fun setSoTimeout(timeout: Int) { raw.soTimeout = timeout }
                override fun setTcpNoDelay(on: Boolean) { raw.tcpNoDelay = on }
                override fun setTrafficClass(tc: Int) { raw.trafficClass = tc }
                override fun shutdownInput() { raw.shutdownInput() }
                override fun shutdownOutput() { raw.shutdownOutput() }
                override fun toString() = raw.toString()
                override fun equals(other: Any?) = raw.equals(other)
                override fun hashCode() = raw.hashCode()
                override fun getKeepAlive() = raw.keepAlive
                override fun setKeepAlive(on: Boolean) { raw.keepAlive = on }
                override fun getReuseAddress() = raw.reuseAddress
                override fun setReuseAddress(on: Boolean) { raw.reuseAddress = on }
            }
        }

        override fun createSocket(host: String, port: Int): Socket {
            val raw = delegate.createSocket()
            this@KvnVpnService.protect(raw)
            raw.connect(java.net.InetSocketAddress(host, port))
            return raw
        }
        override fun createSocket(host: String, port: Int, localHost: InetAddress, localPort: Int): Socket {
            val raw = delegate.createSocket()
            this@KvnVpnService.protect(raw)
            raw.bind(java.net.InetSocketAddress(localHost, localPort))
            raw.connect(java.net.InetSocketAddress(host, port))
            return raw
        }
        override fun createSocket(host: InetAddress, port: Int): Socket {
            val raw = delegate.createSocket()
            this@KvnVpnService.protect(raw)
            raw.connect(java.net.InetSocketAddress(host, port))
            return raw
        }
        override fun createSocket(addr: InetAddress, port: Int, localAddr: InetAddress, localPort: Int): Socket {
            val raw = delegate.createSocket()
            this@KvnVpnService.protect(raw)
            raw.bind(java.net.InetSocketAddress(localAddr, localPort))
            raw.connect(java.net.InetSocketAddress(addr, port))
            return raw
        }
    }

    @Volatile
    private var vpnEstablished = false

    private var preResolvedServerIps: List<InetAddress>? = null
    private var reconnectStarted = false
    private var tunReaderStarted = false

    private fun resolveServerIpsBeforeVpn() {
        preResolvedServerIps = try {
            InetAddress.getAllByName(config.serverAddress).toList()
        } catch (_: Exception) {
            null
        }
    }

    private fun buildOkHttpClient(): OkHttpClient {
        val builder = OkHttpClient.Builder()
            .socketFactory(SmartSocketFactory())
            .connectTimeout(15, java.util.concurrent.TimeUnit.SECONDS)
            .readTimeout(0, java.util.concurrent.TimeUnit.SECONDS)
            .writeTimeout(15, java.util.concurrent.TimeUnit.SECONDS)
            .dns(object : okhttp3.Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    if (hostname == config.serverAddress && preResolvedServerIps != null) {
                        return preResolvedServerIps!!
                    }
                    return okhttp3.Dns.SYSTEM.lookup(hostname)
                }
            })

        when (config.tlsVerifyMode) {
            "insecure" -> {
                val trustAllCerts = arrayOf<TrustManager>(object : X509TrustManager {
                    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
                    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
                    override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
                })
                val sslContext = SSLContext.getInstance("TLS")
                sslContext.init(null, trustAllCerts, SecureRandom())
                builder.sslSocketFactory(sslContext.socketFactory, trustAllCerts[0] as X509TrustManager)
                builder.hostnameVerifier(HostnameVerifier { _, _ -> true })
            }
            "none" -> {
                // Disable TLS entirely — plain WebSocket ws://
            }
            else -> {
                // verify — default OkHttp behavior
                if (config.tlsServerName.isNotBlank()) {
                    builder.hostnameVerifier(HostnameVerifier { hostname, session ->
                        hostname == config.tlsServerName
                    })
                }
            }
        }

        return builder.build()
    }

    // @sk-task kvn-android#T2.1: service lifecycle start (AC-006)
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            killed = true
            safeStop()
            return START_NOT_STICKY
        }
        try {
            killed = false
            return doStart()
        } catch (e: Exception) {
            stopSelf()
            return START_NOT_STICKY
        }
    }

    // @sk-task kvn-android#T5.21: CIDR subtraction for exclude_ranges (AC-008)
    private fun ip4ToLong(addr: InetAddress): Long {
        val b = addr.address
        return ((b[0].toLong() and 0xFF) shl 24) or ((b[1].toLong() and 0xFF) shl 16) or
               ((b[2].toLong() and 0xFF) shl 8) or (b[3].toLong() and 0xFF)
    }

    private fun longToIp4(v: Long): InetAddress =
        InetAddress.getByAddress(byteArrayOf((v shr 24).toByte(), (v shr 16).toByte(), (v shr 8).toByte(), v.toByte()))

    private data class Cidr4(val base: Long, val prefix: Int) {
        val start: Long get() = base
        val end: Long get() = base or ((1L shl (32 - prefix)) - 1)
    }

    private fun parseCidr4(cidr: String): Cidr4? {
        val parts = cidr.split("/")
        if (parts.size != 2) return null
        val addr = try { InetAddress.getByName(parts[0]) } catch (_: Exception) { null } ?: return null
        if (addr !is Inet4Address) return null
        val prefix = parts[1].toIntOrNull() ?: return null
        if (prefix < 0 || prefix > 32) return null
        val ip = ip4ToLong(addr)
        val mask = if (prefix == 0) 0L else (0xFFFFFFFFL shl (32 - prefix)) and 0xFFFFFFFFL
        return Cidr4(ip and mask, prefix)
    }

    private fun subtractExcludes(excludeRanges: List<String>): List<Cidr4> {
        val excludes = excludeRanges.mapNotNull { parseCidr4(it) }
            .sortedBy { it.start }
        if (excludes.isEmpty()) return listOf(Cidr4(0, 0))

        var ranges = listOf(0L..0xFFFFFFFFL)
        for (ex in excludes) {
            val next = mutableListOf<LongRange>()
            for (r in ranges) {
                when {
                    r.last < ex.start || r.first > ex.end -> next.add(r)
                    r.first >= ex.start && r.last <= ex.end -> { /* drop */ }
                    else -> {
                        if (r.first < ex.start) next.add(r.first..<ex.start)
                        if (r.last > ex.end) next.add(ex.end + 1..r.last)
                    }
                }
            }
            ranges = next
        }
        return ranges.flatMap { rangeToCidrs(it.first, it.last) }
    }

    private fun rangeToCidrs(start: Long, end: Long): List<Cidr4> {
        val result = mutableListOf<Cidr4>()
        var s = start
        while (s <= end) {
            val remaining = end - s + 1
            val align = 1L shl java.lang.Long.numberOfTrailingZeros(s or 0x100000000L)
            val size = if (align <= remaining) {
                align
            } else {
                var sz = 1L
                while (sz * 2 <= remaining) sz = sz shl 1
                sz
            }
            val prefix = 32 - java.lang.Long.numberOfTrailingZeros(size)
            result.add(Cidr4(s, prefix))
            s += size
        }
        return result
    }

    private fun computeVpnRoutes(include: List<String>, exclude: List<String>): List<Pair<InetAddress, Int>> {
        val effectiveInclude = include.toMutableList()
        for (ipStr in config.routingIncludeIps) {
            val trimmed = ipStr.trim()
            if (trimmed.isNotBlank()) effectiveInclude.add("$trimmed/32")
        }
        val effectiveExclude = exclude.toMutableList()
        for (ipStr in config.routingExcludeIps) {
            val trimmed = ipStr.trim()
            if (trimmed.isNotBlank()) effectiveExclude.add("$trimmed/32")
        }
        // Always exclude the server IP from VPN to prevent routing loop
        val serverIps = preResolvedServerIps ?: try {
            InetAddress.getAllByName(config.serverAddress).toList()
        } catch (_: Exception) {
            emptyList()
        }
        for (ip in serverIps) {
            val host = ip.hostAddress
            if (host != null && effectiveExclude.none { it.startsWith("$host/") || it == host }) {
                effectiveExclude.add("$host/32")
            }
        }
        // Exclude current network gateways (WiFi / mobile carrier)
        val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager
        val network = cm?.activeNetwork
        val caps = if (network != null) cm.getNetworkCapabilities(network) else null
        val lp = if (network != null) cm.getLinkProperties(network) else null
        if (lp != null) {
            val isCellular = caps?.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) ?: false
            for (route in lp.routes) {
                if (!route.hasGateway()) continue
                val gw = route.gateway ?: continue
                if (gw.isLoopbackAddress || gw.isLinkLocalAddress || gw is java.net.Inet6Address) continue
                if (isCellular) {
                    effectiveExclude.add("${gw.hostAddress}/32")
                }
            }
        }
        if (effectiveInclude.isNotEmpty()) {
            return effectiveInclude.mapNotNull { parseCidr(it) }
        }
        if (effectiveExclude.isEmpty()) {
            return listOf(InetAddress.getByName("0.0.0.0") to 0)
        }
        return subtractExcludes(effectiveExclude)
            .map { longToIp4(it.base) to it.prefix }
    }

    // @sk-task kvn-android#RX-FIX: TUN established after ServerHello with assigned IP
    private fun establishTun(assignedIp: String, assignedIpv6: String) {
        if (assignedIp.isBlank()) {
            safeStop()
            return
        }
        val builder = Builder()
        builder.setSession("KVN Client")
        builder.setMtu(config.mtu)

        builder.addAddress(InetAddress.getByName(assignedIp), 32)

        val effectiveExcludeRanges = config.routingExcludeRanges.toMutableList()
        val routes = computeVpnRoutes(config.routingIncludeRanges, effectiveExcludeRanges)
        for ((addr, prefix) in routes) {
            builder.addRoute(addr, prefix)
        }

        if (assignedIpv6.isNotBlank() && config.ipv6Enabled) {
            builder.addAddress(InetAddress.getByName(assignedIpv6), 128)
            builder.addRoute(InetAddress.getByName("::"), 0)
        }

        // @sk-task android-per-app-dns#T1.3: DNS servers from app-level settings (AC-004)
        for (dns in dnsServers) {
            try {
                builder.addDnsServer(InetAddress.getByName(dns))
            } catch (_: Exception) {}
        }

        // @sk-task android-per-app-dns#T1.3: per-app filtering from app-level settings (AC-003)
        // VpnService.Builder supports only ONE mode: addAllowedApplication XOR addDisallowedApplication
        if (appIncludeList.isNotEmpty()) {
            for (pkg in appIncludeList) {
                try { builder.addAllowedApplication(pkg.trim()) } catch (_: Exception) {}
            }
        } else {
            for (pkg in appExcludeList) {
                try { builder.addDisallowedApplication(pkg.trim()) } catch (_: Exception) {}
            }
        }

        tunFd = builder.establish()
        tunFdRef = tunFd
        if (tunFd == null) {
            safeStop()
            return
        }

        tunInput = ParcelFileDescriptor.AutoCloseInputStream(tunFd!!)
        tunOutput = ParcelFileDescriptor.AutoCloseOutputStream(tunFd!!)
        vpnEstablished = true
    }

    private fun getPhysicalNetwork(): Network? {
        return try {
            val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
            // Try activeNetwork first (API 23+, not deprecated)
            val active = cm.activeNetwork
            if (active != null) {
                val caps = cm.getNetworkCapabilities(active)
                if (caps != null && caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)) {
                    return active
                }
            }
            // Fallback: iterate all networks (deprecated in API 30+, still needed for edge cases)
            @Suppress("DEPRECATION")
            for (network in cm.allNetworks) {
                val caps = cm.getNetworkCapabilities(network)
                if (caps != null && caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)) {
                    return network
                }
            }
            null
        } catch (_: Exception) { null }
    }

    private fun doStart(): Int {
        try {
            startForeground(NOTIFICATION_ID, createNotification(ConnectionState.CONNECTING))
        } catch (_: Exception) {
            stopSelf()
            return START_NOT_STICKY
        }

        resolveServerIpsBeforeVpn()

        // Wire traffic callback from companion
        onTrafficUpdate = trafficCallback

        // @sk-task kvn-android#T2.4: start transport connection (AC-001)
        // @sk-task kvn-android#T3.1: wire reconnect manager (AC-005)
        // @sk-task kvn-android#BUGFIX: dont disconnect old transport in onReconnect — already dead,
        //   and disconnect() triggers DISCONNECTED -> safeStop loop
        reconnectManager = ReconnectManager(
            scope = serviceScope,
            config = config,
            onReconnect = {
                transportClient = null
                transportClient = createTransport()
                transportClient?.connect()
            },
            onStateChange = onConnectionStateChange,
            onRetriesExhausted = {
                reconnectStarted = false
                safeStop()
            }
        )

        transportClient = createTransport()
        transportClient?.connect()

        registerNetworkCallback()

        return START_STICKY
    }

    // @sk-task kvn-android#T5.11: parse CIDR notation "x.x.x.x/prefix"
    private fun parseCidr(cidr: String): Pair<InetAddress, Int> {
        val parts = cidr.split("/")
        val addr = InetAddress.getByName(parts[0])
        val prefix = if (parts.size > 1) parts[1].toIntOrNull() ?: 32 else 32
        return Pair(addr, prefix)
    }

    // @sk-task android-dns-cache#T1.1: closeTun for reconnect (AC-003, AC-005)
    // @sk-task android-dns-cache#T2.4: clear DNS state on disconnect (AC-001)
    // @sk-task android-fakedns-routing#T2.1: cleanup fakeDNS resolver and direct tunnels (AC-005)
    // @sk-task android-fakedns-routing#T3.2: cleanup FakeIpPool on disconnect (AC-005)
    private fun closeTun() {
        tunInput?.close()
        tunOutput?.close()
        tunFd?.close()
        tunInput = null
        tunOutput = null
        tunFd = null
        tunFdRef = null
        vpnEstablished = false
        tunReaderStarted = false
        notificationUpdateJob?.cancel()
        notificationUpdateJob = null
        dnsCache.clear()
        fakeDnsResolver?.clearExcluded()
        fakeIpPool?.clear()
        directDeliverer?.clear()
        fakeDnsResolver = null
        fakeIpPool = null
        directDeliverer = null
    }



    // @sk-task kvn-android#T5.16: safe stop (AC-010)
    private fun safeStop() {
        if (config.killSwitchEnabled && !killed) {
            transportClient?.disconnect()
            transportClient = null
            reconnectManager?.stop()
            stateCallback?.invoke(ConnectionState.DISCONNECTED)
            onStateChange?.invoke(ConnectionState.DISCONNECTED)
            return
        }
        stopSelf()
    }

    // @sk-task kvn-android#T2.1: service lifecycle stop (AC-006)
    override fun onDestroy() {
        super.onDestroy()
        unregisterNetworkCallback()
        reconnectManager?.stop()
        transportClient?.disconnect()
        transportClient = null
        closeTun()
        serviceScope.cancel()
        stateCallback?.invoke(ConnectionState.DISCONNECTED)
    }

    // @sk-task android-dns-cache#T1.2: fix reconnect — closeTun + reset tunReaderStarted (AC-003, AC-005)
    private val onConnectionStateChange: OnStateChange = { state ->
        stateCallback?.invoke(state)
        onStateChange?.invoke(state)
        updateNotification(state)
        when (state) {
            ConnectionState.CONNECTED -> {
                reconnectStarted = false
                performHandshake()
            }
            ConnectionState.DISCONNECTED -> {
                closeTun()
                tunReaderStarted = false

                if (config.autoReconnect && !killed) {
                    if (!reconnectStarted) {
                        reconnectStarted = true
                        reconnectManager?.start()
                    }
                } else {
                    safeStop()
                }
            }
            else -> {}
        }
    }

    private fun registerNetworkCallback() {
        val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onLost(network: Network) {
                if (config.autoReconnect && !killed && transportClient?.isConnected() == true) {
                    transportClient?.disconnect()
                }
            }
        }
        cm.registerNetworkCallback(NetworkRequest.Builder().build(), callback)
        networkCallback = callback
    }

    private fun unregisterNetworkCallback() {
        networkCallback?.let {
            try {
                (getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager)
                    .unregisterNetworkCallback(it)
            } catch (_: Exception) {}
        }
        networkCallback = null
    }

    // @sk-task kvn-android#T2.4: create transport client (AC-001)
    // @sk-task kvn-android#T3.1: extracted factory for reconnect (AC-005)
    // @sk-task kvn-android#T5.8: use TLS-configured OkHttp client (AC-009)
    private fun createTransport(): TransportClient {
        val scheme = if (config.tlsVerifyMode == "none") "ws" else "wss"
        val url = "$scheme://${config.serverAddress}:${config.port}${config.serverPath}"

        return WebSocketClient(
            okHttpClient = buildOkHttpClient(),
            url = url,
            onFrame = { frame -> handleFrame(frame) },
            onStateChange = onConnectionStateChange,
            onFailure = { t ->
                errorCallback?.invoke("WS: ${t.message ?: t.javaClass.simpleName}")
            },
            paddingEnabled = config.obfuscationPaddingEnabled,
            paddingSize = config.obfuscationPaddingSize
        )
    }

    // @sk-task kvn-android#T2.2: send ClientHello handshake (AC-001)
    private fun performHandshake() {
        val hello = ClientHello(
            protoVersion = PROTO_VERSION,
            token = config.token,
            mtu = config.mtu,
            ipv6 = config.ipv6Enabled,
            transport = "tcp"
        )
        val frame = HandshakeCodec.encodeClientHello(hello)
        transportClient?.send(frame)

        // Handshake timeout: if ServerHello doesn't arrive within the window, disconnect
        serviceScope.launch {
            delay(HANDSHAKE_TIMEOUT_MS)
            if (!tunReaderStarted) {
                errorCallback?.invoke("Handshake timeout: no server response within ${HANDSHAKE_TIMEOUT_MS/1000}s")
                safeStop()
            }
        }
    }

    // @sk-task kvn-android#T2.4: handle incoming frames (AC-001)
    // @sk-task kvn-android#T5.14: decrypt data frames if crypto enabled (AC-011)
    private fun handleFrame(frame: Frame) {
        try {
        when (frame.type) {
            FrameTypes.FRAME_TYPE_HELLO -> {
                val serverHello = HandshakeCodec.decodeServerHello(frame)
                serverSessionId = serverHello.sessionId
                // Derive session key from master key + server salt + session ID (HKDF)
                if (config.cryptoEnabled && config.cryptoKey.isNotBlank() && serverHello.cryptoSalt.isNotEmpty()) {
                    val masterKey = config.cryptoKey.toByteArray()
                    val sessionKey = AesGcmCipher.deriveKey(masterKey, serverHello.cryptoSalt, serverHello.sessionId)
                    cipher = AesGcmCipher(sessionKey)
                    cryptoEnabled = true
                }
                // @sk-task kvn-android#RX-FIX: establish TUN after receiving assigned IP
                if (!tunReaderStarted) {
                    establishTun(serverHello.assignedIp, serverHello.assignedIpv6)
                    tunReaderStarted = true
                    // @sk-task android-fakedns-routing#T2.1: initialize fakeDNS resolver (DEC-001)
                    // @sk-task android-fakedns-routing#T3.1: initialize fakeIpPool for include support (AC-002)
                    if (config.routingDomainsEnabled) {
                        dnsCache = DnsCache()
                        fakeIpPool = FakeIpPool()
                        defaultNetwork = getPhysicalNetwork()
                        if (defaultNetwork != null) {
                            AppLogger.i("DNS", "using physical network for DNS resolution and direct delivery")
                        } else {
                            AppLogger.i("DNS", "no physical network found — DNS forwarded through tunnel")
                        }
                        fakeDnsResolver = FakeDnsResolver(
                            config = config,
                            dnsCache = dnsCache,
                            dnsServers = dnsServers,
                            fakeIpPool = fakeIpPool,
                            defaultNetwork = defaultNetwork
                        )
                        directDeliverer = DirectDeliverer()
                    }
                    serviceScope.launch { tunReader() }
                    notificationUpdateJob = serviceScope.launch {
                        while (isActive) {
                            delay(2000)
                            updateNotification(ConnectionState.CONNECTED)
                        }
                    }
                }
            }
            FrameTypes.FRAME_TYPE_AUTH -> {
                val err = HandshakeCodec.decodeAuthError(frame)
                errorCallback?.invoke("Auth: ${err.reason}")
                safeStop()
            }
            FrameTypes.FRAME_TYPE_DATA -> {
                rxBytes.addAndGet(frame.payload.size.toLong())
                onTrafficUpdate?.invoke(rxBytes.get(), txBytes.get())
                val data = if (cryptoEnabled && cipher != null) {
                    try { cipher!!.decrypt(frame.payload) } catch (_: Exception) { frame.payload }
                } else {
                    frame.payload
                }
                writeToTun(data)
            }
            FrameTypes.FRAME_TYPE_PROXY -> {
                // Proxy frames: forward payload to TUN
                rxBytes.addAndGet(frame.payload.size.toLong())
                onTrafficUpdate?.invoke(rxBytes.get(), txBytes.get())
                writeToTun(frame.payload)
            }
            FrameTypes.FRAME_TYPE_CLOSE -> {
                safeStop()
            }
        }
    } catch (_: Exception) {
        // swallow — OkHttp WebSocket closes on exception in onMessage
    }
    }

    // @sk-task android-fakedns-routing#T2.1: tunReader with fakeDNS interception and direct delivery (DEC-001)
    private suspend fun tunReader() = withContext(Dispatchers.IO) {
        val buf = ByteArray(config.mtu)
        while (isActive) {
            try {
                val len = tunInput?.read(buf) ?: break
                if (len > 0) {
                    val data = buf.copyOf(len)
                    txBytes.addAndGet(len.toLong())
                    onTrafficUpdate?.invoke(rxBytes.get(), txBytes.get())

                    // Try to route through fakeDNS / direct delivery first
                    if (config.routingDomainsEnabled && routePacket(data)) {
                        continue // packet consumed
                    }

                    if (config.routingDomainsEnabled) {
                        val proto = when (data[9].toInt() and 0xFF) { 6 -> "TCP"; 17 -> "UDP"; else -> "?" }
                        AppLogger.i("TUN", "fwd $proto ${data.size}B")
                    }

                    val payload = if (cryptoEnabled && cipher != null) {
                        cipher!!.encrypt(data)
                    } else {
                        data
                    }
                    val frame = Frame(FrameTypes.FRAME_TYPE_DATA, FrameFlags.FRAME_FLAG_NONE, payload)
                    transportClient?.send(frame)
                }
            } catch (e: Exception) {
                if (isActive) safeStop()
                break
            }
        }
    }



    private val tunLock = Any()

    private fun writeToTun(data: ByteArray) {
        synchronized(tunLock) {
            try {
                tunOutput?.write(data)
                tunOutput?.flush()
            } catch (_: Exception) {
                safeStop()
            }
        }
    }

    // @sk-task kvn-android#T2.1: notification for foreground service (AC-006)
    private fun createNotification(state: ConnectionState = ConnectionState.CONNECTING): Notification {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID, "KVN VPN",
                NotificationManager.IMPORTANCE_HIGH
            )
            val nm = getSystemService(NotificationManager::class.java)
            nm.createNotificationChannel(channel)
        }

        val intent = Intent(this, MainActivity::class.java)
        val pendingIntent = PendingIntent.getActivity(
            this, 0, intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val stopIntent = Intent(this, KvnVpnService::class.java).apply {
            action = ACTION_STOP
        }
        val stopPendingIntent = PendingIntent.getService(
            this, 1, stopIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val text = when (state) {
            ConnectionState.CONNECTING -> "Connecting…"
            ConnectionState.RECONNECTING -> "Reconnecting…"
            ConnectionState.CONNECTED -> "VPN active"
            ConnectionState.DISCONNECTED -> "Disconnected"
            ConnectionState.DISCONNECTING -> "Disconnecting…"
        }

        return Notification.Builder(this, CHANNEL_ID)
            .setContentTitle("KVN Client")
            .setContentText(text)
            .setContentIntent(pendingIntent)
            .addAction(android.R.drawable.ic_menu_close_clear_cancel, "Disconnect", stopPendingIntent)
            .setOngoing(true)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .build()
    }

    private fun updateNotification(state: ConnectionState) {
        Handler(Looper.getMainLooper()).post {
            try {
                startForeground(NOTIFICATION_ID, createNotification(state))
            } catch (_: Exception) {}
        }
    }

    // @sk-task android-fakedns-routing#T2.1: TCP direct connection state (DEC-001)
    private class TcpDirectState(
        val socket: Socket,
        @Volatile var mySeq: Long,
        @Volatile var appSeq: Long,
        val dstIp: InetAddress,
        val dstPort: Int,
        val srcIp: InetAddress,
        val srcPort: Int
    )

    // @sk-task android-fakedns-routing#T2.1: direct delivery manager (DEC-001)
    private inner class DirectDeliverer {
        private val tcpFlows = ConcurrentHashMap<String, TcpDirectState>()
        private val udpSockets = ConcurrentHashMap<String, DatagramSocket>()
        private val readerJobs = mutableListOf<kotlinx.coroutines.Job>()
        private val pendingTcpFlows = ConcurrentHashMap<String, Boolean>()

        private fun flowKey(srcIp: InetAddress, srcPort: Int, dstIp: InetAddress, dstPort: Int): String =
            "${srcIp.hostAddress}:$srcPort-${dstIp.hostAddress}:$dstPort"

        // @sk-task android-fakedns-routing#T2.1: deliver TCP SYN for excluded IP (DEC-005)
        // Try direct delivery with 2s timeout. Returns true if connected (split tunnel),
        // false to fall back to tunnel forwarding.
        fun handleTcpSyn(packet: ByteArray, ihl: Int, srcIp: InetAddress, dstIp: InetAddress): Boolean {
            val srcPort = ((packet[ihl].toInt() and 0xFF) shl 8) or (packet[ihl + 1].toInt() and 0xFF)
            val dstPort = ((packet[ihl + 2].toInt() and 0xFF) shl 8) or (packet[ihl + 3].toInt() and 0xFF)
            val key = flowKey(srcIp, srcPort, dstIp, dstPort)
            if (tcpFlows.containsKey(key)) return true

            val seqNum = ip4BytesToLong(packet, ihl + 4)

            try {
                val socket = Socket()
                try { defaultNetwork?.bindSocket(socket) } catch (_: Throwable) { }
                try { protect(socket) } catch (_: Throwable) { }
                socket.connect(InetSocketAddress(dstIp, dstPort), 2000)
                val mySeq = 10000L
                val state = TcpDirectState(socket, mySeq + 1, seqNum + 1, dstIp, dstPort, srcIp, srcPort)
                tcpFlows[key] = state

                val synAck = buildTcpResponse(packet, ihl, mySeq, seqNum + 1,
                    tcpFlags = 0x12, payload = ByteArray(0))
                if (synAck != null) writeToTun(synAck)

                val mtu = config.mtu
                val job = serviceScope.launch {
                    try {
                        val input = socket.getInputStream()
                        val buf = ByteArray(65535)
                        val localKey = key
                        val maxPayload = (mtu - 40).coerceAtLeast(1)
                        while (isActive) {
                            val n = input.read(buf)
                            if (n <= 0) break
                            val st = tcpFlows[localKey] ?: break
                            var offset = 0
                            while (offset < n) {
                                val chunkEnd = minOf(offset + maxPayload, n)
                                val responsePkt = buildTcpResponse(packet, ihl,
                                    st.mySeq, st.appSeq,
                                    tcpFlags = 0x10, payload = buf.copyOfRange(offset, chunkEnd))
                                if (responsePkt != null) {
                                    writeToTun(responsePkt)
                                    st.mySeq += (chunkEnd - offset)
                                }
                                offset = chunkEnd
                            }
                        }
                    } catch (_: Throwable) { }
                    tcpFlows.remove(key)
                    try { socket.close() } catch (_: Throwable) { }
                }
                readerJobs.add(job)
                return true
            } catch (_: Throwable) {
                return false
            }
        }

        // @sk-task android-fakedns-routing#T2.1: deliver TCP data for existing connection
        fun handleTcpData(packet: ByteArray, ihl: Int, srcIp: InetAddress, dstIp: InetAddress) {
            val srcPort = ((packet[ihl].toInt() and 0xFF) shl 8) or (packet[ihl + 1].toInt() and 0xFF)
            val dstPort = ((packet[ihl + 2].toInt() and 0xFF) shl 8) or (packet[ihl + 3].toInt() and 0xFF)
            val key = flowKey(srcIp, srcPort, dstIp, dstPort)
            val state = tcpFlows[key] ?: return
            val dataOffset = ((packet[ihl + 12].toInt() and 0xF0) ushr 4) * 4
            if (dataOffset < 20 || dataOffset > packet.size - ihl) return
            val payload = packet.copyOfRange(ihl + dataOffset, packet.size)
            if (payload.isEmpty()) return
            if (payload.size <= 512) {
                val hex = payload.joinToString("") { "%02x".format(it) }
                val asc = payload.take(80).map { if (it >= 32 && it < 127) it.toInt().toChar() else '.' }.joinToString("")
                AppLogger.i("TCP", "data ${payload.size}B hex=$hex asc=$asc")
            }
            try {
                state.socket.getOutputStream().write(payload)
                state.socket.getOutputStream().flush()
                state.appSeq += payload.size
            } catch (_: Throwable) {
                tcpFlows.remove(key)
                try { state.socket.close() } catch (_: Throwable) { }
            }
        }

        // @sk-task android-fakedns-routing#T2.1: deliver UDP packet for excluded IP
        fun handleUdp(packet: ByteArray, ihl: Int, srcIp: InetAddress, dstIp: InetAddress) {
            val srcPort = ((packet[ihl].toInt() and 0xFF) shl 8) or (packet[ihl + 1].toInt() and 0xFF)
            val dstPort = ((packet[ihl + 2].toInt() and 0xFF) shl 8) or (packet[ihl + 3].toInt() and 0xFF)
            val udpLen = ((packet[ihl + 4].toInt() and 0xFF) shl 8) or (packet[ihl + 5].toInt() and 0xFF)
            val payloadStart = ihl + 8
            if (payloadStart > packet.size) return
            val payloadLen = minOf(udpLen - 8, packet.size - payloadStart)
            if (payloadLen <= 0) return
            val key = flowKey(srcIp, srcPort, dstIp, dstPort)
            try {
                val socket = udpSockets.getOrPut(key) {
                    val s = DatagramSocket()
                    try { defaultNetwork?.bindSocket(s) } catch (_: Throwable) { }
                    try { protect(s) } catch (_: Throwable) { }
                    s
                }
                socket.send(DatagramPacket(packet.copyOfRange(payloadStart, payloadStart + payloadLen),
                    payloadLen, dstIp, dstPort))
            } catch (_: Throwable) {
                udpSockets.remove(key)
            }
        }

        private fun ip4BytesToLong(data: ByteArray, off: Int): Long =
            ((data[off].toLong() and 0xFF) shl 24) or
            ((data[off + 1].toLong() and 0xFF) shl 16) or
            ((data[off + 2].toLong() and 0xFF) shl 8) or
            (data[off + 3].toLong() and 0xFF)

        // @sk-task android-fakedns-routing#T2.1: build TCP response IP packet
        private fun buildTcpResponse(
            original: ByteArray, ihl: Int,
            seqNum: Long, ackNum: Long,
            tcpFlags: Int, payload: ByteArray
        ): ByteArray? {
            val srcIp = original.copyOfRange(12, 16)
            val dstIp = original.copyOfRange(16, 20)
            val srcPort = ((original[ihl].toInt() and 0xFF) shl 8) or (original[ihl + 1].toInt() and 0xFF)
            val dstPort = ((original[ihl + 2].toInt() and 0xFF) shl 8) or (original[ihl + 3].toInt() and 0xFF)
            val tcpLen = 20 + payload.size
            val totalLen = 20 + tcpLen
            val buf = ByteArray(totalLen)
            // IP header
            buf[0] = 0x45
            buf[1] = 0
            buf[2] = ((totalLen shr 8) and 0xFF).toByte()
            buf[3] = (totalLen and 0xFF).toByte()
            buf[4] = 0; buf[5] = 0 // ID
            buf[6] = 0; buf[7] = 0 // flags + fragment
            buf[8] = 64 // TTL
            buf[9] = 6 // TCP
            // IP checksum placeholder
            buf[10] = 0; buf[11] = 0
            // src = original dst
            buf[12] = dstIp[0]; buf[13] = dstIp[1]; buf[14] = dstIp[2]; buf[15] = dstIp[3]
            // dst = original src
            buf[16] = srcIp[0]; buf[17] = srcIp[1]; buf[18] = srcIp[2]; buf[19] = srcIp[3]
            // IP checksum
            val ipCsum = ipChecksum(buf, 20)
            buf[10] = ((ipCsum shr 8) and 0xFF).toByte()
            buf[11] = (ipCsum and 0xFF).toByte()
            // TCP header — swap ports for response (src becomes dst, dst becomes src)
            var off = 20
            buf[off] = ((dstPort shr 8) and 0xFF).toByte(); buf[off + 1] = (dstPort and 0xFF).toByte()
            off += 2
            buf[off] = ((srcPort shr 8) and 0xFF).toByte(); buf[off + 1] = (srcPort and 0xFF).toByte()
            off += 2
            buf[off] = ((seqNum shr 24) and 0xFF).toByte()
            buf[off + 1] = ((seqNum shr 16) and 0xFF).toByte()
            buf[off + 2] = ((seqNum shr 8) and 0xFF).toByte()
            buf[off + 3] = (seqNum and 0xFF).toByte()
            off += 4
            buf[off] = ((ackNum shr 24) and 0xFF).toByte()
            buf[off + 1] = ((ackNum shr 16) and 0xFF).toByte()
            buf[off + 2] = ((ackNum shr 8) and 0xFF).toByte()
            buf[off + 3] = (ackNum and 0xFF).toByte()
            off += 4
            buf[off] = (0x50 or ((tcpFlags shr 4) and 0x0F)).toByte() // data offset + flags high
            buf[off + 1] = (tcpFlags and 0x3F).toByte() // flags low + reserved
            off += 2
            buf[off] = (65535 shr 8).toByte(); buf[off + 1] = (65535 and 0xFF).toByte() // window
            off += 2
            buf[off] = 0; buf[off + 1] = 0 // checksum placeholder
            off += 2
            buf[off] = 0; buf[off + 1] = 0 // urgent pointer
            off += 2
            // Payload
            if (payload.isNotEmpty()) {
                System.arraycopy(payload, 0, buf, off, payload.size)
            }
            // TCP checksum (pseudo-header + segment) — cover full TCP segment (header + payload)
            val tcpSum = tcpChecksum(dstIp, srcIp, buf, 20, 20 + tcpLen)
            buf[off - 4] = ((tcpSum shr 8) and 0xFF).toByte()
            buf[off - 3] = (tcpSum and 0xFF).toByte()
            return buf
        }

        // @sk-task android-fakedns-routing#T2.1: IP header checksum (RFC 1071)
        private fun ipChecksum(data: ByteArray, headerLen: Int): Int {
            var sum = 0
            var i = 0
            while (i < headerLen) {
                sum += ((data[i].toInt() and 0xFF) shl 8) or (data[i + 1].toInt() and 0xFF)
                i += 2
            }
            sum = (sum and 0xFFFF) + (sum ushr 16)
            sum = (sum and 0xFFFF) + (sum ushr 16)
            return sum.inv() and 0xFFFF
        }

        // @sk-task android-fakedns-routing#T2.1: TCP pseudo-header checksum
        private fun tcpChecksum(srcIp: ByteArray, dstIp: ByteArray, segment: ByteArray, start: Int, end: Int): Int {
            var sum = 0
            // Pseudo-header: src IP (2 words), dst IP (2 words), zero, protocol, TCP length
            sum += ((srcIp[0].toInt() and 0xFF) shl 8) or (srcIp[1].toInt() and 0xFF)
            sum += ((srcIp[2].toInt() and 0xFF) shl 8) or (srcIp[3].toInt() and 0xFF)
            sum += ((dstIp[0].toInt() and 0xFF) shl 8) or (dstIp[1].toInt() and 0xFF)
            sum += ((dstIp[2].toInt() and 0xFF) shl 8) or (dstIp[3].toInt() and 0xFF)
            sum += 6 // protocol
            sum += (end - start) // TCP segment length
            // TCP segment
            var i = start
            while (i < end - 1) {
                sum += ((segment[i].toInt() and 0xFF) shl 8) or (segment[i + 1].toInt() and 0xFF)
                i += 2
            }
            if (i < end) {
                sum += (segment[i].toInt() and 0xFF) shl 8
            }
            sum = (sum and 0xFFFF) + (sum ushr 16)
            sum = (sum and 0xFFFF) + (sum ushr 16)
            return sum.inv() and 0xFFFF
        }

        fun clear() {
            for ((_, state) in tcpFlows) {
                try { state.socket.close() } catch (_: Exception) { }
            }
            tcpFlows.clear()
            pendingTcpFlows.clear()
            for ((_, socket) in udpSockets) {
                try { socket.close() } catch (_: Exception) { }
            }
            udpSockets.clear()
            readerJobs.forEach { it.cancel() }
            readerJobs.clear()
        }
    }

    // @sk-task android-fakedns-routing#T2.1: parse IP header and route packet (DEC-001, RQ-005)
    // @sk-task android-fakedns-routing#T3.1: include IP rewrite in routePacket (AC-002, AC-006)
    // @sk-task android-fakedns-routing#T3.2: edge cases in routing engine (AC-005, AC-007)
    private fun routePacket(data: ByteArray): Boolean {
        if (data.size < 20) return false
        val versionIhl = data[0].toInt() and 0xFF
        val ihl = (versionIhl and 0x0F) * 4
        if (ihl < 20 || ihl > data.size) return false
        val protocol = data[9].toInt() and 0xFF
        val dstIp = InetAddress.getByAddress(data.copyOfRange(16, 20))
        val srcIp = InetAddress.getByAddress(data.copyOfRange(12, 16))

        // Try direct delivery for excluded IPs (split tunnel)
        val resolver = fakeDnsResolver
        if (resolver != null && resolver.isExcluded(dstIp)) {
            val del = directDeliverer ?: return false
            when (protocol) {
                6 -> {
                    val flags = data[ihl + 13].toInt() and 0x3F
                    if (flags and 0x02 != 0 && flags and 0x10 == 0) {
                        if (del.handleTcpSyn(data, ihl, srcIp, dstIp)) {
                            AppLogger.i("ROUTE", "direct TCP SYN ${dstIp.hostAddress}")
                            return true
                        }
                        // connect failed → fall through to tunnel
                        AppLogger.i("ROUTE", "direct fallback ${dstIp.hostAddress} → tunnel")
                    } else {
                        del.handleTcpData(data, ihl, srcIp, dstIp)
                        return true
                    }
                }
                17 -> {
                    del.handleUdp(data, ihl, srcIp, dstIp)
                    AppLogger.i("ROUTE", "direct UDP ${dstIp.hostAddress}")
                    return true
                }
            }
        }

        // Forward — not consumed
        if (protocol == 6 && config.routingDomainsEnabled) {
            val flags = data[ihl + 13].toInt() and 0x3F
            val dPort = ((data[ihl + 2].toInt() and 0xFF) shl 8) or (data[ihl + 3].toInt() and 0xFF)
            val sPort = ((data[ihl].toInt() and 0xFF) shl 8) or (data[ihl + 1].toInt() and 0xFF)
            val excluded = resolver?.isExcluded(dstIp) == true
            AppLogger.i("ROUTE", "fwd TCP ${dstIp.hostAddress}:$dPort flags=$flags excl=$excluded")
        }

        // Handle DNS interception (UDP/53)
        if (protocol == 17 && config.routingDomainsEnabled) {
            val dstPort = ((data[ihl + 2].toInt() and 0xFF) shl 8) or (data[ihl + 3].toInt() and 0xFF)
            val srcPort = ((data[ihl].toInt() and 0xFF) shl 8) or (data[ihl + 1].toInt() and 0xFF)
            if (dstPort == 53) {
                val r = fakeDnsResolver
                if (r != null) {
                    val response = r.resolve(data.copyOfRange(ihl + 8, data.size))
                    if (response != null) {
                        AppLogger.i("ROUTE", "UDP/53 intercept → DNS response ${response.size}B")
                        val dnsResponse = buildDnsResponsePacket(srcIp, dstIp, srcPort, response)
                        if (dnsResponse != null) {
                            writeToTun(dnsResponse)
                        }
                        return true
                    }
                }
            }
        }

        // Check fake IP pool (include domains) — rewrite dst IP to real IP
        if (config.routingDomainsEnabled) {
            val pool = fakeIpPool
            if (pool != null) {
                val domain = pool.lookup(dstIp)
                if (domain != null) {
                    val cachedIps = dnsCache.get(domain)
                    if (cachedIps != null && cachedIps.isNotEmpty()) {
                        AppLogger.i("ROUTE", "rewrite ${dstIp.hostAddress} → ${cachedIps[0].hostAddress}")
                        rewritePacket(data, ihl, protocol, cachedIps[0])
                    }
                }
            }
        }

        return false // not consumed — forward through tunnel
    }

    // @sk-task android-fakedns-routing#T2.1: wrap DNS response in UDP/IP header and write to TUN
    private fun buildDnsResponsePacket(srcIp: InetAddress, dstIp: InetAddress, querySrcPort: Int, dnsResponse: ByteArray): ByteArray? {
        val srcBytes = srcIp.address
        val dstBytes = dstIp.address
        val udpLen = 8 + dnsResponse.size
        val totalLen = 20 + udpLen
        val buf = ByteArray(totalLen)
        // IP header
        buf[0] = 0x45
        buf[1] = 0
        buf[2] = ((totalLen shr 8) and 0xFF).toByte()
        buf[3] = (totalLen and 0xFF).toByte()
        buf[8] = 64
        buf[9] = 17 // UDP
        // src = original dst (DNS server), dst = original src (app)
        buf[12] = dstBytes[0]; buf[13] = dstBytes[1]; buf[14] = dstBytes[2]; buf[15] = dstBytes[3]
        buf[16] = srcBytes[0]; buf[17] = srcBytes[1]; buf[18] = srcBytes[2]; buf[19] = srcBytes[3]
        val ipCsum = ipChecksum(buf, 20)
        buf[10] = ((ipCsum shr 8) and 0xFF).toByte()
        buf[11] = (ipCsum and 0xFF).toByte()
        // UDP header
        buf[20] = 0; buf[21] = 53   // src port 53 (DNS server)
        buf[22] = ((querySrcPort shr 8) and 0xFF).toByte()
        buf[23] = (querySrcPort and 0xFF).toByte() // dst port from query
        buf[24] = ((udpLen shr 8) and 0xFF).toByte()
        buf[25] = (udpLen and 0xFF).toByte()
        buf[26] = 0; buf[27] = 0   // UDP checksum (0 = no checksum)
        System.arraycopy(dnsResponse, 0, buf, 28, dnsResponse.size)
        return buf
    }

    // @sk-task android-fakedns-routing#T2.1: IP header checksum (RFC 1071)
    private fun ipChecksum(data: ByteArray, headerLen: Int): Int {
        var sum = 0
        var i = 0
        while (i < headerLen) {
            sum += ((data[i].toInt() and 0xFF) shl 8) or (data[i + 1].toInt() and 0xFF)
            i += 2
        }
        sum = (sum and 0xFFFF) + (sum ushr 16)
        sum = (sum and 0xFFFF) + (sum ushr 16)
        return sum.inv() and 0xFFFF
    }

    // @sk-task android-fakedns-routing#T3.1: rewrite dst IP with checksum recalculation (DEC-005, AC-006)
    private fun rewritePacket(data: ByteArray, ihl: Int, protocol: Int, newDstIp: InetAddress) {
        val newDstBytes = newDstIp.address
        // Zero IP checksum for recalculation
        data[10] = 0; data[11] = 0
        // Update dst IP
        data[16] = newDstBytes[0]; data[17] = newDstBytes[1]
        data[18] = newDstBytes[2]; data[19] = newDstBytes[3]
        // IP checksum
        val ipCsum = ipChecksum(data, ihl)
        data[10] = ((ipCsum shr 8) and 0xFF).toByte()
        data[11] = (ipCsum and 0xFF).toByte()
        if (protocol == 6) {
            // TCP: recalculate checksum with new dst IP in pseudo-header
            data[ihl + 16] = 0; data[ihl + 17] = 0
            val srcIp = data.copyOfRange(12, 16)
            val tcpCsum = tcpChecksum(srcIp, newDstBytes, data, ihl, data.size)
            data[ihl + 16] = ((tcpCsum shr 8) and 0xFF).toByte()
            data[ihl + 17] = (tcpCsum and 0xFF).toByte()
        } else if (protocol == 17) {
            // UDP: set checksum to 0 (MVP — sufficient for common networks)
            data[ihl + 6] = 0; data[ihl + 7] = 0
        }
    }

    // @sk-task android-fakedns-routing#T3.1: TCP pseudo-header checksum (RFC 1071)
    private fun tcpChecksum(srcIp: ByteArray, dstIp: ByteArray, segment: ByteArray, start: Int, end: Int): Int {
        var sum = 0
        for (i in 0..3) {
            sum += ((srcIp[i].toInt() and 0xFF) shl 8) or (dstIp[i].toInt() and 0xFF)
        }
        sum += 6
        sum += (end - start)
        var i = start
        while (i < end - 1) {
            sum += ((segment[i].toInt() and 0xFF) shl 8) or (segment[i + 1].toInt() and 0xFF)
            i += 2
        }
        if (i < end) {
            sum += (segment[i].toInt() and 0xFF) shl 8
        }
        sum = (sum and 0xFFFF) + (sum ushr 16)
        sum = (sum and 0xFFFF) + (sum ushr 16)
        return sum.inv() and 0xFFFF
    }
}
