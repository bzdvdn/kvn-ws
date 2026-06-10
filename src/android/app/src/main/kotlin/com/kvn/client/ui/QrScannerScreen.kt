package com.kvn.client.ui

import android.util.Size as AndroidSize
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.*
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage
import com.kvn.client.config.ConnectionConfig
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import org.json.JSONArray
import org.json.JSONObject
import java.util.concurrent.Executors

private val json = Json { ignoreUnknownKeys = true }

// @sk-task kvn-android#T5.2: web JSON config model for cross-compat (AC-007)
@Serializable
data class WebObfuscationCfg(
    val enabled: Boolean = false,
    val utls: WebUtlsCfg? = null,
    val padding: WebPaddingCfg? = null
)

@Serializable
data class WebUtlsCfg(val enabled: Boolean = false)

@Serializable
data class WebPaddingCfg(val enabled: Boolean = false, val size: Int = 0)

@Serializable
data class WebAuthCfg(val token: String = "")

@Serializable
data class WebTlsCfg(
    val verify_mode: String = "verify",
    val server_name: String = "",
    val sni: List<String> = emptyList()
)

@Serializable
data class WebRoutingCfg(
    val default_route: String = "server",
    val include_ranges: List<String> = emptyList(),
    val exclude_ranges: List<String> = emptyList(),
    val include_ips: List<String> = emptyList(),
    val exclude_ips: List<String> = emptyList(),
    val include_domains: List<String> = emptyList(),
    val exclude_domains: List<String> = emptyList()
)

@Serializable
data class WebKillSwitchCfg(val enabled: Boolean = false)

@Serializable
data class WebReconnectCfg(val min_backoff_sec: Int = 1, val max_backoff_sec: Int = 30)

@Serializable
data class WebCryptoCfg(val enabled: Boolean = false, val key: String = "")

@Serializable
private data class WebConfig(
    val server: String = "",
    val transport: String = "tcp",
    val obfuscation: WebObfuscationCfg? = null,
    val auth: WebAuthCfg = WebAuthCfg(),
    val tls: WebTlsCfg = WebTlsCfg(),
    val mtu: Int = 1400,
    val ipv6: Boolean = false,
    val auto_reconnect: Boolean = true,
    val routing: WebRoutingCfg? = null,
    val kill_switch: WebKillSwitchCfg? = null,
    val reconnect: WebReconnectCfg? = null,
    val mode: String = "tun",
    val crypto: WebCryptoCfg = WebCryptoCfg(),
    val multiplex: Boolean = false,
    val max_message_size: Int = 65535
)

