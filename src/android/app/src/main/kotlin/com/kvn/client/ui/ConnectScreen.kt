package com.kvn.client.ui

import android.Manifest
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.kvn.client.ui.theme.KvnError
import com.kvn.client.ui.theme.KvnPrimary
import com.kvn.client.ui.theme.KvnSuccess
import com.kvn.client.ui.theme.KvnWarning
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewmodel.compose.viewModel
import com.kvn.client.config.ConnectionConfig
import com.kvn.client.transport.ConnectionState

private sealed class PendingAction {
    class SwitchServer(val name: String) : PendingAction()
    object Duplicate : PendingAction()
}

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

fun formatBytes(bytes: Long): String {
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

    // Connection form state (only what stays on main screen)
    var serverUrl by remember { mutableStateOf("") }
    var token by remember { mutableStateOf("") }
    var tokenVisible by remember { mutableStateOf(false) }
    var mode by remember { mutableStateOf("tun") }

    fun fillFormFromConfig(c: ConnectionConfig) {
        serverUrl = "${c.serverAddress}:${c.port}${c.serverPath}"
        token = c.token; mode = c.mode
    }

    LaunchedEffect(activeCfg) {
        activeCfg?.let { fillFormFromConfig(it) }
    }

    fun onFieldChange() { vm.markDirty() }

    fun buildConfig(): ConnectionConfig {
        val base = activeCfg ?: ConnectionConfig()
        val (sv, pr, pa) = parseServerUrl(serverUrl)
        return base.copy(
            serverAddress = sv, port = pr, serverPath = pa,
            token = token, mode = mode
        )
    }

    fun requestSwitch(name: String) {
        if (isDirty) {
            pendingAction = PendingAction.SwitchServer(name)
            showDirtyDialog = true
        } else {
            vm.setActiveServer(name)
        }
    }

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

    val notificationPermissionGranted = remember {
        if (Build.VERSION.SDK_INT >= 33) {
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) ==
                    PackageManager.PERMISSION_GRANTED
        } else true
    }

    val notificationLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) vm.connect(buildConfig())
    }

    val vpnPermissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == android.app.Activity.RESULT_OK) {
            if (!notificationPermissionGranted) {
                notificationLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            } else {
                vm.connect(buildConfig())
            }
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
            // Server cards
            if (serverList.isEmpty()) {
                Text(
                    text = "Add your first server",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(vertical = 8.dp)
                )
            } else {
                serverList.forEach { entry ->
                    val isActive = entry.name == activeName
                    Card(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable(enabled = disconnected) {
                                if (!isActive) requestSwitch(entry.name)
                            },
                        colors = CardDefaults.cardColors(
                            containerColor = if (isActive)
                                MaterialTheme.colorScheme.primary.copy(alpha = 0.1f)
                            else
                                MaterialTheme.colorScheme.surface
                        ),
                        border = if (isActive)
                            CardDefaults.outlinedCardBorder().copy(width = 1.dp)
                        else null
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth().padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Surface(
                                modifier = Modifier.size(10.dp),
                                shape = MaterialTheme.shapes.extraSmall,
                                color = if (isActive) KvnSuccess
                                    else MaterialTheme.colorScheme.onSurface.copy(alpha = 0.3f)
                            ) {}
                            Spacer(modifier = Modifier.width(12.dp))
                            Column(modifier = Modifier.weight(1f)) {
                                Text(
                                    text = entry.name,
                                    style = MaterialTheme.typography.titleSmall,
                                    color = MaterialTheme.colorScheme.onSurface
                                )
                                Text(
                                    text = "${entry.config.serverAddress}:${entry.config.port} · ${entry.config.mode}",
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }
                            if (isActive) {
                                Surface(
                                    shape = MaterialTheme.shapes.extraSmall,
                                    color = KvnPrimary.copy(alpha = 0.2f)
                                ) {
                                    Text(
                                        text = "Active",
                                        style = MaterialTheme.typography.labelSmall,
                                        color = KvnPrimary,
                                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp)
                                    )
                                }
                            }
                        }
                    }
                }
            }

            // CRUD buttons
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

            // Status
            val statusText = when (state) {
                ConnectionState.CONNECTED -> "Connected"
                ConnectionState.CONNECTING -> "Connecting..."
                ConnectionState.DISCONNECTING -> "Disconnecting..."
                ConnectionState.RECONNECTING -> "Reconnecting..."
                ConnectionState.DISCONNECTED -> "Disconnected"
            }
            val statusColor = when (state) {
                ConnectionState.CONNECTED -> Color(0xFF4CAF50)
                ConnectionState.CONNECTING -> Color(0xFFFFA726)
                ConnectionState.DISCONNECTING -> Color(0xFFFFA726)
                ConnectionState.RECONNECTING -> Color(0xFFFFA726)
                ConnectionState.DISCONNECTED -> Color(0xFF9E9E9E)
            }
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Box(
                    modifier = Modifier
                        .size(14.dp)
                        .background(statusColor, CircleShape)
                )
                Text(
                    text = statusText,
                    style = MaterialTheme.typography.headlineSmall,
                    color = MaterialTheme.colorScheme.onSurface
                )
            }

            // Connection section
            OutlinedTextField(
                value = serverUrl,
                onValueChange = { serverUrl = it; onFieldChange() },
                label = { Text("Server URL") },
                placeholder = { Text("host:port/path") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
                enabled = disconnected
            )
            OutlinedTextField(
                value = token,
                onValueChange = { token = it; onFieldChange() },
                label = { Text("Token") },
                singleLine = true,
                visualTransformation = if (tokenVisible) VisualTransformation.None else PasswordVisualTransformation(),
                trailingIcon = {
                    IconButton(onClick = { tokenVisible = !tokenVisible }) {
                        Icon(
                            imageVector = if (tokenVisible) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                            contentDescription = if (tokenVisible) "Hide token" else "Show token"
                        )
                    }
                },
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
                            } else if (!notificationPermissionGranted) {
                                notificationLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
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

            // Mini traffic panel
            if (state == ConnectionState.CONNECTED) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Card(
                        modifier = Modifier.weight(1f),
                        colors = CardDefaults.cardColors(
                            containerColor = KvnPrimary.copy(alpha = 0.1f)
                        )
                    ) {
                        Column(
                            modifier = Modifier.fillMaxWidth().padding(12.dp),
                            horizontalAlignment = Alignment.CenterHorizontally
                        ) {
                            Text(
                                text = formatBytes(rxBytes),
                                style = MaterialTheme.typography.titleLarge,
                                color = KvnPrimary
                            )
                            Text(
                                text = "Download",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                        }
                    }
                    Card(
                        modifier = Modifier.weight(1f),
                        colors = CardDefaults.cardColors(
                            containerColor = KvnSuccess.copy(alpha = 0.1f)
                        )
                    ) {
                        Column(
                            modifier = Modifier.fillMaxWidth().padding(12.dp),
                            horizontalAlignment = Alignment.CenterHorizontally
                        ) {
                            Text(
                                text = formatBytes(txBytes),
                                style = MaterialTheme.typography.titleLarge,
                                color = KvnSuccess
                            )
                            Text(
                                text = "Upload",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                        }
                    }
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

            // QR scan
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
