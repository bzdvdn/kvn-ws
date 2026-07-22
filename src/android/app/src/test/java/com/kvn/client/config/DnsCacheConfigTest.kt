package com.kvn.client.config

import com.kvn.client.ui.configToWebJson
import com.kvn.client.ui.parseQrConfig
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

private val json = Json { encodeDefaults = true }

// @sk-test android-dns-cache#T5.2: dnsCacheEnabled serialization round-trip (AC-008)
class DnsCacheConfigSerializationTest {

    @Test
    fun testDnsCacheEnabledRoundTrip() {
        val config = ConnectionConfig(
            serverAddress = "vpn.example.com",
            port = 8443,
            token = "token",
            dnsCacheEnabled = true
        )
        val serialized = json.encodeToString(config)
        val deserialized = json.decodeFromString<ConnectionConfig>(serialized)
        assertTrue(deserialized.dnsCacheEnabled)
    }

    @Test
    fun testDnsCacheEnabledDefaultsToFalse() {
        val config = ConnectionConfig()
        assertFalse(config.dnsCacheEnabled)
    }

    @Test
    fun testDnsCacheEnabledFalseRoundTrip() {
        val config = ConnectionConfig(
            serverAddress = "vpn.example.com",
            port = 8443,
            token = "token",
            dnsCacheEnabled = false
        )
        val serialized = json.encodeToString(config)
        val deserialized = json.decodeFromString<ConnectionConfig>(serialized)
        assertFalse(deserialized.dnsCacheEnabled)
    }

    @Test
    fun testDnsCacheEnabledBackwardCompat() {
        // Old JSON without dnsCacheEnabled field should parse with default false
        val oldJson = """{"serverAddress":"vpn.example.com","port":443,"token":"tok"}"""
        val config = json.decodeFromString<ConnectionConfig>(oldJson)
        assertFalse(config.dnsCacheEnabled)
    }
}

// @sk-test: QR JSON with dns_routing.enabled (web-compat), fallback to dns_cache
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [26])
class DnsCacheQrConfigTest {

    // @sk-test android-web-config-alignment#T1.1: parse dns_routing.enabled + ttl from web JSON
    @Test
    fun testParseWebJsonWithDnsRoutingEnabled() {
        val jsonStr = """{
            "server": "wss://example.com:443/kvn",
            "transport": "tcp",
            "auth": {"token": "tok"},
            "tls": {"verify_mode": "verify", "server_name": "", "sni": []},
            "mtu": 1400, "ipv6": false, "auto_reconnect": true,
            "routing": {
                "default_route": "server",
                "include_ranges": [], "exclude_ranges": [],
                "include_ips": [], "exclude_ips": [],
                "include_domains": [], "exclude_domains": [],
                "dns_routing": {"enabled": true, "ttl": 7200}
            },
            "kill_switch": null, "reconnect": null,
            "mode": "tun", "crypto": {"enabled": false, "key": ""},
            "max_message_size": 65535
        }"""
        val config = parseQrConfig(jsonStr)
        assertNotNull(config)
        assertTrue(config!!.dnsCacheEnabled)
        assertEquals(7200, config.dnsCacheTtl)
    }

    // @sk-test android-web-config-alignment#T1.1: backward compat with dns_cache field name
    @Test
    fun testParseWebJsonWithDnsCacheBackwardCompat() {
        val jsonStr = """{
            "server": "wss://example.com:443/kvn",
            "transport": "tcp",
            "auth": {"token": "tok"},
            "tls": {"verify_mode": "verify", "server_name": "", "sni": []},
            "mtu": 1400, "ipv6": false, "auto_reconnect": true,
            "routing": {
                "default_route": "server",
                "include_ranges": [], "exclude_ranges": [],
                "include_ips": [], "exclude_ips": [],
                "include_domains": [], "exclude_domains": [],
                "dns_cache": {"enabled": true}
            },
            "kill_switch": null, "reconnect": null,
            "mode": "tun", "crypto": {"enabled": false, "key": ""},
            "max_message_size": 65535
        }"""
        val config = parseQrConfig(jsonStr)
        assertNotNull(config)
        assertTrue(config!!.dnsCacheEnabled)
        assertEquals(3600, config.dnsCacheTtl)
    }

    @Test
    fun testParseWebJsonWithoutDnsRouting() {
        val jsonStr = """{
            "server": "wss://example.com:443/kvn",
            "transport": "tcp",
            "auth": {"token": "tok"},
            "tls": {"verify_mode": "verify", "server_name": "", "sni": []},
            "mtu": 1400, "ipv6": false, "auto_reconnect": true,
            "routing": {
                "default_route": "server",
                "include_ranges": [], "exclude_ranges": [],
                "include_ips": [], "exclude_ips": [],
                "include_domains": [], "exclude_domains": []
            },
            "kill_switch": null, "reconnect": null,
            "mode": "tun", "crypto": {"enabled": false, "key": ""},
            "max_message_size": 65535
        }"""
        val config = parseQrConfig(jsonStr)
        assertNotNull(config)
        assertFalse(config!!.dnsCacheEnabled)
        assertEquals(3600, config.dnsCacheTtl)
    }

    // @sk-test android-web-config-alignment#T1.1: export dns_routing (not dns_cache) field name
    @Test
    fun testConfigToWebJsonExportsDnsRouting() {
        val config = ConnectionConfig(
            serverAddress = "example.com",
            port = 443,
            token = "tok",
            dnsCacheEnabled = true,
            dnsCacheTtl = 7200
        )
        val webJson = configToWebJson(config)
        assertTrue("should use dns_routing field name", webJson.contains("dns_routing"))
        assertTrue(webJson.contains("enabled"))
        assertTrue(webJson.contains("true"))
        assertTrue(webJson.contains("\"ttl\":7200") || webJson.contains("\"ttl\": 7200"))
    }

    @Test
    fun testConfigToWebJsonDnsRoutingFalse() {
        val config = ConnectionConfig(
            serverAddress = "example.com",
            port = 443,
            token = "tok",
            dnsCacheEnabled = false
        )
        val webJson = configToWebJson(config)
        assertTrue("should use dns_routing field name", webJson.contains("dns_routing"))
        assertTrue(webJson.contains("enabled"))
        assertTrue(webJson.contains("false"))
    }
}
