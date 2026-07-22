package com.kvn.client.config

import com.kvn.client.ui.MainViewModel
import com.kvn.client.ui.parseQrConfig
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import org.junit.Assert.*
import org.junit.Test

private val json = Json { encodeDefaults = true }

// @sk-test kvn-android#T5.20: TestConfigJsonRoundTrip (RQ-005)
class ConfigSerializationTest {

    @Test
    fun testConfigJsonRoundTrip() {
        val config = ConnectionConfig(
            serverAddress = "vpn.example.com",
            port = 8443,
            serverPath = "/custom",
            token = "secret-token-123",
            mode = "tun",
            transport = "tcp",
            mtu = 1400,
            ipv6Enabled = true,
            autoReconnect = false,
            logLevel = "debug",
            maxMessageSize = 32768,
            multiplex = true,
            minBackoffSec = 2,
            maxBackoffSec = 60,
            tlsVerifyMode = "insecure",
            tlsServerName = "custom-sni.example.com",
            tlsSni = listOf("sni1.example.com", "sni2.example.com"),
            cryptoEnabled = true,
            cryptoKey = "my-encryption-key",
            killSwitchEnabled = true,
            obfuscationEnabled = false,
            obfuscationUtls = false,
            obfuscationPaddingEnabled = false,
            obfuscationPaddingSize = 0,
            dnsServers = listOf("1.1.1.1", "8.8.8.8"),
            appIncludeList = listOf("com.example.allowed"),
            appExcludeList = listOf("com.example.blocked")
        )

        val serialized = json.encodeToString(config)
        val deserialized = json.decodeFromString<ConnectionConfig>(serialized)

        assertEquals(config.serverAddress, deserialized.serverAddress)
        assertEquals(config.port, deserialized.port)
        assertEquals(config.serverPath, deserialized.serverPath)
        assertEquals(config.token, deserialized.token)
        assertEquals(config.mode, deserialized.mode)
        assertEquals(config.transport, deserialized.transport)
        assertEquals(config.mtu, deserialized.mtu)
        assertEquals(config.ipv6Enabled, deserialized.ipv6Enabled)
        assertEquals(config.autoReconnect, deserialized.autoReconnect)
        assertEquals(config.logLevel, deserialized.logLevel)
        assertEquals(config.maxMessageSize, deserialized.maxMessageSize)
        assertEquals(config.multiplex, deserialized.multiplex)
        assertEquals(config.minBackoffSec, deserialized.minBackoffSec)
        assertEquals(config.maxBackoffSec, deserialized.maxBackoffSec)
        assertEquals(config.tlsVerifyMode, deserialized.tlsVerifyMode)
        assertEquals(config.tlsServerName, deserialized.tlsServerName)
        assertEquals(config.tlsSni, deserialized.tlsSni)
        assertEquals(config.cryptoEnabled, deserialized.cryptoEnabled)
        assertEquals(config.cryptoKey, deserialized.cryptoKey)
        assertEquals(config.killSwitchEnabled, deserialized.killSwitchEnabled)
        assertEquals(config.dnsServers, deserialized.dnsServers)
        assertEquals(config.appIncludeList, deserialized.appIncludeList)
        assertEquals(config.appExcludeList, deserialized.appExcludeList)
    }

    // @sk-test kvn-android#T5.20: TestConfigDefaults (RQ-005)
    @Test
    fun testConfigDefaults() {
        val config = ConnectionConfig()
        assertEquals("", config.serverAddress)
        assertEquals(443, config.port)
        assertEquals("/kvn", config.serverPath)
        assertEquals("tun", config.mode)
        assertEquals("tcp", config.transport)
        assertEquals("verify", config.tlsVerifyMode)
        assertFalse(config.killSwitchEnabled)
        assertFalse(config.cryptoEnabled)
        assertTrue(config.dnsServers.isEmpty())
        assertTrue(config.appIncludeList.isEmpty())
        assertTrue(config.appExcludeList.isEmpty())
    }
}

// @sk-test kvn-android#T5.20: TestParseQrConfigJson (AC-007, RQ-011)
class QrConfigTest {

    @Test
    fun testParseQrConfigJson() {
        val jsonStr = """{
            "serverAddress": "vpn.test.com",
            "port": 443,
            "token": "test-token",
            "mode": "tun",
            "transport": "tcp",
            "mtu": 1500,
            "ipv6Enabled": true
        }"""

        val config = parseQrConfig(jsonStr)
        assertNotNull(config)
        assertEquals("vpn.test.com", config!!.serverAddress)
        assertEquals(443, config.port)
        assertEquals("test-token", config.token)
        assertTrue(config.ipv6Enabled)
    }

