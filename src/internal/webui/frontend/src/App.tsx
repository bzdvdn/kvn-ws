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
  system_proxy?: boolean;
  transparent?: boolean;
  dns_proxy?: { listen?: string; upstream?: string };
}

interface ServerEntry {
  name: string;
  server?: string;
  auth?: { token?: string };
  transport?: string;
  // all other ClientConfig fields
  [key: string]: any;
}

interface ServersResponse {
  active_server: string;
  servers: ServerEntry[];
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

function Section({ title, children, defaultOpen }: { title: string; children: React.ReactNode; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen ?? false);
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
  // @sk-task multi-server#T3.1: multi-server state (AC-001, AC-002, AC-003)
  const [servers, setServers] = useState<ServerEntry[]>([]);
  const [activeServer, setActiveServer] = useState("");
  const [serverConfig, setServerConfig] = useState<ClientConfig>({});
  const [globalConfig, setGlobalConfig] = useState<ClientConfig>({});

  const [status, setStatus] = useState<Status>("disconnected");
  const [platform, setPlatform] = useState("linux");
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
  // @sk-task multi-server: original name for PUT URL (AC-002)
  const originalServerRef = useRef(activeServer);
  // @sk-task multi-server#T3.1: dirty tracking (AC-002)
  const [dirty, setDirty] = useState(false);
  const [switchTarget, setSwitchTarget] = useState<string | null>(null);
  // @sk-task multi-server: collapse/expand toggles (AC-001)
  const [allExpanded, setAllExpanded] = useState(false);
  const sectionKey = allExpanded ? "exp" : "col";
  // @sk-task multi-server: delete confirmation modal (AC-003)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  // @sk-task multi-server: last saved indicator
  const [lastSavedAt, setLastSavedAt] = useState(0);
  const [lastSavedText, setLastSavedText] = useState("");

  // @sk-task multi-server#T3.1: load servers on mount (AC-001)
  const loadServers = useCallback(async (switchTo?: string) => {
    try {
      const r = await fetch("/api/servers");
      const data: ServersResponse = await r.json();
      setServers(data.servers);
      const target = switchTo || data.active_server;
      setActiveServer(target);
      originalServerRef.current = target;
      const active = data.servers.find((s) => s.name === target);
      if (active) {
        const { name, ...cfg } = active;
        setServerConfig(cfg);
      }
    } catch {}
  }, []);

  // @sk-task multi-server#T3.1: load global config on mount (AC-001)
  const loadGlobalConfig = useCallback(async () => {
    try {
      const r = await fetch("/api/config");
      const data = await r.json();
      if (data.config) setGlobalConfig(data.config);
    } catch {}
  }, []);

  useEffect(() => {
    loadServers();
    loadGlobalConfig();
    fetch("/api/platform")
      .then((r) => r.json())
      .then((data) => { if (data.os) setPlatform(data.os); })
      .catch(() => {});
  }, [loadServers, loadGlobalConfig]);

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

  // @sk-task multi-server: update last saved indicator
  useEffect(() => {
    if (!lastSavedAt) return;
    const tick = () => {
      const sec = Math.floor((Date.now() - lastSavedAt) / 1000);
      setLastSavedText(sec < 60 ? `${sec}s ago` : `${Math.floor(sec / 60)}m ago`);
    };
    tick();
    const iv = setInterval(tick, 10000);
    return () => clearInterval(iv);
  }, [lastSavedAt]);

  const filteredLogs = logs.filter((e) => {
    if (logSearch && !e.line.toLowerCase().includes(logSearch.toLowerCase()) && !(e.ip || "").toLowerCase().includes(logSearch.toLowerCase())) return false;
    return logFilter[e.level] ?? true;
  });

  const showToast = useCallback((msg: string) => {
    setToast(msg);
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(""), 2500);
  }, []);

  // @sk-task multi-server#T3.1: switch server with dirty check (AC-002)
  const doSwitchServer = useCallback((name: string) => {
    const sv = servers.find((s) => s.name === name);
    if (!sv) return;
    setActiveServer(name);
    originalServerRef.current = name;
    const { name: _, ...cfg } = sv;
    setServerConfig(cfg);
    setDirty(false);
    setSwitchTarget(null);
    setStatus("disconnected");
  }, [servers]);

