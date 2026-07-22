package com.kvn.client.ui

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.ImageFormat
import android.graphics.Rect
import android.graphics.YuvImage
import android.util.Size as AndroidSize
import android.widget.Toast
import androidx.camera.core.CameraSelector
import com.kvn.client.logger.AppLogger
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import com.google.mlkit.vision.barcode.BarcodeScannerOptions
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage
import com.google.zxing.BinaryBitmap
import com.google.zxing.ChecksumException
import com.google.zxing.FormatException
import com.google.zxing.NotFoundException
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.common.GlobalHistogramBinarizer
import com.google.zxing.qrcode.QRCodeReader
import com.kvn.client.config.ConnectionConfig
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

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
    val sni: List<String>? = emptyList()
)

// @sk-task android-dns-cache#T4.2: QR JSON model for dns_cache.enabled (AC-009)
@Serializable
data class WebDnsCacheCfg(val enabled: Boolean = false)

// @sk-task android-web-config-alignment#T1.1: web JSON model for dns_routing with enabled + ttl (web-compat)
@Serializable
data class WebDnsRoutingCfg(val enabled: Boolean = false, val ttl: Int = 3600)

// @sk-task android-web-config-alignment#T1.1: web JSON model for log.level (web-compat)
@Serializable
data class WebLogCfg(val level: String = "info")

// @sk-task android-web-config-alignment#T1.1: web JSON routing model with dns_routing, domains, geoip_url (web-compat)
@Serializable
data class WebRoutingCfg(
    val default_route: String = "server",
    val include_ranges: List<String>? = emptyList(),
    val exclude_ranges: List<String>? = emptyList(),
    val include_ips: List<String>? = emptyList(),
    val exclude_ips: List<String>? = emptyList(),
    val dns_cache: WebDnsCacheCfg? = null,
    val dns_routing: WebDnsRoutingCfg? = null,
    val include_domains: List<String>? = emptyList(),
    val exclude_domains: List<String>? = emptyList(),
    val geoip_url: String? = null
)

@Serializable
data class WebKillSwitchCfg(val enabled: Boolean = false)

@Serializable
data class WebReconnectCfg(val min_backoff_sec: Int = 1, val max_backoff_sec: Int = 30)

@Serializable
data class WebCryptoCfg(val enabled: Boolean = false, val key: String = "")

// @sk-task android-web-config-alignment#T1.1: web JSON config model with name + log (web-compat)
@Serializable
private data class WebConfig(
    val server: String = "",
    val transport: String = "tcp",
    val obfuscation: WebObfuscationCfg? = null,
    val auth: WebAuthCfg = WebAuthCfg(),
    val tls: WebTlsCfg = WebTlsCfg(),
    val mtu: Int = 1400,
    val ipv6: Boolean = false,
    val auto_reconnect: Boolean? = true,
    val routing: WebRoutingCfg? = null,
    val kill_switch: WebKillSwitchCfg? = null,
    val reconnect: WebReconnectCfg? = null,
    val mode: String = "tun",
    val crypto: WebCryptoCfg = WebCryptoCfg(),
    val multiplex: Boolean = false,
    val max_message_size: Int = 65535,
    val name: String? = null,
    val log: WebLogCfg? = null
)

