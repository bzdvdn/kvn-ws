import { useState, useEffect, useRef, useCallback } from "react";

type Status = "disconnected" | "connecting" | "connected" | "error";

interface ClientConfig {
  server?: string;
  auth?: { token?: string };
  transport?: string;
  obfuscation?: boolean;
  mode?: string;
  mtu?: number;
  ipv6?: boolean;
  auto_reconnect?: boolean;
  compression?: boolean;
  multiplex?: boolean;
  proxy_listen?: string;
  proxy_auth?: { username?: string; password?: string };
  log?: { level?: string };
  tls?: { verify_mode?: string; server_name?: string; ca_file?: string };
  kill_switch?: { enabled?: boolean };
  crypto?: { enabled?: boolean; key?: string };
  routing?: {
    default_route?: string;
    include_ranges?: string[];
    exclude_ranges?: string[];
    include_ips?: string[];
    exclude_ips?: string[];
    include_domains?: string[];
    exclude_domains?: string[];
  };
  reconnect?: { min_backoff_sec?: number; max_backoff_sec?: number };
}

interface LogEntry {
  line: string;
  level: string;
  action?: number;
  ip?: string;
  ts?: string;
}

const inp: React.CSSProperties = {
  width: "100%", padding: 6, border: "1px solid #444",
  background: "#222", color: "#e0e0e0", borderRadius: 4, fontSize: 13,
  boxSizing: "border-box",
};
const lbl: React.CSSProperties = {
  display: "block", marginBottom: 8, fontSize: 12, color: "#888", fontWeight: 500,
};

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  const [open, setOpen] = useState(true);
  return (
    <div style={{ marginBottom: 8, border: "1px solid #2a2a2a", borderRadius: 6 }}>
      <div onClick={() => setOpen(!open)}
        style={{ padding: "6px 10px", background: "#222", cursor: "pointer", fontWeight: 600, fontSize: 12, userSelect: "none", borderRadius: 6, letterSpacing: "0.3px" }}>
        {open ? "▾" : "▸"} {title}
      </div>
      {open && <div style={{ padding: "8px 10px" }}>{children}</div>}
    </div>
  );
}

