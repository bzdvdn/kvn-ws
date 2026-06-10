package com.kvn.client.config

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
            obfuscationPaddingSize = 0
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
    }
}
