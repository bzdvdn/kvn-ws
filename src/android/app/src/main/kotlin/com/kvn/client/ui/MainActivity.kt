package com.kvn.client.ui

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.material3.MaterialTheme
import com.kvn.client.ui.theme.DarkKvnWebColorScheme

// @sk-task kvn-android#T1.3: Main activity entry point (AC-001)
// @sk-task multi-server-android-client#T1.2: wrap in darkColorScheme (AC-005)
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme(colorScheme = DarkKvnWebColorScheme) {
                ConnectScreen()
            }
        }
    }
}
