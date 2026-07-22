package com.kvn.client.ui

import android.content.pm.PackageManager
import android.widget.Toast
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.kvn.client.config.ConnectionConfig
import com.kvn.client.ui.theme.KvnPrimary
import com.kvn.client.ui.theme.KvnSuccess
import com.kvn.client.vpn.KvnVpnService

@OptIn(ExperimentalMaterial3Api::class)
@Composable
// @sk-task android-log-tag#T2.4: removed old AlertDialog log viewer (AC-012)
fun SettingsScreen(vm: MainViewModel = viewModel()) {
    val context = LocalContext.current
    val activeCfg by vm.activeServerConfig.collectAsState()
    val activeName by vm.activeServerName.collectAsState()
    val serverList by vm.servers.collectAsState()

    var showAppPicker by remember { mutableStateOf(false) }
    var appPickerModeAllow by remember { mutableStateOf(true) }

    // Per-server DNS state
    var editDns by remember(activeCfg) {
        mutableStateOf(activeCfg?.dnsServers?.joinToString(",") ?: "")
    }

    // Per-server app filter state
    var editAppInclude by remember(activeCfg) {
        mutableStateOf(activeCfg?.appIncludeList?.joinToString(",") ?: "")
    }
    var editAppExclude by remember(activeCfg) {
        mutableStateOf(activeCfg?.appExcludeList?.joinToString(",") ?: "")
    }

    // Per-server config state
    var mtu by remember(activeCfg) { mutableStateOf((activeCfg?.mtu ?: 1500).toString()) }
    var ipv6Enabled by remember(activeCfg) { mutableStateOf(activeCfg?.ipv6Enabled ?: false) }
    var autoReconnect by remember(activeCfg) { mutableStateOf(activeCfg?.autoReconnect ?: true) }
    var logLevel by remember(activeCfg) { mutableStateOf(activeCfg?.logLevel ?: "info") }
    var maxMessageSize by remember(activeCfg) { mutableStateOf((activeCfg?.maxMessageSize ?: 65535).toString()) }
    var multiplex by remember(activeCfg) { mutableStateOf(activeCfg?.multiplex ?: false) }
    var minBackoffSec by remember(activeCfg) { mutableStateOf((activeCfg?.minBackoffSec ?: 1).toString()) }
    var maxBackoffSec by remember(activeCfg) { mutableStateOf((activeCfg?.maxBackoffSec ?: 30).toString()) }
    var tlsVerifyMode by remember(activeCfg) { mutableStateOf(activeCfg?.tlsVerifyMode ?: "verify") }
    var tlsServerName by remember(activeCfg) { mutableStateOf(activeCfg?.tlsServerName ?: "") }
    var routingIncludeRanges by remember(activeCfg) { mutableStateOf(activeCfg?.routingIncludeRanges?.joinToString(",") ?: "") }
    var routingExcludeRanges by remember(activeCfg) { mutableStateOf(activeCfg?.routingExcludeRanges?.joinToString(",") ?: "") }
    var routingIncludeIps by remember(activeCfg) { mutableStateOf(activeCfg?.routingIncludeIps?.joinToString(",") ?: "") }
    var routingExcludeIps by remember(activeCfg) { mutableStateOf(activeCfg?.routingExcludeIps?.joinToString(",") ?: "") }
    var routingIncludeSources by remember(activeCfg) { mutableStateOf(activeCfg?.routingIncludeSources ?: "") }
    var routingExcludeSources by remember(activeCfg) { mutableStateOf(activeCfg?.routingExcludeSources ?: "") }
    var geoipPath by remember(activeCfg) { mutableStateOf(activeCfg?.geoipPath ?: "") }
    var geoipUrl by remember(activeCfg) { mutableStateOf(activeCfg?.geoipUrl ?: "") }
    var geositePath by remember(activeCfg) { mutableStateOf(activeCfg?.geositePath ?: "") }
    var geositeUrl by remember(activeCfg) { mutableStateOf(activeCfg?.geositeUrl ?: "") }
    var sourceTtlHours by remember(activeCfg) { mutableStateOf((activeCfg?.sourceTtlHours ?: 24).toString()) }
    var cryptoEnabled by remember(activeCfg) { mutableStateOf(activeCfg?.cryptoEnabled ?: false) }
    var cryptoKey by remember(activeCfg) { mutableStateOf(activeCfg?.cryptoKey ?: "") }
    var killSwitchEnabled by remember(activeCfg) { mutableStateOf(activeCfg?.killSwitchEnabled ?: false) }
    var obfuscationEnabled by remember(activeCfg) { mutableStateOf(activeCfg?.obfuscationEnabled ?: false) }
    var obfuscationUtls by remember(activeCfg) { mutableStateOf(activeCfg?.obfuscationUtls ?: false) }
    var obfuscationPaddingEnabled by remember(activeCfg) { mutableStateOf(activeCfg?.obfuscationPaddingEnabled ?: false) }
    var obfuscationPaddingSize by remember(activeCfg) { mutableStateOf((activeCfg?.obfuscationPaddingSize ?: 0).toString()) }
    var dnsCacheEnabled by remember(activeCfg) { mutableStateOf(activeCfg?.dnsCacheEnabled ?: false) }

    // @sk-task doze-resilience#T3.2: keep-awake toggle for screen-off stability (AC-007)
    var keepAwakeEnabled by remember(activeCfg) { mutableStateOf(activeCfg?.keepAwakeEnabled ?: false) }

    // DNS routing state
    var routingDomainsEnabled by remember(activeCfg) { mutableStateOf(activeCfg?.routingDomainsEnabled ?: false) }
    var routingExcludeDomains by remember(activeCfg) { mutableStateOf(activeCfg?.routingExcludeDomains?.joinToString(",") ?: "") }
    var routingIncludeDomains by remember(activeCfg) { mutableStateOf(activeCfg?.routingIncludeDomains?.joinToString(",") ?: "") }

    fun buildServerConfig(): ConnectionConfig {
        val base = activeCfg ?: ConnectionConfig()
        return base.copy(
            dnsServers = editDns.split(",").map { it.trim() }.filter { it.isNotBlank() },
            appIncludeList = editAppInclude.split(",").map { it.trim() }.filter { it.isNotBlank() },
            appExcludeList = editAppExclude.split(",").map { it.trim() }.filter { it.isNotBlank() },
            mtu = mtu.toIntOrNull() ?: 1500,
            ipv6Enabled = ipv6Enabled,
            autoReconnect = autoReconnect,
            logLevel = logLevel,
            maxMessageSize = maxMessageSize.toIntOrNull() ?: 65535,
            multiplex = multiplex,
            minBackoffSec = minBackoffSec.toIntOrNull() ?: 1,
            maxBackoffSec = maxBackoffSec.toIntOrNull() ?: 30,
            tlsVerifyMode = tlsVerifyMode,
            tlsServerName = tlsServerName,
            routingIncludeRanges = routingIncludeRanges.split(",").filter { it.isNotBlank() },
            routingExcludeRanges = routingExcludeRanges.split(",").filter { it.isNotBlank() },
            routingIncludeIps = routingIncludeIps.split(",").filter { it.isNotBlank() },
            routingExcludeIps = routingExcludeIps.split(",").filter { it.isNotBlank() },
            routingIncludeSources = routingIncludeSources,
            routingExcludeSources = routingExcludeSources,
            geoipPath = geoipPath,
            geoipUrl = geoipUrl,
            geositePath = geositePath,
            geositeUrl = geositeUrl,
            sourceTtlHours = sourceTtlHours.toIntOrNull() ?: 24,
            cryptoEnabled = cryptoEnabled,
            cryptoKey = cryptoKey,
            killSwitchEnabled = killSwitchEnabled,
            obfuscationEnabled = obfuscationEnabled,
            obfuscationUtls = obfuscationUtls,
            obfuscationPaddingEnabled = obfuscationPaddingEnabled,
            obfuscationPaddingSize = obfuscationPaddingSize.toIntOrNull() ?: 0,
            dnsCacheEnabled = dnsCacheEnabled,
            routingDomainsEnabled = routingDomainsEnabled,
            routingExcludeDomains = routingExcludeDomains.split(",").map { it.trim() }.filter { it.isNotBlank() },
            routingIncludeDomains = routingIncludeDomains.split(",").map { it.trim() }.filter { it.isNotBlank() },
            keepAwakeEnabled = keepAwakeEnabled
        )
    }

    val importTargets = serverList.filter { it.name != activeName }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp)
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        Text(
            text = "Settings",
            style = MaterialTheme.typography.headlineSmall
        )
        Text(
            text = "Server: $activeName",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )

        // DNS
        SettingsSection(title = "DNS Servers") {
            OutlinedTextField(
                value = editDns,
                onValueChange = { editDns = it },
                label = { Text("DNS Servers (comma-separated)") },
                placeholder = { Text("1.1.1.1,8.8.8.8") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            if (importTargets.isNotEmpty()) {
                ImportFromServerSection(
                    label = "Import DNS from",
                    servers = importTargets,
                    onImport = { sourceName ->
                        val source = serverList.find { it.name == sourceName }?.config
                        if (source != null) {
                            editDns = source.dnsServers.joinToString(",")
                            Toast.makeText(context, "DNS imported from $sourceName: ${source.dnsServers.size} server(s)", Toast.LENGTH_SHORT).show()
                        }
                    }
                )
            }
        }

        // DNS Routing
        SettingsSection(title = "DNS Routing") {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Domain-based Routing", modifier = Modifier.weight(1f))
                Switch(checked = routingDomainsEnabled, onCheckedChange = { routingDomainsEnabled = it })
            }
            if (routingDomainsEnabled) {
                OutlinedTextField(
                    value = routingExcludeDomains,
                    onValueChange = { routingExcludeDomains = it },
                    label = { Text("Exclude Domains (suffixes, comma-separated)") },
                    placeholder = { Text(".ru,.org") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(modifier = Modifier.height(4.dp))
                OutlinedTextField(
                    value = routingIncludeDomains,
                    onValueChange = { routingIncludeDomains = it },
                    label = { Text("Include Domains (suffixes, comma-separated)") },
                    placeholder = { Text(".corp,.internal") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
            }
        }

        // Per-app filtering
        SettingsSection(title = "Per-App Filtering") {
            Text("Mode", style = MaterialTheme.typography.bodyMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(
                    selected = appPickerModeAllow,
                    onClick = { appPickerModeAllow = true },
                    label = { Text("Allowed apps") }
                )
                FilterChip(
                    selected = !appPickerModeAllow,
                    onClick = { appPickerModeAllow = false },
                    label = { Text("Blocked apps") }
                )
            }
            val currentList = if (appPickerModeAllow) editAppInclude else editAppExclude
            val count = if (currentList.isBlank()) 0 else currentList.split(",").size
            OutlinedButton(
                onClick = { showAppPicker = true },
                modifier = Modifier.fillMaxWidth()
            ) {
                Text(if (count > 0) "Selected: $count apps" else "Select apps")
            }
            if (count > 0) {
                val packages = currentList.split(",").map { it.trim() }.filter { it.isNotBlank() }
                val displayPackages = packages.take(10)
                val packageLabels = remember(currentList) {
                    val pm = context.packageManager
                    displayPackages.associateWith { pkg ->
                        try {
                            val ai = pm.getApplicationInfo(pkg, PackageManager.GET_META_DATA)
                            pm.getApplicationLabel(ai).toString()
                        } catch (_: Exception) { pkg }
                    }
                }
                Column(modifier = Modifier.padding(top = 4.dp)) {
                    for (pkg in displayPackages) {
                        Text(
                            text = packageLabels[pkg] ?: pkg,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                    if (packages.size > 10) {
                        Text(
                            text = "+ ${packages.size - 10} more",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.primary,
                            modifier = Modifier.padding(top = 2.dp)
                        )
                    }
                }
            }
            if (importTargets.isNotEmpty()) {
                ImportFromServerSection(
                    label = "Import app filters from",
                    servers = importTargets,
                    onImport = { sourceName ->
                        val source = serverList.find { it.name == sourceName }?.config
                        if (source != null) {
                            editAppInclude = source.appIncludeList.joinToString(",")
                            editAppExclude = source.appExcludeList.joinToString(",")
                            Toast.makeText(context, "App filters imported from $sourceName: ${source.appIncludeList.size} allowed + ${source.appExcludeList.size} blocked", Toast.LENGTH_SHORT).show()
                        }
                    }
                )
            }
        }

        // Advanced section
        SettingsSection(title = "Advanced") {
            OutlinedTextField(
                value = mtu,
                onValueChange = { mtu = it },
                label = { Text("MTU") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth()
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("IPv6", modifier = Modifier.weight(1f))
                Switch(checked = ipv6Enabled, onCheckedChange = { ipv6Enabled = it })
            }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Auto Reconnect", modifier = Modifier.weight(1f))
                Switch(checked = autoReconnect, onCheckedChange = { autoReconnect = it })
            }
            OutlinedTextField(
                value = logLevel,
                onValueChange = { logLevel = it },
                label = { Text("Log Level") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = maxMessageSize,
                onValueChange = { maxMessageSize = it },
                label = { Text("Max Message Size") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth()
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Multiplex", modifier = Modifier.weight(1f))
                Switch(checked = multiplex, onCheckedChange = { multiplex = it })
            }
            // @sk-task doze-resilience#T3.2: keep-awake toggle (AC-007)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Keep awake on screen off", modifier = Modifier.weight(1f))
                Switch(checked = keepAwakeEnabled, onCheckedChange = { keepAwakeEnabled = it })
            }
        }

        // Reconnect section
        SettingsSection(title = "Reconnect") {
            OutlinedTextField(
                value = minBackoffSec,
                onValueChange = { minBackoffSec = it },
                label = { Text("Min Backoff (sec)") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = maxBackoffSec,
                onValueChange = { maxBackoffSec = it },
                label = { Text("Max Backoff (sec)") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth()
            )
        }

        // TLS section
        SettingsSection(title = "TLS") {
            Text("Verify Mode", style = MaterialTheme.typography.bodyMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(
                    selected = tlsVerifyMode == "verify",
                    onClick = { tlsVerifyMode = "verify" },
                    label = { Text("Verify") }
                )
                FilterChip(
                    selected = tlsVerifyMode == "insecure",
                    onClick = { tlsVerifyMode = "insecure" },
                    label = { Text("Insecure") }
                )
                FilterChip(
                    selected = tlsVerifyMode == "none",
                    onClick = { tlsVerifyMode = "none" },
                    label = { Text("None") }
                )
            }
            OutlinedTextField(
                value = tlsServerName,
                onValueChange = { tlsServerName = it },
                label = { Text("Server Name (SNI)") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
        }

        // Routing section
        SettingsSection(title = "Routing") {
            OutlinedTextField(
                value = routingIncludeRanges,
                onValueChange = { routingIncludeRanges = it },
                label = { Text("Include Ranges (CIDR, comma-separated)") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = routingExcludeRanges,
                onValueChange = { routingExcludeRanges = it },
                label = { Text("Exclude Ranges (CIDR, comma-separated)") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = routingIncludeIps,
                onValueChange = { routingIncludeIps = it },
                label = { Text("Include IPs (comma-separated)") },
                placeholder = { Text("10.0.0.1,192.168.1.0/24") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = routingExcludeIps,
                onValueChange = { routingExcludeIps = it },
                label = { Text("Exclude IPs (comma-separated)") },
                placeholder = { Text("10.0.0.2,192.168.1.100") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = routingIncludeSources,
                onValueChange = { routingIncludeSources = it },
                label = { Text("Include Sources (type:value, comma-separated)") },
                placeholder = { Text("geoip:RU,cidr:10.0.0.0/8") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = routingExcludeSources,
                onValueChange = { routingExcludeSources = it },
                label = { Text("Exclude Sources (type:value, comma-separated)") },
                placeholder = { Text("geoip:CN,cidr:192.168.0.0/16") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = geoipPath,
                onValueChange = { geoipPath = it },
                label = { Text("GeoIP Database Path") },
                placeholder = { Text("/data/geoip.dat") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = geoipUrl,
                onValueChange = { geoipUrl = it },
                label = { Text("GeoIP Database URL") },
                placeholder = { Text("https://example.com/geoip.dat") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = geositePath,
                onValueChange = { geositePath = it },
                label = { Text("GeoSite Database Path") },
                placeholder = { Text("/data/geosite.dat") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = geositeUrl,
                onValueChange = { geositeUrl = it },
                label = { Text("GeoSite Database URL") },
                placeholder = { Text("https://example.com/geosite.dat") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )
            OutlinedTextField(
                value = sourceTtlHours,
                onValueChange = { sourceTtlHours = it },
                label = { Text("Source TTL (hours)") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth()
            )
            Spacer(modifier = Modifier.height(8.dp))
            Button(
                onClick = { vm.refreshSources(buildServerConfig()) },
                modifier = Modifier.fillMaxWidth().height(44.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = KvnPrimary,
                    contentColor = Color.White
                )
            ) { Text("Refresh Sources") }
        }

        // Encryption section
        SettingsSection(title = "Encryption") {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Enabled", modifier = Modifier.weight(1f))
                Switch(checked = cryptoEnabled, onCheckedChange = { cryptoEnabled = it })
            }
            if (cryptoEnabled) {
                OutlinedTextField(
                    value = cryptoKey,
                    onValueChange = { cryptoKey = it },
                    label = { Text("Encryption Key") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth()
                )
            }
        }

        // @sk-task android-latency-power-fix#T1.1: battery exemption button in UI (AC-001)
        SettingsSection(title = "Battery") {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Ignore battery optimizations", modifier = Modifier.weight(1f))
                Button(onClick = {
                    KvnVpnService.requestBatteryExemption(context)
                }) { Text("Request") }
            }
        }

        // Kill Switch section
        SettingsSection(title = "Kill Switch") {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Enabled", modifier = Modifier.weight(1f))
                Switch(checked = killSwitchEnabled, onCheckedChange = { killSwitchEnabled = it })
            }
        }

        // Obfuscation section
        SettingsSection(title = "Obfuscation") {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Enabled", modifier = Modifier.weight(1f))
                Switch(checked = obfuscationEnabled, onCheckedChange = { obfuscationEnabled = it })
            }
            if (obfuscationEnabled) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("uTLS", modifier = Modifier.weight(1f))
                    Switch(checked = obfuscationUtls, onCheckedChange = { obfuscationUtls = it })
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Padding", modifier = Modifier.weight(1f))
                    Switch(checked = obfuscationPaddingEnabled, onCheckedChange = { obfuscationPaddingEnabled = it })
                }
                if (obfuscationPaddingEnabled) {
                    OutlinedTextField(
                        value = obfuscationPaddingSize,
                        onValueChange = { obfuscationPaddingSize = it },
                        label = { Text("Padding Size") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            }
        }

        // Save Config button
        Button(
            onClick = {
                if (activeCfg != null) {
                    vm.saveCurrentServerConfig(buildServerConfig())
                    Toast.makeText(context, "Server config saved", Toast.LENGTH_SHORT).show()
                }
            },
            modifier = Modifier.fillMaxWidth().height(48.dp),
            colors = ButtonDefaults.buttonColors(
                containerColor = KvnSuccess,
                contentColor = Color.White
            )
        ) { Text("Save Server Config") }

        Spacer(modifier = Modifier.height(32.dp))
    }

    // App picker screen
    if (showAppPicker) {
        val currentList = if (appPickerModeAllow) editAppInclude else editAppExclude
        val initialSet = if (currentList.isBlank()) emptySet()
        else currentList.split(",").map { it.trim() }.filter { it.isNotBlank() }.toSet()
        AppPickerScreen(
            initialSelection = initialSet,
            onSave = { selected ->
                val joined = selected.joinToString(",")
                if (appPickerModeAllow) editAppInclude = joined
                else editAppExclude = joined
                showAppPicker = false
            },
            onBack = { showAppPicker = false }
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ImportFromServerSection(
    label: String,
    servers: List<com.kvn.client.config.ServerEntry>,
    onImport: (sourceName: String) -> Unit
) {
    var expanded by remember { mutableStateOf(false) }
    var selected by remember { mutableStateOf("") }

    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = it }
    ) {
        OutlinedTextField(
            value = selected,
            onValueChange = {},
            readOnly = true,
            label = { Text(label) },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
            modifier = Modifier.menuAnchor().fillMaxWidth()
        )
        ExposedDropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false }
        ) {
            servers.forEach { entry ->
                DropdownMenuItem(
                    text = { Text(entry.name) },
                    onClick = {
                        selected = entry.name
                        expanded = false
                    }
                )
            }
        }
    }
    if (selected.isNotBlank()) {
        Button(
            onClick = {
                onImport(selected)
                selected = ""
            },
            modifier = Modifier.fillMaxWidth()
        ) { Text("Import from $selected") }
    }
}
