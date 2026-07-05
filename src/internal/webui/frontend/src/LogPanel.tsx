import { useState, useRef, useEffect, useCallback } from "react";
import type { LogEntry } from "./types";
import { ACTION_MAP } from "./types";
import { useApp } from "./context";
import { colors, borderRadius, fontSize } from "./theme";

interface LogPanelProps {
  logs: LogEntry[];
}

const LEVEL_COLORS: Record<string, string> = {
  error: colors.error,
  warn: colors.warning,
  info: colors.info,
  debug: colors.textMuted,
};

const LEVEL_BG: Record<string, string> = {
  error: colors.errorBg,
  warn: colors.warningBg,
  info: colors.infoBg,
  debug: "#111",
};

const LEVEL_SHORT: Record<string, string> = {
  error: "ERR",
  warn: "WRN",
  info: "INF",
  debug: "DBG",
};

function formatTS(ts?: string): string {
  if (!ts) return "?";
  try {
    const d = new Date(ts);
    if (isNaN(d.getTime())) return ts;
    return d.toTimeString().slice(0, 8) + "." + d.getMilliseconds().toString().padStart(3, "0");
  } catch {
    return ts;
  }
}

function formatAction(action?: number): string {
  if (action === undefined || action === null) return "";
  return ACTION_MAP[String(action)] ?? String(action);
}

