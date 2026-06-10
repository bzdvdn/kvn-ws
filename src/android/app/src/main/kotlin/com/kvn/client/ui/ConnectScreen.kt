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
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewmodel.compose.viewModel
import com.kvn.client.config.ConnectionConfig
import com.kvn.client.transport.ConnectionState

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
    val savedConfig by vm.savedConfig.collectAsState()
    val qrConfig by vm.qrConfig.collectAsState()
    val rxBytes by vm.rxBytes.collectAsState()
    val txBytes by vm.txBytes.collectAsState()
    val errorMessage by vm.errorMessage.collectAsState()
    var showQrScanner by remember { mutableStateOf(false) }
    var showExportQr by remember { mutableStateOf(false) }

    // Connection — single URL like kvn-web
    var serverUrl by remember { mutableStateOf("") }
    var token by remember { mutableStateOf("") }
    var mode by remember { mutableStateOf("tun") }
    var transport by remember { mutableStateOf("tcp") }
    // Advanced
    var mtu by remember { mutableStateOf("1500") }
    var ipv6Enabled by remember { mutableStateOf(false) }
    var autoReconnect by remember { mutableStateOf(true) }
    var logLevel by remember { mutableStateOf("info") }
    var maxMessageSize by remember { mutableStateOf("65535") }
    var multiplex by remember { mutableStateOf(false) }
    // Reconnect
    var minBackoffSec by remember { mutableStateOf("1") }
    var maxBackoffSec by remember { mutableStateOf("30") }
    // TLS
    var tlsVerifyMode by remember { mutableStateOf("verify") }
    var tlsServerName by remember { mutableStateOf("") }
    var tlsSni by remember { mutableStateOf(listOf<String>()) }
    // Routing
    var routingDefaultRoute by remember { mutableStateOf("server") }
    var routingIncludeRanges by remember { mutableStateOf("") }
    var routingExcludeRanges by remember { mutableStateOf("") }
    var routingIncludeIps by remember { mutableStateOf("") }
    var routingExcludeIps by remember { mutableStateOf("") }
    var routingIncludeDomains by remember { mutableStateOf("") }
    var routingExcludeDomains by remember { mutableStateOf("") }
    // Encryption
    var cryptoEnabled by remember { mutableStateOf(false) }
    var cryptoKey by remember { mutableStateOf("") }
    // Kill Switch
    var killSwitchEnabled by remember { mutableStateOf(false) }
    // Obfuscation
    var obfuscationEnabled by remember { mutableStateOf(false) }
    var obfuscationUtls by remember { mutableStateOf(false) }
    var obfuscationPaddingEnabled by remember { mutableStateOf(false) }
    var obfuscationPaddingSize by remember { mutableStateOf("0") }

    // Restore saved config on first composition
    LaunchedEffect(savedConfig) {
        if (savedConfig.serverAddress.isNotEmpty() && serverUrl.isEmpty()) {
            serverUrl = "${savedConfig.serverAddress}:${savedConfig.port}${savedConfig.serverPath}"
            token = savedConfig.token
            mode = savedConfig.mode
            transport = savedConfig.transport
            mtu = savedConfig.mtu.toString()
            ipv6Enabled = savedConfig.ipv6Enabled
            autoReconnect = savedConfig.autoReconnect
            logLevel = savedConfig.logLevel
            maxMessageSize = savedConfig.maxMessageSize.toString()
            multiplex = savedConfig.multiplex
            minBackoffSec = savedConfig.minBackoffSec.toString()
            maxBackoffSec = savedConfig.maxBackoffSec.toString()
            tlsVerifyMode = savedConfig.tlsVerifyMode
            tlsServerName = savedConfig.tlsServerName
            tlsSni = savedConfig.tlsSni
            routingDefaultRoute = savedConfig.routingDefaultRoute
            routingIncludeRanges = savedConfig.routingIncludeRanges.joinToString(",")
            routingExcludeRanges = savedConfig.routingExcludeRanges.joinToString(",")
            routingIncludeIps = savedConfig.routingIncludeIps.joinToString(",")
            routingExcludeIps = savedConfig.routingExcludeIps.joinToString(",")
            routingIncludeDomains = savedConfig.routingIncludeDomains.joinToString(",")
            routingExcludeDomains = savedConfig.routingExcludeDomains.joinToString(",")
            cryptoEnabled = savedConfig.cryptoEnabled
            cryptoKey = savedConfig.cryptoKey
            killSwitchEnabled = savedConfig.killSwitchEnabled
            obfuscationEnabled = savedConfig.obfuscationEnabled
            obfuscationUtls = savedConfig.obfuscationUtls
            obfuscationPaddingEnabled = savedConfig.obfuscationPaddingEnabled
            obfuscationPaddingSize = savedConfig.obfuscationPaddingSize.toString()
        }
    }

    fun fillFormFromConfig(c: ConnectionConfig) {
        serverUrl = "${c.serverAddress}:${c.port}${c.serverPath}"
        token = c.token
        mode = c.mode
        transport = c.transport
        mtu = c.mtu.toString()
        ipv6Enabled = c.ipv6Enabled
        autoReconnect = c.autoReconnect
        logLevel = c.logLevel
        maxMessageSize = c.maxMessageSize.toString()
        multiplex = c.multiplex
        minBackoffSec = c.minBackoffSec.toString()
        maxBackoffSec = c.maxBackoffSec.toString()
        tlsVerifyMode = c.tlsVerifyMode
        tlsServerName = c.tlsServerName
        tlsSni = c.tlsSni
        routingDefaultRoute = c.routingDefaultRoute
        routingIncludeRanges = c.routingIncludeRanges.joinToString(",")
        routingExcludeRanges = c.routingExcludeRanges.joinToString(",")
        routingIncludeIps = c.routingIncludeIps.joinToString(",")
        routingExcludeIps = c.routingExcludeIps.joinToString(",")
        routingIncludeDomains = c.routingIncludeDomains.joinToString(",")
        routingExcludeDomains = c.routingExcludeDomains.joinToString(",")
        cryptoEnabled = c.cryptoEnabled
        cryptoKey = c.cryptoKey
        killSwitchEnabled = c.killSwitchEnabled
        obfuscationEnabled = c.obfuscationEnabled
        obfuscationUtls = c.obfuscationUtls
        obfuscationPaddingEnabled = c.obfuscationPaddingEnabled
        obfuscationPaddingSize = c.obfuscationPaddingSize.toString()
    }

    // Apply config from QR scanner
    LaunchedEffect(qrConfig) {
        qrConfig?.let { c ->
            fillFormFromConfig(c)
            vm.consumeQrConfig()
        }
    }

    fun buildConfig(): ConnectionConfig {
        val (sv, pr, pa) = parseServerUrl(serverUrl)
        return ConnectionConfig(
            serverAddress = sv,
            port = pr,
            serverPath = pa,
            token = token,
            mode = mode,
            transport = transport,
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
            tlsSni = tlsSni,
            routingDefaultRoute = routingDefaultRoute,
            routingIncludeRanges = routingIncludeRanges.split(",").filter { it.isNotBlank() },
            routingExcludeRanges = routingExcludeRanges.split(",").filter { it.isNotBlank() },
            routingIncludeIps = routingIncludeIps.split(",").filter { it.isNotBlank() },
            routingExcludeIps = routingExcludeIps.split(",").filter { it.isNotBlank() },
            routingIncludeDomains = routingIncludeDomains.split(",").filter { it.isNotBlank() },
            routingExcludeDomains = routingExcludeDomains.split(",").filter { it.isNotBlank() },
            cryptoEnabled = cryptoEnabled,
            cryptoKey = cryptoKey,
            killSwitchEnabled = killSwitchEnabled,
            obfuscationEnabled = obfuscationEnabled,
            obfuscationUtls = obfuscationUtls,
            obfuscationPaddingEnabled = obfuscationPaddingEnabled,
            obfuscationPaddingSize = obfuscationPaddingSize.toIntOrNull() ?: 0
        )
    }

    val cameraPermissionGranted = remember {
        ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) ==
                PackageManager.PERMISSION_GRANTED
    }

    val cameraLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) {
            showQrScanner = true
        }
    }

    val vpnPermissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == android.app.Activity.RESULT_OK) {
            vm.connect(buildConfig())
        }
    }

    Scaffold { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
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
                    onValueChange = { serverUrl = it },
                    label = { Text("Server URL") },
                    placeholder = { Text("wss://host:port/path") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                OutlinedTextField(
                    value = token,
                    onValueChange = { token = it },
                    label = { Text("Token") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                // Mode selector
                Text("Mode", style = MaterialTheme.typography.bodyMedium)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterChip(
                        selected = mode == "tun",
                        onClick = { mode = "tun" },
                        label = { Text("TUN") },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                    FilterChip(
                        selected = mode == "proxy",
                        onClick = { mode = "proxy" },
                        label = { Text("Proxy") },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
                // Transport
                OutlinedTextField(
                    value = transport,
                    onValueChange = { transport = it },
                    label = { Text("Transport") },
                    placeholder = { Text("tcp") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
            }

            // @sk-task kvn-android#T5.5: Advanced section (RQ-003)
            SettingsSection(title = "Advanced") {
                OutlinedTextField(
                    value = mtu,
                    onValueChange = { mtu = it },
                    label = { Text("MTU") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("IPv6", modifier = Modifier.weight(1f))
                    Switch(
                        checked = ipv6Enabled,
                        onCheckedChange = { ipv6Enabled = it },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Auto Reconnect", modifier = Modifier.weight(1f))
                    Switch(
                        checked = autoReconnect,
                        onCheckedChange = { autoReconnect = it },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
                OutlinedTextField(
                    value = logLevel,
                    onValueChange = { logLevel = it },
                    label = { Text("Log Level") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                OutlinedTextField(
                    value = maxMessageSize,
                    onValueChange = { maxMessageSize = it },
                    label = { Text("Max Message Size") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Multiplex", modifier = Modifier.weight(1f))
                    Switch(
                        checked = multiplex,
                        onCheckedChange = { multiplex = it },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
            }

            // @sk-task kvn-android#T5.6: Reconnect section (RQ-003)
            SettingsSection(title = "Reconnect") {
                OutlinedTextField(
                    value = minBackoffSec,
                    onValueChange = { minBackoffSec = it },
                    label = { Text("Min Backoff (sec)") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                OutlinedTextField(
                    value = maxBackoffSec,
                    onValueChange = { maxBackoffSec = it },
                    label = { Text("Max Backoff (sec)") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
            }

            // @sk-task kvn-android#T5.7: TLS section (RQ-003)
            SettingsSection(title = "TLS") {
                Text("Verify Mode", style = MaterialTheme.typography.bodyMedium)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterChip(
                        selected = tlsVerifyMode == "verify",
                        onClick = { tlsVerifyMode = "verify" },
                        label = { Text("Verify") },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                    FilterChip(
                        selected = tlsVerifyMode == "insecure",
                        onClick = { tlsVerifyMode = "insecure" },
                        label = { Text("Insecure") },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                    FilterChip(
                        selected = tlsVerifyMode == "none",
                        onClick = { tlsVerifyMode = "none" },
                        label = { Text("None") },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
                OutlinedTextField(
                    value = tlsServerName,
                    onValueChange = { tlsServerName = it },
                    label = { Text("Server Name (SNI)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
            }

            // @sk-task kvn-android#T5.10: Routing section (RQ-003)
            SettingsSection(title = "Routing") {
                OutlinedTextField(
                    value = routingIncludeRanges,
                    onValueChange = { routingIncludeRanges = it },
                    label = { Text("Include Ranges (CIDR, comma-separated)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                OutlinedTextField(
                    value = routingExcludeRanges,
                    onValueChange = { routingExcludeRanges = it },
                    label = { Text("Exclude Ranges (CIDR, comma-separated)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                OutlinedTextField(
                    value = routingIncludeDomains,
                    onValueChange = { routingIncludeDomains = it },
                    label = { Text("Include Domains (comma-separated)") },
                    placeholder = { Text("example.com,.domain.com") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                OutlinedTextField(
                    value = routingExcludeDomains,
                    onValueChange = { routingExcludeDomains = it },
                    label = { Text("Exclude Domains (comma-separated)") },
                    placeholder = { Text("ads.com,.tracker.com") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                OutlinedTextField(
                    value = routingIncludeIps,
                    onValueChange = { routingIncludeIps = it },
                    label = { Text("Include IPs (comma-separated)") },
                    placeholder = { Text("10.0.0.1,192.168.1.0/24") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
                OutlinedTextField(
                    value = routingExcludeIps,
                    onValueChange = { routingExcludeIps = it },
                    label = { Text("Exclude IPs (comma-separated)") },
                    placeholder = { Text("10.0.0.2,192.168.1.100") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    enabled = state == ConnectionState.DISCONNECTED
                )
            }

            // @sk-task kvn-android#T5.13: Encryption section (RQ-003)
            SettingsSection(title = "Encryption") {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enabled", modifier = Modifier.weight(1f))
                    Switch(
                        checked = cryptoEnabled,
                        onCheckedChange = { cryptoEnabled = it },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
                if (cryptoEnabled) {
                    OutlinedTextField(
                        value = cryptoKey,
                        onValueChange = { cryptoKey = it },
                        label = { Text("Encryption Key") },
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation(),
                        modifier = Modifier.fillMaxWidth(),
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
            }

            // @sk-task kvn-android#T5.15: Kill Switch section (RQ-003)
            SettingsSection(title = "Kill Switch") {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enabled", modifier = Modifier.weight(1f))
                    Switch(
                        checked = killSwitchEnabled,
                        onCheckedChange = { killSwitchEnabled = it },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
            }

            // @sk-task kvn-android#T5.17: Obfuscation section (RQ-003)
            SettingsSection(title = "Obfuscation") {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enabled", modifier = Modifier.weight(1f))
                    Switch(
                        checked = obfuscationEnabled,
                        onCheckedChange = { obfuscationEnabled = it },
                        enabled = state == ConnectionState.DISCONNECTED
                    )
                }
                if (obfuscationEnabled) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("uTLS", modifier = Modifier.weight(1f))
                        Switch(
                            checked = obfuscationUtls,
                            onCheckedChange = { obfuscationUtls = it },
                            enabled = state == ConnectionState.DISCONNECTED
                        )
                    }
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("Padding", modifier = Modifier.weight(1f))
                        Switch(
                            checked = obfuscationPaddingEnabled,
                            onCheckedChange = { obfuscationPaddingEnabled = it },
                            enabled = state == ConnectionState.DISCONNECTED
                        )
                    }
                    if (obfuscationPaddingEnabled) {
                        OutlinedTextField(
                            value = obfuscationPaddingSize,
                            onValueChange = { obfuscationPaddingSize = it },
                            label = { Text("Padding Size") },
                            singleLine = true,
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                            modifier = Modifier.fillMaxWidth(),
                            enabled = state == ConnectionState.DISCONNECTED
                        )
                    }
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            // Connect/Disconnect button — always clickable when not disconnected
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
                modifier = Modifier
                    .fillMaxWidth()
                    .height(48.dp),
                enabled = true
            ) {
                Text(
                    if (state == ConnectionState.DISCONNECTED) "Connect" else "Disconnect"
                )
            }

            // Error message
            if (errorMessage != null) {
                Text(
                    text = errorMessage!!,
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(vertical = 4.dp)
                )
            }

            // Traffic counters
            // @sk-task kvn-android#T3.2: RX/TX counters display (AC-002)
            if (state == ConnectionState.CONNECTED) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceEvenly
                ) {
                    Text("RX: ${formatBytes(rxBytes)}")
                    Text("TX: ${formatBytes(txBytes)}")
                }
            }

            // Emergency Close — stops VPN and closes the app
            if (state != ConnectionState.DISCONNECTED || errorMessage != null) {
                OutlinedButton(
                    onClick = {
                        vm.disconnect()
                        (context as? android.app.Activity)?.finishAndRemoveTask()
                    },
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(48.dp),
                    colors = ButtonDefaults.outlinedButtonColors(
                        contentColor = MaterialTheme.colorScheme.error
                    )
                ) {
                    Text("Close App")
                }
            }

            // QR scan button
            // @sk-task kvn-android#T5.2: QR scan button (AC-007, RQ-011)
            OutlinedButton(
                onClick = {
                    if (!cameraPermissionGranted) {
                        cameraLauncher.launch(Manifest.permission.CAMERA)
                    } else {
                        showQrScanner = true
                    }
                },
                modifier = Modifier.fillMaxWidth(),
                enabled = state == ConnectionState.DISCONNECTED
            ) {
                Text("Scan QR Code")
            }

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
                enabled = state == ConnectionState.DISCONNECTED
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                OutlinedButton(
                    onClick = {
                        val cfg = parseQrConfig(pasteText)
                        if (cfg != null) {
                            fillFormFromConfig(cfg)
                            Toast.makeText(context, "Config loaded", Toast.LENGTH_SHORT).show()
                        } else {
                            Toast.makeText(context, "Invalid config JSON", Toast.LENGTH_SHORT).show()
                        }
                    },
                    modifier = Modifier.weight(1f),
                    enabled = state == ConnectionState.DISCONNECTED && pasteText.isNotBlank()
                ) {
                    Text("Load")
                }
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
                                Toast.makeText(context, "Config imported", Toast.LENGTH_SHORT).show()
                            } else {
                                Toast.makeText(context, "Invalid config", Toast.LENGTH_SHORT).show()
                            }
                        } else {
                            Toast.makeText(context, "Clipboard is empty", Toast.LENGTH_SHORT).show()
                        }
                    },
                    modifier = Modifier.weight(1f),
                    enabled = state == ConnectionState.DISCONNECTED
                ) {
                    Text("Paste + Load")
                }
            }

            // Export QR
            OutlinedButton(
                onClick = { showExportQr = true },
                modifier = Modifier.fillMaxWidth(),
                enabled = state == ConnectionState.DISCONNECTED
            ) {
                Text("Export QR Code")
            }

            Spacer(modifier = Modifier.height(32.dp))
        }
    }

    // QR scanner screen
    if (showQrScanner) {
        QrScannerScreen(
            onQrScanned = { cfg ->
                fillFormFromConfig(cfg)
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
