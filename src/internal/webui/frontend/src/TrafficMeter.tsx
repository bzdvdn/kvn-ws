import type { MetricSnapshot } from "./types";
import { colors, borderRadius, fontSize } from "./theme";

interface TrafficMeterProps {
  metrics: MetricSnapshot[];
  latest: MetricSnapshot | null;
}

const meterContainer: React.CSSProperties = {
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: 8,
  marginTop: 8,
};

const meterItem: React.CSSProperties = {
  background: colors.cardBg,
  borderRadius: borderRadius.lg,
  padding: "10px 14px",
};

const meterTop: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
};

const label: React.CSSProperties = {
  fontSize: fontSize.sm,
  color: colors.textDim,
  textTransform: "uppercase" as const,
  letterSpacing: "0.5px",
};

const valueStyle: React.CSSProperties = {
  fontSize: 20,
  fontWeight: 700,
  fontVariantNumeric: "tabular-nums" as const,
};

const totalStyle: React.CSSProperties = {
  fontSize: fontSize.sm,
  color: colors.textMuted,
  textAlign: "right" as const,
};

function sparklineStyle(_dir: "down" | "up"): React.CSSProperties {
  return {
    height: 24,
    marginTop: 4,
    display: "flex",
    alignItems: "flex-end",
    gap: 1,
  };
}

function barStyle(val: number, maxVal: number, dir: "down" | "up"): React.CSSProperties {
  const pct = maxVal > 0 ? (val / maxVal) * 100 : 5;
  const color = dir === "down" ? colors.success : "#60a5fa";
  return {
    width: 3,
    height: `${Math.max(pct, 4)}%`,
    borderRadius: "1px 1px 0 0",
    background: `linear-gradient(to top, ${color}44, ${color})`,
  };
}

const subRow: React.CSSProperties = {
  display: "flex",
  gap: 16,
  fontSize: fontSize.sm,
  color: colors.textDim,
  marginTop: 6,
};

// @sk-task kvn-web-redesign#T2.3: traffic meter with RX/TX sparkline, latency, uptime (AC-001, AC-002, AC-014)
export default function TrafficMeter({ metrics, latest }: TrafficMeterProps) {
  const rxSpeed = latest?.rx_speed?.toFixed(1) ?? "0.0";
  const txSpeed = latest?.tx_speed?.toFixed(1) ?? "0.0";
  const rxTotal = latest?.rx_bytes ? formatBytes(latest.rx_bytes) : "0 B";
  const txTotal = latest?.tx_bytes ? formatBytes(latest.tx_bytes) : "0 B";
  const latency = latest?.latency_ms?.toFixed(0) ?? "—";
  const uptime = latest?.uptime_s ? formatUptime(latest.uptime_s) : "—";
  const reconnects = latest?.reconnects ?? 0;

  const bars = metrics.slice(-30);
  const maxRx = Math.max(...bars.map((m) => m.rx_speed), 0.1);
  const maxTx = Math.max(...bars.map((m) => m.tx_speed), 0.1);

  return (
    <div>
      <div style={meterContainer}>
        <div style={meterItem}>
          <div style={meterTop}>
            <div>
              <div style={label}>⬇ Download</div>
              <div>
                <span style={{ ...valueStyle, color: colors.success }}>{rxSpeed}</span>
                <span style={{ fontSize: fontSize.sm, color: colors.textDim, marginLeft: 2 }}>Mbps</span>
              </div>
            </div>
            <div style={totalStyle}>{rxTotal}</div>
          </div>
          <div style={sparklineStyle("down")}>
            {bars.map((m, i) => (
              <div key={i} style={barStyle(m.rx_speed, maxRx, "down")} />
            ))}
          </div>
        </div>
        <div style={meterItem}>
          <div style={meterTop}>
            <div>
              <div style={label}>⬆ Upload</div>
              <div>
                <span style={{ ...valueStyle, color: "#60a5fa" }}>{txSpeed}</span>
                <span style={{ fontSize: fontSize.sm, color: colors.textDim, marginLeft: 2 }}>Mbps</span>
              </div>
            </div>
            <div style={totalStyle}>{txTotal}</div>
          </div>
          <div style={sparklineStyle("up")}>
            {bars.map((m, i) => (
              <div key={i} style={barStyle(m.tx_speed, maxTx, "up")} />
            ))}
          </div>
        </div>
      </div>
      <div style={subRow}>
        <div>Latency <span style={{ color: latencyColor(parseFloat(latency)), fontWeight: 600 }}>{latency}ms</span></div>
        <div>Uptime <span style={{ color: colors.text, fontWeight: 600 }}>{uptime}</span></div>
        <div>Reconnects <span style={{ color: colors.warning, fontWeight: 600 }}>{reconnects}</span></div>
      </div>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatUptime(s: number): string {
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${sec}s`;
  return `${sec}s`;
}

function latencyColor(ms: number): string {
  if (ms < 50) return colors.success;
  if (ms < 150) return colors.warning;
  return colors.error;
}
