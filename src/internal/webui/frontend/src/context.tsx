import { createContext, useContext, useState, useEffect, useRef, useCallback } from "react";
import type { Status, ServerEntry, ClientConfig, LogEntry, MetricSnapshot, ServersResponse } from "./types";

export interface AppState {
  servers: ServerEntry[];
  activeServer: string;
  serverConfig: ClientConfig;
  globalConfig: ClientConfig;
  status: Status;
  platform: string;
  logs: LogEntry[];
  metrics: MetricSnapshot[];
  latestMetric: MetricSnapshot | null;
  dirty: boolean;
  saving: boolean;
  toast: string;
  formValid: boolean;
  setFormValid: (v: boolean) => void;
  logPaused: boolean;
  setLogPaused: (v: boolean) => void;
  serverName: string;
  setServerName: (v: string) => void;

  connect: () => void;
  disconnect: () => void;
  saveAll: () => Promise<void>;
  addServer: () => Promise<void>;
  deleteServer: (name: string) => Promise<void>;
  selectServer: (name: string) => void;
  exportConfig: () => void;
  doImport: (json: string) => Promise<void>;
  showToast: (msg: string) => void;

  updateServer: (key: string, val: any) => void;
  nestServer: (parent: string, key: string, val: any) => void;
  nestServer2: (parent: string, child: string, key: string, val: any) => void;
  updateGlobal: (key: string, val: any) => void;
  nestGlobal: (parent: string, key: string, val: any) => void;

  addSourceRule: (list: "include_sources" | "exclude_sources") => void;
  removeSourceRule: (list: "include_sources" | "exclude_sources", idx: number) => void;
  updateSourceRule: (list: "include_sources" | "exclude_sources", idx: number, field: string, val: string | undefined) => void;
  refreshSources: () => Promise<void>;
}

const AppCtx = createContext<AppState | null>(null);

export function useApp(): AppState {
  const ctx = useContext(AppCtx);
  if (!ctx) throw new Error("useApp must be used within AppProvider");
  return ctx;
}

