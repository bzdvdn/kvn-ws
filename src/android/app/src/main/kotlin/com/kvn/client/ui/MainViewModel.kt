package com.kvn.client.ui

import android.app.Application
import android.content.Context
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.kvn.client.config.AppConfig
import com.kvn.client.config.ConnectionConfig
import com.kvn.client.transport.ConnectionState
import com.kvn.client.vpn.KvnVpnService
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.launch

// @sk-task kvn-android#T2.3: main screen state holder (AC-001)
class MainViewModel(application: Application) : AndroidViewModel(application) {

    private val appConfig = AppConfig(application)
    private val prefs = application.getSharedPreferences("kvn_state", Context.MODE_PRIVATE)

    // @sk-task kvn-android#T2.3: connection state (AC-001)
    private val _connectionState = MutableStateFlow(loadSavedState())
    val connectionState: StateFlow<ConnectionState> = _connectionState.asStateFlow()

    private val _errorMessage = MutableStateFlow<String?>(null)
    val errorMessage: StateFlow<String?> = _errorMessage.asStateFlow()

    // @sk-task kvn-android#T2.3: saved config (AC-003)
    val savedConfig: StateFlow<ConnectionConfig> = appConfig.configFlow
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), ConnectionConfig())

    // @sk-task kvn-android#T5.2: apply config from QR code (AC-007, RQ-011)
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

    // @sk-task kvn-android#T2.3: connect to server (AC-001, AC-006)
    // @sk-task kvn-android#T5.1: pass full ConnectionConfig (RQ-005)
    fun connect(config: ConnectionConfig) {
        viewModelScope.launch {
            appConfig.save(config)

            _rxBytes.value = 0L
            _txBytes.value = 0L
            _connectionState.value = ConnectionState.CONNECTING
            _errorMessage.value = null
            saveState(ConnectionState.CONNECTING)
            KvnVpnService.start(
                getApplication(),
                config,
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
}
