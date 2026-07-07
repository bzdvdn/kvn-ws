package com.kvn.client.logger

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch
import java.io.File
import java.time.ZoneId
import java.time.format.DateTimeFormatter

private val levelColors = mapOf(
    LogLevel.DEBUG to Color(0xFF9E9E9E),
    LogLevel.INFO to Color(0xFF4CAF50),
    LogLevel.WARN to Color(0xFFFF9800),
    LogLevel.ERROR to Color(0xFFF44336)
)

// @sk-task android-log-tag#T2.1: LogViewerScreen with live streaming + auto-scroll (AC-001)
// @sk-task android-log-tag#T3.3: filter by level and tag (AC-002, AC-003)
// @sk-task android-log-tag#T3.4: text search with highlight (AC-004)
// @sk-task android-log-tag#T3.5: pause/resume auto-scroll (AC-005)
// @sk-task android-log-tag#T4.1: copy single entry on long-press (AC-006)
@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun LogViewerScreen() {
    val allEntries = remember { mutableStateListOf<LogEntry>().also { it.addAll(AppLogger.snapshot()) } }
    val listState = rememberLazyListState()
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val clipboard = LocalClipboardManager.current
    val fmt = remember { DateTimeFormatter.ofPattern("HH:mm:ss.SSS").withZone(ZoneId.systemDefault()) }

    var searchQuery by remember { mutableStateOf("") }
    var selectedLevels by remember { mutableStateOf(setOf<LogLevel>()) }
    var selectedTags by remember { mutableStateOf(setOf<String>()) }
    var isPaused by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        AppLogger.logFlow.collect { allEntries.add(it) }
    }

    val allTags = remember(allEntries.size) {
        allEntries.map { it.tag }.distinct().sorted()
    }

    val filteredEntries by remember(allEntries.size, selectedLevels, selectedTags, searchQuery) {
        derivedStateOf {
            allEntries.filter { entry ->
                (selectedLevels.isEmpty() || entry.level in selectedLevels) &&
                (selectedTags.isEmpty() || entry.tag in selectedTags) &&
                (searchQuery.isEmpty() || entry.message.contains(searchQuery, ignoreCase = true) || entry.tag.contains(searchQuery, ignoreCase = true))
            }
        }
    }

    LaunchedEffect(filteredEntries.size) {
        if (!isPaused && filteredEntries.isNotEmpty()) {
            val lastVisible = listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index
            if (lastVisible == null || lastVisible >= filteredEntries.size - 2) {
                listState.animateScrollToItem(filteredEntries.size - 1)
            }
        }
    }

    Column(modifier = Modifier.fillMaxSize()) {
        // Search bar
        OutlinedTextField(
            value = searchQuery,
            onValueChange = { searchQuery = it },
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp),
            placeholder = { Text("Search...") },
            leadingIcon = { Icon(Icons.Default.Search, contentDescription = null) },
            trailingIcon = {
                if (searchQuery.isNotEmpty()) {
                    IconButton(onClick = { searchQuery = "" }) {
                        Icon(Icons.Default.Close, contentDescription = "Clear search")
                    }
                }
            },
            singleLine = true,
            textStyle = MaterialTheme.typography.bodySmall
        )

        // Level filter chips
        Row(
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            FilterChip(
                selected = selectedLevels.isEmpty(),
                onClick = { selectedLevels = emptySet() },
                label = { Text("All", fontSize = 11.sp) }
            )
            LogLevel.entries.forEach { level ->
                val color = levelColors[level] ?: Color.Gray
                FilterChip(
                    selected = level in selectedLevels || selectedLevels.isEmpty(),
                    onClick = {
                        selectedLevels = if (level in selectedLevels) selectedLevels - level
                        else selectedLevels + level
                    },
                    label = { Text(level.name, fontSize = 11.sp, color = color) },
                    colors = FilterChipDefaults.filterChipColors(
                        selectedContainerColor = color.copy(alpha = 0.2f)
                    )
                )
            }
        }

        // Tag filter chips
        if (allTags.isNotEmpty()) {
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                FilterChip(
                    selected = selectedTags.isEmpty(),
                    onClick = { selectedTags = emptySet() },
                    label = { Text("All tags", fontSize = 11.sp) }
                )
                allTags.forEach { tag ->
                    FilterChip(
                        selected = tag in selectedTags || selectedTags.isEmpty(),
                        onClick = {
                            selectedTags = if (tag in selectedTags) selectedTags - tag
                            else selectedTags + tag
                        },
                        label = { Text(tag, fontSize = 11.sp) }
                    )
                }
            }
        }

        // Action buttons row
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 2.dp),
            horizontalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            SmallButton(onClick = { AppLogger.clear(); allEntries.clear() }) {
                Icon(Icons.Default.Delete, contentDescription = "Clear", modifier = Modifier.size(16.dp))
                Text("Clear", fontSize = 11.sp)
            }
            SmallButton(onClick = {
                val text = buildLogText(filteredEntries, fmt)
                clipboard.setText(AnnotatedString(text))
            }) {
                Icon(Icons.Default.ContentCopy, contentDescription = "Copy", modifier = Modifier.size(16.dp))
                Text("Copy", fontSize = 11.sp)
            }
            SmallButton(onClick = {
                scope.launch {
                    val text = buildLogText(allEntries, fmt)
                    val file = saveLogFile(context, text)
                    if (file != null) {
                        val uri = androidx.core.content.FileProvider.getUriForFile(
                            context, "${context.packageName}.fileprovider", file
                        )
                        val intent = android.content.Intent(android.content.Intent.ACTION_SEND).apply {
                            type = "text/plain"
                            putExtra(android.content.Intent.EXTRA_STREAM, uri)
                            addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION)
                        }
                        context.startActivity(android.content.Intent.createChooser(intent, "Share logs"))
                    }
                }
            }) {
                Icon(Icons.Default.Share, contentDescription = "Share", modifier = Modifier.size(16.dp))
                Text("Share", fontSize = 11.sp)
            }
            SmallButton(onClick = {
                scope.launch {
                    val text = buildLogText(allEntries, fmt)
                    saveLogFile(context, text)
                }
            }) {
                Icon(Icons.Default.FileDownload, contentDescription = "Export", modifier = Modifier.size(16.dp))
                Text("Export", fontSize = 11.sp)
            }
        }

        // Log list or empty state
        if (filteredEntries.isEmpty()) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text("No logs", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "Use the app to generate log entries.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
        } else {
            Box(modifier = Modifier.weight(1f)) {
                LazyColumn(
                    state = listState,
                    modifier = Modifier.fillMaxSize().padding(horizontal = 4.dp),
                    verticalArrangement = Arrangement.spacedBy(2.dp)
                ) {
                    items(filteredEntries, key = { "${it.timestamp}-${it.thread}-${System.identityHashCode(it)}" }) { entry ->
                        val levelColor = levelColors[entry.level] ?: Color.Gray
                        val ts = fmt.format(entry.timestamp)
                        val lineText = "[$ts][${entry.level.name.padEnd(5)}][${entry.tag}] ${entry.message}"
                        val displayText = if (searchQuery.isNotEmpty()) {
                            highlightText(lineText, searchQuery)
                        } else {
                            AnnotatedString(lineText)
                        }

                        Text(
                            text = displayText,
                            fontFamily = FontFamily.Monospace,
                            fontSize = 10.sp,
                            color = levelColor,
                            maxLines = 3,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.fillMaxWidth().combinedClickable(
                                onClick = {},
                                onLongClick = {
                                    clipboard.setText(AnnotatedString(lineText))
                                }
                            )
                        )
                    }
                }

                // Scroll-to-bottom FAB
                if (isPaused || (!isPaused && listState.layoutInfo.visibleItemsInfo.lastOrNull()?.let {
                        it.index < filteredEntries.size - 1
                    } == true)) {
                    FloatingActionButton(
                        onClick = {
                            isPaused = false
                            scope.launch {
                                if (filteredEntries.isNotEmpty()) {
                                    listState.animateScrollToItem(filteredEntries.size - 1)
                                }
                            }
                        },
                        modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp).size(40.dp),
                        containerColor = MaterialTheme.colorScheme.primaryContainer
                    ) {
                        Icon(Icons.Default.ArrowDownward, contentDescription = "Scroll to bottom")
                    }
                }

                // Detect user scroll away from bottom → pause
                LaunchedEffect(listState.layoutInfo.visibleItemsInfo) {
                    val last = listState.layoutInfo.visibleItemsInfo.lastOrNull()
                    if (last != null && last.index < filteredEntries.size - 1) {
                        isPaused = true
                    } else if (last != null && last.index >= filteredEntries.size - 1) {
                        isPaused = false
                    }
                }
            }
        }
    }
}

