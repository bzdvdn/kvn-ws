import { useState, useEffect, useRef, useCallback } from "react";
// @sk-task import-export--qr-config-ui#T3.1: QR code generation (AC-003)
import QRCode from "qrcode";

type Status = "disconnected" | "connecting" | "connected" | "error";

interface ClientConfig {
  server?: string;
  auth?: { token?: string };
  transport?: string;
  obfuscation?: {
    enabled?: boolean;
    utls?: { enabled?: boolean; fallback?: boolean };
    padding?: { enabled?: boolean; size?: number };
  };
  mode?: string;
  mtu?: number;
  ipv6?: boolean;
  auto_reconnect?: boolean;
  multiplex?: boolean;
  max_message_size?: number;
  proxy_listen?: string;
  proxy_auth?: { username?: string; password?: string };
  log?: { level?: string };
  tls?: { verify_mode?: string; server_name?: string; ca_file?: string; sni?: string[] };
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
  const [logFilter, setLogFilter] = useState<Record<string, boolean>>({ debug: true, info: true, warn: true, error: true });
  const [logSearch, setLogSearch] = useState("");
  // @sk-task import-export--qr-config-ui#T2.1: import textarea state (AC-002)
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState("");
  const [importError, setImportError] = useState("");
  // @sk-task import-export--qr-config-ui#T2.1: save button highlight on import (AC-002)
  const [importDirty, setImportDirty] = useState(false);
  // @sk-task import-export--qr-config-ui#T3.1: QR modal state (AC-003)
  const [qrOpen, setQrOpen] = useState(false);
  const qrCanvasRef = useRef<HTMLCanvasElement>(null);
  // @sk-task import-export--qr-config-ui#T1.1: toast notification (AC-001)
  const [toast, setToast] = useState("");
  const toastTimer = useRef<ReturnType<typeof setTimeout>>();
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

  const filteredLogs = logs.filter((e) => {
    if (logSearch && !e.line.toLowerCase().includes(logSearch.toLowerCase()) && !(e.ip || "").toLowerCase().includes(logSearch.toLowerCase())) return false;
    return logFilter[e.level] ?? true;
  });