// @sk-task kvn-android#T5.2: QR code scanner screen with finder overlay (AC-007, RQ-011)
@Composable
fun QrScannerScreen(
    onQrScanned: (ConnectionConfig) -> Unit,
    onCancel: () -> Unit
) {
    val context = LocalContext.current
    val cameraProviderFuture = remember { ProcessCameraProvider.getInstance(context) }
    val analyzer = remember {
        QrCodeAnalyzer { barcode ->
            barcode.rawValue?.let { raw ->
                parseQrConfig(raw)?.let { config ->
                    onQrScanned(config)
                }
            }
        }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        // Camera preview
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { ctx ->
                val previewView = PreviewView(ctx)
                val cameraProvider = cameraProviderFuture.get()

                val preview = Preview.Builder().build()
                preview.setSurfaceProvider(previewView.surfaceProvider)

                val imageAnalysis = ImageAnalysis.Builder()
                    .setTargetResolution(AndroidSize(1280, 720))
                    .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                    .build()
                    .also {
                        it.setAnalyzer(Executors.newSingleThreadExecutor(), analyzer)
                    }

                val cameraSelector = CameraSelector.DEFAULT_BACK_CAMERA

                try {
                    cameraProvider.unbindAll()
                    cameraProvider.bindToLifecycle(
                        ctx as androidx.lifecycle.LifecycleOwner,
                        cameraSelector,
                        preview,
                        imageAnalysis
                    )
                } catch (_: Exception) { }

                previewView
            }
        )

        // Semi-transparent overlay with cutout
        androidx.compose.foundation.Canvas(modifier = Modifier.fillMaxSize()) {
            val w = size.width
            val h = size.height
            // Semi-transparent background
            val overlayColor = android.graphics.Color.argb(140, 0, 0, 0)
            // Cutout box (60% of width, centered)
            val boxSize = (w * 0.6f).coerceAtMost(h * 0.5f)
            val left = (w - boxSize) / 2
            val top = (h - boxSize) / 2
            val right = left + boxSize
            val bottom = top + boxSize

            // Draw four dark areas around the cutout
            drawRect(androidx.compose.ui.graphics.Color(overlayColor),
                topLeft = androidx.compose.ui.geometry.Offset(0f, 0f),
                size = androidx.compose.ui.geometry.Size(w, top))
            drawRect(androidx.compose.ui.graphics.Color(overlayColor),
                topLeft = androidx.compose.ui.geometry.Offset(0f, bottom),
                size = androidx.compose.ui.geometry.Size(w, h - bottom))
            drawRect(androidx.compose.ui.graphics.Color(overlayColor),
                topLeft = androidx.compose.ui.geometry.Offset(0f, top),
                size = androidx.compose.ui.geometry.Size(left, boxSize))
            drawRect(androidx.compose.ui.graphics.Color(overlayColor),
                topLeft = androidx.compose.ui.geometry.Offset(right, top),
                size = androidx.compose.ui.geometry.Size(w - right, boxSize))

            // Corner markers
            val cornerColor = androidx.compose.ui.graphics.Color.White
            val cornerLen = boxSize * 0.12f
            val strokeWidth = 4f
            // Top-left
            drawLine(cornerColor, androidx.compose.ui.geometry.Offset(left, top + cornerLen),
                androidx.compose.ui.geometry.Offset(left, top), strokeWidth)
            drawLine(cornerColor, androidx.compose.ui.geometry.Offset(left, top),
                androidx.compose.ui.geometry.Offset(left + cornerLen, top), strokeWidth)
            // Top-right
            drawLine(cornerColor, androidx.compose.ui.geometry.Offset(right - cornerLen, top),
                androidx.compose.ui.geometry.Offset(right, top), strokeWidth)
            drawLine(cornerColor, androidx.compose.ui.geometry.Offset(right, top),
                androidx.compose.ui.geometry.Offset(right, top + cornerLen), strokeWidth)
            // Bottom-left
            drawLine(cornerColor, androidx.compose.ui.geometry.Offset(left, bottom - cornerLen),
                androidx.compose.ui.geometry.Offset(left, bottom), strokeWidth)
            drawLine(cornerColor, androidx.compose.ui.geometry.Offset(left, bottom),
                androidx.compose.ui.geometry.Offset(left + cornerLen, bottom), strokeWidth)
            // Bottom-right
            drawLine(cornerColor, androidx.compose.ui.geometry.Offset(right - cornerLen, bottom),
                androidx.compose.ui.geometry.Offset(right, bottom), strokeWidth)
            drawLine(cornerColor, androidx.compose.ui.geometry.Offset(right, bottom),
                androidx.compose.ui.geometry.Offset(right, bottom - cornerLen), strokeWidth)
        }

        // Cancel button at bottom
        Button(
            onClick = onCancel,
            modifier = Modifier
                .align(androidx.compose.ui.Alignment.BottomCenter)
                .padding(32.dp)
                .fillMaxWidth(0.6f)
                .height(48.dp),
            colors = ButtonDefaults.buttonColors(
                containerColor = androidx.compose.ui.graphics.Color.White,
                contentColor = androidx.compose.ui.graphics.Color.Black
            )
        ) {
            Text("Cancel")
        }
    }
}

// @sk-task kvn-android#T5.2: QR barcode analyzer (AC-007)
class QrCodeAnalyzer(
    private val onBarcode: (Barcode) -> Unit
) : ImageAnalysis.Analyzer {

    private val scanner = BarcodeScanning.getClient()

    @androidx.camera.core.ExperimentalGetImage
    override fun analyze(imageProxy: ImageProxy) {
        val mediaImage = imageProxy.image
        if (mediaImage != null) {
            val image = InputImage.fromMediaImage(mediaImage, imageProxy.imageInfo.rotationDegrees)
            scanner.process(image)
                .addOnSuccessListener { barcodes ->
                    for (barcode in barcodes) {
                        onBarcode(barcode)
                    }
                }
                .addOnCompleteListener {
                    imageProxy.close()
                }
        } else {
            imageProxy.close()
        }
    }
}

// @sk-task kvn-android#T5.2: parse QR content — Android JSON, web JSON, or legacy "server:port:token" (AC-007, RQ-011)
fun parseQrConfig(raw: String): ConnectionConfig? {
    if (!raw.trimStart().startsWith("{")) {
        // Legacy format: "server:port:token"
        val parts = raw.split(":", limit = 3)
        if (parts.size < 3) return null
        val port = parts[1].toIntOrNull() ?: return null
        return ConnectionConfig(serverAddress = parts[0], port = port, token = parts[2])
    }

    // Try Android format first (only accept if serverAddress was populated)
    try {
        val cfg = json.decodeFromString<ConnectionConfig>(raw)
        if (cfg.serverAddress.isNotEmpty()) return cfg
    } catch (_: Exception) { }

    // Try kvn-web format
    try {
        val web = json.decodeFromString<WebConfig>(raw)
        return webToAndroidConfig(web)
    } catch (_: Exception) { }

    return null
}

