package com.kvn.client.config

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "kvn_config")

private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

// @sk-task kvn-android#T5.1: full connection config matching kvn-web (RQ-003)
@Serializable
data class ConnectionConfig(
    // Connection
    val serverAddress: String = "",
    val port: Int = 443,
    val serverPath: String = "/kvn",
    val token: String = "",
    val mode: String = "tun",
    val transport: String = "tcp",
    // Advanced
    val mtu: Int = 1500,
    val ipv6Enabled: Boolean = false,
    val autoReconnect: Boolean = true,
    val logLevel: String = "info",
    val maxMessageSize: Int = 65535,
    val multiplex: Boolean = false,
    // Reconnect
    val minBackoffSec: Int = 1,
    val maxBackoffSec: Int = 30,
    // TLS
    val tlsVerifyMode: String = "verify",
    val tlsServerName: String = "",
    val tlsSni: List<String> = emptyList(),
    // Routing
    val routingDefaultRoute: String = "server",
    val routingIncludeRanges: List<String> = emptyList(),
    val routingExcludeRanges: List<String> = emptyList(),
    val routingIncludeIps: List<String> = emptyList(),
    val routingExcludeIps: List<String> = emptyList(),
    val routingIncludeDomains: List<String> = emptyList(),
    val routingExcludeDomains: List<String> = emptyList(),
    // Encryption
    val cryptoEnabled: Boolean = false,
    val cryptoKey: String = "",
    // Kill Switch
    val killSwitchEnabled: Boolean = false,
    // Obfuscation
    val obfuscationEnabled: Boolean = false,
    val obfuscationUtls: Boolean = false,
    val obfuscationPaddingEnabled: Boolean = false,
    val obfuscationPaddingSize: Int = 0
)

// @sk-task kvn-android#T5.1: DataStore-backed JSON config persistence (RQ-005)
class AppConfig(private val context: Context) {

    companion object {
        private val KEY_CONFIG = stringPreferencesKey("config_json")
    }

    // @sk-task kvn-android#T5.1: observe saved config (AC-003, RQ-005)
    val configFlow: Flow<ConnectionConfig> = context.dataStore.data.map { prefs ->
        val raw = prefs[KEY_CONFIG] ?: return@map ConnectionConfig()
        try {
            json.decodeFromString<ConnectionConfig>(raw)
        } catch (_: Exception) {
            ConnectionConfig()
        }
    }

    // @sk-task kvn-android#T5.1: save config as JSON (AC-003, RQ-005)
    suspend fun save(config: ConnectionConfig) {
        context.dataStore.edit { prefs ->
            prefs[KEY_CONFIG] = json.encodeToString(config)
        }
    }
}