  const showToast = useCallback((msg: string) => {
    setToast(msg);
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(""), 2500);
  }, []);

  // @sk-task import-export--qr-config-ui#T1.1: export to clipboard (AC-001)
  const exportConfig = useCallback(async () => {
    const json = JSON.stringify(config);
    try {
      await navigator.clipboard.writeText(json);
      showToast("Config copied to clipboard");
    } catch {
      showToast("Failed to copy");
    }
  }, [config, showToast]);

  // @sk-task import-export--qr-config-ui#T2.1: import from JSON (AC-002)
  const doImport = useCallback(() => {
    setImportError("");
    try {
      const parsed = JSON.parse(importText);
      if (typeof parsed !== "object" || parsed === null) throw new Error("not an object");
      // @sk-task import-export--qr-config-ui#T4.1: merge, don't replace (AC-004)
      setConfig((prev) => ({ ...prev, ...parsed }));
      setImportOpen(false);
      setImportText("");
      setImportDirty(true);
      showToast("Config imported — review and Save");
    } catch (e: any) {
      setImportError(e.message || "Invalid JSON");
    }
  }, [importText, showToast]);

  // @sk-task import-export--qr-config-ui#T3.1: QR code generation (AC-003)
  const openQr = useCallback(async () => {
    setQrOpen(true);
  }, []);

  useEffect(() => {
    if (!qrOpen || !qrCanvasRef.current) return;
    const json = JSON.stringify(config);
    QRCode.toCanvas(qrCanvasRef.current, json, { width: 280, margin: 2 }, (err) => {
      if (err) showToast("QR generation failed");
    });
  }, [qrOpen, config, showToast]);

  const hasConfig = Object.keys(config).length > 0;

  const saveConfig = useCallback(async () => {
    setSaving(true);
    try { await fetch("/api/config", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ config }) }); } finally { setSaving(false); setImportDirty(false); }
  }, [config]);

  const connect = useCallback(async () => {
    await saveConfig();
    await fetch("/api/connect", { method: "POST" });
  }, [saveConfig]);

  const disconnect = useCallback(async () => { await fetch("/api/disconnect", { method: "POST" }); }, []);

  const update = <K extends keyof ClientConfig>(key: K, value: ClientConfig[K]) => setConfig((prev) => ({ ...prev, [key]: value }));
  const nest = (parent: string, key: string, value: any) =>
    setConfig((prev) => ({ ...prev, [parent]: { ...((prev as any)[parent] || {}), [key]: value } }));
  const nest2 = (parent: string, child: string, key: string, value: any) =>
    setConfig((prev) => ({ ...prev, [parent]: { ...((prev as any)[parent] || {}), [child]: { ...((((prev as any)[parent] || {}) as any)[child] || {}), [key]: value } } }));

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
        <div style={{ padding: "8px 12px", display: "flex", gap: 6, borderBottom: "1px solid #2a2a2a", flexWrap: "wrap" }}>
          <button onClick={connect} disabled={status === "connecting" || status === "connected"}
            style={{ flex: 1, padding: "7px 0", background: "#2e7d32", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer", fontWeight: 600, opacity: status === "connected" || status === "connecting" ? 0.6 : 1 }}>
            Connect
          </button>
          <button onClick={disconnect} disabled={status === "disconnected"}
            style={{ flex: 1, padding: "7px 0", background: "#b71c1c", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer", fontWeight: 600, opacity: status === "disconnected" ? 0.6 : 1 }}>
            Disconnect
          </button>
          <button onClick={saveConfig} disabled={saving}
            style={{ padding: "7px 12px", background: importDirty ? "#f57c00" : "#1a5a9e", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer", fontWeight: 600, opacity: saving ? 0.6 : 1 }}>
            {saving ? "..." : importDirty ? "Save ⚡" : "Save"}
          </button>
          {/* @sk-task import-export--qr-config-ui#T1.1: Export button (AC-001) */}
          <button onClick={exportConfig}
            style={{ padding: "7px 10px", background: "#333", border: "1px solid #555", borderRadius: 4, color: "#ccc", fontSize: 12, cursor: "pointer" }}>
            Export
          </button>
          {/* @sk-task import-export--qr-config-ui#T2.1: Import button (AC-002) */}
          <button onClick={() => { setImportOpen(!importOpen); setImportError(""); }}
            style={{ padding: "7px 10px", background: importOpen ? "#555" : "#333", border: "1px solid #555", borderRadius: 4, color: "#ccc", fontSize: 12, cursor: "pointer" }}>
            Import
          </button>
          {/* @sk-task import-export--qr-config-ui#T3.1: QR button (AC-003) */}
          <button onClick={openQr} disabled={!hasConfig}
            style={{ padding: "7px 10px", background: "#333", border: "1px solid #555", borderRadius: 4, color: "#ccc", fontSize: 12, cursor: "pointer", opacity: hasConfig ? 1 : 0.4 }}>
            QR
          </button>
        </div>

        {/* @sk-task import-export--qr-config-ui#T2.1: Import textarea (AC-002) */}
        {importOpen && (
          <div style={{ padding: "8px 12px", borderBottom: "1px solid #2a2a2a" }}>
            <textarea style={{ ...inp, minHeight: 100, fontSize: 11, fontFamily: "monospace" }}
              placeholder="Paste JSON config here..."
              value={importText} onChange={(e) => setImportText(e.target.value)} />
            <div style={{ display: "flex", gap: 6, marginTop: 6 }}>
              <button onClick={doImport}
                style={{ padding: "5px 12px", background: "#1a5a9e", border: "none", borderRadius: 4, color: "#fff", fontSize: 12, cursor: "pointer" }}>
                Apply
              </button>
              <button onClick={() => { setImportOpen(false); setImportText(""); setImportError(""); }}
                style={{ padding: "5px 12px", background: "#555", border: "none", borderRadius: 4, color: "#ccc", fontSize: 12, cursor: "pointer" }}>
                Cancel
              </button>
            </div>
            {importError && <div style={{ color: "#f44336", fontSize: 11, marginTop: 4 }}>{importError}</div>}
          </div>
        )}

        {/* @sk-task import-export--qr-config-ui#T3.1: QR modal (AC-003) */}
        {qrOpen && (
          <div style={{
            position: "fixed", inset: 0, background: "rgba(0,0,0,0.7)", display: "flex",
            alignItems: "center", justifyContent: "center", zIndex: 1000,
          }} onClick={() => setQrOpen(false)}>
            <div style={{
              background: "#fff", padding: 24, borderRadius: 12, display: "flex",
              flexDirection: "column", alignItems: "center", gap: 12,
            }} onClick={(e) => e.stopPropagation()}>
              <canvas ref={qrCanvasRef} />
              <button onClick={() => { setQrOpen(false); navigator.clipboard.writeText(JSON.stringify(config)); showToast("Config copied"); }}
                style={{ padding: "6px 16px", background: "#1a5a9e", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer" }}>
                Copy & Close
              </button>
            </div>
          </div>
        )}

        {/* @sk-task import-export--qr-config-ui#T1.1: Toast notification (AC-001) */}
        {toast && (
          <div style={{
            position: "fixed", bottom: 24, left: "50%", transform: "translateX(-50%)",
            background: "#333", color: "#e0e0e0", padding: "8px 20px", borderRadius: 8,
            fontSize: 13, zIndex: 1001, border: "1px solid #555",
          }}>
            {toast}
          </div>
        )}

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
            {/* @sk-task whitelist-obfuscation#T3.3: SNI chip list (AC-004) */}
            <ChipList label="Custom SNI (random on connect)" values={config.tls?.sni} onChange={(v) => nest("tls", "sni", v)} />
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
                <div style={{ fontSize: 10, color: "#555", marginTop: 2 }}>Client log level (file/terminal). Live Log filter below.</div>
              </label>
            </div>
            <label style={lbl}>Max Message Size (bytes)
              <input type="number" style={inp} value={config.max_message_size ?? 10485760} onChange={(e) => update("max_message_size", e.target.value ? parseInt(e.target.value) : 10485760)} />
              <div style={{ fontSize: 10, color: "#555", marginTop: 2 }}>Max QUIC/WS message size (default 10MB).</div>
            </label>
            <Checkbox checked={config.ipv6 ?? false} onChange={(v) => update("ipv6", v)} label="Enable IPv6" />
            <Checkbox checked={config.auto_reconnect ?? true} onChange={(v) => update("auto_reconnect", v)} label="Auto Reconnect" />
            <Checkbox checked={config.multiplex ?? false} onChange={(v) => update("multiplex", v)} label="Multiplex" />
            {/* @sk-task whitelist-obfuscation#T3.3: obfuscation sub-settings (AC-005) */}
            <Checkbox checked={config.obfuscation?.enabled ?? false} onChange={(v) => nest("obfuscation", "enabled", v)} label="Obfuscation (anti-DPI)" />
            {config.obfuscation?.enabled && <div style={{ marginLeft: 16, marginTop: 4, padding: "6px 8px", borderLeft: "2px solid #333" }}>
              <Checkbox checked={config.obfuscation?.utls?.enabled ?? false} onChange={(v) => nest2("obfuscation", "utls", "enabled", v)} label="uTLS (Chrome JA3 fingerprint)" />
              {config.obfuscation?.utls?.enabled && <div style={{ marginLeft: 16 }}>
                <Checkbox checked={config.obfuscation?.utls?.fallback ?? true} onChange={(v) => nest2("obfuscation", "utls", "fallback", v)} label="uTLS Fallback (crypto/tls on error)" />
              </div>}
              <Checkbox checked={config.obfuscation?.padding?.enabled ?? false} onChange={(v) => nest2("obfuscation", "padding", "enabled", v)} label="WS Padding (fixed packet size)" />
              {config.obfuscation?.padding?.enabled && <div style={{ marginLeft: 16 }}>
                <label style={lbl}>Padding Size
                  <input type="number" style={inp} value={config.obfuscation?.padding?.size ?? 512} onChange={(e) => nest2("obfuscation", "padding", "size", parseInt(e.target.value) || 512)} />
                </label>
              </div>}
            </div>}
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
        <div style={{ padding: "6px 12px", borderBottom: "1px solid #2a2a2a", display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
          <span style={{ fontSize: 12, fontWeight: 600, color: "#666", letterSpacing: "0.5px" }}>LIVE LOG</span>
          {["debug", "info", "warn", "error"].map((lvl) => (
            <label key={lvl} style={{ fontSize: 11, color: "#888", cursor: "pointer", display: "flex", alignItems: "center", gap: 3 }}>
              <input type="checkbox" checked={logFilter[lvl] ?? true} onChange={(e) => setLogFilter((p) => ({ ...p, [lvl]: e.target.checked }))}
                style={{ cursor: "pointer" }} />
              <span style={{ color: lvl === "error" ? "#f44336" : lvl === "warn" ? "#ff9800" : lvl === "info" ? "#c0c0c0" : "#666" }}>{lvl}</span>
            </label>
          ))}
          <input style={{ flex: 1, minWidth: 100, padding: "3px 6px", border: "1px solid #444", background: "#222", color: "#e0e0e0", borderRadius: 4, fontSize: 11 }}
            placeholder="Filter IP or text..." value={logSearch} onChange={(e) => setLogSearch(e.target.value)} />
          {filteredLogs.length !== logs.length && <span style={{ fontSize: 10, color: "#555" }}>{filteredLogs.length}/{logs.length}</span>}
        </div>
        <pre style={{ flex: 1, margin: 0, padding: 12, overflowY: "auto", background: "#0d0d0d", color: "#c0c0c0", fontSize: 12, lineHeight: 1.6, fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace" }}>
          {filteredLogs.length === 0 && <span style={{ color: "#444" }}>No entries yet. Connect to see logs.</span>}
          {filteredLogs.map((entry, i) => (
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
