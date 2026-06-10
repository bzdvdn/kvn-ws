package com.kvn.client.vpn

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
import java.net.Inet4Address
import java.net.InetAddress
import java.net.Socket
import java.net.SocketAddress
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

        // @sk-task kvn-android#T2.1: start VPN service (AC-001, AC-006)
        fun start(
            context: Context,
            cfg: ConnectionConfig,
            onStateChange: ((ConnectionState) -> Unit)? = null,
            onTrafficUpdate: ((rx: Long, tx: Long) -> Unit)? = null,
            onError: ((String) -> Unit)? = null
        ) {
            config = cfg
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
            if (this@KvnVpnService.vpnEstablished) {
                this@KvnVpnService.protect(raw)
            }
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
            if (this@KvnVpnService.vpnEstablished) {
                this@KvnVpnService.protect(raw)
            }
            raw.connect(java.net.InetSocketAddress(host, port))
            return raw
        }
        override fun createSocket(host: String, port: Int, localHost: InetAddress, localPort: Int): Socket {
            val raw = delegate.createSocket()
            if (this@KvnVpnService.vpnEstablished) {
                this@KvnVpnService.protect(raw)
            }
            raw.bind(java.net.InetSocketAddress(localHost, localPort))
            raw.connect(java.net.InetSocketAddress(host, port))
            return raw
        }
        override fun createSocket(host: InetAddress, port: Int): Socket {
            val raw = delegate.createSocket()
            if (this@KvnVpnService.vpnEstablished) {
                this@KvnVpnService.protect(raw)
            }
            raw.connect(java.net.InetSocketAddress(host, port))
            return raw
        }
        override fun createSocket(addr: InetAddress, port: Int, localAddr: InetAddress, localPort: Int): Socket {
            val raw = delegate.createSocket()
            if (this@KvnVpnService.vpnEstablished) {
                this@KvnVpnService.protect(raw)
            }
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
            .pingInterval(30, java.util.concurrent.TimeUnit.SECONDS)
            .dns(object : okhttp3.Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    if (hostname == config.serverAddress && preResolvedServerIps != null) {
                        return preResolvedServerIps!!
                    }
                    if (hostname == config.serverAddress) {
                        throw java.net.UnknownHostException("DNS not resolved before VPN")
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
        val effectiveExclude = exclude.toMutableList()
        // Always exclude the server IP from VPN to prevent routing loop
        if (preResolvedServerIps != null) {
            for (ip in preResolvedServerIps!!) {
                val host = ip.hostAddress
                if (host != null && effectiveExclude.none { it.startsWith("$host/") || it == host }) {
                    effectiveExclude.add("$host/32")
                }
            }
        }
        if (include.isNotEmpty()) {
            return include.mapNotNull { parseCidr(it) }
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

        val routes = computeVpnRoutes(config.routingIncludeRanges, config.routingExcludeRanges)
        for ((addr, prefix) in routes) {
            builder.addRoute(addr, prefix)
        }

        if (assignedIpv6.isNotBlank() && config.ipv6Enabled) {
            builder.addAddress(InetAddress.getByName(assignedIpv6), 128)
            builder.addRoute(InetAddress.getByName("::"), 0)
        }

        builder.addDnsServer(InetAddress.getByName("1.1.1.1"))

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

    private fun doStart(): Int {
        startForeground(NOTIFICATION_ID, createNotification(ConnectionState.CONNECTING))

        // Resolve server IPs before VPN is established (DNS would hang through TUN)
        resolveServerIpsBeforeVpn()

        // Wire traffic callback from companion
        onTrafficUpdate = trafficCallback

        // @sk-task kvn-android#T2.4: start transport connection (AC-001)
        // @sk-task kvn-android#T3.1: wire reconnect manager (AC-005)
        reconnectManager = ReconnectManager(
            scope = serviceScope,
            config = config,
            onReconnect = {
                transportClient?.disconnect()
                transportClient = null
                transportClient = createTransport()
                transportClient?.connect()
            },
            onStateChange = onConnectionStateChange
        )

        transportClient = createTransport()
        transportClient?.connect()

        return START_STICKY
    }

    // @sk-task kvn-android#T5.11: parse CIDR notation "x.x.x.x/prefix"
    private fun parseCidr(cidr: String): Pair<InetAddress, Int> {
        val parts = cidr.split("/")
        val addr = InetAddress.getByName(parts[0])
        val prefix = if (parts.size > 1) parts[1].toIntOrNull() ?: 32 else 32
        return Pair(addr, prefix)
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
        reconnectManager?.stop()
        transportClient?.disconnect()
        transportClient = null
        tunInput?.close()
        tunOutput?.close()
        tunFd?.close()
        tunInput = null
        tunOutput = null
        tunFd = null
        tunFdRef = null
        vpnEstablished = false
        tunReaderStarted = false
        serviceScope.cancel()
        stateCallback?.invoke(ConnectionState.DISCONNECTED)
    }

    // Shared connection state handler for both initial connect and reconnect
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
                if (config.autoReconnect && !killed && !reconnectStarted) {
                    reconnectStarted = true
                    reconnectManager?.start()
                } else {
                    safeStop()
                }
            }
            else -> {}
        }
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
                    serviceScope.launch { tunReader() }
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
            FrameTypes.FRAME_TYPE_DNS -> {
                // DNS frames: forward to TUN
                rxBytes.addAndGet(frame.payload.size.toLong())
                onTrafficUpdate?.invoke(rxBytes.get(), txBytes.get())
                writeToTun(frame.payload)
            }
            FrameTypes.FRAME_TYPE_CLOSE -> {
                safeStop()
            }
        }
    }

    // @sk-task kvn-android#T2.1: read from TUN, send to transport (AC-001)
    // @sk-task kvn-android#T5.14: encrypt data frames if crypto enabled (AC-011)
    private suspend fun tunReader() = withContext(Dispatchers.IO) {
        val buf = ByteArray(config.mtu)
        while (isActive) {
            try {
                val len = tunInput?.read(buf) ?: break
                if (len > 0) {
                    val data = buf.copyOf(len)
                    txBytes.addAndGet(len.toLong())
                    onTrafficUpdate?.invoke(rxBytes.get(), txBytes.get())
                    val payload = if (cryptoEnabled && cipher != null) {
                        cipher!!.encrypt(data)
                    } else {
                        data
                    }
                    val frame = Frame(FrameTypes.FRAME_TYPE_DATA, FrameFlags.FRAME_FLAG_NONE, payload)
                    transportClient?.send(frame)
                }
            } catch (e: Exception) {
                if (isActive) {
                    safeStop()
                }
                break
            }
        }
    }

    // @sk-task kvn-android#T2.1: write to TUN from incoming data (AC-001)
    private fun writeToTun(data: ByteArray) {
        try {
            tunOutput?.write(data)
            tunOutput?.flush()
        } catch (_: Exception) {
            safeStop()
        }
    }

    // @sk-task kvn-android#T2.1: notification for foreground service (AC-006)
    private fun createNotification(state: ConnectionState = ConnectionState.CONNECTING): Notification {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID, "KVN VPN",
                NotificationManager.IMPORTANCE_LOW
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
            ConnectionState.CONNECTED -> "VPN tunnel active"
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
}