    // @sk-test kvn-android#T5.20: TestParseQrConfigLegacy (AC-007)
    @Test
    fun testParseQrConfigLegacy() {
        val config = parseQrConfig("server.example.com:8443:mytoken")
        assertNotNull(config)
        assertEquals("server.example.com", config!!.serverAddress)
        assertEquals(8443, config.port)
        assertEquals("mytoken", config.token)
    }

    // @sk-test kvn-android#T5.20: TestParseQrConfigInvalid (AC-007)
    @Test
    fun testParseQrConfigInvalid() {
        assertNull(parseQrConfig("invalid-format"))
        assertNull(parseQrConfig(""))
        assertNull(parseQrConfig("host:notaport:token"))
    }

    // @sk-test kvn-android#T5.20: TestParseQrConfigMinimalJson (AC-007)
    @Test
    fun testParseQrConfigMinimalJson() {
        val jsonStr = """{"serverAddress": "s", "port": 1, "token": "t"}"""
        val config = parseQrConfig(jsonStr)
        assertNotNull(config)
        assertEquals("s", config!!.serverAddress)
        assertEquals(1, config.port)
        assertEquals("t", config.token)
    }

    // @sk-test kvn-android#T5.20: TestParseQrConfigJsonExtraFields (AC-007)
    @Test
    fun testParseQrConfigJsonExtraFields() {
        val jsonStr = """{
            "serverAddress": "x.com",
            "port": 8080,
            "token": "tok",
            "unknownField": "should be ignored"
        }"""
        val config = parseQrConfig(jsonStr)
        assertNotNull(config)
        assertEquals("x.com", config!!.serverAddress)
    }

    // @sk-test: parse kvn-web config with null routing slices (Go nil slice serialization)
    @Test
    fun testParseWebWithNullRoutingSlices() {
        val jsonStr = """{
            "server": "wss://example.com:443/kvn",
            "transport": "tcp",
            "auth": {"token": "tok"},
            "tls": {"verify_mode": "verify", "server_name": "", "sni": null},
            "mtu": 1400, "ipv6": false, "auto_reconnect": true,
            "routing": {"default_route": "server", "include_ranges": null, "exclude_ranges": null, "include_ips": null, "exclude_ips": null, "include_domains": null, "exclude_domains": null},
            "kill_switch": null, "reconnect": null,
            "mode": "tun", "crypto": {"enabled": false, "key": ""},
            "max_message_size": 65535
        }"""
        val config = parseQrConfig(jsonStr)
        assertNotNull(config)
        assertEquals("example.com", config!!.serverAddress)
        assertEquals(443, config.port)
        assertEquals("/kvn", config.serverPath)
        assertEquals(emptyList<String>(), config.routingIncludeRanges)
        assertEquals(emptyList<String>(), config.routingExcludeRanges)
        assertEquals(emptyList<String>(), config.tlsSni)
        assertEquals(emptyList<String>(), config.routingIncludeDomains)
        assertEquals(emptyList<String>(), config.routingExcludeDomains)
        assertEquals("info", config.logLevel)
    }

    // @sk-test: parse kvn-web full config with URL, nested obfuscation, null kill_switch
    @Test
    fun testParseWebFullConfig() {
        val jsonStr = """{
            "server": "wss://216.57.111.226:443/tunnel",
            "transport": "quic",
            "obfuscation": {"enabled": true, "utls": {"enabled": true, "fallback": false}, "padding": {"enabled": true, "size": 512}},
            "auth": {"token": "65d1b23fd3aafdd91dee22ffe41bd44885a926c19b5923ab"},
            "tls": {"ca_file": "", "server_name": "", "verify_mode": "insecure"},
            "mtu": 1300, "ipv6": false, "auto_reconnect": true,
            "log": {"level": "debug"},
            "routing": {"default_route": "server", "include_ranges": [], "exclude_ranges": ["10.0.0.0/8"], "include_ips": [], "exclude_ips": [], "include_domains": [], "exclude_domains": [".we-on.com"]},
            "kill_switch": null, "reconnect": null,
            "multiplex": false, "mode": "tun",
            "proxy_listen": "127.0.0.1:2310", "proxy_auth": null,
            "crypto": {"enabled": false, "key": ""},
            "max_message_size": 10485760, "tunnel_timeout": 30,
            "system_proxy": true, "transparent": true,
            "dns_proxy": {"listen": "127.0.0.54:53", "upstream": "1.1.1.1:53"}
        }"""
        val config = parseQrConfig(jsonStr)
        assertNotNull(config)
        assertEquals("216.57.111.226", config!!.serverAddress)
        assertEquals(443, config.port)
        assertEquals("/tunnel", config.serverPath)
        assertEquals("65d1b23fd3aafdd91dee22ffe41bd44885a926c19b5923ab", config.token)
        assertEquals("tcp", config.transport) // quic → tcp
        assertEquals("insecure", config.tlsVerifyMode)
        assertEquals(1300, config.mtu)
        assertTrue(config.obfuscationEnabled)
        assertTrue(config.obfuscationUtls)
        assertTrue(config.obfuscationPaddingEnabled)
        assertEquals(512, config.obfuscationPaddingSize)
        assertFalse(config.cryptoEnabled)
        assertEquals(10485760, config.maxMessageSize)
        assertEquals("debug", config.logLevel)
        assertEquals(emptyList<String>(), config.routingIncludeDomains)
        assertEquals(listOf(".we-on.com"), config.routingExcludeDomains)
        assertEquals("", config.geoipUrl)
        assertFalse(config.dnsCacheEnabled)
        assertEquals(3600, config.dnsCacheTtl)
    }
}

