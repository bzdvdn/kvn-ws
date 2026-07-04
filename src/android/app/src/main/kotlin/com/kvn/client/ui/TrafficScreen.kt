package com.kvn.client.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.kvn.client.transport.ConnectionState
import com.kvn.client.ui.theme.KvnPrimary
import com.kvn.client.ui.theme.KvnSuccess

private data class SpeedSample(val speedRx: Float, val speedTx: Float)

private fun computeSpeed(points: List<TrafficPoint>): List<SpeedSample> {
    if (points.size < 2) return emptyList()
    val samples = mutableListOf<SpeedSample>()
    for (i in 1 until points.size) {
        val dt = points[i].timestamp - points[i - 1].timestamp
        if (dt <= 0) continue
        val drx = (points[i].rxBytes - points[i - 1].rxBytes).toFloat() / dt * 1000f
        val dtx = (points[i].txBytes - points[i - 1].txBytes).toFloat() / dt * 1000f
        samples.add(SpeedSample(drx.coerceAtLeast(0f), dtx.coerceAtLeast(0f)))
    }
    return samples
}

private fun formatSpeed(bytesPerSec: Float): String = when {
    bytesPerSec < 1024 -> "%.0f B/s".format(bytesPerSec)
    bytesPerSec < 1024 * 1024 -> "%.0f KB/s".format(bytesPerSec / 1024)
    else -> "%.1f MB/s".format(bytesPerSec / (1024 * 1024))
}

private fun formatDuration(seconds: Long): String {
    val h = seconds / 3600
    val m = (seconds % 3600) / 60
    val s = seconds % 60
    return if (h > 0) "%d:%02d:%02d".format(h, m, s)
    else "%02d:%02d".format(m, s)
}

// @sk-task android-per-server-override-ui#T3.3: full traffic screen with stat cards and line chart (AC-008)
@Composable
fun TrafficScreen(vm: MainViewModel = viewModel()) {
    val state by vm.connectionState.collectAsState()
    val rxBytes by vm.rxBytes.collectAsState()
    val txBytes by vm.txBytes.collectAsState()
    val historyPoints by vm.trafficHistoryPoints.collectAsState()

    if (state != ConnectionState.CONNECTED) {
        Column(
            modifier = Modifier.fillMaxSize().padding(16.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                text = "Connect to see traffic",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        return
    }

    val speeds = remember(historyPoints) { computeSpeed(historyPoints) }
    val currentSpeed = speeds.lastOrNull()
    val peakSpeed = speeds.maxOfOrNull { maxOf(it.speedRx, it.speedTx) } ?: 0f
    val sessionDuration = if (historyPoints.isNotEmpty())
        (System.currentTimeMillis() - historyPoints.first().timestamp) / 1000
    else 0L

    Column(
        modifier = Modifier.fillMaxSize().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        Text(
            text = "Traffic",
            style = MaterialTheme.typography.headlineSmall
        )

        // Stat cards
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            StatCard(
                modifier = Modifier.weight(1f),
                value = formatBytes(rxBytes),
                sub = "Download",
                speed = currentSpeed?.let { formatSpeed(it.speedRx) } ?: "",
                color = KvnPrimary
            )
            StatCard(
                modifier = Modifier.weight(1f),
                value = formatBytes(txBytes),
                sub = "Upload",
                speed = currentSpeed?.let { formatSpeed(it.speedTx) } ?: "",
                color = KvnSuccess
            )
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            StatCard(
                modifier = Modifier.weight(1f),
                value = formatDuration(sessionDuration),
                sub = "Session Time",
                speed = "",
                color = MaterialTheme.colorScheme.onSurface
            )
            StatCard(
                modifier = Modifier.weight(1f),
                value = formatSpeed(peakSpeed),
                sub = "Peak Speed",
                speed = "",
                color = MaterialTheme.colorScheme.onSurface
            )
        }

        // Line chart
        Card(
            modifier = Modifier.fillMaxWidth().height(160.dp),
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
        ) {
            Column {
                Text(
                    text = "Speed (last 60s)",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 12.dp, top = 8.dp)
                )
                if (speeds.size >= 2) {
                    TrafficLineChart(
                        speeds = speeds,
                        modifier = Modifier.fillMaxSize().padding(4.dp)
                    )
                } else {
                    Box(
                        modifier = Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "Collecting data...",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                }
            }
        }

        // Legend
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            LegendItem(color = KvnPrimary, label = "Download")
            LegendItem(color = KvnSuccess, label = "Upload")
        }
    }
}

@Composable
private fun StatCard(
    modifier: Modifier = Modifier,
    value: String,
    sub: String,
    speed: String,
    color: Color
) {
    Card(
        modifier = modifier,
        colors = CardDefaults.cardColors(containerColor = color.copy(alpha = 0.08f))
    ) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                text = value,
                style = MaterialTheme.typography.titleLarge,
                color = color
            )
            if (speed.isNotEmpty()) {
                Text(
                    text = speed,
                    style = MaterialTheme.typography.bodySmall,
                    color = color.copy(alpha = 0.7f)
                )
            }
            Text(
                text = sub,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

@Composable
private fun TrafficLineChart(speeds: List<SpeedSample>, modifier: Modifier = Modifier) {
    val rxColor = KvnPrimary
    val txColor = KvnSuccess
    val maxSpeed = speeds.maxOf { maxOf(it.speedRx, it.speedTx) }.coerceAtLeast(1f)

    Canvas(modifier = modifier) {
        val w = size.width
        val h = size.height
        val stepX = w / (speeds.size - 1).coerceAtLeast(1)

        // RX line
        val rxPath = Path().apply {
            moveTo(0f, h - (speeds[0].speedRx / maxSpeed) * h)
            for (i in 1 until speeds.size) {
                lineTo(i * stepX, h - (speeds[i].speedRx / maxSpeed) * h)
            }
        }
        drawPath(rxPath, rxColor, style = Stroke(width = 2f))

        // TX line
        val txPath = Path().apply {
            moveTo(0f, h - (speeds[0].speedTx / maxSpeed) * h)
            for (i in 1 until speeds.size) {
                lineTo(i * stepX, h - (speeds[i].speedTx / maxSpeed) * h)
            }
        }
        drawPath(txPath, txColor, style = Stroke(width = 2f))
    }
}

@Composable
private fun LegendItem(color: Color, label: String) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Surface(
            modifier = Modifier.size(8.dp),
            shape = MaterialTheme.shapes.extraSmall,
            color = color
        ) {}
        Spacer(modifier = Modifier.width(4.dp))
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}
