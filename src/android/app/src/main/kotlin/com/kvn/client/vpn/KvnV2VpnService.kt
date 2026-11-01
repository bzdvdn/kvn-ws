package com.kvn.client.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.IntentFilter
import android.content.Intent
import android.net.ConnectivityManager
import android.net.Network
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.os.PowerManager
import com.kvn.client.logger.AppLogger
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import com.kvn.client.transport.ConnectionState
import mobile.Client
import mobile.Mobile
import org.json.JSONObject

/** v2 VpnService: Kotlin owns TUN/routes/DNS, Go owns the v2 data plane. */
class KvnV2VpnService : VpnService() {
    companion object {
        private const val CHANNEL_ID = "kvn_v2_vpn"
        private const val NOTIFICATION_ID = 2
        private const val EXTRA_CONFIG = "config_json"
        private const val ACTION_STOP = "com.kvn.client.action.STOP_V2_VPN"
        private const val EXTRA_INCLUDE = "include_apps"
        private const val EXTRA_EXCLUDE = "exclude_apps"

        @Volatile private var stateListener: ((ConnectionState) -> Unit)? = null
        @Volatile private var errorListener: ((String) -> Unit)? = null
        @Volatile private var trafficListener: ((Long, Long) -> Unit)? = null

        fun start(
            context: Context,
            configJson: String,
            onState: ((ConnectionState) -> Unit)? = null,
            onError: ((String) -> Unit)? = null,
            onTraffic: ((Long, Long) -> Unit)? = null,
            includeApps: List<String> = emptyList(),
            excludeApps: List<String> = emptyList(),
        ) {
            stateListener = onState
            errorListener = onError
            trafficListener = onTraffic
            val i = Intent(context, KvnV2VpnService::class.java)
                .putExtra(EXTRA_CONFIG, configJson)
                .putStringArrayListExtra(EXTRA_INCLUDE, ArrayList(includeApps))
                .putStringArrayListExtra(EXTRA_EXCLUDE, ArrayList(excludeApps))
            context.startService(i)
        }

        fun stop(context: Context) {
            val i = Intent(context, KvnV2VpnService::class.java).setAction(ACTION_STOP)
            context.startService(i)
        }
    }