// @sk-task kvn-android#T5.2: convert web JSON config to Android ConnectionConfig
private fun webToAndroidConfig(web: WebConfig): ConnectionConfig {
    // Parse server URL: "wss://host:port/path" or "host:port" or "host"
    val serverUrl = web.server.trim()
    var host = serverUrl
    var port = 443
    var path = "/kvn"
    try {
        val noScheme = serverUrl.substringAfter("://")
        val slashIdx = noScheme.indexOf("/")
        val hostPort = if (slashIdx >= 0) noScheme.substring(0, slashIdx) else noScheme
        if (slashIdx >= 0) path = noScheme.substring(slashIdx)
        val parts = hostPort.split(":")
        host = parts[0]
        if (parts.size > 1) port = parts[1].toIntOrNull() ?: 443
    } catch (_: Exception) { }

    val routing = web.routing
    val ks = web.kill_switch
    val rc = web.reconnect
    val ob = web.obfuscation

    val transport = if (web.transport == "quic") "tcp" else web.transport

    return ConnectionConfig(
        serverAddress = host,
        port = port,
        serverPath = path,
        transport = transport,
        token = web.auth.token,
        mode = web.mode,
        mtu = web.mtu,
        ipv6Enabled = web.ipv6,
        autoReconnect = web.auto_reconnect,
        maxMessageSize = web.max_message_size,
        multiplex = web.multiplex,
        minBackoffSec = rc?.min_backoff_sec ?: 1,
        maxBackoffSec = rc?.max_backoff_sec ?: 30,
        tlsVerifyMode = web.tls.verify_mode,
        tlsServerName = web.tls.server_name,
        tlsSni = web.tls.sni,
        routingDefaultRoute = routing?.default_route ?: "server",
        routingIncludeRanges = routing?.include_ranges ?: emptyList(),
        routingExcludeRanges = routing?.exclude_ranges ?: emptyList(),
        routingIncludeIps = routing?.include_ips ?: emptyList(),
        routingExcludeIps = routing?.exclude_ips ?: emptyList(),
        routingIncludeDomains = routing?.include_domains ?: emptyList(),
        routingExcludeDomains = routing?.exclude_domains ?: emptyList(),
        cryptoEnabled = web.crypto.enabled,
        cryptoKey = web.crypto.key,
        killSwitchEnabled = ks?.enabled ?: false,
        obfuscationEnabled = ob?.enabled ?: false,
        obfuscationUtls = ob?.utls?.enabled ?: false,
        obfuscationPaddingEnabled = ob?.padding?.enabled ?: false,
        obfuscationPaddingSize = ob?.padding?.size ?: 0
    )
}

// @sk-task kvn-android#T5.2: convert Android ConnectionConfig to web JSON string for QR export (web-compatible format)
fun configToWebJson(config: ConnectionConfig): String {
    val root = JSONObject()

    root.put("server", "${config.serverAddress}:${config.port}${config.serverPath}")
    root.put("transport", config.transport)
    root.put("mode", config.mode)
    root.put("mtu", config.mtu)
    root.put("ipv6", config.ipv6Enabled)
    root.put("auto_reconnect", config.autoReconnect)
    root.put("max_message_size", config.maxMessageSize)
    root.put("multiplex", config.multiplex)
    root.put("auth", JSONObject().apply { put("token", config.token) })
    root.put("tls", JSONObject().apply {
        put("verify_mode", config.tlsVerifyMode)
        put("server_name", config.tlsServerName)
        put("sni", JSONArray(config.tlsSni))
    })
    root.put("routing", JSONObject().apply {
        put("default_route", config.routingDefaultRoute)
        put("include_ranges", JSONArray(config.routingIncludeRanges))
        put("exclude_ranges", JSONArray(config.routingExcludeRanges))
        put("include_ips", JSONArray(config.routingIncludeIps))
        put("exclude_ips", JSONArray(config.routingExcludeIps))
        put("include_domains", JSONArray(config.routingIncludeDomains))
        put("exclude_domains", JSONArray(config.routingExcludeDomains))
    })
    root.put("kill_switch", JSONObject().apply { put("enabled", config.killSwitchEnabled) })
    root.put("reconnect", JSONObject().apply {
        put("min_backoff_sec", config.minBackoffSec)
        put("max_backoff_sec", config.maxBackoffSec)
    })
    root.put("crypto", JSONObject().apply {
        put("enabled", config.cryptoEnabled)
        put("key", config.cryptoKey)
    })

    if (config.obfuscationEnabled || config.obfuscationUtls || config.obfuscationPaddingEnabled) {
        root.put("obfuscation", JSONObject().apply {
            put("enabled", config.obfuscationEnabled)
            put("utls", JSONObject().apply { put("enabled", config.obfuscationUtls) })
            put("padding", JSONObject().apply {
                put("enabled", config.obfuscationPaddingEnabled)
                put("size", config.obfuscationPaddingSize)
            })
        })
    }

    return root.toString()
}