function App() {
  const [config, setConfig] = useState<ClientConfig>({});
  const [status, setStatus] = useState<Status>("disconnected");
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [saving, setSaving] = useState(false);
  const [showToken, setShowToken] = useState(false);
  const logEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    fetch("/api/config")
      .then((r) => r.json())
      .then((data) => { if (data.config) setConfig(data.config); })
      .catch(() => {});
  }, []);

  useEffect(() => {
    const es = new EventSource("/api/logs");
    es.addEventListener("status", (e) => {
      try { const d = JSON.parse(e.data); if (d.status) setStatus(d.status); } catch {}
    });
    es.addEventListener("log", (e) => {
      try { const entry = JSON.parse(e.data) as LogEntry; setLogs((prev) => [...prev.slice(-999), entry]); } catch {}
    });
    return () => es.close();
  }, []);

  useEffect(() => { logEndRef.current?.scrollIntoView({ behavior: "smooth" }); }, [logs]);

  const saveConfig = useCallback(async () => {
    setSaving(true);
    try { await fetch("/api/config", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ config }) }); } finally { setSaving(false); }
  }, [config]);

  const connect = useCallback(async () => {
    await saveConfig();
    await fetch("/api/connect", { method: "POST" });
  }, [saveConfig]);

  const disconnect = useCallback(async () => { await fetch("/api/disconnect", { method: "POST" }); }, []);

  const update = <K extends keyof ClientConfig>(key: K, value: ClientConfig[K]) => setConfig((prev) => ({ ...prev, [key]: value }));
  const nest = (parent: string, key: string, value: any) =>
    setConfig((prev) => ({ ...prev, [parent]: { ...((prev as any)[parent] || {}), [key]: value } }));

  const sc = status === "connected" ? "#4caf50" : status === "error" ? "#f44336" : status === "connecting" ? "#ff9800" : "#666";

  return (
    <div style={{ display: "flex", height: "100vh", fontFamily: "'Segoe UI', system-ui, sans-serif", color: "#d0d0d0", background: "#161616" }}>

      {/* Left: Settings */}
      <div style={{ width: 480, minWidth: 480, display: "flex", flexDirection: "column", borderRight: "1px solid #2a2a2a" }}>
        {/* Header */}
        <div style={{ padding: "12px 16px", borderBottom: "1px solid #2a2a2a", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <span style={{ fontWeight: 700, fontSize: 15 }}>KVN Web UI</span>
          <span style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <span style={{ width: 8, height: 8, borderRadius: "50%", background: sc, display: "inline-block" }} />
            <span style={{ color: sc, fontSize: 11, textTransform: "uppercase", fontWeight: 600 }}>{status}</span>
          </span>
        </div>

        {/* Buttons */}
        <div style={{ padding: "8px 12px", display: "flex", gap: 6, borderBottom: "1px solid #2a2a2a" }}>
          <button onClick={connect} disabled={status === "connecting" || status === "connected"}
            style={{ flex: 1, padding: "7px 0", background: "#2e7d32", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer", fontWeight: 600, opacity: status === "connected" || status === "connecting" ? 0.6 : 1 }}>
            Connect
          </button>
          <button onClick={disconnect} disabled={status === "disconnected"}
            style={{ flex: 1, padding: "7px 0", background: "#b71c1c", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer", fontWeight: 600, opacity: status === "disconnected" ? 0.6 : 1 }}>
            Disconnect
          </button>
          <button onClick={saveConfig} disabled={saving}
            style={{ padding: "7px 12px", background: "#1a5a9e", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer", fontWeight: 600, opacity: saving ? 0.6 : 1 }}>
            {saving ? "..." : "Save"}
          </button>
        </div>

        {/* Scrollable settings */}
        <div style={{ flex: 1, overflowY: "auto", padding: "8px 12px" }}>
          <Section title="Connection">
            <label style={lbl}>Server
              <input style={inp} placeholder="wss://example.com/tunnel" value={config.server || ""} onChange={(e) => update("server", e.target.value)} />
            </label>
            <label style={lbl}>Token
              <div style={{ display: "flex", gap: 3 }}>
                <input type={showToken ? "text" : "password"} style={inp} placeholder="auth token" value={config.auth?.token || ""}
                  onChange={(e) => nest("auth", "token", e.target.value)} />
                <button onClick={() => setShowToken(!showToken)} style={{ padding: "4px 6px", background: "#333", border: "1px solid #444", borderRadius: 4, color: "#aaa", cursor: "pointer", fontSize: 11, whiteSpace: "nowrap" }}>
                  {showToken ? "Hide" : "Show"}
                </button>
              </div>
            </label>
            <label style={lbl}>Mode
              <select style={inp} value={config.mode || "proxy"} onChange={(e) => update("mode", e.target.value)}>
                <option value="proxy">Proxy (SOCKS5/HTTP)</option>
                <option value="tun">TUN</option>
              </select>
            </label>
            <label style={lbl}>Transport
              <select style={inp} value={config.transport || "tcp"} onChange={(e) => update("transport", e.target.value)}>
                <option value="tcp">TCP (WebSocket)</option>
                <option value="quic">QUIC (UDP)</option>
              </select>
            </label>
            {config.mode === "proxy" && <>
              <label style={lbl}>Proxy Listen
                <input style={inp} placeholder="127.0.0.1:2310" value={config.proxy_listen || "127.0.0.1:2310"} onChange={(e) => update("proxy_listen", e.target.value)} />
              </label>
              <label style={lbl}>Proxy Username
                <input style={inp} value={config.proxy_auth?.username || ""} onChange={(e) => nest("proxy_auth", "username", e.target.value)} />
              </label>
              <label style={lbl}>Proxy Password
                <input type="password" style={inp} value={config.proxy_auth?.password || ""} onChange={(e) => nest("proxy_auth", "password", e.target.value)} />
              </label>
            </>}
            {config.mode === "tun" && <div style={{ color: "#666", fontSize: 11, fontStyle: "italic" }}>Proxy settings not applicable in TUN mode.</div>}
          </Section>

          <Section title="TLS">
            <label style={lbl}>Verify Mode
              <select style={inp} value={config.tls?.verify_mode || "verify"} onChange={(e) => nest("tls", "verify_mode", e.target.value)}>
                <option value="verify">Verify</option>
                <option value="insecure">Insecure</option>
                <option value="none">None</option>
              </select>
            </label>
            <label style={lbl}>Server Name (SNI)
              <input style={inp} placeholder="example.com" value={config.tls?.server_name || ""} onChange={(e) => nest("tls", "server_name", e.target.value)} />
            </label>
            <label style={lbl}>CA File
              <input style={inp} placeholder="/path/to/ca.pem" value={config.tls?.ca_file || ""} onChange={(e) => nest("tls", "ca_file", e.target.value)} />
            </label>
          </Section>

          <Section title="Advanced">
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
              <label style={lbl}>MTU
                <input type="number" style={inp} value={config.mtu ?? 1400} onChange={(e) => update("mtu", e.target.value ? parseInt(e.target.value) : 1400)} />
              </label>
              <label style={lbl}>Log Level
                <select style={inp} value={config.log?.level || "info"} onChange={(e) => nest("log", "level", e.target.value)}>
                  <option value="debug">Debug</option>
                  <option value="info">Info</option>
                  <option value="warn">Warn</option>
                  <option value="error">Error</option>
                </select>
              </label>
            </div>
            <Checkbox checked={config.ipv6 ?? false} onChange={(v) => update("ipv6", v)} label="Enable IPv6" />
            <Checkbox checked={config.auto_reconnect ?? true} onChange={(v) => update("auto_reconnect", v)} label="Auto Reconnect" />
            <Checkbox checked={config.compression ?? false} onChange={(v) => update("compression", v)} label="Compression" />
            <Checkbox checked={config.multiplex ?? false} onChange={(v) => update("multiplex", v)} label="Multiplex" />
            <Checkbox checked={config.obfuscation ?? false} onChange={(v) => update("obfuscation", v)} label="QUIC Obfuscation (anti-DPI)" />
          </Section>

          <Section title="Routing">
            <label style={lbl}>Default Route
              <select style={inp} value={config.routing?.default_route || "server"} onChange={(e) => nest("routing", "default_route", e.target.value)}>
                <option value="server">Server (VPN)</option>
                <option value="direct">Direct (bypass)</option>
              </select>
            </label>
            <ChipList label="Include CIDR" values={config.routing?.include_ranges} onChange={(v) => nest("routing", "include_ranges", v)} />
            <ChipList label="Exclude CIDR" values={config.routing?.exclude_ranges} onChange={(v) => nest("routing", "exclude_ranges", v)} />
            <ChipList label="Include IPs" values={config.routing?.include_ips} onChange={(v) => nest("routing", "include_ips", v)} />
            <ChipList label="Exclude IPs" values={config.routing?.exclude_ips} onChange={(v) => nest("routing", "exclude_ips", v)} />
            <ChipList label="Include Domains" values={config.routing?.include_domains} onChange={(v) => nest("routing", "include_domains", v)} />
            <ChipList label="Exclude Domains" values={config.routing?.exclude_domains} onChange={(v) => nest("routing", "exclude_domains", v)} />
          </Section>

          <Section title="Kill Switch & Reconnect">
            <Checkbox checked={config.kill_switch?.enabled ?? false} onChange={(v) => nest("kill_switch", "enabled", v)} label="Kill Switch (block on disconnect)" />
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6, marginTop: 6 }}>
              <label style={lbl}>Min Backoff (s)
                <input type="number" style={inp} value={config.reconnect?.min_backoff_sec ?? 1} onChange={(e) => nest("reconnect", "min_backoff_sec", parseInt(e.target.value) || 1)} />
              </label>
              <label style={lbl}>Max Backoff (s)
                <input type="number" style={inp} value={config.reconnect?.max_backoff_sec ?? 30} onChange={(e) => nest("reconnect", "max_backoff_sec", parseInt(e.target.value) || 30)} />
              </label>
            </div>
          </Section>

          <Section title="Encryption">
            <Checkbox checked={config.crypto?.enabled ?? false} onChange={(v) => nest("crypto", "enabled", v)} label="AES-256-GCM Encryption" />
            {config.crypto?.enabled && (
              <label style={lbl}>Key
                <input type="password" style={inp} placeholder="hex 256-bit key" value={config.crypto?.key || ""} onChange={(e) => nest("crypto", "key", e.target.value)} />
              </label>
            )}
          </Section>
        </div>
      </div>

      {/* Right: Logs */}
      <div style={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0 }}>
        <div style={{ padding: "10px 16px", borderBottom: "1px solid #2a2a2a", fontSize: 12, fontWeight: 600, color: "#666", letterSpacing: "0.5px" }}>
          LIVE LOG
        </div>
        <pre style={{ flex: 1, margin: 0, padding: 12, overflowY: "auto", background: "#0d0d0d", color: "#c0c0c0", fontSize: 12, lineHeight: 1.6, fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace" }}>
          {logs.length === 0 && <span style={{ color: "#444" }}>No entries yet. Connect to see logs.</span>}
          {logs.map((entry, i) => (
            <div key={i} style={{ color: entry.level === "error" ? "#f44336" : entry.level === "warn" ? "#ff9800" : "#c0c0c0" }}>
              {entry.ts && <span style={{ color: "#666" }}>{entry.ts} </span>}
              <span>{entry.line}</span>
              {entry.action !== undefined && <span style={{ color: "#888" }}> action:{entry.action}</span>}
              {entry.ip && <span style={{ color: "#888" }}> ip:{entry.ip}</span>}
            </div>
          ))}
          <div ref={logEndRef} />
        </pre>
      </div>

    </div>
  );
}

function Checkbox({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, color: "#aaa", marginBottom: 4, cursor: "pointer" }}>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} style={{ cursor: "pointer" }} />
      {label}
    </label>
  );
}

function ChipList({ label, values, onChange }: { label: string; values?: string[]; onChange: (v: string[]) => void }) {
  const [input, setInput] = useState("");
  const list = values || [];
  const add = () => {
    const trimmed = input.trim();
    if (trimmed && !list.includes(trimmed)) { onChange([...list, trimmed]); setInput(""); }
  };
  return (
    <div style={{ marginBottom: 6 }}>
      <div style={{ fontSize: 12, color: "#888", fontWeight: 500, marginBottom: 3 }}>{label}</div>
      <div style={{ display: "flex", gap: 3, marginBottom: 3 }}>
        <input style={{ ...inp, flex: 1, fontSize: 12 }} value={input} onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") add(); }} placeholder="Add..." />
        <button onClick={add} style={{ padding: "3px 8px", background: "#333", border: "1px solid #444", borderRadius: 4, color: "#aaa", cursor: "pointer", fontSize: 13 }}>+</button>
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 3 }}>
        {list.map((v, i) => (
          <span key={i} style={{ display: "inline-flex", alignItems: "center", gap: 3, background: "#2a2a2a", padding: "1px 6px", borderRadius: 10, fontSize: 11 }}>
            {v}
            <button onClick={() => onChange(list.filter((_, j) => j !== i))} style={{ background: "none", border: "none", color: "#f44336", cursor: "pointer", padding: 0, fontSize: 13, lineHeight: 1 }}>×</button>
          </span>
        ))}
      </div>
    </div>
  );
}

export default App;
