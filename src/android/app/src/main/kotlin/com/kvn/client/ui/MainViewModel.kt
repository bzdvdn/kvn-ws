package com.kvn.client.ui

import android.app.Application
import android.content.Context
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.kvn.client.config.AppConfig
import com.kvn.client.config.AppConfigStore
import com.kvn.client.config.ConnectionConfig
import com.kvn.client.config.ServerEntry
import com.kvn.client.transport.ConnectionState
import com.kvn.client.vpn.KvnVpnService
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.launch

// @sk-task kvn-android#T2.3: main screen state holder (AC-001)
// @sk-task multi-server-android-client#T2.1: multi-server state + CRUD (AC-001, AC-002, AC-003)
class MainViewModel(application: Application) : AndroidViewModel(application) {

    private val appConfigStore = AppConfigStore(application)
    private val prefs = application.getSharedPreferences("kvn_state", Context.MODE_PRIVATE)

    // @sk-task kvn-android#T2.3: connection state (AC-001)
    private val _connectionState = MutableStateFlow(loadSavedState())
    val connectionState: StateFlow<ConnectionState> = _connectionState.asStateFlow()

    private val _errorMessage = MutableStateFlow<String?>(null)
    val errorMessage: StateFlow<String?> = _errorMessage.asStateFlow()

