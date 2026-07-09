import { useState } from "react";
import QRCode from "qrcode";
import { AppProvider, useApp } from "./context";
import ServerCards from "./ServerCards";
import TabbedForm from "./TabbedForm";
import TrafficMeter from "./TrafficMeter";
import LogPanel from "./LogPanel";
import { colors, borderRadius, fontSize } from "./theme";

const cardStyle: React.CSSProperties = {
  background: colors.card,
  borderRadius: borderRadius.xl,
  padding: 16,
  border: `1px solid ${colors.cardBorder}`,
};

const btnPrimary: React.CSSProperties = {
  padding: "7px 14px",
  borderRadius: borderRadius.md,
  border: "none",
  fontSize: 12,
  fontWeight: 600,
  cursor: "pointer",
  background: colors.accent,
  color: "#fff",
};

const btnSuccess: React.CSSProperties = {
  ...btnPrimary,
  background: colors.success,
  color: "#111",
};

const btnInfo: React.CSSProperties = {
  ...btnPrimary,
  background: colors.info,
};

const btnDanger: React.CSSProperties = {
  ...btnPrimary,
  background: colors.errorBg,
  color: colors.error,
  border: `1px solid ${colors.errorBorder}`,
};

const btnOutline: React.CSSProperties = {
  padding: "7px 14px",
  borderRadius: borderRadius.md,
  border: `1px solid ${colors.cardBorder}`,
  background: "transparent",
  color: colors.textDim,
  cursor: "pointer",
  fontSize: 12,
  fontWeight: 600,
};

const sectionLabel: React.CSSProperties = {
  fontSize: fontSize.sm,
  color: colors.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.4px",
  marginBottom: 6,
};