// @sk-task kvn-web-redesign#T2.4: log panel with level-badge, search, pause, action mapping (AC-008, AC-009, AC-010, AC-012)
export default function LogPanel({ logs }: LogPanelProps) {
  const { logPaused, setLogPaused } = useApp();
  const [filter, setFilter] = useState<Record<string, boolean>>({
    error: true,
    warn: true,
    info: true,
    debug: false,
  });
  const [search, setSearch] = useState("");
  const [paused, setPaused] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const [newSincePause, setNewSincePause] = useState(0);
  const prevCountRef = useRef(logs.length);

  const toggleLevel = useCallback((level: string) => {
    setFilter((f) => ({ ...f, [level]: !f[level] }));
  }, []);

  const filtered = logs.filter((e) => {
    if (!filter[e.level] && filter[e.level] !== undefined) return false;
    if (filter[e.level] === undefined && e.level !== "debug") return false;
    if (search) {
      const q = search.toLowerCase();
      const lineMatch = e.line.toLowerCase().includes(q);
      const ipMatch = (e.ip || "").toLowerCase().includes(q);
      const actionMatch = formatAction(e.action).toLowerCase().includes(q);
      if (!lineMatch && !ipMatch && !actionMatch) return false;
    }
    return true;
  });

  // auto-scroll
  useEffect(() => {
    if (!paused && endRef.current) {
      endRef.current.scrollIntoView({ behavior: "smooth" });
    }
    if (paused && logs.length > prevCountRef.current) {
      setNewSincePause((n) => n + (logs.length - prevCountRef.current));
    }
    prevCountRef.current = logs.length;
  }, [logs.length, paused]);

  const handleScroll = useCallback(() => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    const atBottom = scrollHeight - scrollTop - clientHeight < 50;
    if (!atBottom && !paused) {
      setPaused(true);
    } else if (atBottom && paused) {
      setPaused(false);
      setNewSincePause(0);
    }
  }, [paused]);

  const scrollToBottom = useCallback(() => {
    setPaused(false);
    setNewSincePause(0);
    endRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  const handleCopy = useCallback((entry: LogEntry) => {
    const text = `${formatTS(entry.ts)} ${entry.level} ${entry.line}${entry.action !== undefined ? ` action:${formatAction(entry.action)}` : ""}${entry.ip ? ` ip:${entry.ip}` : ""}`;
    navigator.clipboard.writeText(text).catch(() => {});
  }, []);

  // @sk-task kvn-web-redesign#T3.2: export logs to .txt file (AC-011)
  const handleExport = useCallback(() => {
    const text = logs.map((e) => {
      return `${formatTS(e.ts)} ${e.level} ${e.line}${e.action !== undefined ? ` action:${formatAction(e.action)}` : ""}${e.ip ? ` ip:${e.ip}` : ""}`;
    }).join("\n");
    const blob = new Blob([text], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `kvn-log-${new Date().toISOString().slice(0, 19)}.txt`;
    a.click();
    URL.revokeObjectURL(url);
  }, [logs]);

  // @sk-task kvn-web-redesign#T3.2: clear logs (AC-011)
  const handleClear = useCallback(() => {
    window.location.reload();
  }, []);

  function highlightText(text: string, query: string) {
    if (!query) return text;
    const parts = text.split(new RegExp(`(${escapeRegex(query)})`, "gi"));
    return parts.map((part, i) =>
      part.toLowerCase() === query.toLowerCase()
        ? <mark key={i} style={{ background: `${colors.accent}44`, color: colors.text, borderRadius: 2, padding: "0 2px" }}>{part}</mark>
        : part
    );
  }

  return (
    <div style={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", flexShrink: 0, marginBottom: 8 }}>
        <h3
          style={{ fontSize: 13, fontWeight: 600, color: colors.textDim, textTransform: "uppercase", letterSpacing: "0.5px", cursor: "pointer", userSelect: "none" }}
          onClick={() => setLogPaused(!logPaused)}
          title={logPaused ? "Resume logs" : "Pause logs"}
        >
          {logPaused ? "⏸ Live Log" : "📋 Live Log"}
        </h3>
        <div style={{ display: "flex", gap: 4 }}>
          {(["error", "warn", "info", "debug"] as const).map((lvl) => (
            <button
              key={lvl}
              onClick={() => toggleLevel(lvl)}
              style={{
                padding: "3px 10px",
                borderRadius: 12,
                border: `1px solid ${filter[lvl] ? (lvl === "error" ? colors.errorBorder : lvl === "warn" ? "#5a4a2a" : colors.cardBorder) : colors.cardBorder}`,
                background: filter[lvl] ? (lvl === "error" ? colors.errorBg : lvl === "warn" ? colors.warningBg : colors.cardBorder) : "transparent",
                color: filter[lvl] ? (lvl === "error" ? colors.error : lvl === "warn" ? colors.warning : colors.text) : colors.textDim,
                fontSize: 10,
                fontWeight: 600,
                cursor: "pointer",
                transition: "all 0.12s",
              }}
            >
              {LEVEL_SHORT[lvl]}
            </button>
          ))}
        </div>
        <div style={{ position: "relative", flex: 1, minWidth: 120 }}>
          <span style={{ position: "absolute", left: 8, top: "50%", transform: "translateY(-50%)", color: colors.textMuted, fontSize: 12 }}>🔍</span>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search logs..."
            style={{
              width: "100%",
              padding: "5px 10px 5px 28px",
              borderRadius: borderRadius.md,
              border: `1px solid ${colors.inputBorder}`,
              background: colors.inputBg,
              color: colors.text,
              fontSize: 12,
              outline: "none",
            }}
          />
        </div>
        <span style={{ fontSize: fontSize.sm, color: colors.textMuted, whiteSpace: "nowrap" }}>{filtered.length} / {logs.length}</span>
        <div style={{ display: "flex", gap: 4 }}>
          <button
            onClick={handleExport}
            disabled={logs.length === 0}
            style={{
              padding: "4px 10px",
              borderRadius: borderRadius.md,
              border: `1px solid ${colors.cardBorder}`,
              background: "transparent",
              color: logs.length === 0 ? colors.textMuted : colors.textDim,
              cursor: logs.length === 0 ? "not-allowed" : "pointer",
              fontSize: fontSize.sm,
              fontWeight: 600,
              opacity: logs.length === 0 ? 0.4 : 1,
            }}
            title="Export logs"
          >
            ⤓
          </button>
          <button
            onClick={handleClear}
            disabled={logs.length === 0}
            style={{
              padding: "4px 10px",
              borderRadius: borderRadius.md,
              border: `1px solid ${colors.cardBorder}`,
              background: "transparent",
              color: logs.length === 0 ? colors.textMuted : colors.textDim,
              cursor: logs.length === 0 ? "not-allowed" : "pointer",
              fontSize: fontSize.sm,
              fontWeight: 600,
              opacity: logs.length === 0 ? 0.4 : 1,
            }}
            title="Clear"
          >
            ✕
          </button>
        </div>
      </div>

      {/* Log body */}
      <div
        style={{
          flex: 1,
          background: colors.logBg,
          border: `1px solid ${colors.logBorder}`,
          borderRadius: borderRadius.lg,
          overflow: "hidden",
          display: "flex",
          flexDirection: "column",
        }}
      >
        <div
          ref={scrollRef}
          onScroll={handleScroll}
          style={{
            flex: 1,
            overflowY: "auto",
            padding: "8px 0",
            fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
            fontSize: 12,
            lineHeight: 1.6,
          }}
        >
          {filtered.length === 0 ? (
            <div style={{ padding: "24px 16px", textAlign: "center", color: colors.textMuted, fontSize: 12 }}>
              {logs.length === 0 ? "No entries yet. Connect to see logs." : "No entries match the current filter."}
            </div>
          ) : (
            filtered.map((entry, i) => (
              <div
                key={i}
                onClick={() => handleCopy(entry)}
                style={{
                  display: "flex",
                  alignItems: "baseline",
                  gap: 8,
                  padding: "1px 12px",
                  cursor: "pointer",
                  transition: "background 0.1s",
                }}
                onMouseEnter={(e) => { e.currentTarget.style.background = "#111122"; }}
                onMouseLeave={(e) => { e.currentTarget.style.background = "transparent"; }}
              >
                <span style={{ color: colors.textMuted, fontSize: 11, flexShrink: 0, minWidth: 80 }}>
                  {formatTS(entry.ts)}
                </span>
                <span
                  style={{
                    flexShrink: 0,
                    minWidth: 32,
                    fontSize: 10,
                    fontWeight: 700,
                    padding: "1px 5px",
                    borderRadius: 3,
                    textAlign: "center" as const,
                    color: LEVEL_COLORS[entry.level] || colors.textDim,
                    background: LEVEL_BG[entry.level] || "#111",
                  }}
                >
                  {LEVEL_SHORT[entry.level] || "---"}
                </span>
                <span style={{ flex: 1, color: "#c0c0c0" }}>
                  {search ? highlightText(entry.line, search) : entry.line}
                </span>
                <span style={{ color: colors.textMuted, fontSize: 11, flexShrink: 0 }}>
                  {entry.action !== undefined && (
                    <span>action:{formatAction(entry.action)}</span>
                  )}
                  {entry.ip && (
                    <span style={{ color: `${colors.success}44`, marginLeft: entry.action !== undefined ? 4 : 0 }}>ip:{entry.ip}</span>
                  )}
                </span>
              </div>
            ))
          )}
          <div ref={endRef} />
        </div>

        {/* Pause indicator */}
        {paused && (
          <div
            onClick={scrollToBottom}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              padding: "6px 12px",
              background: colors.card,
              borderTop: `1px solid ${colors.cardBorder}`,
              fontSize: fontSize.sm,
              color: colors.textDim,
              cursor: "pointer",
              flexShrink: 0,
            }}
            onMouseEnter={(e) => { e.currentTarget.style.background = "#22223a"; }}
            onMouseLeave={(e) => { e.currentTarget.style.background = colors.card; }}
          >
            <span>⏸ Paused</span>
            {newSincePause > 0 && <span style={{ color: colors.accent }}>· {newSincePause} new entries</span>}
            <span style={{ marginLeft: "auto" }}>▼ Scroll to bottom</span>
          </div>
        )}
      </div>
    </div>
  );
}

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