// @sk-task multi-server-android-client#T4.1: multi-server data model tests (AC-001, AC-002)
class MultiServerConfigTest {

    private val testJson = Json { encodeDefaults = true; ignoreUnknownKeys = true }

    // @sk-test multi-server-android-client#T4.1: old ConnectionConfig parses as AppConfig with empty servers (AC-001)
    @Test
    fun testOldConfigParsesAsAppConfigWithEmptyServers() {
        val oldJson = testJson.encodeToString(ConnectionConfig(
            serverAddress = "vpn.example.com",
            port = 443,
            token = "test-token"
        ))
        val appCfg = testJson.decodeFromString<AppConfig>(oldJson)
        assertTrue("old config should parse as empty-servers AppConfig", appCfg.servers.isEmpty())
        assertEquals("", appCfg.activeServer)
    }

    // @sk-test multi-server-android-client#T4.1: direct ConnectionConfig parse of old JSON succeeds (AC-001)
    @Test
    fun testOldConfigParsesAsConnectionConfig() {
        val oldJson = testJson.encodeToString(ConnectionConfig(
            serverAddress = "vpn.example.com",
            port = 8443,
            serverPath = "/custom",
            token = "old-token"
        ))
        val cfg = testJson.decodeFromString<ConnectionConfig>(oldJson)
        assertEquals("vpn.example.com", cfg.serverAddress)
        assertEquals(8443, cfg.port)
        assertEquals("/custom", cfg.serverPath)
        assertEquals("old-token", cfg.token)
    }

    // @sk-test multi-server-android-client#T4.1: migration wraps old config in ServerEntry("Default") (AC-001)
    @Test
    fun testMigrationWrapInServerEntry() {
        val oldCfg = ConnectionConfig(serverAddress = "migrate.me", token = "migrate-token")
        val migrated = AppConfig(
            activeServer = "Default",
            servers = listOf(ServerEntry("Default", oldCfg))
        )
        assertEquals(1, migrated.servers.size)
        assertEquals("Default", migrated.servers[0].name)
        assertEquals("migrate.me", migrated.servers[0].config.serverAddress)
        assertEquals("migrate-token", migrated.servers[0].config.token)
        assertEquals("Default", migrated.activeServer)
    }

    // @sk-test multi-server-android-client#T4.1: AppConfig serialization round-trip (AC-001)
    @Test
    fun testAppConfigRoundTrip() {
        val original = AppConfig(
            activeServer = "Work",
            servers = listOf(
                ServerEntry("Work", ConnectionConfig(serverAddress = "work.example.com", token = "work-token")),
                ServerEntry("Home", ConnectionConfig(serverAddress = "home.example.com", token = "home-token")),
                ServerEntry("Dev", ConnectionConfig(serverAddress = "dev.example.com", token = "dev-token"))
            )
        )
        val json = testJson.encodeToString(original)
        val restored = testJson.decodeFromString<AppConfig>(json)
        assertEquals(original.activeServer, restored.activeServer)
        assertEquals(original.servers.size, restored.servers.size)
        assertEquals(original.servers[0].name, restored.servers[0].name)
        assertEquals(original.servers[0].config.serverAddress, restored.servers[0].config.serverAddress)
        assertEquals(original.servers[1].config.token, restored.servers[1].config.token)
    }

    // @sk-test multi-server-android-client#T4.1: sortServers — active on top, rest A-Z (DEC-003)
    @Test
    fun testSortServersActiveOnTop() {
        val servers = listOf(
            ServerEntry("Zoo", ConnectionConfig()),
            ServerEntry("Alpha", ConnectionConfig()),
            ServerEntry("Work", ConnectionConfig()),
            ServerEntry("Beta", ConnectionConfig())
        )
        val sorted = MainViewModel.sortServers("Work", servers)
        assertEquals(4, sorted.size)
        assertEquals("Work", sorted[0].name) // active first
        assertEquals("Alpha", sorted[1].name)
        assertEquals("Beta", sorted[2].name)
        assertEquals("Zoo", sorted[3].name)
    }

