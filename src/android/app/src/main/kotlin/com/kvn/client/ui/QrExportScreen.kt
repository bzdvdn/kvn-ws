package com.kvn.client.ui

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.unit.dp
import com.google.zxing.qrcode.QRCodeWriter
import com.kvn.client.config.ConnectionConfig

// @sk-task kvn-android#T5.2: QR code export screen
@Composable
fun QrExportScreen(
    config: ConnectionConfig,
    onDismiss: () -> Unit
) {
    val bitmap = remember {
        val json = configToWebJson(config)
        generateQrBitmap(json, 512)
    }

    Scaffold { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Text(
                text = "Export Config QR",
                style = MaterialTheme.typography.headlineSmall
            )

            Text(
                text = "Scan this QR code with another device to import config",
                style = MaterialTheme.typography.bodyMedium
            )

            bitmap?.let {
                Image(
                    bitmap = it.asImageBitmap(),
                    contentDescription = "Config QR Code",
                    modifier = Modifier
                        .size(280.dp)
                        .padding(8.dp)
                )
            } ?: Text("Failed to generate QR code")

            Spacer(modifier = Modifier.weight(1f))

            Button(
                onClick = onDismiss,
                modifier = Modifier
                    .fillMaxWidth()
                    .height(48.dp)
            ) {
                Text("Close")
            }
        }
    }
}

// @sk-task kvn-android#T5.2: generate QR code bitmap from text (ZXing)
fun generateQrBitmap(text: String, size: Int = 512): Bitmap? {
    return try {
        val writer = QRCodeWriter()
        val bitMatrix = writer.encode(text, com.google.zxing.BarcodeFormat.QR_CODE, size, size)
        val bitmap = Bitmap.createBitmap(size, size, Bitmap.Config.RGB_565)
        for (x in 0 until size) {
            for (y in 0 until size) {
                bitmap.setPixel(x, y, if (bitMatrix[x, y]) android.graphics.Color.BLACK else android.graphics.Color.WHITE)
            }
        }
        bitmap
    } catch (_: Exception) {
        null
    }
}
