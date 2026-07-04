package com.kvn.client.ui

import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.widget.ImageView
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

private data class AppItem(
    val packageName: String,
    val appName: String,
    val isSystem: Boolean
)

@OptIn(ExperimentalMaterial3Api::class)
// @sk-task android-per-app-dns#T1.1: full-screen app picker with search and checkboxes (AC-001, AC-002)
@Composable
fun AppPickerScreen(
    initialSelection: Set<String>,
    onSave: (Set<String>) -> Unit,
    onBack: () -> Unit
) {
    val context = LocalContext.current
    var searchQuery by remember { mutableStateOf("") }
    var selected by remember { mutableStateOf(initialSelection) }
    var showSystemApps by remember { mutableStateOf(false) }

    val apps by produceState<List<AppItem>>(initialValue = emptyList()) {
        value = withContext(Dispatchers.Default) {
            val pm = context.packageManager
            val installed = pm.getInstalledApplications(PackageManager.ApplicationInfoFlags.of(0))
            installed.map { info ->
                val label = pm.getApplicationLabel(info).toString()
                AppItem(
                    packageName = info.packageName,
                    appName = label,
                    isSystem = info.flags and ApplicationInfo.FLAG_SYSTEM != 0
                )
            }.sortedBy { it.appName.lowercase() }
        }
    }

    val filtered = remember(searchQuery, apps, showSystemApps) {
        val filteredBySearch = if (searchQuery.isBlank()) apps
        else apps.filter {
            it.appName.contains(searchQuery, ignoreCase = true) ||
            it.packageName.contains(searchQuery, ignoreCase = true)
        }
        if (showSystemApps) filteredBySearch
        else filteredBySearch.filter { !it.isSystem }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Select apps") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                actions = {
                    TextButton(onClick = { onSave(selected) }) {
                        Text("Save (${selected.size})")
                    }
                }
            )
        }
    ) { padding ->
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(horizontal = 16.dp)
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                OutlinedTextField(
                    value = searchQuery,
                    onValueChange = { searchQuery = it },
                    placeholder = { Text("Search apps...") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp)
                )

                Row(
                    modifier = Modifier.fillMaxWidth().padding(bottom = 4.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Checkbox(
                        checked = showSystemApps,
                        onCheckedChange = { showSystemApps = it }
                    )
                    Text(
                        text = "Show system apps",
                        style = MaterialTheme.typography.bodyMedium
                    )
                }

                LazyColumn(
                    modifier = Modifier.weight(1f).fillMaxWidth(),
                    verticalArrangement = Arrangement.spacedBy(2.dp)
                ) {
                    items(filtered, key = { it.packageName }) { app ->
                        val checked = app.packageName in selected
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable {
                                    selected = if (checked) selected - app.packageName
                                    else selected + app.packageName
                                }
                                .padding(vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            val iconDrawable = remember(app.packageName) {
                                try {
                                    context.packageManager.getApplicationIcon(app.packageName)
                                } catch (_: Exception) { null }
                            }
                            AndroidView(
                                factory = { ctx ->
                                    ImageView(ctx).apply {
                                        scaleType = ImageView.ScaleType.FIT_CENTER
                                    }
                                },
                                update = { iv ->
                                    iv.setImageDrawable(iconDrawable)
                                },
                                modifier = Modifier.size(40.dp).padding(end = 12.dp)
                            )
                            Column(modifier = Modifier.weight(1f)) {
                                Text(
                                    text = app.appName,
                                    style = MaterialTheme.typography.bodyMedium,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                                Text(
                                    text = app.packageName,
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }
                            Checkbox(
                                checked = checked,
                                onCheckedChange = {
                                    selected = if (checked) selected - app.packageName
                                    else selected + app.packageName
                                }
                            )
                        }
                    }
                }
            }

            if (apps.isEmpty()) {
                Text(
                    text = "Loading apps...",
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.align(Alignment.Center)
                )
            }
        }
    }
}