// @sk-task kvn-android#T5.2: QR code scanner screen with finder overlay (AC-007, RQ-011)
// @sk-task android-log-tag#T3.2: migrated android.util.Log to AppLogger (AC-012)
@Composable
fun QrScannerScreen(
    onQrScanned: (ConnectionConfig, name: String) -> Unit,
    onCancel: () -> Unit
) {
    val context = LocalContext.current
    val cameraProviderFuture = remember { ProcessCameraProvider.getInstance(context) }
    val analyzer = remember {
        QrCodeAnalyzer(
            onReady = { AppLogger.d("QrScannerScreen", "analyzer ready") },
            onResult = { raw ->
                val config = parseQrConfig(raw)
                if (config != null) {
                    AppLogger.d("QrScannerScreen", "QR parsed OK: ${config.serverAddress}")
                    Toast.makeText(context, "Config loaded: ${config.serverAddress}", Toast.LENGTH_SHORT).show()
                    val name = try { JSONObject(raw).optString("name", "").ifBlank { "Imported" } } catch (_: Exception) { "Imported" }
                    onQrScanned(config, name)
                } else {
                    AppLogger.w("QrScannerScreen", "QR parse failed, raw=${raw.take(120)}")
                    Toast.makeText(context, "QR format not supported", Toast.LENGTH_LONG).show()
                }
            },
            onError = { msg ->
                Toast.makeText(context, "Scanner: $msg", Toast.LENGTH_LONG).show()
            }
        )
    }

    val lifecycleOwner = LocalLifecycleOwner.current

    // Гарантированно освобождаем камеру при выходе с экрана
    DisposableEffect(lifecycleOwner) {
        onDispose {
            cameraProviderFuture.get().unbindAll()
        }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        // Camera preview
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { ctx: android.content.Context ->
                val previewView = PreviewView(ctx)
                val cameraProvider = cameraProviderFuture.get()

                val preview = Preview.Builder().build()
                preview.setSurfaceProvider(previewView.surfaceProvider)

                val imageAnalysis = ImageAnalysis.Builder()
                    .setTargetResolution(AndroidSize(640, 480))
                    .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                    .build()
                    .also {
                        it.setAnalyzer(Executors.newSingleThreadExecutor(), analyzer)
                    }

                val cameraSelector = CameraSelector.DEFAULT_BACK_CAMERA

                try {
                    cameraProvider.unbindAll()
                    cameraProvider.bindToLifecycle(
                        lifecycleOwner,
                        cameraSelector,
                        preview,
                        imageAnalysis
                    )
                } catch (e: Exception) {
                    AppLogger.e("QrScannerScreen", "Camera bind failed", e)
                }

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

// @sk-task kvn-android#T5.2: QR barcode analyzer — ZXing primary + ML Kit fallback (AC-007)
class QrCodeAnalyzer(
    private val onReady: () -> Unit,
    private val onResult: (String) -> Unit,
    private val onError: ((String) -> Unit)? = null
) : ImageAnalysis.Analyzer {

    private val processing = AtomicBoolean(false)
    private var started = false
    private var frameCount = 0

    // ML Kit fallback
    private val mlOptions = BarcodeScannerOptions.Builder()
        .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
        .build()
    private val mlScanner = BarcodeScanning.getClient(mlOptions)

    override fun analyze(imageProxy: ImageProxy) {
        if (!processing.compareAndSet(false, true)) {
            imageProxy.close()
            return
        }

        if (!started) {
            started = true
            onReady()
        }

        frameCount++
        val w = imageProxy.width
        val h = imageProxy.height
        val fmt = imageProxy.format
        AppLogger.d("QrCodeAnalyzer", "frame#$frameCount ${w}x$h fmt=$fmt planes=${imageProxy.planes.size}")

        try {
            // 1) Try ZXing first (pure Java, no model download)
            var text = decodeZxing(imageProxy)
            if (text != null) {
                AppLogger.d("QrCodeAnalyzer", "ZXing decoded: ${text.take(80)}…")
                onResult(text)
                processing.set(false)
                imageProxy.close()
                return
            }

            // 2) Fallback to ML Kit BarcodeScanner
            val mediaImage = imageProxy.image
            if (mediaImage != null) {
                val input = InputImage.fromMediaImage(mediaImage, imageProxy.imageInfo.rotationDegrees)
                mlScanner.process(input)
                    .addOnSuccessListener { barcodes ->
                        if (barcodes.isEmpty()) {
                            AppLogger.d("QrCodeAnalyzer", "ML Kit: no barcodes")
                        } else {
                            for (b in barcodes) {
                                AppLogger.d("QrCodeAnalyzer", "ML Kit decoded: ${b.rawValue?.take(80)}…")
                                b.rawValue?.let { onResult(it) }
                            }
                        }
                    }
                    .addOnFailureListener { e ->
                        AppLogger.e("QrCodeAnalyzer", "ML Kit failed", e)
                        onError?.invoke("ML Kit: ${e.message}")
                    }
                    .addOnCompleteListener {
                        processing.set(false)
                        imageProxy.close()
                    }
            } else {
                val bitmap = imageProxyToBitmap(imageProxy)
                if (bitmap != null) {
                    AppLogger.d("QrCodeAnalyzer", "ML Kit bitmap fallback ${bitmap.width}x${bitmap.height}")
                    val input = InputImage.fromBitmap(bitmap, imageProxy.imageInfo.rotationDegrees)
                    mlScanner.process(input)
                        .addOnSuccessListener { barcodes ->
                            if (barcodes.isEmpty()) {
                                AppLogger.d("QrCodeAnalyzer", "ML Kit bitmap: no barcodes")
                            } else {
                                for (b in barcodes) {
                                    AppLogger.d("QrCodeAnalyzer", "ML Kit bitmap decoded: ${b.rawValue?.take(80)}…")
                                    b.rawValue?.let { onResult(it) }
                                }
                            }
                        }
                        .addOnFailureListener { e ->
                            AppLogger.e("QrCodeAnalyzer", "ML Kit bitmap fallback failed", e)
                            onError?.invoke("ML Kit: ${e.message}")
                        }
                        .addOnCompleteListener {
                            processing.set(false)
                            imageProxy.close()
                        }
                } else {
                    AppLogger.w("QrCodeAnalyzer", "mediaImage=null AND bitmap=null — cannot scan frame")
                    onError?.invoke("Camera frame not available")
                    processing.set(false)
                    imageProxy.close()
                }
            }
        } catch (e: Exception) {
            AppLogger.e("QrCodeAnalyzer", "analyze error", e)
            onError?.invoke("Scan error: ${e.message}")
            processing.set(false)
            imageProxy.close()
        }
    }

    private fun decodeZxing(proxy: ImageProxy): String? {
        return try {
            val result = when (proxy.planes.size) {
                3 -> {
                    val yPlane = proxy.planes[0]
                    val buf = yPlane.buffer.apply { rewind() }
                    val rowStride = yPlane.rowStride
                    val pixelStride = yPlane.pixelStride
                    val w = proxy.width
                    val h = proxy.height
                    // Downscale if too large — QR needs only ~640px max
                    val maxDim = 800
                    val scale = if (w.coerceAtLeast(h) > maxDim) {
                        (w.coerceAtLeast(h).toFloat() / maxDim).coerceAtLeast(1f)
                    } else 1f
                    val sw = if (scale > 1f) (w / scale).toInt() else w
                    val sh = if (scale > 1f) (h / scale).toInt() else h
                    val step = scale.toInt().coerceAtLeast(1)
                    if (scale > 1f) AppLogger.d("QrCodeAnalyzer", "ZXing: downscale ${w}x$h -> ${sw}x$sh (step=$step)")
                    val luminance = ByteArray(sw * sh)
                    for (row in 0 until sh) {
                        val srcRow = (row * step).coerceAtMost(h - 1)
                        val rowBase = srcRow * rowStride
                        val dstBase = row * sw
                        for (col in 0 until sw) {
                            val srcCol = (col * step).coerceAtMost(w - 1)
                            luminance[dstBase + col] = buf.get(rowBase + srcCol * pixelStride)
                        }
                    }
                    val source = PlanarYUVLuminanceSource(luminance, sw, sh, 0, 0, sw, sh, false)
                    val zxing = QRCodeReader()
                    val res = zxing.decode(BinaryBitmap(GlobalHistogramBinarizer(source)))
                    zxing.reset()
                    res.text
                }
                1 -> {
                    val buf = proxy.planes[0].buffer.apply { rewind() }
                    val bpp = proxy.planes[0].pixelStride
                    val w = proxy.width
                    val h = proxy.height
                    val maxDim = 800
                    val scale = if (w.coerceAtLeast(h) > maxDim) {
                        (w.coerceAtLeast(h).toFloat() / maxDim).coerceAtLeast(1f)
                    } else 1f
                    val sw = if (scale > 1f) (w / scale).toInt() else w
                    val sh = if (scale > 1f) (h / scale).toInt() else h
                    val step = scale.toInt().coerceAtLeast(1)
                    if (scale > 1f) AppLogger.d("QrCodeAnalyzer", "ZXing: downscale ${w}x$h -> ${sw}x$sh (step=$step)")
                    val luminance = ByteArray(sw * sh)
                    for (row in 0 until sh) {
                        val srcRow = (row * step).coerceAtMost(h - 1)
                        val dstBase = row * sw
                        for (col in 0 until sw) {
                            val srcCol = (col * step).coerceAtMost(w - 1)
                            val srcIdx = srcRow * w * bpp + srcCol * bpp
                            val r = buf.get(srcIdx).toInt() and 0xFF
                            val g = buf.get(srcIdx + 1).toInt() and 0xFF
                            val b = buf.get(srcIdx + 2).toInt() and 0xFF
                            luminance[dstBase + col] = ((r + g + g + b) / 4).toByte()
                        }
                    }
                    val source = PlanarYUVLuminanceSource(luminance, sw, sh, 0, 0, sw, sh, false)
                    val zxing = QRCodeReader()
                    val res = zxing.decode(BinaryBitmap(GlobalHistogramBinarizer(source)))
                    res.text
                }
                else -> {
                    AppLogger.w("QrCodeAnalyzer", "ZXing: unsupported plane count ${proxy.planes.size}")
                    null
                }
            }
            result
        } catch (_: NotFoundException) { AppLogger.d("QrCodeAnalyzer", "ZXing: not found"); null }
          catch (_: ChecksumException) { AppLogger.d("QrCodeAnalyzer", "ZXing: checksum error"); null }
          catch (_: FormatException) { AppLogger.d("QrCodeAnalyzer", "ZXing: format error"); null }
    }

    private fun imageProxyToBitmap(proxy: ImageProxy): Bitmap? {
        if (proxy.planes.size != 3) return null
        val w = proxy.width
        val h = proxy.height
        // Downscale NV21 for performance — ML Kit needs only ~800px max
        val maxDim = 800
        val scale = if (w.coerceAtLeast(h) > maxDim) {
            (w.coerceAtLeast(h).toFloat() / maxDim).coerceAtLeast(1f)
        } else 1f
        val sw = if (scale > 1f) (w / scale).toInt() else w
        val sh = if (scale > 1f) (h / scale).toInt() else h
        val step = scale.toInt().coerceAtLeast(1)
        if (scale > 1f) AppLogger.d("QrCodeAnalyzer", "bitmap: downscale ${w}x$h -> ${sw}x$sh")

        val yPlane = proxy.planes[0]
        val uPlane = proxy.planes[1]
        val vPlane = proxy.planes[2]
        val yBuf = yPlane.buffer.apply { rewind() }
        val uBuf = uPlane.buffer.apply { rewind() }
        val vBuf = vPlane.buffer.apply { rewind() }

        val ySize = sw * sh
        val uvSize = sw * sh / 2
        val nv21 = ByteArray(ySize + uvSize)
        val yRowStride = yPlane.rowStride
        val yPixelStride = yPlane.pixelStride
        val uRowStride = uPlane.rowStride
        val vRowStride = vPlane.rowStride
        val uvPixelStride = uPlane.pixelStride

        // Downsampled Y
        var yPos = 0
        for (row in 0 until sh) {
            val srcRow = (row * step).coerceAtMost(h - 1)
            val rowBase = srcRow * yRowStride
            for (col in 0 until sw) {
                val srcCol = (col * step).coerceAtMost(w - 1)
                nv21[yPos++] = yBuf.get(rowBase + srcCol * yPixelStride)
            }
        }

        // Downsampled UV (interleaved V/U for NV21)
        val uvW = w / 2
        val uvH = h / 2
        val suvW = sw / 2
        val suvH = sh / 2
        val uvStep = step.coerceAtLeast(2)
        var chromaPos = ySize
        for (row in 0 until suvH) {
            val srcRow = (row * uvStep / 2).coerceAtMost(uvH - 1)
            val vRowBase = srcRow * vRowStride
            val uRowBase = srcRow * uRowStride
            for (col in 0 until suvW) {
                val srcCol = (col * uvStep / 2).coerceAtMost(uvW - 1)
                nv21[chromaPos++] = vBuf.get(vRowBase + srcCol * uvPixelStride)
                nv21[chromaPos++] = uBuf.get(uRowBase + srcCol * uvPixelStride)
            }
        }

        val yuv = YuvImage(nv21, ImageFormat.NV21, sw, sh, null)
        val out = ByteArrayOutputStream()
        yuv.compressToJpeg(Rect(0, 0, sw, sh), 85, out)
        return BitmapFactory.decodeByteArray(out.toByteArray(), 0, out.size())
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
    } catch (e: Exception) {
        AppLogger.d("parseQrConfig", "android format failed: ${e.message}")
    }

    // Try kvn-web format
    try {
        val web = json.decodeFromString<WebConfig>(raw)
        return webToAndroidConfig(web)
    } catch (e: Exception) {
        AppLogger.e("parseQrConfig", "web format failed: ${e.message}")
        AppLogger.d("parseQrConfig", "raw QR: ${raw.take(200)}")
    }

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
        autoReconnect = web.auto_reconnect ?: true,
        maxMessageSize = web.max_message_size,
        multiplex = web.multiplex,
        logLevel = web.log?.level ?: "info",
        minBackoffSec = rc?.min_backoff_sec ?: 1,
        maxBackoffSec = rc?.max_backoff_sec ?: 30,
        tlsVerifyMode = web.tls.verify_mode,
        tlsServerName = web.tls.server_name,
        tlsSni = web.tls.sni ?: emptyList(),
        routingDefaultRoute = routing?.default_route ?: "server",
        routingIncludeRanges = routing?.include_ranges ?: emptyList(),
        routingExcludeRanges = routing?.exclude_ranges ?: emptyList(),
        routingIncludeIps = routing?.include_ips ?: emptyList(),
        routingExcludeIps = routing?.exclude_ips ?: emptyList(),
        routingIncludeDomains = routing?.include_domains ?: emptyList(),
        routingExcludeDomains = routing?.exclude_domains ?: emptyList(),
        geoipUrl = routing?.geoip_url ?: "",
        cryptoEnabled = web.crypto.enabled,
        cryptoKey = web.crypto.key,
        killSwitchEnabled = ks?.enabled ?: false,
        obfuscationEnabled = ob?.enabled ?: false,
        obfuscationUtls = ob?.utls?.enabled ?: false,
        obfuscationPaddingEnabled = ob?.padding?.enabled ?: false,
        obfuscationPaddingSize = ob?.padding?.size ?: 0,
        // @sk-task android-web-config-alignment#T1.1: map dns_routing (preferred) or dns_cache (backward compat) from QR JSON
        dnsCacheEnabled = routing?.dns_routing?.enabled ?: routing?.dns_cache?.enabled ?: false,
        // @sk-task android-web-config-alignment#T1.1: map dns_routing.ttl from QR JSON
        dnsCacheTtl = routing?.dns_routing?.ttl ?: 3600
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
    root.put("log", JSONObject().apply { put("level", config.logLevel) })
    root.put("routing", JSONObject().apply {
        put("default_route", config.routingDefaultRoute)
        put("include_ranges", JSONArray(config.routingIncludeRanges))
        put("exclude_ranges", JSONArray(config.routingExcludeRanges))
        put("include_ips", JSONArray(config.routingIncludeIps))
        put("exclude_ips", JSONArray(config.routingExcludeIps))
        put("include_domains", JSONArray(config.routingIncludeDomains))
        put("exclude_domains", JSONArray(config.routingExcludeDomains))
        put("geoip_url", config.geoipUrl)
        // @sk-task android-web-config-alignment#T1.1: export dns_routing to QR JSON (web-compatible field name)
        put("dns_routing", JSONObject().apply {
            put("enabled", config.dnsCacheEnabled)
            put("ttl", config.dnsCacheTtl)
        })
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
