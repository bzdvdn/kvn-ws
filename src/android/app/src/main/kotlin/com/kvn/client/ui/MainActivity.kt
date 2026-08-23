package com.kvn.client.ui

import android.content.ClipboardManager
import android.content.Context
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.List
import androidx.compose.material.icons.filled.ShowChart
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import com.kvn.client.logger.LogViewerScreen
import com.kvn.client.ui.theme.DarkKvnWebColorScheme
import java.io.File
import java.io.PrintWriter
import java.io.StringWriter

// @sk-task android-crash-report#T1: persist uncaught exception stack to file for on-screen report (AC-001)
private fun installCrashHandler(context: Context) {
    val prev = Thread.getDefaultUncaughtExceptionHandler()
    Thread.setDefaultUncaughtExceptionHandler { _, throwable ->
        runCatching {
            val sw = StringWriter()
            PrintWriter(sw).use { throwable.printStackTrace(it) }
            File(context.filesDir, "crash.txt").writeText(sw.toString())
        }
        prev?.uncaughtException(Thread.currentThread(), throwable) ?: Runtime.getRuntime().exit(2)
    }
}

// @sk-task kvn-android#T1.3: Main activity entry point (AC-001)
// @sk-task multi-server-android-client#T1.2: wrap in darkColorScheme (AC-005)
// @sk-task android-per-server-override-ui#T2.5: Bottom Navigation with 3 tabs (AC-005)
// @sk-task android-log-tag#T2.2: 4th tab Logs between Settings and Traffic (RQ-019)
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // @sk-task android-crash-report#T1: persist crash stack for on-screen report without adb (AC-001)
        installCrashHandler(applicationContext)
        val crashFile = File(applicationContext.filesDir, "crash.txt")
        val crashText = if (crashFile.exists()) crashFile.readText() else null
        crashFile.delete()
        setContent {
            MaterialTheme(colorScheme = DarkKvnWebColorScheme) {
                var lastCrash by remember { mutableStateOf(crashText) }
                // @sk-task android-crash-report#T1: show last crash dialog (AC-001)
                lastCrash?.let { stack ->
                    AlertDialog(
                        onDismissRequest = { lastCrash = null },
                        title = { Text("App crashed on last run") },
                        text = {
                            Column(
                                modifier = Modifier
                                    .verticalScroll(rememberScrollState())
                                    .heightIn(max = 320.dp)
                            ) {
                                Text(
                                    text = stack,
                                    style = MaterialTheme.typography.bodySmall,
                                    fontFamily = FontFamily.Monospace,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant
                                )
                            }
                        },
                        confirmButton = {
                            TextButton(onClick = {
                                val cm = applicationContext.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                                cm.setPrimaryClip(android.content.ClipData.newPlainText("kvn crash", stack))
                                Toast.makeText(applicationContext, "Crash stack copied", Toast.LENGTH_SHORT).show()
                            }) { Text("Copy") }
                        },
                        dismissButton = {
                            TextButton(onClick = { lastCrash = null }) { Text("Close") }
                        }
                    )
                }
                var selectedTab by remember { mutableStateOf(0) }

                Column(modifier = Modifier.fillMaxSize()) {
                    Box(modifier = Modifier.weight(1f)) {
                        when (selectedTab) {
                            0 -> ConnectScreen()
                            1 -> SettingsScreen()
                            2 -> LogViewerScreen()
                            3 -> TrafficScreen()
                        }
                    }
                    NavigationBar {
                        NavigationBarItem(
                            selected = selectedTab == 0,
                            onClick = { selectedTab = 0 },
                            icon = { Icon(Icons.Default.Home, contentDescription = "Connect") },
                            label = { Text("Connect") }
                        )
                        NavigationBarItem(
                            selected = selectedTab == 1,
                            onClick = { selectedTab = 1 },
                            icon = { Icon(Icons.Default.Dns, contentDescription = "Settings") },
                            label = { Text("Settings") }
                        )
                        NavigationBarItem(
                            selected = selectedTab == 2,
                            onClick = { selectedTab = 2 },
                            icon = { Icon(Icons.Default.List, contentDescription = "Logs") },
                            label = { Text("Logs") }
                        )
                        NavigationBarItem(
                            selected = selectedTab == 3,
                            onClick = { selectedTab = 3 },
                            icon = { Icon(Icons.Default.ShowChart, contentDescription = "Traffic") },
                            label = { Text("Traffic") }
                        )
                    }
                }
            }
        }
    }
}