    private var client: Client? = null
    private var tunFd: ParcelFileDescriptor? = null
    private var trafficJob: Job? = null
    private var restartJob: Job? = null
    private var networkCallback: ConnectivityManager.NetworkCallback? = null
    private var wakeLock: PowerManager.WakeLock? = null
    private var lastConfigJson: String? = null
    private var lastIncludeApps: List<String> = emptyList()
    private var lastExcludeApps: List<String> = emptyList()
    private val networkTracker = NetworkTracker()
    private val networksByHandle = HashMap<Long, Network>()
    private val restartGate = RestartGate()
    private var keepAwake = false
    private var screenReceiverRegistered = false
    private val screenReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action != Intent.ACTION_USER_PRESENT) return
            val active = client ?: return
            scope.launch {
                val healthy = try {
                    active.health()
                    true
                } catch (e: Exception) {
                    false
                }
                if (SessionRecovery.shouldRestartOnUnlock(healthy)) {
                    AppLogger.i("V2VPN", "device unlocked with stale session; refreshing")
                    restartV2()
                }
            }
        }
    }
    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopV2()
            return START_NOT_STICKY
        }
        val configJson = intent?.getStringExtra(EXTRA_CONFIG)
        val includeApps = intent?.getStringArrayListExtra(EXTRA_INCLUDE).orEmpty()
        val excludeApps = intent?.getStringArrayListExtra(EXTRA_EXCLUDE).orEmpty()
        if (configJson.isNullOrBlank()) {
            stopSelf()
            return START_NOT_STICKY
        }
        val previousConfig = lastConfigJson
        lastConfigJson = configJson
        lastIncludeApps = includeApps
        lastExcludeApps = excludeApps
        if (client != null) {
            if (configJson != previousConfig) {
                AppLogger.i("V2VPN", "config changed while connected; restarting")
                stateListener?.invoke(ConnectionState.CONNECTING)
                restartV2()
            } else {
                // Re-emit so a fresh UI listener cannot stay in CONNECTING.
                stateListener?.invoke(ConnectionState.CONNECTED)
            }
            return START_STICKY
        }
        startForeground(NOTIFICATION_ID, notification())
        scope.launch {
            try {
                startV2(configJson, includeApps, excludeApps)
            } catch (e: Exception) {
                AppLogger.e("V2VPN", "start failed: ${e.message}")
                errorListener?.invoke(e.message ?: "v2 start failed")
                stateListener?.invoke(ConnectionState.DISCONNECTED)
                stopV2()
            }
        }
        return START_STICKY
    }

    private fun startV2(configJson: String, includeApps: List<String>, excludeApps: List<String>) {
        keepAwake = runCatching { JSONObject(configJson).optBoolean("keep_awake", false) }.getOrDefault(false)
        val session = JSONObject(Mobile.resolve(configJson))
        val address = session.getString("tun_address")
        val slash = address.indexOf('/')
        if (slash <= 0) error("invalid tun_address $address")
        val ip = address.substring(0, slash)
        val prefix = address.substring(slash + 1).toIntOrNull() ?: 24
        val gateway = session.optString("tun_gateway", "10.66.0.1")

        val builder = Builder()
            .setSession("KVN v2")
            .setMtu(1200)
            .addAddress(ip, prefix)
            .addRoute("0.0.0.0", 0)
        for (dns in VpnConfigParser.dnsServers(configJson)) {
            runCatching { builder.addDnsServer(dns) }
        }
        val included = includeApps.filter { it != packageName }
        if (included.isNotEmpty()) {
            for (pkg in included) runCatching { builder.addAllowedApplication(pkg) }
        } else {
            for (pkg in excludeApps.filter { it != packageName }) {
                runCatching { builder.addDisallowedApplication(pkg) }
            }
        }
        // Keep our own process outside the tunnel in both modes: the Go
        // transport and in-process direct-DNS resolution must not loop back.
        runCatching { builder.addDisallowedApplication(packageName) }
        val pfd = builder.establish() ?: error("VpnService.establish returned null")
        tunFd = pfd
        try {
            // Do not detach: Kotlin keeps ownership and closes the actual VPN
            // fd on disconnect. Go uses the descriptor without closing it.
            client = Mobile.start(configJson, pfd.fd.toLong())
        } catch (e: Exception) {
            runCatching { pfd.close() }
            tunFd = null
            throw e
        }
        stateListener?.invoke(ConnectionState.CONNECTED)
        acquireWakeLock()
        registerNetworkCallback()
        registerScreenReceiver()
        trafficJob = scope.launch {
            var healthTick = 0
            var healthFailures = 0
            while (true) {
                val active = client ?: break
                if (active.error().isNotBlank()) {
                    AppLogger.w("V2VPN", "Go session failed: ${active.error()}")
                    scope.launch { restartV2() }
                    break
                }
                trafficListener?.invoke(active.rxBytes(), active.txBytes())
                healthTick++
                if (healthTick % 30 == 0) {
                    // Doze/NAT can silently blackhole the HTTP/2 connection
                    // while the VpnService fd (and VPN icon) stays alive.
                    try {
                        // Probe the live session transport, not a new
                        // connection: a blackholed HTTP/2 conn must be
                        // reported as unhealthy.
                        active.health()
                        healthFailures = 0
                    } catch (e: Exception) {
                        healthFailures++
                        AppLogger.w("V2VPN", "health probe failed ($healthFailures/2): ${e.message}")
                        if (healthFailures >= 2) {
                            scope.launch { restartV2() }
                            break
                        }
                    }
                }
                delay(1000)
            }
        }
        AppLogger.i("V2VPN", "started ip=$address gateway=$gateway")
    }

    private fun stopClientAndTun() {
        restartJob?.cancel()
        restartJob = null
        trafficJob?.cancel()
        trafficJob = null
        unregisterNetworkCallback()
        unregisterScreenReceiver()
        releaseWakeLock()
        val pfd = tunFd
        tunFd = null
        val active = client
        client = null
        if (active == null) {
            runCatching { pfd?.close() }
            return
        }
        // Stop the Go pump before freeing the fd. Closing the descriptor while
        // a read is in flight can leak the tunnel or let the kernel reuse the
        // fd for an unrelated socket.
        scope.launch {
            val watchdog = launch {
                delay(3000)
                AppLogger.w("V2VPN", "stop watchdog: forcing TUN fd close")
                runCatching { pfd?.close() }
            }
            try {
                withTimeoutOrNull(3000) { runCatching { active.stop() } }
            } finally {
                watchdog.cancel()
                runCatching { pfd?.close() }
            }
        }
    }

    private fun stopV2() {
        lastConfigJson = null
        stopClientAndTun()
        networkTracker.reset()
        stateListener?.invoke(ConnectionState.DISCONNECTED)
        stateListener = null
        errorListener = null
        trafficListener = null
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun restartV2() {
        val cfg = lastConfigJson ?: return
        if (!restartGate.tryEnter()) return
        AppLogger.i("V2VPN", "restarting after network/session change")
        stateListener?.invoke(ConnectionState.CONNECTING)
        stopClientAndTun()
        restartJob = scope.launch {
            try {
                delay(750)
                if (lastConfigJson != cfg) return@launch
                try {
                    startV2(cfg, lastIncludeApps, lastExcludeApps)
                } catch (e: Exception) {
                    AppLogger.e("V2VPN", "restart failed: ${e.message}")
                    errorListener?.invoke(e.message ?: "v2 restart failed")
                    stateListener?.invoke(ConnectionState.DISCONNECTED)
                    stopV2()
                }
            } finally {
                restartGate.exit()
            }
        }
    }

    private fun registerNetworkCallback() {
        unregisterNetworkCallback()
        val cm = getSystemService(ConnectivityManager::class.java) ?: return
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                networksByHandle[network.networkHandle] = network
                applyNetworkSelection(networkTracker.onAvailable(network.networkHandle))
            }

            override fun onLost(network: Network) {
                networksByHandle.remove(network.networkHandle)
                applyNetworkSelection(networkTracker.onLost(network.networkHandle))
            }
        }
        networkCallback = callback
        networkTracker.beginRegistration()
        // The default-network callback reports a single active network, so
        // extra available networks (Wi-Fi + LTE) cannot trigger a restart loop.
        runCatching { cm.registerDefaultNetworkCallback(callback) }
    }

    private fun applyNetworkSelection(selection: NetworkSelection) {
        val target = selection.activeHandle?.let { networksByHandle[it] }
        runCatching {
            if (target != null) setUnderlyingNetworks(arrayOf(target)) else setUnderlyingNetworks(null)
        }
        if (selection.changed && !selection.lostAll && client != null) {
            AppLogger.i("V2VPN", "underlying network changed; restarting")
            scope.launch { restartV2() }
        }
    }

    private fun unregisterNetworkCallback() {
        val callback = networkCallback ?: return
        networkCallback = null
        runCatching {
            getSystemService(ConnectivityManager::class.java)?.unregisterNetworkCallback(callback)
        }
        networksByHandle.clear()
    }

    private fun registerScreenReceiver() {
        if (screenReceiverRegistered) return
        val filter = IntentFilter(Intent.ACTION_USER_PRESENT)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            registerReceiver(screenReceiver, filter, Context.RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("UnspecifiedRegisterReceiverFlag")
            registerReceiver(screenReceiver, filter)
        }
        screenReceiverRegistered = true
    }

    private fun unregisterScreenReceiver() {
        if (!screenReceiverRegistered) return
        runCatching { unregisterReceiver(screenReceiver) }
        screenReceiverRegistered = false
    }

    private fun acquireWakeLock() {
        releaseWakeLock()
        if (!keepAwake) return
        val pm = getSystemService(PowerManager::class.java) ?: return
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "KVN:Vpn").apply {
            setReferenceCounted(false)
            acquire()
        }
    }

    private fun releaseWakeLock() {
        wakeLock?.let { runCatching { it.release() } }
        wakeLock = null
    }

    override fun onDestroy() {
        stopV2()
        super.onDestroy()
    }

    override fun onRevoke() {
        stopV2()
        super.onRevoke()
    }

    private fun notification(): Notification {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val nm = getSystemService(NotificationManager::class.java)
            nm.createNotificationChannel(NotificationChannel(CHANNEL_ID, "KVN v2", NotificationManager.IMPORTANCE_LOW))
        }
        val b = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) Notification.Builder(this, CHANNEL_ID) else Notification.Builder(this)
        return b.setContentTitle("KVN v2").setContentText("VPN is running").setSmallIcon(android.R.drawable.ic_lock_lock).build()
    }
}
