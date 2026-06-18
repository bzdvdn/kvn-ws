package com.kvn.client.ui.theme

import androidx.compose.material3.darkColorScheme
import androidx.compose.ui.graphics.Color

// @sk-task multi-server-android-client#T1.2: kvn-web dark palette (AC-005)
val KvnBg = Color(0xFF161616)
val KvnSurface = Color(0xFF222222)
val KvnTextPrimary = Color(0xFFD0D0D0)
val KvnTextSecondary = Color(0xFF888888)
val KvnPrimary = Color(0xFF1A5A9E)
val KvnSuccess = Color(0xFF2E7D32)
val KvnError = Color(0xFFB71C1C)
val KvnWarning = Color(0xFFFF9800)
val KvnBorder = Color(0xFF2A2A2A)

// @sk-task multi-server-android-client#T1.2: darkColorScheme matching kvn-web UI (AC-005)
val DarkKvnWebColorScheme = darkColorScheme(
    primary = KvnPrimary,
    onPrimary = Color.White,
    primaryContainer = KvnPrimary.copy(alpha = 0.3f),
    secondary = KvnTextSecondary,
    background = KvnBg,
    onBackground = KvnTextPrimary,
    surface = KvnSurface,
    onSurface = KvnTextPrimary,
    surfaceVariant = KvnSurface,
    onSurfaceVariant = KvnTextSecondary,
    error = KvnError,
    onError = Color.White,
    outline = KvnBorder
)