// @sk-task win-tun#T5.2: wire tunSupported from context to TabbedForm (AC-011)
function AppInner() {
  const {
    servers, activeServer, serverConfig, globalConfig, status, logs, metrics, latestMetric,
    dirty, saving, toast, connect, disconnect, saveAll, addServer, deleteServer, selectServer,
    exportConfig, doImport, showToast, setFormValid, serverName, setServerName, tunSupported,
    updateServer, nestServer, nestServer2, updateGlobal, nestGlobal,
    addSourceRule, removeSourceRule, updateSourceRule, refreshSources,
    addRoutingString, removeRoutingString,
  } = useApp();

  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState("");
  const [qrOpen, setQrOpen] = useState(false);
  const [qrData, setQrData] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);

  const handleQr = async () => {
    const data = JSON.stringify({ ...serverConfig, name: activeServer });
    setQrData(data);
    setQrOpen(true);
  };

  const isConnected = status === "connected" || status === "connecting";

  return (
    <div style={{ display: "flex", gap: 16, maxWidth: 1260, width: "100%", margin: "0 auto", height: "calc(100vh - 48px)" }}>
      {/* Left Panel */}
      <div style={{ width: 500, flexShrink: 0, display: "flex", flexDirection: "column", gap: 12 }}>
        {/* Header card */}
        <div style={cardStyle}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
            <div style={{ fontSize: 18, fontWeight: 700 }}>
              <span style={{ color: colors.accent }}>KVN</span> <span style={{ color: colors.text }}>Web UI</span>
            </div>
            <div style={{
              display: "flex", alignItems: "center", gap: 6,
              padding: "4px 12px", borderRadius: 20,
              background: status === "connected" ? colors.successBg : status === "error" ? colors.errorBg : colors.cardBg,
              color: status === "connected" ? colors.success : status === "error" ? colors.error : colors.textDim,
              fontSize: 12, fontWeight: 600,
            }}>
              <span style={{
                width: 8, height: 8, borderRadius: "50%",
                background: status === "connected" ? colors.success : status === "error" ? colors.error : colors.textMuted,
                boxShadow: status === "connected" ? `0 0 6px ${colors.success}55` : "none",
              }} />
              {status}
            </div>
          </div>

          {/* Traffic Meter - only when connected */}
          {status === "connected" && (
            <TrafficMeter metrics={metrics} latest={latestMetric} />
          )}
        </div>

        {/* Server selector + actions */}
        <div style={{ ...cardStyle, padding: "12px 16px" }}>
          <div style={sectionLabel}>Active Server</div>
          <ServerCards
            servers={servers}
            activeServer={activeServer}
            status={status}
            onSelect={selectServer}
            onAdd={addServer}
            onDelete={(name) => setDeleteTarget(name)}
            onCopyConfig={(name) => {
              const srv = servers.find(s => s.name === name);
              if (srv) navigator.clipboard.writeText(JSON.stringify(srv, null, 2)).then(() => showToast("Config copied")).catch(() => {});
            }}
          />

          <div style={{ display: "flex", gap: 6, marginTop: 8, flexWrap: "wrap" }}>
            {isConnected ? (
              <button style={btnDanger} onClick={disconnect}>Disconnect</button>
            ) : (
              <button style={btnPrimary} onClick={connect} disabled={saving}>
                Connect
              </button>
            )}
            <button style={btnSuccess} onClick={saveAll} disabled={saving}>
              Save{dirty ? " ●" : ""}
            </button>
            <button style={btnInfo} onClick={exportConfig}>Export</button>
            <button style={btnInfo} onClick={() => setImportOpen(!importOpen)}>Import</button>
            <button style={btnPrimary} onClick={handleQr} title="QR Code">
              <svg viewBox="0 0 20 20" width="14" height="14" fill="currentColor" style={{ display: "block" }}>
                <rect x="1" y="1" width="7" height="7" rx="1.2" />
                <rect x="3" y="3" width="3" height="3" rx="0.5" fill="#111" />
                <rect x="12" y="1" width="7" height="3" rx="0.8" />
                <rect x="1" y="12" width="7" height="3" rx="0.8" />
                <rect x="12" y="8" width="3" height="7" rx="0.8" />
                <rect x="12" y="16" width="7" height="3" rx="0.8" />
                <rect x="16" y="12" width="3" height="3" rx="0.5" />
              </svg>
            </button>
          </div>

          {/* Import panel */}
          {importOpen && (
            <div style={{ marginTop: 8 }}>
              <textarea
                value={importText}
                onChange={(e) => setImportText(e.target.value)}
                placeholder="Paste server JSON config..."
                style={{
                  width: "100%", height: 80, padding: 8,
                  borderRadius: borderRadius.md,
                  border: `1px solid ${colors.inputBorder}`,
                  background: colors.inputBg,
                  color: colors.text,
                  fontSize: 12, fontFamily: "monospace",
                  resize: "vertical",
                }}
              />
              <div style={{ display: "flex", gap: 6, marginTop: 4 }}>
                <button style={btnPrimary} onClick={() => { doImport(importText); setImportOpen(false); setImportText(""); }}>
                  Import
                </button>
                <button style={btnOutline} onClick={() => setImportOpen(false)}>Cancel</button>
              </div>
            </div>
          )}
        </div>

        {/* Settings form */}
        <div style={{ ...cardStyle, flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
          <div style={sectionLabel}>Server Settings</div>
          <TabbedForm
            serverConfig={serverConfig}
            globalConfig={globalConfig}
            serverName={serverName}
            tunSupported={tunSupported}
            onServerNameChange={setServerName}
            onUpdateServer={updateServer}
            onNestServer={nestServer}
            onNestServer2={nestServer2}
            onUpdateGlobal={updateGlobal}
            onNestGlobal={nestGlobal}
            onAddSourceRule={addSourceRule}
            onRemoveSourceRule={removeSourceRule}
            onUpdateSourceRule={updateSourceRule}
            onRefreshSources={refreshSources}
            onAddRoutingString={addRoutingString}
            onRemoveRoutingString={removeRoutingString}
            onFormValidityChange={setFormValid}
          />
        </div>
      </div>

      {/* Right Panel - Logs */}
      <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        <div style={{ ...cardStyle, flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          <LogPanel logs={logs} />
        </div>
      </div>

      {/* QR Modal */}
      {qrOpen && (
        <div style={{
          position: "fixed", inset: 0, background: "rgba(0,0,0,0.7)",
          display: "flex", alignItems: "center", justifyContent: "center", zIndex: 100,
        }} onClick={() => setQrOpen(false)}>
          <div style={{
            background: colors.card, borderRadius: borderRadius.xl,
            padding: 24, border: `1px solid ${colors.cardBorder}`,
          }} onClick={(e) => e.stopPropagation()}>
            <QRCodeSVG data={qrData} />
            <div style={{ textAlign: "center", marginTop: 8, fontSize: 12, color: colors.textDim }}>
              Scan with KVN Android app
            </div>
            <button style={{ ...btnOutline, marginTop: 8, width: "100%" }} onClick={() => setQrOpen(false)}>Close</button>
          </div>
        </div>
      )}

      {/* Delete confirmation */}
      {deleteTarget && (
        <div style={{
          position: "fixed", inset: 0, background: "rgba(0,0,0,0.7)",
          display: "flex", alignItems: "center", justifyContent: "center", zIndex: 100,
        }} onClick={() => setDeleteTarget(null)}>
          <div style={{
            background: colors.card, borderRadius: borderRadius.xl,
            padding: 24, border: `1px solid ${colors.cardBorder}`, maxWidth: 400,
          }} onClick={(e) => e.stopPropagation()}>
            <div style={{ fontSize: 16, fontWeight: 600, marginBottom: 8 }}>Delete server?</div>
            <div style={{ fontSize: 13, color: colors.textDim, marginBottom: 16 }}>
              Are you sure you want to delete "{deleteTarget}"? This cannot be undone.
            </div>
            <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
              <button style={btnOutline} onClick={() => setDeleteTarget(null)}>Cancel</button>
              <button style={btnDanger} onClick={() => { deleteServer(deleteTarget); setDeleteTarget(null); }}>Delete</button>
            </div>
          </div>
        </div>
      )}

      {/* Toast */}
      {toast && (
        <div style={{
          position: "fixed", bottom: 24, left: "50%", transform: "translateX(-50%)",
          padding: "10px 20px", borderRadius: borderRadius.lg,
          background: colors.card, color: colors.text,
          border: `1px solid ${colors.cardBorder}`, zIndex: 100,
          animation: "fi 0.2s",
        }}>
          {toast}
        </div>
      )}
    </div>
  );
}

function QRCodeSVG({ data }: { data: string }) {
  const ref = React.useRef<HTMLCanvasElement>(null);
  React.useEffect(() => {
    if (ref.current) QRCode.toCanvas(ref.current, data, { width: 320 });
  }, [data]);
  return <canvas ref={ref} width={320} height={320} />;
}

// @sk-task kvn-web-redesign#T2.5: app shell with component composition and context (AC-005)
export default function App() {
  return (
    <AppProvider>
      <AppInner />
    </AppProvider>
  );
}

import React from "react";