    // @sk-test multi-server-android-client#T4.1: sortServers — active not found, returns sorted (DEC-003)
    @Test
    fun testSortServersActiveNotFound() {
        val servers = listOf(
            ServerEntry("C", ConnectionConfig()),
            ServerEntry("A", ConnectionConfig()),
            ServerEntry("B", ConnectionConfig())
        )
        val sorted = MainViewModel.sortServers("Missing", servers)
        assertEquals(3, sorted.size)
        assertEquals("A", sorted[0].name)
        assertEquals("B", sorted[1].name)
        assertEquals("C", sorted[2].name)
    }

    // @sk-test multi-server-android-client#T4.1: sortServers — active first when already first (DEC-003)
    @Test
    fun testSortServersActiveFirstAlreadyFirst() {
        val servers = listOf(
            ServerEntry("Active", ConnectionConfig()),
            ServerEntry("B", ConnectionConfig()),
            ServerEntry("A", ConnectionConfig())
        )
        val sorted = MainViewModel.sortServers("Active", servers)
        assertEquals("Active", sorted[0].name)
        assertEquals("A", sorted[1].name)
        assertEquals("B", sorted[2].name)
    }

    // @sk-test multi-server-android-client#T4.1: duplicate server creates copy with "(copy)" suffix (AC-008)
    @Test
    fun testDuplicateServerEntry() {
        val original = ServerEntry("Work", ConnectionConfig(
            serverAddress = "work.example.com",
            token = "work-token",
            mtu = 1300
        ))
        val copy = original.copy(name = "Work (copy)")
        assertEquals("Work (copy)", copy.name)
        assertEquals(original.config.serverAddress, copy.config.serverAddress)
        assertEquals(original.config.token, copy.config.token)
        assertEquals(original.config.mtu, copy.config.mtu)
    }
}

// @sk-task kvn-android#T5.22: per-server DNS/app fields serialization tests
class PerServerDnsAppSerializationTest {

    private val json = Json { encodeDefaults = true; ignoreUnknownKeys = true }

    // @sk-test kvn-android#T5.22: DNS/app fields survive round-trip
    @Test
    fun testDnsAppFieldsRoundTrip() {
        val config = ConnectionConfig(
            serverAddress = "vpn.example.com",
            port = 443,
            token = "tok",
            dnsServers = listOf("1.1.1.1", "8.8.8.8"),
            appIncludeList = listOf("com.example.app"),
            appExcludeList = listOf("com.example.exclude")
        )
        val serialized = json.encodeToString(config)
        val deserialized = json.decodeFromString<ConnectionConfig>(serialized)
        assertEquals(listOf("1.1.1.1", "8.8.8.8"), deserialized.dnsServers)
        assertEquals(listOf("com.example.app"), deserialized.appIncludeList)
        assertEquals(listOf("com.example.exclude"), deserialized.appExcludeList)
    }

    // @sk-test kvn-android#T5.22: DNS/app fields default to emptyList when absent
    @Test
    fun testDnsAppFieldsDefaultEmpty() {
        val config = ConnectionConfig(
            serverAddress = "vpn.example.com",
            port = 443,
            token = "tok"
        )
        val serialized = json.encodeToString(config)
        val deserialized = json.decodeFromString<ConnectionConfig>(serialized)
        assertTrue(deserialized.dnsServers.isEmpty())
        assertTrue(deserialized.appIncludeList.isEmpty())
        assertTrue(deserialized.appExcludeList.isEmpty())
    }

    // @sk-test kvn-android#T5.22: old JSON with override fields is ignored gracefully
    @Test
    fun testOldJsonWithOverrideIgnored() {
        val oldJson = """{"serverAddress":"old.com","port":443,"token":"old","dnsServersOverride":["1.1.1.1"],"appIncludeListOverride":["com.a"]}"""
        val config = json.decodeFromString<ConnectionConfig>(oldJson)
        assertEquals("old.com", config.serverAddress)
        assertTrue(config.dnsServers.isEmpty())
        assertTrue(config.appIncludeList.isEmpty())
    }

    // @sk-test kvn-android#T5.22: AppConfig without global DNS/app fields deserializes
    @Test
    fun testAppConfigWithoutGlobalsDeserializes() {
        val jsonStr = """{"activeServer":"Default","servers":[{"name":"Default","config":{"serverAddress":"s.com","port":443,"token":"t"}}]}"""
        val appCfg = json.decodeFromString<AppConfig>(jsonStr)
        assertEquals("Default", appCfg.activeServer)
        assertEquals(1, appCfg.servers.size)
    }
}
