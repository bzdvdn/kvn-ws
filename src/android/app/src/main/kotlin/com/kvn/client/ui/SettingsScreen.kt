package com.kvn.client.ui

import android.widget.Toast
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.kvn.client.config.ConnectionConfig

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SettingsCard(
    title: String,
    subtitle: String? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.45f),
            contentColor = MaterialTheme.colorScheme.onSurface,
        ),
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
            )
            if (subtitle != null) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            content()
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(vm: MainViewModel = viewModel()) {
    val context = LocalContext.current
    val activeCfg by vm.activeServerConfig.collectAsState()
    val activeName by vm.activeServerName.collectAsState()
    val serverList by vm.servers.collectAsState()
    val importTargets = serverList.filter { it.name != activeName }

    var dns by remember(activeCfg) { mutableStateOf(activeCfg?.dnsServers?.joinToString(", ") ?: "") }
    var includeApps by remember(activeCfg) { mutableStateOf(activeCfg?.appIncludeList?.joinToString(", ") ?: "") }
    var excludeApps by remember(activeCfg) { mutableStateOf(activeCfg?.appExcludeList?.joinToString(", ") ?: "") }
    var excludeDomains by remember(activeCfg) { mutableStateOf(activeCfg?.routingExcludeDomains?.joinToString(", ") ?: "") }
    var keepAwake by remember(activeCfg) { mutableStateOf(activeCfg?.keepAwakeEnabled ?: false) }
    var logLevel by remember(activeCfg) { mutableStateOf(activeCfg?.logLevel ?: "info") }
    var insecureTls by remember(activeCfg) { mutableStateOf(activeCfg?.tlsVerifyMode == "insecure") }
    var showAppPicker by remember { mutableStateOf(false) }
    var appPickerInclude by remember { mutableStateOf(true) }
    var showImportDialog by remember { mutableStateOf(false) }

    fun csv(value: String): List<String> =
        value.split(",").map { it.trim() }.filter { it.isNotBlank() }

    fun save() {
        val base = activeCfg ?: ConnectionConfig()
        vm.saveCurrentServerConfig(
            base.copy(
                dnsServers = csv(dns),
                appIncludeList = csv(includeApps),
                appExcludeList = csv(excludeApps),
                routingExcludeDomains = csv(excludeDomains),
                keepAwakeEnabled = keepAwake,
                logLevel = logLevel,
                tlsVerifyMode = if (insecureTls) "insecure" else "verify",
                mtu = 1200,
            )
        )
        Toast.makeText(context, "Settings saved", Toast.LENGTH_SHORT).show()
    }

    if (showAppPicker) {
        AppPickerScreen(
            initialSelection = if (appPickerInclude) csv(includeApps).toSet() else csv(excludeApps).toSet(),
            onSave = { selected ->
                val value = selected.joinToString(", ")
                if (appPickerInclude) includeApps = value else excludeApps = value
                showAppPicker = false
            },
            onBack = { showAppPicker = false },
        )
        return
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            text = "KVN settings",
            style = MaterialTheme.typography.headlineMedium,
            fontWeight = FontWeight.Bold,
        )
        Text(
            text = activeCfg?.let { "${it.serverAddress}:${it.port}" } ?: "No active server",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        SettingsCard(
            title = "Tunnel",
            subtitle = "TUN address and gateway are assigned by the server.",
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text("Keep awake", style = MaterialTheme.typography.bodyLarge)
                    Text(
                        "Stable connection while the screen is off",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(checked = keepAwake, onCheckedChange = { keepAwake = it })
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text("Allow insecure TLS", style = MaterialTheme.typography.bodyLarge)
                    Text(
                        "For self-signed certificates; disable for production",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(checked = insecureTls, onCheckedChange = { insecureTls = it })
            }
        }

        SettingsCard(
            title = "DNS",
            subtitle = "Comma-separated resolvers used inside the tunnel.",
        ) {
            OutlinedTextField(
                value = dns,
                onValueChange = { dns = it },
                label = { Text("DNS servers") },
                placeholder = { Text("1.1.1.1, 8.8.8.8") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                textStyle = MaterialTheme.typography.bodyLarge,
            )
            OutlinedTextField(
                value = excludeDomains,
                onValueChange = { excludeDomains = it },
                label = { Text("Direct domains") },
                placeholder = { Text(".ru, .su") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                textStyle = MaterialTheme.typography.bodyLarge,
            )
        }

        SettingsCard(
            title = "Applications",
            subtitle = "Choose whole apps or leave the lists empty to route all apps.",
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(
                    onClick = { appPickerInclude = true; showAppPicker = true },
                    modifier = Modifier.weight(1f),
                ) {
                    Text("Included (${csv(includeApps).size})")
                }
                OutlinedButton(
                    onClick = { appPickerInclude = false; showAppPicker = true },
                    modifier = Modifier.weight(1f),
                ) {
                    Text("Excluded (${csv(excludeApps).size})")
                }
            }
            if (importTargets.isNotEmpty()) {
                OutlinedButton(
                    onClick = { showImportDialog = true },
                    modifier = Modifier.fillMaxWidth(),
                ) { Text("Import DNS / routing from another server") }
            }
            Text(
                "Tap a button to review or change the selection.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        SettingsCard(
            title = "Diagnostics",
            subtitle = "Debug shows session, DNS/direct and stream details in kvn-web.",
        ) {
            Text("Log level", style = MaterialTheme.typography.bodyLarge)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf("debug", "info", "warn", "error").forEach { level ->
                    FilterChip(
                        selected = logLevel == level,
                        onClick = { logLevel = level },
                        label = { Text(level) },
                    )
                }
            }
        }

        if (showImportDialog && importTargets.isNotEmpty()) {
            AlertDialog(
                onDismissRequest = { showImportDialog = false },
                title = { Text("Import from another server") },
                text = {
                    Column {
                        importTargets.forEach { entry ->
                            TextButton(
                                onClick = {
                                    vm.importFromServer(entry.name)
                                    showImportDialog = false
                                },
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(entry.name, modifier = Modifier.fillMaxWidth())
                            }
                        }
                    }
                },
                confirmButton = {},
                dismissButton = {
                    TextButton(onClick = { showImportDialog = false }) { Text("Cancel") }
                },
            )
        }

        Button(
            onClick = { save() },
            modifier = Modifier.fillMaxWidth().height(50.dp),
            shape = RoundedCornerShape(14.dp),
        ) {
            Text("Save settings", style = MaterialTheme.typography.titleSmall)
        }
        Spacer(Modifier.height(8.dp))
    }
}