private fun highlightText(text: String, query: String): AnnotatedString {
    return buildAnnotatedString {
        var start = 0
        while (true) {
            val idx = text.indexOf(query, start, ignoreCase = true)
            if (idx < 0) {
                append(text.substring(start))
                break
            }
            append(text.substring(start, idx))
            pushStyle(SpanStyle(background = Color(0xFFFFEB3B)))
            append(text.substring(idx, idx + query.length))
            pop()
            start = idx + query.length
        }
    }
}

private fun buildLogText(entries: List<LogEntry>, fmt: DateTimeFormatter): String {
    return entries.joinToString("\n") { entry ->
        val ts = fmt.format(entry.timestamp)
        "[$ts][${entry.level.name.padEnd(5)}][${entry.tag}] ${entry.message}"
    }
}

private fun saveLogFile(context: android.content.Context, text: String): File? {
    return try {
        val dir = context.getExternalFilesDir(android.os.Environment.DIRECTORY_DOWNLOADS)
        val file = File(dir, "kvn-logs-${java.time.LocalDateTime.now().format(
            java.time.format.DateTimeFormatter.ofPattern("yyyy-MM-dd-HHmmss")
        )}.txt")
        file.parentFile?.mkdirs()
        file.writeText(text)
        file
    } catch (_: Exception) {
        null
    }
}

@Composable
private fun SmallButton(onClick: () -> Unit, content: @Composable RowScope.() -> Unit) {
    OutlinedButton(
        onClick = onClick,
        contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp),
        modifier = Modifier.height(28.dp)
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(2.dp)) {
            content()
        }
    }
}