// @sk-task kvn-web-redesign#T2.5: global state via React Context (AC-005)
export function AppProvider({ children }: { children: React.ReactNode }) {
  const [servers, setServers] = useState<ServerEntry[]>([]);
  const [activeServer, setActiveServer] = useState("");
  const [serverConfig, setServerConfig] = useState<ClientConfig>({});
  const [globalConfig, setGlobalConfig] = useState<ClientConfig>({});
  const [status, setStatus] = useState<Status>("disconnected");
  const [platform, setPlatform] = useState("");
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [logPaused, setLogPaused] = useState(false);
  const logPausedRef = useRef(false);
  logPausedRef.current = logPaused;
  const [metrics, setMetrics] = useState<MetricSnapshot[]>([]);
  const [latestMetric, setLatestMetric] = useState<MetricSnapshot | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState("");
  const [formValid, setFormValid] = useState(true);
  const [serverName, setServerName] = useState("");
  const toastTimer = useRef<ReturnType<typeof setTimeout>>();

  // Sync serverName when activeServer changes
  useEffect(() => {
    setServerName(activeServer);
  }, [activeServer]);

  const showToast = useCallback((msg: string) => {
    setToast(msg);
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(""), 2500);
  }, []);

  // Load servers
  const loadServers = useCallback(async (switchTo?: string) => {
    try {
      const r = await fetch("/api/servers");
      const data: ServersResponse = await r.json();
      setServers(data.servers);
      const target = switchTo || data.active_server || data.servers[0]?.name || "";
      setActiveServer(target);
      const srv = data.servers.find((s) => s.name === target);
      setServerConfig({ ...srv });
    } catch (e) {
      showToast("Failed to load servers");
    }
  }, [showToast]);

  const loadGlobalConfig = useCallback(async () => {
    try {
      const r = await fetch("/api/config");
      const data = await r.json();
      setGlobalConfig(data.config || data || {});
    } catch {
      showToast("Failed to load config");
    }
  }, [showToast]);

  // SSE
  useEffect(() => {
    loadServers();
    loadGlobalConfig();
    fetch("/api/platform").then(r => r.json()).then(d => setPlatform(d.os || "")).catch(() => {});
  }, [loadServers, loadGlobalConfig]);

  useEffect(() => {
    const es = new EventSource("/api/logs");
    es.addEventListener("status", (e) => {
      try { const d = JSON.parse(e.data); if (d.status) setStatus(d.status); } catch {}
    });
    es.addEventListener("log", (e) => {
      try { const entry = JSON.parse(e.data) as LogEntry; if (!logPausedRef.current) setLogs((prev) => [...prev.slice(-999), entry]); } catch {}
    });
    es.addEventListener("metric", (e) => {
      try {
        const m = JSON.parse(e.data) as MetricSnapshot;
        setLatestMetric(m);
        setMetrics((prev) => [...prev.slice(-59), m]);
      } catch {}
    });
    return () => es.close();
  }, []);

  // API calls
  const disconnect = useCallback(async () => {
    await fetch("/api/disconnect", { method: "POST" });
  }, []);

  const saveAll = useCallback(async () => {
    if (!formValid) {
      showToast("Fix validation errors before saving");
      return;
    }
    setSaving(true);
    try {
      await fetch("/api/config/global", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...globalConfig, active_server: serverName }),
      });
      const srv = servers.find((s) => s.name === activeServer);
      if (srv) {
        await fetch(`/api/servers/${encodeURIComponent(srv.name)}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ...srv, ...serverConfig, name: serverName }),
        });
      }
      setDirty(false);
      if (serverName !== activeServer) {
        await loadServers(serverName);
      } else {
        await loadServers();
      }
      showToast("Config saved");
    } catch {
      showToast("Save failed");
    } finally {
      setSaving(false);
    }
  }, [formValid, globalConfig, activeServer, servers, serverConfig, showToast, serverName, loadServers]);

  const connect = useCallback(async () => {
    if (dirty) await saveAll();
    await fetch("/api/config/global", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...globalConfig, active_server: activeServer }),
    });
    const r = await fetch("/api/connect", { method: "POST" });
    if (!r.ok) showToast("Connect failed");
  }, [dirty, saveAll, globalConfig, activeServer, showToast]);

  const addServer = useCallback(async () => {
    const name = `server-${Date.now()}`;
    await fetch("/api/servers", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    });
    await loadServers(name);
    showToast(`Server "${name}" created`);
  }, [loadServers, showToast]);

  const deleteServer = useCallback(async (name: string) => {
    await fetch(`/api/servers/${encodeURIComponent(name)}`, { method: "DELETE" });
    await loadServers();
    showToast(`Server "${name}" deleted`);
  }, [loadServers, showToast]);

  const selectServer = useCallback((name: string) => {
    setActiveServer(name);
    const srv = servers.find((s) => s.name === name);
    if (srv) setServerConfig({ ...srv });
  }, [servers]);

  const exportConfig = useCallback(() => {
    const data = JSON.stringify({ servers, active_server: activeServer, ...globalConfig }, null, 2);
    navigator.clipboard.writeText(data).then(() => showToast("Config copied")).catch(() => showToast("Copy failed"));
  }, [servers, activeServer, globalConfig, showToast]);

  const doImport = useCallback(async (json: string) => {
    try {
      const data = JSON.parse(json);
      const name = data.name || `imported-${Date.now()}`;
      await fetch("/api/servers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, ...data }),
      });
      await loadServers(name);
      showToast(`Imported as "${name}"`);
    } catch {
      showToast("Invalid JSON");
    }
  }, [loadServers, showToast]);

  // Server config updaters
  const updateServer = useCallback((key: string, val: any) => {
    setServerConfig((prev) => ({ ...prev, [key]: val }));
    setDirty(true);
  }, []);

  const nestServer = useCallback((parent: string, key: string, val: any) => {
    setServerConfig((prev) => ({
      ...prev,
      [parent]: { ...((prev as any)[parent] || {}), [key]: val },
    }));
    setDirty(true);
  }, []);

  const nestServer2 = useCallback((parent: string, child: string, key: string, val: any) => {
    setServerConfig((prev) => ({
      ...prev,
      [parent]: {
        ...((prev as any)[parent] || {}),
        [child]: { ...(((prev as any)[parent] || {})[child] || {}), [key]: val },
      },
    }));
    setDirty(true);
  }, []);

  // Global config updaters
  const updateGlobal = useCallback((key: string, val: any) => {
    setGlobalConfig((prev) => ({ ...prev, [key]: val }));
    setDirty(true);
  }, []);

  const nestGlobal = useCallback((parent: string, key: string, val: any) => {
    setGlobalConfig((prev) => ({
      ...prev,
      [parent]: { ...((prev as any)[parent] || {}), [key]: val },
    }));
    setDirty(true);
  }, []);

  // Source rules
  const updateSourceRule = useCallback((list: "include_sources" | "exclude_sources", idx: number, field: string, val: string | undefined) => {
    setServerConfig((prev) => {
      const routing = { ...prev.routing };
      const sources = [...(routing[list] || [])];
      const src = { ...sources[idx] };
      if (val === "" || field === "geoip" || field === "geosite" || field === "cidr" || field === "url") {
        ["geoip", "geosite", "cidr", "url"].forEach((f) => { if (f !== field) delete (src as any)[f]; });
      }
      (src as any)[field] = val || undefined;
      sources[idx] = src;
      routing[list] = sources;
      return { ...prev, routing };
    });
    setDirty(true);
  }, []);

  const addSourceRule = useCallback((list: "include_sources" | "exclude_sources") => {
    setServerConfig((prev) => {
      const routing = { ...prev.routing };
      const sources = [...(routing[list] || []), {} as any];
      routing[list] = sources;
      return { ...prev, routing };
    });
    setDirty(true);
  }, []);

  const removeSourceRule = useCallback((list: "include_sources" | "exclude_sources", idx: number) => {
    setServerConfig((prev) => {
      const routing = { ...prev.routing };
      const sources = [...(routing[list] || [])];
      sources.splice(idx, 1);
      routing[list] = sources;
      return { ...prev, routing };
    });
    setDirty(true);
  }, []);

  const refreshSources = useCallback(async () => {
    try {
      const r = await fetch("/api/config/refresh-sources", { method: "POST" });
      const data = await r.json();
      showToast(data.status === "ok" ? "Sources refreshed" : "Refresh done");
    } catch {
      showToast("Refresh failed");
    }
  }, [showToast]);

  const ctx: AppState = {
    servers, activeServer, serverConfig, globalConfig, status, platform, logs, metrics, latestMetric, dirty, saving, toast,
    connect, disconnect, saveAll, addServer, deleteServer, selectServer, exportConfig, doImport, showToast,
    updateServer, nestServer, nestServer2, updateGlobal, nestGlobal,
    addSourceRule, removeSourceRule, updateSourceRule, refreshSources,
    formValid, setFormValid,
    logPaused, setLogPaused,
    serverName, setServerName,
  };

  return <AppCtx.Provider value={ctx}>{children}</AppCtx.Provider>;
}