  // @sk-task multi-server#T3.1: switch server with dirty check (AC-002)
  const handleSwitchServer = useCallback((name: string) => {
    if (dirty) {
      setSwitchTarget(name);
    } else {
      doSwitchServer(name);
    }
  }, [dirty, doSwitchServer]);

  // @sk-task multi-server#T3.1: confirm or cancel server switch (AC-002)
  const confirmSwitch = useCallback(async (action: "save" | "discard") => {
    if (!switchTarget) return;
    if (action === "save") {
      await saveAll();
    }
    doSwitchServer(switchTarget);
  }, [switchTarget, doSwitchServer]);

  const cancelSwitch = useCallback(() => {
    setSwitchTarget(null);
  }, []);

  // @sk-task import-export--qr-config-ui#T1.1: export to clipboard (AC-001)
  // @sk-task multi-server#T3.2: export selected server config (AC-005)
  const exportConfig = useCallback(async () => {
    const json = JSON.stringify(serverConfig);
    try {
      await navigator.clipboard.writeText(json);
      showToast("Config copied to clipboard");
    } catch {
      showToast("Failed to copy");
    }
  }, [serverConfig, showToast]);

  // @sk-task import-export--qr-config-ui#T2.1: import from JSON (AC-002)
  // @sk-task multi-server#T3.2: import creates new server (AC-004)
  const doImport = useCallback(async () => {
    setImportError("");
    let parsed: any;
    try {
      parsed = JSON.parse(importText);
      if (typeof parsed !== "object" || parsed === null) throw new Error("not an object");
    } catch (e: any) {
      setImportError(e.message || "Invalid JSON");
      return;
    }
    const ts = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
    const name = `Imported ${ts}`;
    try {
      const r = await fetch("/api/servers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, ...parsed }),
      });
      if (!r.ok) {
        const err = await r.text();
        setImportError(err);
        return;
      }
      setImportOpen(false);
      setImportText("");
      showToast("Server imported — switch to it and connect");
      await loadServers();
    } catch (e: any) {
      setImportError(e.message || "Import failed");
    }
  }, [importText, showToast, loadServers]);

  // @sk-task import-export--qr-config-ui#T3.1: QR code generation (AC-003)
  // @sk-task multi-server#T3.2: QR from selected server config (AC-005)
  const openQr = useCallback(async () => {
    setQrOpen(true);
  }, []);

  useEffect(() => {
    if (!qrOpen || !qrCanvasRef.current) return;
    const json = JSON.stringify(serverConfig);
    QRCode.toCanvas(qrCanvasRef.current, json, { width: 280, margin: 2 }, (err) => {
      if (err) showToast("QR generation failed");
    });
  }, [qrOpen, serverConfig, showToast]);

  const hasConfig = Object.keys(serverConfig).length > 0;

  // @sk-task multi-server#T3.1: save global + server config (AC-001, AC-003)
  const saveAll = useCallback(async () => {
    setSaving(true);
    try {
      const r1 = await fetch("/api/config/global", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ active_server: activeServer, ...globalConfig }),
      });
      if (!r1.ok) throw new Error("save global failed");
      const originName = originalServerRef.current;
      if (originName) {
        const r2 = await fetch(`/api/servers/${encodeURIComponent(originName)}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name: activeServer, ...serverConfig }),
        });
        if (!r2.ok) throw new Error("save server failed");
        originalServerRef.current = activeServer;
      }
      setDirty(false);
      setImportDirty(false);
      await loadServers();
      setLastSavedAt(Date.now());
      setLastSavedText("just now");
      showToast("Saved");
    } catch (e) {
      showToast("Save failed");
    } finally {
      setSaving(false);
    }
  }, [globalConfig, activeServer, serverConfig, loadServers, showToast]);

  // @sk-task multi-server#T3.1: add new server cloning current config (AC-003)
  const addServer = useCallback(async () => {
    const n = servers.length + 1;
    const name = `Server ${n}`;
    try {
      const r = await fetch("/api/servers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, ...serverConfig }),
      });
      if (!r.ok) return;
      await loadServers(name);
      setDirty(false);
    } catch {}
  }, [servers, loadServers, serverConfig]);

  // @sk-task multi-server: delete server with modal (AC-003)
  const deleteServer = useCallback(() => {
    if (!activeServer || servers.length <= 1) return;
    setDeleteTarget(activeServer);
  }, [activeServer, servers]);

  const confirmDelete = useCallback(async () => {
    if (!deleteTarget) return;
    try {
      const r = await fetch(`/api/servers/${encodeURIComponent(deleteTarget)}`, { method: "DELETE" });
      if (!r.ok) return;
      await loadServers();
      setDirty(false);
      showToast(`Deleted "${deleteTarget}"`);
    } catch {
      showToast("Delete failed");
    } finally {
      setDeleteTarget(null);
    }
  }, [deleteTarget, loadServers, showToast]);

  const saveConfig = useCallback(async () => {
    await saveAll();
  }, [saveAll]);

  // @sk-task multi-server#T3.1: connect uses selected server (AC-006)
  const connect = useCallback(async () => {
    if (dirty) await saveAll();
    if (!activeServer) { showToast("No server selected"); return; }
    // Sync active_server to backend before connecting
    await fetch("/api/config/global", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ active_server: activeServer, ...globalConfig }),
    });
    await fetch("/api/connect", { method: "POST" });
  }, [dirty, saveAll, activeServer, globalConfig, showToast]);

  const disconnect = useCallback(async () => { await fetch("/api/disconnect", { method: "POST" }); }, []);

  // @sk-task multi-server#T3.1: mark dirty on any server field change (AC-002)
  const updateServer = <K extends keyof ClientConfig>(key: K, value: ClientConfig[K]) => {
    setServerConfig((prev) => ({ ...prev, [key]: value }));
    setDirty(true);
  };
  const nestServer = (parent: string, key: string, value: any) => {
    setServerConfig((prev) => ({ ...prev, [parent]: { ...((prev as any)[parent] || {}), [key]: value } }));
    setDirty(true);
  };
  const nestServer2 = (parent: string, child: string, key: string, value: any) => {
    setServerConfig((prev) => ({ ...prev, [parent]: { ...((prev as any)[parent] || {}), [child]: { ...((((prev as any)[parent] || {}) as any)[child] || {}), [key]: value } } }));
    setDirty(true);
  };
  const updateGlobal = <K extends keyof ClientConfig>(key: K, value: ClientConfig[K]) => {
    setGlobalConfig((prev) => ({ ...prev, [key]: value }));
    setDirty(true);
  };
  const nestGlobal = (parent: string, key: string, value: any) => {
    setGlobalConfig((prev) => ({ ...prev, [parent]: { ...((prev as any)[parent] || {}), [key]: value } }));
    setDirty(true);
  };

  const sc = status === "connected" ? "#4caf50" : status === "error" ? "#f44336" : status === "connecting" ? "#ff9800" : "#666";

  return (
    <div style={{ display: "flex", height: "100vh", fontFamily: "'Segoe UI', system-ui, sans-serif", color: "#d0d0d0", background: "#161616" }}>

      {/* Left: Settings */}
      <div style={{ width: 480, minWidth: 480, display: "flex", flexDirection: "column", borderRight: "1px solid #2a2a2a" }}>
        {/* Header */}
        <div style={{ padding: "8px 12px", borderBottom: "1px solid #2a2a2a", display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
          <span style={{ fontWeight: 700, fontSize: 14, marginRight: "auto" }}>KVN Web UI</span>
          {/* @sk-task multi-server#T3.1: server selector dropdown (AC-001) */}
          <select value={activeServer} onChange={(e) => handleSwitchServer(e.target.value)}
            style={{ background: "#222", color: "#e0e0e0", border: "1px solid #444", borderRadius: 4, padding: "3px 6px", fontSize: 12 }}>
            {servers.map((s) => (
              <option key={s.name} value={s.name}>{s.name}</option>
            ))}
          </select>
          <button onClick={connect} disabled={status === "connecting" || status === "connected"}
            style={{ padding: "4px 10px", background: "#2e7d32", border: "none", borderRadius: 4, color: "#fff", fontSize: 11, cursor: "pointer", fontWeight: 600, opacity: status === "connected" || status === "connecting" ? 0.5 : 1 }}>
            Connect
          </button>
          <button onClick={disconnect} disabled={status === "disconnected"}
            style={{ padding: "4px 10px", background: "#b71c1c", border: "none", borderRadius: 4, color: "#fff", fontSize: 11, cursor: "pointer", fontWeight: 600, opacity: status === "disconnected" ? 0.5 : 1 }}>
            Disconnect
          </button>
          <span style={{ width: 6, height: 6, borderRadius: "50%", background: sc, display: "inline-block" }} />
          <span style={{ color: sc, fontSize: 10, textTransform: "uppercase", fontWeight: 600 }}>{status}</span>
        </div>

        {/* Switch confirm dialog */}
        {switchTarget && (
          <div style={{ padding: "8px 12px", borderBottom: "1px solid #ff9800", background: "#2a2a00" }}>
            <div style={{ fontSize: 12, color: "#ff9800", marginBottom: 6 }}>Unsaved changes — save before switching?</div>
            <div style={{ display: "flex", gap: 6 }}>
              <button onClick={() => confirmSwitch("save")}
                style={{ padding: "4px 10px", background: "#1a5a9e", border: "none", borderRadius: 4, color: "#fff", fontSize: 11, cursor: "pointer" }}>Save & Switch</button>
              <button onClick={() => confirmSwitch("discard")}
                style={{ padding: "4px 10px", background: "#555", border: "none", borderRadius: 4, color: "#ccc", fontSize: 11, cursor: "pointer" }}>Discard & Switch</button>
              <button onClick={cancelSwitch}
                style={{ padding: "4px 10px", background: "#333", border: "1px solid #555", borderRadius: 4, color: "#ccc", fontSize: 11, cursor: "pointer" }}>Cancel</button>
            </div>
          </div>
        )}

        {/* Buttons */}
        <div style={{ padding: "8px 12px", display: "flex", gap: 6, borderBottom: "1px solid #2a2a2a", flexWrap: "wrap", alignItems: "center" }}>
          <button onClick={saveConfig} disabled={saving}
            style={{ padding: "7px 12px", background: importDirty || dirty ? "#f57c00" : "#1a5a9e", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer", fontWeight: 600, opacity: saving ? 0.6 : 1 }}>
            {saving ? "..." : importDirty || dirty ? "Save ⚡" : "Save"}
          </button>
          {lastSavedText && <span style={{ fontSize: 10, color: "#555" }}>{lastSavedText}</span>}
          {/* @sk-task multi-server#T3.1: Add/Delete server buttons (AC-003) */}
          <button onClick={addServer}
            style={{ padding: "7px 10px", background: "#2a5a2a", border: "1px solid #3a7a3a", borderRadius: 4, color: "#ccc", fontSize: 12, cursor: "pointer" }}>
            + Add
          </button>
          <button onClick={deleteServer} disabled={servers.length <= 1}
            style={{ padding: "7px 10px", background: "#5a2a2a", border: "1px solid #7a3a3a", borderRadius: 4, color: "#ccc", fontSize: 12, cursor: "pointer", opacity: servers.length <= 1 ? 0.4 : 1 }}>
            Delete
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
          <div style={{ marginLeft: "auto", display: "flex", gap: 6, alignItems: "center" }}>
            <button onClick={() => setAllExpanded(!allExpanded)}
              style={{ padding: "4px 8px", background: "#333", border: "1px solid #555", borderRadius: 4, color: "#aaa", fontSize: 11, cursor: "pointer" }}>
              {allExpanded ? "▴ Collapse" : "▾ Expand"}
            </button>
          </div>
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
                Import as new server
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
              <button onClick={() => { setQrOpen(false); navigator.clipboard.writeText(JSON.stringify(serverConfig)); showToast("Config copied"); }}
                style={{ padding: "6px 16px", background: "#1a5a9e", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer" }}>
                Copy & Close
              </button>
            </div>
          </div>
        )}

        {/* @sk-task multi-server: Delete confirmation modal (AC-003) */}
        {deleteTarget && (
          <div style={{
            position: "fixed", inset: 0, background: "rgba(0,0,0,0.7)", display: "flex",
            alignItems: "center", justifyContent: "center", zIndex: 1000,
          }} onClick={() => setDeleteTarget(null)}>
            <div style={{
              background: "#1e1e1e", padding: 24, borderRadius: 12, display: "flex",
              flexDirection: "column", alignItems: "center", gap: 16, border: "1px solid #7a3a3a",
            }} onClick={(e) => e.stopPropagation()}>
              <div style={{ fontSize: 14, fontWeight: 600, color: "#f44336" }}>Delete server</div>
              <div style={{ fontSize: 13, color: "#ccc" }}>
                Delete <strong style={{ color: "#e0e0e0" }}>{deleteTarget}</strong>?
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                <button onClick={confirmDelete}
                  style={{ padding: "7px 16px", background: "#b71c1c", border: "none", borderRadius: 4, color: "#fff", fontSize: 13, cursor: "pointer", fontWeight: 600 }}>
                  Delete
                </button>
                <button onClick={() => setDeleteTarget(null)}
                  style={{ padding: "7px 16px", background: "#333", border: "1px solid #555", borderRadius: 4, color: "#ccc", fontSize: 13, cursor: "pointer" }}>
                  Cancel
                </button>
              </div>
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
          {/* @sk-task multi-server#T3.1: server-specific settings section (AC-003) */}
          <Section key={`srv-${sectionKey}`} title={`Server: ${activeServer}`} defaultOpen={true}>
            <label style={lbl}>Name
              <input style={inp} placeholder="Server name" value={activeServer} onChange={(e) => {
                const newName = e.target.value;
                setActiveServer(newName);
                setDirty(true);
              }} />
            </label>
            <label style={lbl}>Server
              <input style={inp} placeholder="wss://example.com/tunnel" value={serverConfig.server || ""} onChange={(e) => updateServer("server", e.target.value)} />
            </label>
            <label style={lbl}>Token
              <div style={{ display: "flex", gap: 3 }}>
                <input type={showToken ? "text" : "password"} style={inp} placeholder="auth token" value={serverConfig.auth?.token || ""}
                  onChange={(e) => nestServer("auth", "token", e.target.value)} />
                <button onClick={() => setShowToken(!showToken)} style={{ padding: "4px 6px", background: "#333", border: "1px solid #444", borderRadius: 4, color: "#aaa", cursor: "pointer", fontSize: 11, whiteSpace: "nowrap" }}>
                  {showToken ? "Hide" : "Show"}
                </button>
              </div>
            </label>
            <label style={lbl}>Mode
              <select style={inp} value={serverConfig.mode || "proxy"} onChange={(e) => updateServer("mode", e.target.value)}>
                <option value="proxy">Proxy (SOCKS5/HTTP)</option>
                {platform === "linux" && <option value="tun">TUN</option>}
              </select>
              {platform !== "linux" && <div style={{ color: "#888", fontSize: 10, marginTop: 2 }}>TUN mode not available on {platform}</div>}
            </label>
            <label style={lbl}>Transport
              <select style={inp} value={serverConfig.transport || "tcp"} onChange={(e) => updateServer("transport", e.target.value)}>
                <option value="tcp">TCP (WebSocket)</option>
                <option value="quic">QUIC (UDP)</option>
              </select>
            </label>
          </Section>

          <Section key={`tls-${sectionKey}`} title="TLS" defaultOpen={allExpanded}>
            <label style={lbl}>Verify Mode
              <select style={inp} value={serverConfig.tls?.verify_mode || "verify"} onChange={(e) => nestServer("tls", "verify_mode", e.target.value)}>
                <option value="verify">Verify</option>
                <option value="insecure">Insecure</option>
                <option value="none">None</option>
              </select>
            </label>
            <label style={lbl}>Server Name (SNI)
              <input style={inp} placeholder="example.com" value={serverConfig.tls?.server_name || ""} onChange={(e) => nestServer("tls", "server_name", e.target.value)} />
            </label>
            <label style={lbl}>CA File
              <input style={inp} placeholder="/path/to/ca.pem" value={serverConfig.tls?.ca_file || ""} onChange={(e) => nestServer("tls", "ca_file", e.target.value)} />
            </label>
            {/* @sk-task whitelist-obfuscation#T3.3: SNI chip list (AC-004) */}
            <ChipList label="Custom SNI (random on connect)" values={serverConfig.tls?.sni} onChange={(v) => nestServer("tls", "sni", v)} />
          </Section>

          <Section key={`adv-${sectionKey}`} title="Advanced" defaultOpen={allExpanded}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
              <label style={lbl}>MTU
                <input type="number" style={inp} value={serverConfig.mtu ?? 1400} onChange={(e) => updateServer("mtu", e.target.value ? parseInt(e.target.value) : 1400)} />
              </label>
              <label style={lbl}>Max Message Size (bytes)
                <input type="number" style={inp} value={serverConfig.max_message_size ?? 10485760} onChange={(e) => updateServer("max_message_size", e.target.value ? parseInt(e.target.value) : 10485760)} />
              </label>
            </div>
            <Checkbox checked={serverConfig.ipv6 ?? false} onChange={(v) => updateServer("ipv6", v)} label="Enable IPv6" />
            <Checkbox checked={serverConfig.auto_reconnect ?? true} onChange={(v) => updateServer("auto_reconnect", v)} label="Auto Reconnect" />
            <Checkbox checked={serverConfig.multiplex ?? false} onChange={(v) => updateServer("multiplex", v)} label="Multiplex" />
            {/* @sk-task whitelist-obfuscation#T3.3: obfuscation sub-settings (AC-005) */}
            <Checkbox checked={serverConfig.obfuscation?.enabled ?? false} onChange={(v) => nestServer("obfuscation", "enabled", v)} label="Obfuscation (anti-DPI)" />
            {serverConfig.obfuscation?.enabled && <div style={{ marginLeft: 16, marginTop: 4, padding: "6px 8px", borderLeft: "2px solid #333" }}>
              <Checkbox checked={serverConfig.obfuscation?.utls?.enabled ?? false} onChange={(v) => nestServer2("obfuscation", "utls", "enabled", v)} label="uTLS (Chrome JA3 fingerprint)" />
              {serverConfig.obfuscation?.utls?.enabled && <div style={{ marginLeft: 16 }}>
                <Checkbox checked={serverConfig.obfuscation?.utls?.fallback ?? true} onChange={(v) => nestServer2("obfuscation", "utls", "fallback", v)} label="uTLS Fallback (crypto/tls on error)" />
              </div>}
              <Checkbox checked={serverConfig.obfuscation?.padding?.enabled ?? false} onChange={(v) => nestServer2("obfuscation", "padding", "enabled", v)} label="WS Padding (fixed packet size)" />
              {serverConfig.obfuscation?.padding?.enabled && <div style={{ marginLeft: 16 }}>
                <label style={lbl}>Padding Size
                  <input type="number" style={inp} value={serverConfig.obfuscation?.padding?.size ?? 512} onChange={(e) => nestServer2("obfuscation", "padding", "size", parseInt(e.target.value) || 512)} />
                </label>
              </div>}
            </div>}
          </Section>

          <Section key={`route-${sectionKey}`} title="Routing" defaultOpen={allExpanded}>
            <label style={lbl}>Default Route
              <select style={inp} value={serverConfig.routing?.default_route || "server"} onChange={(e) => nestServer("routing", "default_route", e.target.value)}>
                <option value="server">Server (VPN)</option>
                <option value="direct">Direct (bypass)</option>
              </select>
            </label>
            <ChipList label="Include CIDR" values={serverConfig.routing?.include_ranges} onChange={(v) => nestServer("routing", "include_ranges", v)} />
            <ChipList label="Exclude CIDR" values={serverConfig.routing?.exclude_ranges} onChange={(v) => nestServer("routing", "exclude_ranges", v)} />
            <ChipList label="Include IPs" values={serverConfig.routing?.include_ips} onChange={(v) => nestServer("routing", "include_ips", v)} />
            <ChipList label="Exclude IPs" values={serverConfig.routing?.exclude_ips} onChange={(v) => nestServer("routing", "exclude_ips", v)} />
            <ChipList label="Include Domains" values={serverConfig.routing?.include_domains} onChange={(v) => nestServer("routing", "include_domains", v)} />
            <ChipList label="Exclude Domains" values={serverConfig.routing?.exclude_domains} onChange={(v) => nestServer("routing", "exclude_domains", v)} />
          </Section>

          <Section key={`ks-${sectionKey}`} title="Kill Switch & Reconnect" defaultOpen={allExpanded}>
            <Checkbox checked={serverConfig.kill_switch?.enabled ?? false} onChange={(v) => nestServer("kill_switch", "enabled", v)} label="Kill Switch (block on disconnect)" />
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6, marginTop: 6 }}>
              <label style={lbl}>Min Backoff (s)
                <input type="number" style={inp} value={serverConfig.reconnect?.min_backoff_sec ?? 1} onChange={(e) => nestServer("reconnect", "min_backoff_sec", parseInt(e.target.value) || 1)} />
              </label>
              <label style={lbl}>Max Backoff (s)
                <input type="number" style={inp} value={serverConfig.reconnect?.max_backoff_sec ?? 30} onChange={(e) => nestServer("reconnect", "max_backoff_sec", parseInt(e.target.value) || 30)} />
              </label>
            </div>
          </Section>

          <Section key={`enc-${sectionKey}`} title="Encryption" defaultOpen={allExpanded}>
            <Checkbox checked={serverConfig.crypto?.enabled ?? false} onChange={(v) => nestServer("crypto", "enabled", v)} label="AES-256-GCM Encryption" />
            {serverConfig.crypto?.enabled && (
              <label style={lbl}>Key
                <input type="password" style={inp} placeholder="hex 256-bit key" value={serverConfig.crypto?.key || ""} onChange={(e) => nestServer("crypto", "key", e.target.value)} />
              </label>
            )}
          </Section>

          {/* @sk-task multi-server#T3.1: global settings section (AC-001) */}
          <Section key={`global-${sectionKey}`} title="Global Settings" defaultOpen={allExpanded}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
              <label style={lbl}>Log Level
                <select style={inp} value={globalConfig.log?.level || "info"} onChange={(e) => nestGlobal("log", "level", e.target.value)}>
                  <option value="debug">Debug</option>
                  <option value="info">Info</option>
                  <option value="warn">Warn</option>
                  <option value="error">Error</option>
                </select>
                <div style={{ fontSize: 10, color: "#555", marginTop: 2 }}>Client log level (file/terminal). Live Log filter below.</div>
              </label>
            </div>
            <label style={lbl}>Proxy Listen
              <input style={inp} placeholder="127.0.0.1:2310" value={globalConfig.proxy_listen || "127.0.0.1:2310"} onChange={(e) => updateGlobal("proxy_listen", e.target.value)} />
            </label>
            <label style={lbl}>Proxy Username
              <input style={inp} value={globalConfig.proxy_auth?.username || ""} onChange={(e) => nestGlobal("proxy_auth", "username", e.target.value)} />
            </label>
            <label style={lbl}>Proxy Password
              <input type="password" style={inp} value={globalConfig.proxy_auth?.password || ""} onChange={(e) => nestGlobal("proxy_auth", "password", e.target.value)} />
            </label>
            {/* @sk-task system-proxy#T2.2: system proxy checkbox (AC-001) */}
            <Checkbox checked={globalConfig.system_proxy ?? true} onChange={(v) => updateGlobal("system_proxy", v)} label="Use as system proxy" />
            {/* @sk-task transparent-proxy#T3.1: transparent proxy checkbox + DNS proxy settings (AC-001, AC-009) */}
            {platform === "linux" ? (
              <Checkbox checked={globalConfig.transparent ?? false} onChange={(v) => updateGlobal("transparent", v)} label="Transparent proxy (iptables REDIRECT)" />
            ) : (
              <div style={{ color: "#888", fontSize: 11, marginTop: 4 }}>Transparent proxy not available on {platform}</div>
            )}
            {globalConfig.transparent && <div style={{ marginLeft: 16, marginTop: 4, padding: "6px 8px", borderLeft: "2px solid #333" }}>
              <label style={lbl}>DNS Proxy Listen
                <input style={inp} placeholder="127.0.0.54:53" value={globalConfig.dns_proxy?.listen || "127.0.0.54:53"} onChange={(e) => nestGlobal("dns_proxy", "listen", e.target.value)} />
              </label>
              <label style={lbl}>DNS Upstream (trusted resolver)
                <input style={inp} placeholder="1.1.1.1:53" value={globalConfig.dns_proxy?.upstream || "1.1.1.1:53"} onChange={(e) => nestGlobal("dns_proxy", "upstream", e.target.value)} />
                <div style={{ fontSize: 10, color: "#555", marginTop: 2 }}>DNS queries are forwarded through the tunnel to this resolver.</div>
              </label>
            </div>}
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
