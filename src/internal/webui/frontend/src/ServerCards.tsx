import { useState, useRef, useEffect } from "react";
import type { ServerEntry, Status } from "./types";
import { colors, borderRadius } from "./theme";

interface ServerCardsProps {
  servers: ServerEntry[];
  activeServer: string;
  status: Status;
  onSelect: (name: string) => void;
  onAdd: () => void;
  onDelete: (name: string) => void;
  onCopyConfig: (name: string) => void;
}

const styles: Record<string, React.CSSProperties> = {
  pill: {
    display: "flex",
    alignItems: "center",
    gap: 10,
    padding: "10px 14px",
    background: colors.cardBg,
    borderRadius: borderRadius.lg,
    cursor: "pointer",
    border: `1px solid transparent`,
    marginTop: 4,
  },
  pillIcon: {
    width: 32,
    height: 32,
    borderRadius: borderRadius.lg,
    background: `linear-gradient(135deg, ${colors.accent}, #5c3cfc)`,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    fontWeight: 700,
    fontSize: 14,
    color: "#fff",
    flexShrink: 0,
  },
  pillInfo: { flex: 1, minWidth: 0 },
  pillName: { fontSize: 13, fontWeight: 600, whiteSpace: "nowrap" as const, overflow: "hidden", textOverflow: "ellipsis" },
  pillUrl: { fontSize: 11, color: colors.textDim, whiteSpace: "nowrap" as const, overflow: "hidden", textOverflow: "ellipsis" },
  chevron: { color: colors.textMuted, fontSize: 18 },
  list: {
    display: "flex",
    flexDirection: "column" as const,
    gap: 4,
    marginTop: 6,
    background: colors.cardBg,
    borderRadius: borderRadius.xl,
    padding: 6,
  },
  card: {
    display: "flex",
    alignItems: "center",
    gap: 10,
    padding: "8px 10px",
    borderRadius: borderRadius.lg,
    cursor: "pointer",
    border: "1px solid transparent",
    transition: "background 0.12s",
  },
  cardInfo: { flex: 1, minWidth: 0 },
  cardName: { fontSize: 13, fontWeight: 500 },
  cardUrl: { fontSize: 11, color: colors.textDim, whiteSpace: "nowrap" as const, overflow: "hidden", textOverflow: "ellipsis" },
  cardError: { fontSize: 10, color: colors.error },
  cardStatus: { fontSize: 10, color: colors.textMuted, textTransform: "uppercase" as const, letterSpacing: "0.3px" },
  actions: { display: "flex", gap: 4 },
  actionBtn: {
    background: "none",
    border: "none",
    color: colors.textDim,
    cursor: "pointer",
    fontSize: 14,
    padding: "2px 4px",
    borderRadius: borderRadius.sm,
  },
  addBtnContainer: { padding: "4px 2px 0" },
  addBtn: {
    width: "100%",
    padding: "6px 10px",
    borderRadius: borderRadius.md,
    border: `1px solid ${colors.cardBorder}`,
    background: "transparent",
    color: colors.textDim,
    cursor: "pointer",
    fontSize: 11,
    fontWeight: 600,
  },
};

function statusColor(st: string): string {
  switch (st) {
    case "connected": return colors.success;
    case "error": return colors.error;
    default: return colors.textMuted;
  }
}

function statusDot(st: string): React.CSSProperties {
  return {
    width: 8,
    height: 8,
    borderRadius: "50%",
    background: statusColor(st),
    flexShrink: 0,
  };
}

// @sk-task kvn-web-redesign#T2.1: server card list with status dots and actions (AC-003, AC-004)
export default function ServerCards({ servers, activeServer, status, onSelect, onAdd, onDelete, onCopyConfig }: ServerCardsProps) {
  const [open, setOpen] = useState(false);
  const listRef = useRef<HTMLDivElement>(null);

  const active = servers.find((s) => s.name === activeServer);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (listRef.current && !listRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const handleSelect = (name: string) => {
    onSelect(name);
    setOpen(false);
  };

  return (
    <div ref={listRef}>
      <div
        style={styles.pill}
        onClick={() => setOpen(!open)}
        onMouseEnter={(e) => { e.currentTarget.style.borderColor = colors.cardBorder; }}
        onMouseLeave={(e) => { e.currentTarget.style.borderColor = "transparent"; }}
      >
        <div style={styles.pillIcon}>{active?.name?.charAt(0)?.toUpperCase() || "?"}</div>
        <div style={styles.pillInfo}>
          <div style={styles.pillName}>{active?.name || "No server"}</div>
          <div style={styles.pillUrl}>{active?.server || ""}</div>
        </div>
        <span style={styles.chevron}>{open ? "▴" : "▾"}</span>
      </div>

      {open && (
        <div style={styles.list}>
          {servers.length === 0 && (
            <div style={{ padding: "12px 8px", textAlign: "center", color: colors.textMuted, fontSize: 12 }}>
              No servers configured
            </div>
          )}
          {servers.map((srv) => {
            const isActive = srv.name === activeServer;
            const connStatus = isActive ? status : "disconnected";
            return (
              <div
                key={srv.name}
                style={{
                  ...styles.card,
                  background: isActive ? "#1a1a3a" : "transparent",
                  borderColor: isActive ? `${colors.accent}44` : "transparent",
                }}
                onClick={() => handleSelect(srv.name)}
                onMouseEnter={(e) => { if (!isActive) e.currentTarget.style.background = "#1a1a3a"; e.currentTarget.style.borderColor = colors.cardBorder; }}
                onMouseLeave={(e) => { if (!isActive) e.currentTarget.style.background = "transparent"; e.currentTarget.style.borderColor = "transparent"; }}
              >
                <div style={statusDot(connStatus)} />
                <div style={styles.cardInfo}>
                  <div style={styles.cardName}>{srv.name}</div>
                  <div style={styles.cardUrl}>{srv.server || ""}</div>
                  {connStatus === "error" && srv.auth?.token === undefined && (
                    <div style={styles.cardError}>✗ Connection error</div>
                  )}
                </div>
                <div style={{ ...styles.cardStatus, color: statusColor(connStatus) }}>
                  {connStatus}
                </div>
                <div style={styles.actions}>
                  <button
                    style={styles.actionBtn}
                    title="Copy config"
                    onClick={(e) => { e.stopPropagation(); onCopyConfig(srv.name); }}
                    onMouseEnter={(e) => { e.currentTarget.style.color = colors.text; e.currentTarget.style.background = colors.cardBorder; }}
                    onMouseLeave={(e) => { e.currentTarget.style.color = colors.textDim; e.currentTarget.style.background = "none"; }}
                  >
                    ⎘
                  </button>
                  <button
                    style={{ ...styles.actionBtn, color: colors.textDim }}
                    title="Delete"
                    onClick={(e) => { e.stopPropagation(); onDelete(srv.name); }}
                    onMouseEnter={(e) => { e.currentTarget.style.color = colors.error; e.currentTarget.style.background = colors.errorBg; }}
                    onMouseLeave={(e) => { e.currentTarget.style.color = colors.textDim; e.currentTarget.style.background = "none"; }}
                  >
                    ✕
                  </button>
                </div>
              </div>
            );
          })}
          <div style={styles.addBtnContainer}>
            <button style={styles.addBtn} onClick={onAdd} onMouseEnter={(e) => { e.currentTarget.style.borderColor = colors.accent; e.currentTarget.style.color = colors.accent; }} onMouseLeave={(e) => { e.currentTarget.style.borderColor = colors.cardBorder; e.currentTarget.style.color = colors.textDim; }}>
              + Add Server
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