    // @sk-task multi-server-android-client#T2.1: observed AppConfig (AC-001)
    val savedAppConfig: StateFlow<AppConfig> = appConfigStore.appConfigFlow
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), AppConfig())

    // @sk-task multi-server-android-client#T2.1: derived active server name (AC-002)
    val activeServerName: StateFlow<String> = savedAppConfig.map { it.activeServer }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), "")

    // @sk-task multi-server-android-client#T2.1: derived sorted server list (DEC-003)
    val servers: StateFlow<List<ServerEntry>> = savedAppConfig.map { appCfg ->
        sortServers(appCfg.activeServer, appCfg.servers)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), emptyList())

    // @sk-task multi-server-android-client#T2.1: derived active server config (AC-004)
    val activeServerConfig: StateFlow<ConnectionConfig?> = savedAppConfig.map { appCfg ->
        appCfg.servers.find { it.name == appCfg.activeServer }?.config
            ?: appCfg.servers.firstOrNull()?.config
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), null)

    // @sk-task multi-server-android-client#T2.1: dirty flag (AC-003)
    private val _isDirty = MutableStateFlow(false)
    val isDirty: StateFlow<Boolean> = _isDirty.asStateFlow()

    fun markDirty() { _isDirty.value = true }

    fun markClean() { _isDirty.value = false }

    // @sk-task android-per-app-dns#T1.3: app-level settings flows (AC-003, AC-004, AC-005)
    val appIncludeList: StateFlow<List<String>> = savedAppConfig.map { it.appIncludeList }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), emptyList())

    val appExcludeList: StateFlow<List<String>> = savedAppConfig.map { it.appExcludeList }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), emptyList())

    val dnsServers: StateFlow<List<String>> = savedAppConfig.map { it.dnsServers }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), listOf("1.1.1.1", "8.8.8.8"))

    // @sk-task android-per-app-dns#T1.3: persist app-level settings to DataStore (AC-005)
    fun saveAppSettings(include: List<String>, exclude: List<String>, dns: List<String>) {
        viewModelScope.launch {
            val current = savedAppConfig.value
            val updated = current.copy(
                appIncludeList = include,
                appExcludeList = exclude,
                dnsServers = dns
            )
            appConfigStore.save(updated)
        }
    }

    // @sk-task kvn-android#T5.2: apply config from QR code (AC-007, RQ-011)
    // @sk-task multi-server-android-client#T2.1: QR adds a new server (AC-006)
    fun applyQrConfig(config: ConnectionConfig) {
        _qrConfig.value = config
    }

    private val _qrConfig = MutableStateFlow<ConnectionConfig?>(null)
    val qrConfig: StateFlow<ConnectionConfig?> = _qrConfig.asStateFlow()

    fun consumeQrConfig() {
        _qrConfig.value = null
    }

    // @sk-task kvn-android#T3.2: traffic counters (AC-002)
    private val _rxBytes = MutableStateFlow(0L)
    val rxBytes: StateFlow<Long> = _rxBytes.asStateFlow()
    private val _txBytes = MutableStateFlow(0L)
    val txBytes: StateFlow<Long> = _txBytes.asStateFlow()

    // @sk-task multi-server-android-client#T2.1: add new server (AC-001, AC-002)
    fun addServer(name: String, config: ConnectionConfig) {
        viewModelScope.launch {
            val current = savedAppConfig.value
            val updated = current.copy(
                activeServer = name,
                servers = current.servers + ServerEntry(name, config)
            )
            appConfigStore.save(updated)
            _isDirty.value = false
        }
    }

    // @sk-task multi-server-android-client#T2.1: duplicate active server (AC-008)
    fun duplicateServer(newName: String) {
        viewModelScope.launch {
            val current = savedAppConfig.value
            val activeCfg = current.servers.find { it.name == current.activeServer }?.config
                ?: return@launch
            val updated = current.copy(
                activeServer = newName,
                servers = current.servers + ServerEntry(newName, activeCfg)
            )
            appConfigStore.save(updated)
            _isDirty.value = false
        }
    }

    // @sk-task multi-server-android-client#T2.1: delete server by name (AC-001)
    fun deleteServer(name: String) {
        viewModelScope.launch {
            val current = savedAppConfig.value
            if (current.servers.size <= 1) return@launch
            val remaining = current.servers.filter { it.name != name }
            val newActive = if (name == current.activeServer) {
                remaining.firstOrNull()?.name ?: ""
            } else {
                current.activeServer
            }
            val updated = current.copy(activeServer = newActive, servers = remaining)
            appConfigStore.save(updated)
            _isDirty.value = false
        }
    }

    // @sk-task multi-server-android-client#T2.1: rename server (AC-001)
    fun renameServer(oldName: String, newName: String) {
        viewModelScope.launch {
            val current = savedAppConfig.value
            val updatedServers = current.servers.map {
                if (it.name == oldName) it.copy(name = newName) else it
            }
            val newActive = if (current.activeServer == oldName) newName else current.activeServer
            val updated = current.copy(activeServer = newActive, servers = updatedServers)
            appConfigStore.save(updated)
            _isDirty.value = false
        }
    }

    // @sk-task multi-server-android-client#T2.1: set active server (AC-002)
    fun setActiveServer(name: String) {
        viewModelScope.launch {
            val current = savedAppConfig.value
            val updated = current.copy(activeServer = name)
            appConfigStore.save(updated)
            _isDirty.value = false
        }
    }

    // @sk-task multi-server-android-client#T2.1: save current server's config (AC-004)
    fun saveCurrentServerConfig(config: ConnectionConfig) {
        viewModelScope.launch {
            val current = savedAppConfig.value
            val updatedServers = current.servers.map {
                if (it.name == current.activeServer) it.copy(config = config) else it
            }
            val updated = current.copy(servers = updatedServers)
            appConfigStore.save(updated)
            _isDirty.value = false
        }
    }

    // @sk-task geoip-geosite-integration#T5.2: refresh sources — save config, reconnect to apply resolved sources
    fun refreshSources(config: ConnectionConfig) {
        viewModelScope.launch {
            saveCurrentServerConfig(config)
            if (_connectionState.value == ConnectionState.CONNECTED) {
                KvnVpnService.stop(getApplication())
                _connectionState.value = ConnectionState.DISCONNECTED
            }
        }
    }

    // @sk-task kvn-android#T2.3: connect to server (AC-001, AC-006)
    // @sk-task kvn-android#T5.1: pass full ConnectionConfig (RQ-005)
    // @sk-task multi-server-android-client#T2.1: save config to active server before connect (AC-004)
    fun connect(
        config: ConnectionConfig,
        appIncludeList: List<String>? = null,
        appExcludeList: List<String>? = null,
        dnsServers: List<String>? = null
    ) {
        viewModelScope.launch {
            saveCurrentServerConfig(config)

            _rxBytes.value = 0L
            _txBytes.value = 0L
            _connectionState.value = ConnectionState.CONNECTING
            _errorMessage.value = null
            saveState(ConnectionState.CONNECTING)
            val appCfg = savedAppConfig.value
            KvnVpnService.start(
                getApplication(),
                config,
                appIncludeList = appIncludeList ?: appCfg.appIncludeList,
                appExcludeList = appExcludeList ?: appCfg.appExcludeList,
                dnsServers = dnsServers ?: appCfg.dnsServers,
                onStateChange = { state ->
                    _connectionState.value = state
                    saveState(state)
                },
                onTrafficUpdate = { rx, tx ->
                    _rxBytes.value = rx
                    _txBytes.value = tx
                },
                onError = { msg ->
                    _errorMessage.value = msg
                }
            )
        }
    }

    // @sk-task kvn-android#T2.3: disconnect from server (AC-006)
    fun disconnect() {
        KvnVpnService.stop(getApplication())
        _connectionState.value = ConnectionState.DISCONNECTED
        _errorMessage.value = null
        saveState(ConnectionState.DISCONNECTED)
    }

    private fun saveState(state: ConnectionState) {
        prefs.edit().putString("connection_state", state.name).apply()
    }

    private fun loadSavedState(): ConnectionState {
        return try {
            val name = prefs.getString("connection_state", ConnectionState.DISCONNECTED.name) ?: ConnectionState.DISCONNECTED.name
            ConnectionState.valueOf(name)
        } catch (_: Exception) {
            ConnectionState.DISCONNECTED
        }
    }

    companion object {
        // @sk-task multi-server-android-client#T2.1: sort: active first, rest A-Z (DEC-003)
        fun sortServers(activeServer: String, servers: List<ServerEntry>): List<ServerEntry> {
            val sorted = servers.sortedBy { it.name.lowercase() }
            val active = sorted.find { it.name == activeServer }
            if (active != null) {
                return listOf(active) + sorted.filter { it.name != activeServer }
            }
            return sorted
        }
    }
}
