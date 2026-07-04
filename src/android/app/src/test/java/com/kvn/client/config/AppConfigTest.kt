package com.kvn.client.config

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import org.junit.Assert.*
import org.junit.Test

private val json = Json { encodeDefaults = true; ignoreUnknownKeys = true }

// @sk-task kvn-android#T5.22: per-server DNS/app fields — direct config usage (no override)
class AppConfigTest {

    // @sk-test kvn-android#T5.22: server config stores and retrieves DNS/app fields
    @Test
    fun testServerConfigDnsAppFields() {
        val config = ConnectionConfig(
            serverAddress = "s.example.com",
            token = "t",
            dnsServers = listOf("1.1.1.1", "8.8.8.8"),
            appIncludeList = listOf("com.work.app"),
            appExcludeList = listOf("com.bad.app")
        )
        assertEquals(listOf("1.1.1.1", "8.8.8.8"), config.dnsServers)
        assertEquals(listOf("com.work.app"), config.appIncludeList)
        assertEquals(listOf("com.bad.app"), config.appExcludeList)
    }

    // @sk-test kvn-android#T5.22: empty defaults for DNS/app fields
    @Test
    fun testEmptyDefaults() {
        val config = ConnectionConfig(serverAddress = "s.com", token = "t")
        assertTrue(config.dnsServers.isEmpty())
        assertTrue(config.appIncludeList.isEmpty())
        assertTrue(config.appExcludeList.isEmpty())
    }

    // @sk-test kvn-android#T5.22: AppConfig without global fields works
    @Test
    fun testAppConfigWithoutGlobalFields() {
        val appCfg = AppConfig(
            activeServer = "Work",
            servers = listOf(
                ServerEntry("Work", ConnectionConfig(serverAddress = "w.com", token = "t"))
            )
        )
        assertEquals("Work", appCfg.activeServer)
        assertEquals(1, appCfg.servers.size)
        assertEquals("w.com", appCfg.servers[0].config.serverAddress)
    }

    // @sk-test kvn-android#T5.22: import copies DNS/app from source to target server
    @Test
    fun testImportCopiesDnsApp() {
        val source = ConnectionConfig(
            serverAddress = "src.com",
            token = "s",
            dnsServers = listOf("9.9.9.9"),
            appIncludeList = listOf("com.src"),
            appExcludeList = listOf("com.src.bad")
        )
        val target = ConnectionConfig(
            serverAddress = "dst.com",
            token = "d"
        )
        val imported = target.copy(
            dnsServers = source.dnsServers,
            appIncludeList = source.appIncludeList,
            appExcludeList = source.appExcludeList
        )
        assertEquals(listOf("9.9.9.9"), imported.dnsServers)
        assertEquals(listOf("com.src"), imported.appIncludeList)
        assertEquals(listOf("com.src.bad"), imported.appExcludeList)
    }
}
