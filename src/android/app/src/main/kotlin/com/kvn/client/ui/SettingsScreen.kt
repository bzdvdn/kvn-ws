package com.kvn.client.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.kvn.client.config.ConnectionConfig

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(vm: MainViewModel = viewModel()) {
    val activeCfg by vm.activeServerConfig.collectAsState()
    var dns by remember(activeCfg) { mutableStateOf(activeCfg?.dnsServers?.joinToString(",") ?: "") }
    var includeApps by remember(activeCfg) { mutableStateOf(activeCfg?.appIncludeList?.joinToString(",") ?: "") }
    var excludeDomains by remember(activeCfg) { mutableStateOf(activeCfg?.routingExcludeDomains?.joinToString(",") ?: "") }
    var excludeApps by remember(activeCfg) { mutableStateOf(activeCfg?.appExcludeList?.joinToString(",") ?: "") }
    var keepAwake by remember(activeCfg) { mutableStateOf(activeCfg?.keepAwakeEnabled ?: false) }
    var logLevel by remember(activeCfg) { mutableStateOf(activeCfg?.logLevel ?: "info") }

    fun save() {
        val base = activeCfg ?: ConnectionConfig()
        vm.saveCurrentServerConfig(
            base.copy(
                dnsServers = dns.split(",").map { it.trim() }.filter { it.isNotBlank() },
                appIncludeList = includeApps.split(",").map { it.trim() }.filter { it.isNotBlank() },
                appExcludeList = excludeApps.split(",").map { it.trim() }.filter { it.isNotBlank() },
                routingExcludeDomains = excludeDomains.split(",").map { it.trim() }.filter { it.isNotBlank() },
                keepAwakeEnabled = keepAwake,
                logLevel = logLevel,
                mtu = 1200,
            )
        )
    }

    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text("KVN v2 settings", style = MaterialTheme.typography.headlineSmall)
        Text(
            "TUN address and gateway are assigned by the server.",
            style = MaterialTheme.typography.bodyMedium,
        )

        OutlinedTextField(
            value = dns,
            onValueChange = { dns = it },
            label = { Text("DNS servers (comma-separated)") },
            modifier = Modifier.fillMaxWidth(),
            singleLine = true,
        )
        OutlinedTextField(
            value = includeApps,
            onValueChange = { includeApps = it },
            label = { Text("Included apps (packages)") },
            modifier = Modifier.fillMaxWidth(),
            singleLine = true,
        )
        OutlinedTextField(
            value = excludeApps,
            onValueChange = { excludeApps = it },
            label = { Text("Excluded apps (packages)") },
            modifier = Modifier.fillMaxWidth(),
            singleLine = true,
        )
        OutlinedTextField(
            value = excludeDomains,
            onValueChange = { excludeDomains = it },
            label = { Text("Direct domains (e.g. .ru,.su)") },
            modifier = Modifier.fillMaxWidth(),
            singleLine = true,
        )

        Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
            Switch(checked = keepAwake, onCheckedChange = { keepAwake = it })
            Spacer(Modifier.width(8.dp))
            Text("Keep awake")
        }

        Text("Log level", style = MaterialTheme.typography.titleSmall)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("debug", "info", "warn", "error").forEach { level ->
                FilterChip(
                    selected = logLevel == level,
                    onClick = { logLevel = level },
                    label = { Text(level) },
                )
            }
        }

        Button(onClick = { save() }, modifier = Modifier.fillMaxWidth()) {
            Text("Save")
        }
    }
}
