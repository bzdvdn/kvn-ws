package com.kvn.client.ui

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.ShowChart
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import com.kvn.client.ui.theme.DarkKvnWebColorScheme

// @sk-task kvn-android#T1.3: Main activity entry point (AC-001)
// @sk-task multi-server-android-client#T1.2: wrap in darkColorScheme (AC-005)
// @sk-task android-per-server-override-ui#T2.5: Bottom Navigation with 3 tabs (AC-005)
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme(colorScheme = DarkKvnWebColorScheme) {
                var selectedTab by remember { mutableStateOf(0) }

                Column(modifier = Modifier.fillMaxSize()) {
                    Box(modifier = Modifier.weight(1f)) {
                        when (selectedTab) {
                            0 -> ConnectScreen()
                            1 -> SettingsScreen()
                            2 -> TrafficScreen()
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
                            icon = { Icon(Icons.Default.ShowChart, contentDescription = "Traffic") },
                            label = { Text("Traffic") }
                        )
                    }
                }
            }
        }
    }
}
