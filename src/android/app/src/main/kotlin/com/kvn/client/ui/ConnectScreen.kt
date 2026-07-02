package com.kvn.client.ui

import android.Manifest
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.kvn.client.ui.theme.KvnError
import com.kvn.client.ui.theme.KvnPrimary
import com.kvn.client.ui.theme.KvnSuccess
import com.kvn.client.ui.theme.KvnWarning
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewmodel.compose.viewModel
import com.kvn.client.config.ConnectionConfig
import com.kvn.client.config.ServerEntry
import com.kvn.client.transport.ConnectionState

// @sk-task multi-server-android-client#T3.2: pending action for dirty dialog (AC-003, AC-008)
private sealed class PendingAction {
    class SwitchServer(val name: String) : PendingAction()
    object Duplicate : PendingAction()
}

// @sk-task kvn-android#T5.3: main connect screen with collapsible sections (AC-001, DEC-007)
private fun parseServerUrl(url: String): Triple<String, Int, String> {
    var host = url.trim()
    var port = 443
    var path = "/kvn"
    try {
        val noScheme = host.substringAfter("://")
        val slashIdx = noScheme.indexOf("/")
        val hostPort = if (slashIdx >= 0) noScheme.substring(0, slashIdx) else noScheme
        if (slashIdx >= 0) path = noScheme.substring(slashIdx)
        val parts = hostPort.split(":")
        host = parts[0]
        if (parts.size > 1) port = parts[1].toIntOrNull() ?: 443
    } catch (_: Exception) { }
    return Triple(host, port, path)
}

