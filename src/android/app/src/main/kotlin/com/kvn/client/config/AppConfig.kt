package com.kvn.client.config

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "kvn_config")

private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

// @sk-task kvn-android#T5.1: full connection config matching kvn-web (RQ-003)
// @sk-task geoip-geosite-integration#T5.2: routing source fields (include/exclude sources, geoip/geosite paths, source TTL)
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
    // Routing Sources
    val routingIncludeSources: String = "",
    val routingExcludeSources: String = "",
    val geoipPath: String = "",
    val geoipUrl: String = "",
    val geositePath: String = "",
    val geositeUrl: String = "",
    val sourceTtlHours: Int = 24,
    // Encryption
    val cryptoEnabled: Boolean = false,
    val cryptoKey: String = "",
    // Kill Switch
    val killSwitchEnabled: Boolean = false,
    // Obfuscation
    val obfuscationEnabled: Boolean = false,
    val obfuscationUtls: Boolean = false,
    val obfuscationPaddingEnabled: Boolean = false,
    val obfuscationPaddingSize: Int = 0,
    // @sk-task android-dns-cache#T4.1: DNS cache toggle field (AC-008)
    val dnsCacheEnabled: Boolean = false,
    // @sk-task kvn-android#T5.22: per-server DNS and app filter settings
    val dnsServers: List<String> = emptyList(),
    val appIncludeList: List<String> = emptyList(),
    val appExcludeList: List<String> = emptyList()
)

// @sk-task multi-server-android-client#T1.1: server entry with name + full config (AC-001)
@Serializable
data class ServerEntry(
    val name: String,
    val config: ConnectionConfig
)

// @sk-task multi-server-android-client#T1.1: multi-server AppConfig wrapper (AC-001)
// @sk-task android-per-app-dns#T1.3: app-level per-app filtering and DNS settings (AC-003, AC-004, AC-005)
@Serializable
data class AppConfig(
    val activeServer: String = "",
    val servers: List<ServerEntry> = emptyList()
)

// @sk-task multi-server-android-client#T1.1: DataStore-backed multi-server persistence (AC-001)
class AppConfigStore(private val context: Context) {

    companion object {
        private val KEY_CONFIG = stringPreferencesKey("config_json")
    }

    // @sk-task multi-server-android-client#T1.1: observe full AppConfig with auto-migration (AC-001)
    val appConfigFlow: Flow<AppConfig> = context.dataStore.data.map { prefs ->
        val raw = prefs[KEY_CONFIG] ?: return@map AppConfig()

        val parsed = try {
            json.decodeFromString<AppConfig>(raw)
        } catch (_: Exception) { null }

        if (parsed != null && parsed.servers.isNotEmpty()) {
            return@map parsed
        }

        val oldConfig = try {
            json.decodeFromString<ConnectionConfig>(raw)
        } catch (_: Exception) { return@map AppConfig() }

        val migrated = AppConfig(
            activeServer = "Default",
            servers = listOf(ServerEntry("Default", oldConfig))
        )
        runBlocking { save(migrated) }
        migrated
    }

    // @sk-task multi-server-android-client#T1.1: derive active server's ConnectionConfig (AC-004)
    val activeConfigFlow: Flow<ConnectionConfig?> = appConfigFlow.map { appCfg ->
        appCfg.servers.find { it.name == appCfg.activeServer }?.config
            ?: appCfg.servers.firstOrNull()?.config
    }

    // @sk-task multi-server-android-client#T1.1: save AppConfig as JSON (AC-001)
    suspend fun save(config: AppConfig) {
        context.dataStore.edit { prefs ->
            prefs[KEY_CONFIG] = json.encodeToString(config)
        }
    }
}