private fun formatBytes(bytes: Long): String {
    return when {
        bytes < 1024 -> "$bytes B"
        bytes < 1024 * 1024 -> "${bytes / 1024} KB"
        bytes < 1024 * 1024 * 1024 -> "${"%.1f".format(bytes.toDouble() / (1024 * 1024))} MB"
        else -> "${"%.2f".format(bytes.toDouble() / (1024 * 1024 * 1024))} GB"
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConnectScreen(vm: MainViewModel = viewModel()) {
    val context = LocalContext.current
    val state by vm.connectionState.collectAsState()
    val serverList by vm.servers.collectAsState()
    val activeCfg by vm.activeServerConfig.collectAsState()
    val activeName by vm.activeServerName.collectAsState()
    val isDirty by vm.isDirty.collectAsState()
    val rxBytes by vm.rxBytes.collectAsState()
    val txBytes by vm.txBytes.collectAsState()
    val errorMessage by vm.errorMessage.collectAsState()

    var showQrScanner by remember { mutableStateOf(false) }
    var showExportQr by remember { mutableStateOf(false) }
    var showDirtyDialog by remember { mutableStateOf(false) }
    var pendingAction by remember { mutableStateOf<PendingAction?>(null) }
    var showDeleteConfirm by remember { mutableStateOf(false) }
    var showRenameDialog by remember { mutableStateOf(false) }
    var renameText by remember { mutableStateOf("") }

    // Connection form state
    var serverUrl by remember { mutableStateOf("") }
    var token by remember { mutableStateOf("") }
    var mode by remember { mutableStateOf("tun") }
    var transport by remember { mutableStateOf("tcp") }
    var mtu by remember { mutableStateOf("1500") }
    var ipv6Enabled by remember { mutableStateOf(false) }
    var autoReconnect by remember { mutableStateOf(true) }
    var logLevel by remember { mutableStateOf("info") }
    var maxMessageSize by remember { mutableStateOf("65535") }
    var multiplex by remember { mutableStateOf(false) }
    var minBackoffSec by remember { mutableStateOf("1") }
    var maxBackoffSec by remember { mutableStateOf("30") }
    var tlsVerifyMode by remember { mutableStateOf("verify") }
    var tlsServerName by remember { mutableStateOf("") }
    var tlsSni by remember { mutableStateOf(listOf<String>()) }
    var routingDefaultRoute by remember { mutableStateOf("server") }
    var routingIncludeRanges by remember { mutableStateOf("") }
    var routingExcludeRanges by remember { mutableStateOf("") }
    var routingIncludeIps by remember { mutableStateOf("") }
    var routingExcludeIps by remember { mutableStateOf("") }
    var routingIncludeDomains by remember { mutableStateOf("") }
    var routingExcludeDomains by remember { mutableStateOf("") }
    // @sk-task geoip-geosite-integration#T5.2: routing source fields (include/exclude sources, paths, TTL)
    var routingIncludeSources by remember { mutableStateOf("") }
    var routingExcludeSources by remember { mutableStateOf("") }
    var geoipPath by remember { mutableStateOf("") }
    var geoipUrl by remember { mutableStateOf("") }
    var geositePath by remember { mutableStateOf("") }
    var geositeUrl by remember { mutableStateOf("") }
    var sourceTtlHours by remember { mutableStateOf("24") }
    var cryptoEnabled by remember { mutableStateOf(false) }
    var cryptoKey by remember { mutableStateOf("") }
    var killSwitchEnabled by remember { mutableStateOf(false) }
    var obfuscationEnabled by remember { mutableStateOf(false) }
    var obfuscationUtls by remember { mutableStateOf(false) }
    var obfuscationPaddingEnabled by remember { mutableStateOf(false) }
    var obfuscationPaddingSize by remember { mutableStateOf("0") }
    var dnsCacheEnabled by remember { mutableStateOf(false) }

    // @sk-task multi-server-android-client#T2.2: fill form from active config (AC-002)
    fun fillFormFromConfig(c: ConnectionConfig) {
        serverUrl = "${c.serverAddress}:${c.port}${c.serverPath}"
        token = c.token; mode = c.mode; transport = c.transport
        mtu = c.mtu.toString(); ipv6Enabled = c.ipv6Enabled; autoReconnect = c.autoReconnect
        logLevel = c.logLevel; maxMessageSize = c.maxMessageSize.toString(); multiplex = c.multiplex
        minBackoffSec = c.minBackoffSec.toString(); maxBackoffSec = c.maxBackoffSec.toString()
        tlsVerifyMode = c.tlsVerifyMode; tlsServerName = c.tlsServerName; tlsSni = c.tlsSni
        routingDefaultRoute = c.routingDefaultRoute
        routingIncludeRanges = c.routingIncludeRanges.joinToString(",")
        routingExcludeRanges = c.routingExcludeRanges.joinToString(",")
        routingIncludeIps = c.routingIncludeIps.joinToString(",")
        routingExcludeIps = c.routingExcludeIps.joinToString(",")
        routingIncludeDomains = c.routingIncludeDomains.joinToString(",")
        routingExcludeDomains = c.routingExcludeDomains.joinToString(",")
        routingIncludeSources = c.routingIncludeSources
        routingExcludeSources = c.routingExcludeSources
        geoipPath = c.geoipPath
        geoipUrl = c.geoipUrl
        geositePath = c.geositePath
        geositeUrl = c.geositeUrl
        sourceTtlHours = c.sourceTtlHours.toString()
        cryptoEnabled = c.cryptoEnabled; cryptoKey = c.cryptoKey
        killSwitchEnabled = c.killSwitchEnabled
        obfuscationEnabled = c.obfuscationEnabled; obfuscationUtls = c.obfuscationUtls
        obfuscationPaddingEnabled = c.obfuscationPaddingEnabled
        obfuscationPaddingSize = c.obfuscationPaddingSize.toString()
        dnsCacheEnabled = c.dnsCacheEnabled
    }

    // @sk-task multi-server-android-client#T2.2: load active config into form on switch (AC-002)
    LaunchedEffect(activeCfg) {
        activeCfg?.let { fillFormFromConfig(it) }
    }

    // Mark dirty on any field change
    fun onFieldChange() { vm.markDirty() }

    fun buildConfig(): ConnectionConfig {
        val (sv, pr, pa) = parseServerUrl(serverUrl)
        return ConnectionConfig(
            serverAddress = sv, port = pr, serverPath = pa, token = token,
            mode = mode, transport = transport,
            mtu = mtu.toIntOrNull() ?: 1500, ipv6Enabled = ipv6Enabled,
            autoReconnect = autoReconnect, logLevel = logLevel,
            maxMessageSize = maxMessageSize.toIntOrNull() ?: 65535, multiplex = multiplex,
            minBackoffSec = minBackoffSec.toIntOrNull() ?: 1,
            maxBackoffSec = maxBackoffSec.toIntOrNull() ?: 30,
            tlsVerifyMode = tlsVerifyMode, tlsServerName = tlsServerName, tlsSni = tlsSni,
            routingDefaultRoute = routingDefaultRoute,
            routingIncludeRanges = routingIncludeRanges.split(",").filter { it.isNotBlank() },
            routingExcludeRanges = routingExcludeRanges.split(",").filter { it.isNotBlank() },
            routingIncludeIps = routingIncludeIps.split(",").filter { it.isNotBlank() },
            routingExcludeIps = routingExcludeIps.split(",").filter { it.isNotBlank() },
            routingIncludeDomains = routingIncludeDomains.split(",").filter { it.isNotBlank() },
            routingExcludeDomains = routingExcludeDomains.split(",").filter { it.isNotBlank() },
            routingIncludeSources = routingIncludeSources,
            routingExcludeSources = routingExcludeSources,
            geoipPath = geoipPath,
            geoipUrl = geoipUrl,
            geositePath = geositePath,
            geositeUrl = geositeUrl,
            sourceTtlHours = sourceTtlHours.toIntOrNull() ?: 24,
            cryptoEnabled = cryptoEnabled, cryptoKey = cryptoKey,
            killSwitchEnabled = killSwitchEnabled,
            obfuscationEnabled = obfuscationEnabled, obfuscationUtls = obfuscationUtls,
            obfuscationPaddingEnabled = obfuscationPaddingEnabled,
            obfuscationPaddingSize = obfuscationPaddingSize.toIntOrNull() ?: 0,
            dnsCacheEnabled = dnsCacheEnabled
        )
    }

    // @sk-task multi-server-android-client#T2.2: dirty confirm before server switch (AC-003)
    fun requestSwitch(name: String) {
        if (isDirty) {
            pendingAction = PendingAction.SwitchServer(name)
            showDirtyDialog = true
        } else {
            vm.setActiveServer(name)
        }
    }

    // @sk-task multi-server-android-client#T3.2: dirty confirm before duplicate (AC-008)
    fun requestDuplicate() {
        if (isDirty) {
            pendingAction = PendingAction.Duplicate
            showDirtyDialog = true
        } else {
            vm.duplicateServer("$activeName (copy)")
        }
    }

    val cameraPermissionGranted = remember {
        ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) ==
                PackageManager.PERMISSION_GRANTED
    }

    val cameraLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) showQrScanner = true
    }

    val vpnPermissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == android.app.Activity.RESULT_OK) {
            vm.connect(buildConfig())
        }
    }

    // Dirty confirm dialog
    if (showDirtyDialog && pendingAction != null) {
        val actionLabel = when (pendingAction) {
            is PendingAction.SwitchServer -> "Switch"
            is PendingAction.Duplicate -> "Duplicate"
            null -> ""
        }
        AlertDialog(
            onDismissRequest = {
                showDirtyDialog = false
                pendingAction = null
            },
            title = { Text("Unsaved changes") },
            text = { Text("Save changes before $actionLabel?") },
            confirmButton = {
                TextButton(onClick = {
                    when (val action = pendingAction) {
                        is PendingAction.SwitchServer -> {
                            vm.saveCurrentServerConfig(buildConfig())
                            vm.setActiveServer(action.name)
                        }
                        is PendingAction.Duplicate -> {
                            vm.saveCurrentServerConfig(buildConfig())
                            vm.duplicateServer("$activeName (copy)")
                        }
                        null -> {}
                    }
                    showDirtyDialog = false
                    pendingAction = null
                }) { Text("Save & $actionLabel") }
            },
            dismissButton = {
                TextButton(onClick = {
                    when (val action = pendingAction) {
                        is PendingAction.SwitchServer -> {
                            vm.setActiveServer(action.name)
                        }
                        is PendingAction.Duplicate -> {
                            vm.duplicateServer("$activeName (copy)")
                        }
                        null -> {}
                    }
                    showDirtyDialog = false
                    pendingAction = null
                }) { Text("Discard & $actionLabel") }
            }
        )
    }

    // Delete confirm dialog
    if (showDeleteConfirm && activeName.isNotEmpty()) {
        AlertDialog(
            onDismissRequest = { showDeleteConfirm = false },
            title = { Text("Delete server") },
            text = { Text("Delete $activeName?") },
            confirmButton = {
                TextButton(onClick = {
                    vm.deleteServer(activeName)
                    showDeleteConfirm = false
                }) { Text("Delete", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = {
                TextButton(onClick = { showDeleteConfirm = false }) { Text("Cancel") }
            }
        )
    }

    // Rename dialog
    if (showRenameDialog) {
        AlertDialog(
            onDismissRequest = { showRenameDialog = false },
            title = { Text("Rename server") },
            text = {
                OutlinedTextField(
                    value = renameText,
                    onValueChange = { renameText = it },
                    label = { Text("Server name") },
                    singleLine = true
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    if (renameText.isNotBlank()) {
                        vm.renameServer(activeName, renameText)
                        showRenameDialog = false
                    }
                }) { Text("Rename") }
            },
            dismissButton = {
                TextButton(onClick = { showRenameDialog = false }) { Text("Cancel") }
            }
        )
    }

    val disconnected = state == ConnectionState.DISCONNECTED

    Scaffold { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            // @sk-task multi-server-android-client#T2.2: server selector + CRUD bar (AC-001, AC-002)
            // Server selector
            var selectorExpanded by remember { mutableStateOf(false) }
            ExposedDropdownMenuBox(
                expanded = selectorExpanded,
                onExpandedChange = { selectorExpanded = it }
            ) {
                OutlinedTextField(
                    value = activeName,
                    onValueChange = {},
                    readOnly = true,
                    label = { Text("Server") },
                    trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = selectorExpanded) },
                    modifier = Modifier.menuAnchor().fillMaxWidth(),
                    enabled = disconnected
                )
                ExposedDropdownMenu(
                    expanded = selectorExpanded,
                    onDismissRequest = { selectorExpanded = false }
                ) {
                    serverList.forEach { entry ->
                        DropdownMenuItem(
                            text = { Text(entry.name) },
                            onClick = {
                                selectorExpanded = false
                                if (entry.name != activeName) {
                                    requestSwitch(entry.name)
                                }
                            }
                        )
                    }
                }
            }

            // CRUD buttons row — filled buttons with semantic colors, equal width
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.fillMaxWidth()
            ) {
                Button(
                    onClick = {
                        val idx = (1..100).firstOrNull { n ->
                            serverList.none { it.name == "Server $n" }
                        } ?: 1
                        vm.addServer("Server $idx", ConnectionConfig())
                    },
                    modifier = Modifier.weight(1f).height(40.dp),
                    enabled = disconnected,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = KvnSuccess,
                        contentColor = Color.White,
                        disabledContainerColor = KvnSuccess.copy(alpha = 0.12f),
                        disabledContentColor = KvnSuccess.copy(alpha = 0.38f)
                    )
                ) { Icon(Icons.Default.Add, contentDescription = "Add server") }

                Button(
                    onClick = { requestDuplicate() },
                    modifier = Modifier.weight(1f).height(40.dp),
                    enabled = disconnected && serverList.isNotEmpty(),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = KvnPrimary,
                        contentColor = Color.White,
                        disabledContainerColor = KvnPrimary.copy(alpha = 0.12f),
                        disabledContentColor = KvnPrimary.copy(alpha = 0.38f)
                    )
                ) { Icon(Icons.Default.ContentCopy, contentDescription = "Copy server") }

                Button(
                    onClick = {
                        renameText = activeName
                        showRenameDialog = true
                    },
                    modifier = Modifier.weight(1f).height(40.dp),
                    enabled = disconnected && serverList.isNotEmpty(),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = KvnWarning,
                        contentColor = Color.White,
                        disabledContainerColor = KvnWarning.copy(alpha = 0.12f),
                        disabledContentColor = KvnWarning.copy(alpha = 0.38f)
                    )
                ) { Icon(Icons.Default.Edit, contentDescription = "Rename server") }

                Button(
                    onClick = { showDeleteConfirm = true },
                    modifier = Modifier.weight(1f).height(40.dp),
                    enabled = disconnected && serverList.size > 1,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = KvnError,
                        contentColor = Color.White,
                        disabledContainerColor = KvnError.copy(alpha = 0.12f),
                        disabledContentColor = KvnError.copy(alpha = 0.38f)
                    )
                ) { Icon(Icons.Default.Delete, contentDescription = "Delete server") }
            }

            // Status indicator
            val statusText = when (state) {
                ConnectionState.CONNECTED -> "Connected"
                ConnectionState.CONNECTING -> "Connecting..."
                ConnectionState.DISCONNECTING -> "Disconnecting..."
                ConnectionState.RECONNECTING -> "Reconnecting..."
                ConnectionState.DISCONNECTED -> "Disconnected"
            }
            val statusColor = when (state) {
                ConnectionState.CONNECTED -> MaterialTheme.colorScheme.primary
                else -> MaterialTheme.colorScheme.onSurface
            }

            Text(
                text = "Status: $statusText",
                style = MaterialTheme.typography.headlineSmall,
                color = statusColor
            )

            // @sk-task kvn-android#T5.4: Connection section (RQ-003)
            SettingsSection(title = "Connection", initialExpanded = true) {
                OutlinedTextField(
                    value = serverUrl,
                    onValueChange = { serverUrl = it; onFieldChange() },
                    label = { Text("Server URL") },
                    placeholder = { Text("wss://host:port/path") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = token,
                    onValueChange = { token = it; onFieldChange() },
                    label = { Text("Token") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                Text("Mode", style = MaterialTheme.typography.bodyMedium)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterChip(
                        selected = mode == "tun",
                        onClick = { mode = "tun"; onFieldChange() },
                        label = { Text("TUN") },
                        enabled = disconnected
                    )
                    FilterChip(
                        selected = mode == "proxy",
                        onClick = { mode = "proxy"; onFieldChange() },
                        label = { Text("Proxy") },
                        enabled = disconnected
                    )
                }
                OutlinedTextField(
                    value = transport,
                    onValueChange = { transport = it; onFieldChange() },
                    label = { Text("Transport") },
                    placeholder = { Text("tcp") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
            }

            // @sk-task kvn-android#T5.5: Advanced section (RQ-003)
            SettingsSection(title = "Advanced") {
                OutlinedTextField(
                    value = mtu,
                    onValueChange = { mtu = it; onFieldChange() },
                    label = { Text("MTU") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("IPv6", modifier = Modifier.weight(1f))
                    Switch(
                        checked = ipv6Enabled,
                        onCheckedChange = { ipv6Enabled = it; onFieldChange() },
                        enabled = disconnected
                    )
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Auto Reconnect", modifier = Modifier.weight(1f))
                    Switch(
                        checked = autoReconnect,
                        onCheckedChange = { autoReconnect = it; onFieldChange() },
                        enabled = disconnected
                    )
                }
                OutlinedTextField(
                    value = logLevel,
                    onValueChange = { logLevel = it; onFieldChange() },
                    label = { Text("Log Level") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = maxMessageSize,
                    onValueChange = { maxMessageSize = it; onFieldChange() },
                    label = { Text("Max Message Size") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Multiplex", modifier = Modifier.weight(1f))
                    Switch(
                        checked = multiplex,
                        onCheckedChange = { multiplex = it; onFieldChange() },
                        enabled = disconnected
                    )
                }
            }

            // @sk-task kvn-android#T5.6: Reconnect section (RQ-003)
            SettingsSection(title = "Reconnect") {
                OutlinedTextField(
                    value = minBackoffSec,
                    onValueChange = { minBackoffSec = it; onFieldChange() },
                    label = { Text("Min Backoff (sec)") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = maxBackoffSec,
                    onValueChange = { maxBackoffSec = it; onFieldChange() },
                    label = { Text("Max Backoff (sec)") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
            }

            // @sk-task kvn-android#T5.7: TLS section (RQ-003)
            SettingsSection(title = "TLS") {
                Text("Verify Mode", style = MaterialTheme.typography.bodyMedium)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterChip(
                        selected = tlsVerifyMode == "verify",
                        onClick = { tlsVerifyMode = "verify"; onFieldChange() },
                        label = { Text("Verify") },
                        enabled = disconnected
                    )
                    FilterChip(
                        selected = tlsVerifyMode == "insecure",
                        onClick = { tlsVerifyMode = "insecure"; onFieldChange() },
                        label = { Text("Insecure") },
                        enabled = disconnected
                    )
                    FilterChip(
                        selected = tlsVerifyMode == "none",
                        onClick = { tlsVerifyMode = "none"; onFieldChange() },
                        label = { Text("None") },
                        enabled = disconnected
                    )
                }
                OutlinedTextField(
                    value = tlsServerName,
                    onValueChange = { tlsServerName = it; onFieldChange() },
                    label = { Text("Server Name (SNI)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
            }

            // @sk-task kvn-android#T5.10: Routing section (RQ-003)
            // @sk-task geoip-geosite-integration#T5.2: routing source fields (include/exclude sources, paths, TTL, Refresh button)
            SettingsSection(title = "Routing") {
                OutlinedTextField(
                    value = routingIncludeRanges,
                    onValueChange = { routingIncludeRanges = it; onFieldChange() },
                    label = { Text("Include Ranges (CIDR, comma-separated)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = routingExcludeRanges,
                    onValueChange = { routingExcludeRanges = it; onFieldChange() },
                    label = { Text("Exclude Ranges (CIDR, comma-separated)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = routingIncludeDomains,
                    onValueChange = { routingIncludeDomains = it; onFieldChange() },
                    label = { Text("Include Domains (comma-separated)") },
                    placeholder = { Text("example.com,.domain.com") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = routingExcludeDomains,
                    onValueChange = { routingExcludeDomains = it; onFieldChange() },
                    label = { Text("Exclude Domains (comma-separated)") },
                    placeholder = { Text("ads.com,.tracker.com") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = routingIncludeIps,
                    onValueChange = { routingIncludeIps = it; onFieldChange() },
                    label = { Text("Include IPs (comma-separated)") },
                    placeholder = { Text("10.0.0.1,192.168.1.0/24") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = routingExcludeIps,
                    onValueChange = { routingExcludeIps = it; onFieldChange() },
                    label = { Text("Exclude IPs (comma-separated)") },
                    placeholder = { Text("10.0.0.2,192.168.1.100") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = routingIncludeSources,
                    onValueChange = { routingIncludeSources = it; onFieldChange() },
                    label = { Text("Include Sources (type:value, comma-separated)") },
                    placeholder = { Text("geoip:RU,cidr:10.0.0.0/8") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = routingExcludeSources,
                    onValueChange = { routingExcludeSources = it; onFieldChange() },
                    label = { Text("Exclude Sources (type:value, comma-separated)") },
                    placeholder = { Text("geoip:CN,cidr:192.168.0.0/16") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = geoipPath,
                    onValueChange = { geoipPath = it; onFieldChange() },
                    label = { Text("GeoIP Database Path") },
                    placeholder = { Text("/data/geoip.dat") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = geoipUrl,
                    onValueChange = { geoipUrl = it; onFieldChange() },
                    label = { Text("GeoIP Database URL") },
                    placeholder = { Text("https://example.com/geoip.dat") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = geositePath,
                    onValueChange = { geositePath = it; onFieldChange() },
                    label = { Text("GeoSite Database Path") },
                    placeholder = { Text("/data/geosite.dat") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = geositeUrl,
                    onValueChange = { geositeUrl = it; onFieldChange() },
                    label = { Text("GeoSite Database URL") },
                    placeholder = { Text("https://example.com/geosite.dat") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                OutlinedTextField(
                    value = sourceTtlHours,
                    onValueChange = { sourceTtlHours = it; onFieldChange() },
                    label = { Text("Source TTL (hours)") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = disconnected
                )
                Spacer(modifier = Modifier.height(8.dp))
                Button(
                    onClick = { vm.refreshSources(buildConfig()) },
                    modifier = Modifier.fillMaxWidth().height(44.dp),
                    enabled = disconnected,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = KvnPrimary,
                        contentColor = Color.White,
                        disabledContainerColor = KvnPrimary.copy(alpha = 0.12f),
                        disabledContentColor = KvnPrimary.copy(alpha = 0.38f)
                    )
                ) { Text("Refresh Sources") }
            }

            // @sk-task android-dns-cache#T4.4: DNS Cache toggle (AC-008)
            SettingsSection(title = "DNS") {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("DNS Cache", modifier = Modifier.weight(1f))
                    Switch(
                        checked = dnsCacheEnabled,
                        onCheckedChange = { dnsCacheEnabled = it; onFieldChange() },
                        enabled = disconnected
                    )
                }
            }

            // @sk-task kvn-android#T5.13: Encryption section (RQ-003)
            SettingsSection(title = "Encryption") {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enabled", modifier = Modifier.weight(1f))
                    Switch(
                        checked = cryptoEnabled,
                        onCheckedChange = { cryptoEnabled = it; onFieldChange() },
                        enabled = disconnected
                    )
                }
                if (cryptoEnabled) {
                    OutlinedTextField(
                        value = cryptoKey,
                        onValueChange = { cryptoKey = it; onFieldChange() },
                        label = { Text("Encryption Key") },
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation(),
                        modifier = Modifier.fillMaxWidth(),
                        enabled = disconnected
                    )
                }
            }

            // @sk-task kvn-android#T5.15: Kill Switch section (RQ-003)
            SettingsSection(title = "Kill Switch") {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enabled", modifier = Modifier.weight(1f))
                    Switch(
                        checked = killSwitchEnabled,
                        onCheckedChange = { killSwitchEnabled = it; onFieldChange() },
                        enabled = disconnected
                    )
                }
            }

            // @sk-task kvn-android#T5.17: Obfuscation section (RQ-003)
            SettingsSection(title = "Obfuscation") {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enabled", modifier = Modifier.weight(1f))
                    Switch(
                        checked = obfuscationEnabled,
                        onCheckedChange = { obfuscationEnabled = it; onFieldChange() },
                        enabled = disconnected
                    )
                }
                if (obfuscationEnabled) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("uTLS", modifier = Modifier.weight(1f))
                        Switch(
                            checked = obfuscationUtls,
                            onCheckedChange = { obfuscationUtls = it; onFieldChange() },
                            enabled = disconnected
                        )
                    }
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("Padding", modifier = Modifier.weight(1f))
                        Switch(
                            checked = obfuscationPaddingEnabled,
                            onCheckedChange = { obfuscationPaddingEnabled = it; onFieldChange() },
                            enabled = disconnected
                        )
                    }
                    if (obfuscationPaddingEnabled) {
                        OutlinedTextField(
                            value = obfuscationPaddingSize,
                            onValueChange = { obfuscationPaddingSize = it; onFieldChange() },
                            label = { Text("Padding Size") },
                            singleLine = true,
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                            modifier = Modifier.fillMaxWidth(),
                            enabled = disconnected
                        )
                    }
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            // Connect/Disconnect button
            Button(
                onClick = {
                    when (state) {
                        ConnectionState.DISCONNECTED -> {
                            val cfg = buildConfig()
                            if (cfg.serverAddress.isBlank()) {
                                Toast.makeText(context, "Enter Server URL", Toast.LENGTH_SHORT).show()
                                return@Button
                            }
                            val intent = VpnService.prepare(context)
                            if (intent != null) {
                                vpnPermissionLauncher.launch(intent)
                            } else {
                                vm.connect(cfg)
                            }
                        }
                        else -> vm.disconnect()
                    }
                },
                modifier = Modifier.fillMaxWidth().height(48.dp),
                enabled = true
            ) {
                Text(if (state == ConnectionState.DISCONNECTED) "Connect" else "Disconnect")
            }

            // Error message
            if (errorMessage != null) {
                Text(
                    text = errorMessage!!,
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)
                )
            }

            // Traffic counters
            if (state == ConnectionState.CONNECTED) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceEvenly
                ) {
                    Text("RX: ${formatBytes(rxBytes)}")
                    Text("TX: ${formatBytes(txBytes)}")
                }
            }

            // Emergency Close
            if (state != ConnectionState.DISCONNECTED || errorMessage != null) {
                OutlinedButton(
                    onClick = {
                        vm.disconnect()
                        (context as? android.app.Activity)?.finishAndRemoveTask()
                    },
                    modifier = Modifier.fillMaxWidth().height(48.dp),
                    colors = ButtonDefaults.outlinedButtonColors(
                        contentColor = MaterialTheme.colorScheme.error
                    )
                ) { Text("Close App") }
            }

            // QR scan button
            OutlinedButton(
                onClick = {
                    if (!cameraPermissionGranted) {
                        cameraLauncher.launch(Manifest.permission.CAMERA)
                    } else {
                        showQrScanner = true
                    }
                },
                modifier = Modifier.fillMaxWidth(),
                enabled = disconnected
            ) { Text("Scan QR Code") }

            // Import from clipboard
            var pasteText by remember { mutableStateOf("") }
            OutlinedTextField(
                value = pasteText,
                onValueChange = { pasteText = it },
                label = { Text("Paste Config JSON") },
                placeholder = { Text("Paste JSON config or scan QR") },
                singleLine = false,
                minLines = 2,
                maxLines = 4,
                modifier = Modifier.fillMaxWidth(),
                enabled = disconnected
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                OutlinedButton(
                    onClick = {
                        val cfg = parseQrConfig(pasteText)
                        if (cfg != null) {
                            fillFormFromConfig(cfg)
                            vm.markDirty()
                            Toast.makeText(context, "Config loaded", Toast.LENGTH_SHORT).show()
                        } else {
                            Toast.makeText(context, "Invalid config JSON", Toast.LENGTH_SHORT).show()
                        }
                    },
                    modifier = Modifier.weight(1f),
                    enabled = disconnected && pasteText.isNotBlank()
                ) { Text("Load") }
                OutlinedButton(
                    onClick = {
                        val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                        val clip = clipboard.primaryClip
                        val text = clip?.getItemAt(0)?.text?.toString()
                        if (text != null) {
                            pasteText = text
                            val cfg = parseQrConfig(text)
                            if (cfg != null) {
                                fillFormFromConfig(cfg)
                                vm.markDirty()
                                Toast.makeText(context, "Config imported", Toast.LENGTH_SHORT).show()
                            } else {
                                Toast.makeText(context, "Invalid config", Toast.LENGTH_SHORT).show()
                            }
                        } else {
                            Toast.makeText(context, "Clipboard is empty", Toast.LENGTH_SHORT).show()
                        }
                    },
                    modifier = Modifier.weight(1f),
                    enabled = disconnected
                ) { Text("Paste + Load") }
            }

            // Export QR
            OutlinedButton(
                onClick = { showExportQr = true },
                modifier = Modifier.fillMaxWidth(),
                enabled = disconnected
            ) { Text("Export QR Code") }

            Spacer(modifier = Modifier.height(32.dp))
        }
    }

    // QR scanner screen
    // @sk-task multi-server-android-client#T3.1: QR import adds new server (AC-006)
    if (showQrScanner) {
        QrScannerScreen(
            onQrScanned = { cfg ->
                vm.addServer("Imported ${System.currentTimeMillis()}", cfg)
                showQrScanner = false
            },
            onCancel = { showQrScanner = false }
        )
    }

    // Export QR screen
    if (showExportQr) {
        QrExportScreen(
            config = buildConfig(),
            onDismiss = { showExportQr = false }
        )
    }
}
