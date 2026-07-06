import { useState, useRef } from "react";
import type { ClientConfig, SourceRule } from "./types";
import { colors, borderRadius, fontSize } from "./theme";
import FormField, { useFieldValidation, rules } from "./FormField";

interface TabbedFormProps {
  serverConfig: ClientConfig;
  globalConfig: ClientConfig;
  serverName: string;
  onServerNameChange: (name: string) => void;
  onUpdateServer: (key: string, val: any) => void;
  onNestServer: (parent: string, key: string, val: any) => void;
  onNestServer2: (parent: string, child: string, key: string, val: any) => void;
  onUpdateGlobal: (key: string, val: any) => void;
  onNestGlobal: (parent: string, key: string, val: any) => void;
  onAddSourceRule: (list: "include_sources" | "exclude_sources") => void;
  onRemoveSourceRule: (list: "include_sources" | "exclude_sources", idx: number) => void;
  onUpdateSourceRule: (list: "include_sources" | "exclude_sources", idx: number, field: string, val: string | undefined) => void;
  onRefreshSources: () => void;
  onAddRoutingString: (list: string, val: string) => void;
  onRemoveRoutingString: (list: string, idx: number) => void;
  onFormValidityChange?: (isValid: boolean) => void;
}

const TABS = ["General", "TLS", "Routing", "Advanced", "Global"];

const tabBarStyle: React.CSSProperties = {
  display: "flex",
  gap: 2,
  background: colors.cardBg,
  borderRadius: borderRadius.lg,
  padding: 3,
  flexShrink: 0,
};

function tabStyle(active: boolean): React.CSSProperties {
  return {
    flex: 1,
    padding: "7px 4px",
    textAlign: "center",
    fontSize: fontSize.sm,
    fontWeight: 500,
    color: active ? "#fff" : colors.textDim,
    borderRadius: borderRadius.md,
    cursor: "pointer",
    border: "none",
    background: active ? colors.cardBorder : "none",
    transition: "all 0.12s",
  };
}

const tabContentStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 8,
  overflowY: "auto",
  flex: 1,
  paddingRight: 2,
};

function fieldStyle(): React.CSSProperties {
  return { display: "flex", flexDirection: "column", gap: 3 };
}

function labelStyle(): React.CSSProperties {
  return { display: "flex", justifyContent: "space-between", fontSize: fontSize.sm, color: colors.textDim, textTransform: "uppercase" as const, letterSpacing: "0.4px" };
}

const inputStyle: React.CSSProperties = {
  padding: "7px 10px",
  borderRadius: borderRadius.md,
  border: `1px solid ${colors.inputBorder}`,
  background: colors.inputBg,
  color: colors.text,
  fontSize: fontSize.md,
  outline: "none",
};

const selectStyle: React.CSSProperties = {
  ...inputStyle,
  appearance: "none",
  paddingRight: 24,
  backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%3E%3Cpath d='M0 0l5 6 5-6z' fill='%23888'/%3E%3C/svg%3E")`,
  backgroundRepeat: "no-repeat",
  backgroundPosition: "right 8px center",
  cursor: "pointer",
};

const subBlockStyle: React.CSSProperties = {
  padding: "8px 10px",
  border: `1px solid #222244`,
  borderRadius: borderRadius.md,
  background: "#14142a",
};

const subTitleStyle: React.CSSProperties = {
  fontSize: fontSize.sm,
  color: "#777",
  fontWeight: 600,
  marginBottom: 6,
  textTransform: "uppercase" as const,
  letterSpacing: "0.3px",
};

const chipStyle: React.CSSProperties = {
  padding: "5px 8px",
  borderRadius: borderRadius.sm,
  border: `1px solid ${colors.inputBorder}`,
  background: colors.inputBg,
  color: colors.text,
  fontSize: fontSize.sm,
  outline: "none",
  width: "100%",
};

const btnSmall: React.CSSProperties = {
  padding: "4px 10px",
  borderRadius: borderRadius.md,
  border: `1px solid ${colors.cardBorder}`,
  background: "transparent",
  color: colors.textDim,
  cursor: "pointer",
  fontSize: fontSize.sm,
  fontWeight: 600,
};

function SrcRow({ src, idx, list, onUpdate, onRemove }: {
  src: SourceRule;
  idx: number;
  list: "include_sources" | "exclude_sources";
  onUpdate: (list: "include_sources" | "exclude_sources", idx: number, field: string, val: string | undefined) => void;
  onRemove: (list: "include_sources" | "exclude_sources", idx: number) => void;
}) {
  const type = src.geoip ? "geoip" : src.geosite ? "geosite" : src.cidr ? "cidr" : src.url ? "url" : "geoip";
  return (
    <div style={{ display: "flex", gap: 4, marginBottom: 4, alignItems: "center" }}>
      <select
        style={{ ...chipStyle, width: 80 }}
        value={type}
        onChange={(e) => onUpdate(list, idx, e.target.value, "")}
      >
        <option value="geoip">GeoIP</option>
        <option value="geosite">GeoSite</option>
        <option value="cidr">CIDR</option>
        <option value="url">URL</option>
      </select>
      <input
        style={{ ...chipStyle, flex: 1 }}
        placeholder="value"
        value={src[type as keyof SourceRule] || ""}
        onChange={(e) => onUpdate(list, idx, type, e.target.value)}
      />
      <button
        style={{ padding: "2px 8px", background: colors.errorBg, border: `1px solid ${colors.errorBorder}`, borderRadius: borderRadius.sm, color: colors.error, cursor: "pointer", fontSize: 13 }}
        onClick={() => onRemove(list, idx)}
      >
        ×
      </button>
    </div>
  );
}

// @sk-task kvn-web-config-update#T2.1: ChipList component for string[] routing fields (AC-001, AC-002, AC-003)
function ChipList({ items, label, placeholder, onAdd, onRemove }: {
  items: string[];
  label: string;
  placeholder: string;
  onAdd: (val: string) => void;
  onRemove: (idx: number) => void;
}) {
  const [inputVal, setInputVal] = useState("");
  const handleAdd = () => {
    if (!inputVal.trim()) return;
    if (items.includes(inputVal.trim())) { setInputVal(""); return; }
    onAdd(inputVal.trim());
    setInputVal("");
  };
  return (
    <div style={fieldStyle()}>
      <span style={labelStyle()}>{label}</span>
      <div style={{ display: "flex", gap: 4, marginBottom: 4 }}>
        <input
          style={{ ...chipStyle, flex: 1 }}
          placeholder={placeholder}
          value={inputVal}
          onChange={(e) => setInputVal(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); handleAdd(); } }}
        />
        <button style={{ ...btnSmall, padding: "2px 10px" }} onClick={handleAdd}>+</button>
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
        {items.map((item, i) => (
          <span key={i} style={{
            display: "inline-flex", alignItems: "center", gap: 4,
            padding: "2px 6px", background: colors.cardBg, borderRadius: borderRadius.sm,
            border: `1px solid ${colors.cardBorder}`, fontSize: 12, color: colors.text,
          }}>
            {item}
            <span style={{ cursor: "pointer", color: colors.error, fontWeight: 700, fontSize: 13, lineHeight: 1 }} onClick={() => onRemove(i)}>×</span>
          </span>
        ))}
      </div>
    </div>
  );
}

// @sk-task kvn-web-redesign#T2.2: tabbed settings form with 5 tabs (AC-005)
export default function TabbedForm(props: TabbedFormProps) {
  const [activeTab, setActiveTab] = useState(0);
  const [showToken, setShowToken] = useState(false);
  const { serverConfig, globalConfig } = props;
  const { errors, validate, isValid } = useFieldValidation();
  const validityRef = useRef(isValid);
  validityRef.current = isValid;
  const prevValid = useRef(isValid);
  if (prevValid.current !== isValid) {
    prevValid.current = isValid;
    props.onFormValidityChange?.(isValid);
  }

  const renderGeneral = () => (
    <>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>Name</span>
        <input style={inputStyle} value={props.serverName} onChange={(e) => props.onServerNameChange(e.target.value)} placeholder="Server name" />
      </div>
      <FormField label="Server URL" error={errors.serverUrl}>
        <input
          style={errors.serverUrl ? { ...inputStyle, borderColor: colors.error, boxShadow: `0 0 0 1px ${colors.error}33` } : inputStyle}
          value={serverConfig.server || ""}
          onChange={(e) => { props.onUpdateServer("server", e.target.value); validate("serverUrl", e.target.value, [rules.wsUrl()]); }}
          onBlur={(e) => validate("serverUrl", e.target.value, [rules.wsUrl()], true)}
          placeholder="ws(s)://host:port"
        />
      </FormField>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>Auth Token</span>
        <div style={{ display: "flex", gap: 4, alignItems: "center" }}>
          <input style={{ ...inputStyle, flex: 1 }} type={showToken ? "text" : "password"} value={serverConfig.auth?.token || ""} onChange={(e) => props.onNestServer("auth", "token", e.target.value)} placeholder="Token" />
          <button style={{ ...btnSmall, padding: "2px 8px", minWidth: 32 }} onClick={() => setShowToken(!showToken)}>{showToken ? "🙈" : "👁️"}</button>
        </div>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Mode</span>
          <select style={selectStyle} value={serverConfig.mode || "tun"} onChange={(e) => props.onUpdateServer("mode", e.target.value)}>
            <option value="tun">TUN</option>
            <option value="proxy">Proxy</option>
          </select>
        </div>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Transport</span>
          <select style={selectStyle} value={serverConfig.transport || "websocket"} onChange={(e) => props.onUpdateServer("transport", e.target.value)}>
            <option value="websocket">WebSocket</option>
            <option value="quic">QUIC</option>
            <option value="tcp">TCP</option>
          </select>
        </div>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>MTU</span>
          <input style={inputStyle} type="number" value={serverConfig.mtu ?? 1400} onChange={(e) => props.onUpdateServer("mtu", parseInt(e.target.value) || 1400)} />
        </div>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Max Message Size</span>
          <input style={inputStyle} type="number" value={serverConfig.max_message_size ?? 10485760} onChange={(e) => props.onUpdateServer("max_message_size", parseInt(e.target.value) || 10485760)} />
        </div>
      </div>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>Keepalive (seconds)</span>
        <input style={inputStyle} type="number" value={serverConfig.tunnel_timeout || 30} onChange={(e) => props.onUpdateServer("tunnel_timeout", parseInt(e.target.value) || 30)}
          placeholder="30" />
      </div>
    </>
  );

  const renderTLS = () => (
    <>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>Verify Mode</span>
        <select style={selectStyle} value={serverConfig.tls?.verify_mode || "verify"} onChange={(e) => props.onNestServer("tls", "verify_mode", e.target.value)}>
          <option value="verify">verify</option>
          <option value="insecure">insecure</option>
          <option value="none">none</option>
        </select>
      </div>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>Server Name (SNI)</span>
        <input style={inputStyle} value={serverConfig.tls?.server_name || ""} onChange={(e) => props.onNestServer("tls", "server_name", e.target.value)} placeholder="Override SNI" />
      </div>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>CA File</span>
        <input style={inputStyle} value={serverConfig.tls?.ca_file || ""} onChange={(e) => props.onNestServer("tls", "ca_file", e.target.value)} placeholder="/path/to/ca.pem" />
      </div>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>Custom SNI Domains</span>
        <input style={inputStyle} value={(serverConfig.tls?.sni || []).join(", ")} onChange={(e) => props.onNestServer("tls", "sni", e.target.value.split(",").map(s => s.trim()).filter(Boolean))} placeholder="domain.com, example.org" />
      </div>
    </>
  );

  const renderRouting = () => (
    <>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Default Route</span>
          <select style={selectStyle} value={serverConfig.routing?.default_route || "server"} onChange={(e) => props.onNestServer("routing", "default_route", e.target.value)}>
            <option value="server">Server (VPN)</option>
            <option value="direct">Direct (bypass)</option>
          </select>
        </div>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Source TTL (hours)</span>
          <input style={inputStyle} type="number" value={serverConfig.routing?.source_ttl_hours ?? 24} onChange={(e) => props.onNestServer("routing", "source_ttl_hours", parseInt(e.target.value) || 24)} />
        </div>
      </div>

      <div style={subBlockStyle}>
        <div style={subTitleStyle}>Include Rules</div>
        <ChipList
          items={serverConfig.routing?.include_ranges || []}
          label="CIDR"
          placeholder="10.0.0.0/8"
          onAdd={(v) => props.onAddRoutingString("include_ranges", v)}
          onRemove={(i) => props.onRemoveRoutingString("include_ranges", i)}
        />
        <ChipList
          items={serverConfig.routing?.include_ips || []}
          label="IPs"
          placeholder="1.1.1.1"
          onAdd={(v) => props.onAddRoutingString("include_ips", v)}
          onRemove={(i) => props.onRemoveRoutingString("include_ips", i)}
        />
        <ChipList
          items={serverConfig.routing?.include_domains || []}
          label="Domains"
          placeholder="example.com"
          onAdd={(v) => props.onAddRoutingString("include_domains", v)}
          onRemove={(i) => props.onRemoveRoutingString("include_domains", i)}
        />
      </div>

      <div style={subBlockStyle}>
        <div style={subTitleStyle}>Exclude Rules</div>
        <ChipList
          items={serverConfig.routing?.exclude_ranges || []}
          label="CIDR"
          placeholder="192.168.0.0/16"
          onAdd={(v) => props.onAddRoutingString("exclude_ranges", v)}
          onRemove={(i) => props.onRemoveRoutingString("exclude_ranges", i)}
        />
        <ChipList
          items={serverConfig.routing?.exclude_ips || []}
          label="IPs"
          placeholder="10.0.0.1"
          onAdd={(v) => props.onAddRoutingString("exclude_ips", v)}
          onRemove={(i) => props.onRemoveRoutingString("exclude_ips", i)}
        />
        <ChipList
          items={serverConfig.routing?.exclude_domains || []}
          label="Domains"
          placeholder="local"
          onAdd={(v) => props.onAddRoutingString("exclude_domains", v)}
          onRemove={(i) => props.onRemoveRoutingString("exclude_domains", i)}
        />
      </div>

      <div style={subBlockStyle}>
        <div style={subTitleStyle}>GeoIP / GeoSite Databases</div>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>GeoIP Path</span>
          <input style={chipStyle} placeholder="/etc/geoip.dat" value={serverConfig.routing?.geoip_path || ""} onChange={(e) => props.onNestServer("routing", "geoip_path", e.target.value)} />
        </div>
        <div style={{ ...fieldStyle(), marginTop: 4 }}>
          <span style={labelStyle()}>GeoIP URL</span>
          <input style={chipStyle} placeholder="https://example.com/geoip.dat" value={serverConfig.routing?.geoip_url || ""} onChange={(e) => props.onNestServer("routing", "geoip_url", e.target.value)} />
        </div>
        <div style={{ ...fieldStyle(), marginTop: 4 }}>
          <span style={labelStyle()}>GeoSite Path</span>
          <input style={chipStyle} placeholder="/etc/geosite.dat" value={serverConfig.routing?.geosite_path || ""} onChange={(e) => props.onNestServer("routing", "geosite_path", e.target.value)} />
        </div>
        <div style={{ ...fieldStyle(), marginTop: 4 }}>
          <span style={labelStyle()}>GeoSite URL</span>
          <input style={chipStyle} placeholder="https://example.com/geosite.dat" value={serverConfig.routing?.geosite_url || ""} onChange={(e) => props.onNestServer("routing", "geosite_url", e.target.value)} />
        </div>
      </div>

      <div style={subBlockStyle}>
        <div style={subTitleStyle}>Include Sources</div>
        {(serverConfig.routing?.include_sources || []).map((src, i) => (
          <SrcRow key={i} src={src} idx={i} list="include_sources" onUpdate={props.onUpdateSourceRule} onRemove={props.onRemoveSourceRule} />
        ))}
        <button style={{ ...btnSmall, marginTop: 2 }} onClick={() => props.onAddSourceRule("include_sources")}>+ Add Source</button>
      </div>

      <div style={subBlockStyle}>
        <div style={subTitleStyle}>Exclude Sources</div>
        {(serverConfig.routing?.exclude_sources || []).map((src, i) => (
          <SrcRow key={i} src={src} idx={i} list="exclude_sources" onUpdate={props.onUpdateSourceRule} onRemove={props.onRemoveSourceRule} />
        ))}
        <button style={{ ...btnSmall, marginTop: 2 }} onClick={() => props.onAddSourceRule("exclude_sources")}>+ Add Source</button>
      </div>

      <div style={{ display: "flex", gap: 6 }}>
        <button style={{ ...btnSmall, color: colors.info, borderColor: colors.info }} onClick={props.onRefreshSources}>Refresh Sources</button>
      </div>

      <div style={subBlockStyle}>
        <div style={subTitleStyle}>DNS Cache</div>
        <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: colors.text, cursor: "pointer", marginBottom: 4 }}>
          <input type="checkbox" checked={serverConfig.routing?.dns_cache?.enabled ?? false} onChange={(e) => props.onNestServer2("routing", "dns_cache", "enabled", e.target.checked)} />
          DNS Cache (IP→domain tracking)
        </label>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>TTL (seconds)</span>
          <input style={chipStyle} type="number" value={serverConfig.routing?.dns_cache?.ttl ?? 60} onChange={(e) => props.onNestServer2("routing", "dns_cache", "ttl", parseInt(e.target.value) || 60)} />
        </div>
      </div>
    </>
  );

  const renderAdvanced = () => (
    <>
      <div style={subTitleStyle}>Tuning</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>MTU</span>
          <input style={inputStyle} type="number" value={serverConfig.mtu ?? 1400} onChange={(e) => props.onUpdateServer("mtu", parseInt(e.target.value) || 1400)} />
        </div>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Max Message Size</span>
          <input style={inputStyle} type="number" value={serverConfig.max_message_size ?? 10485760} onChange={(e) => props.onUpdateServer("max_message_size", parseInt(e.target.value) || 10485760)} />
        </div>
        <div style={fieldStyle()}>
          {/* @sk-task kvn-web-config-update#T2.2: proxy_connections tuning field (AC-005, AC-006) */}
          <span style={labelStyle()}>Proxy Connections</span>
          <input style={inputStyle} type="number" value={serverConfig.proxy_connections || 10} onChange={(e) => props.onUpdateServer("proxy_connections", parseInt(e.target.value) || 10)} />
        </div>
      </div>

      <div style={subTitleStyle}>Features</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
        {[
          { key: "ipv6", label: "IPv6 Support", desc: "Allow IPv6 traffic" },
          { key: "auto_reconnect", label: "Auto Reconnect", desc: "Exponential backoff" },
          { key: "multiplex", label: "Multiplex", desc: "Connection multiplexing" },
          { key: "obfuscation", label: "Obfuscation", desc: "Anti-DPI" },
          { key: "crypto", label: "AES-256-GCM", desc: "Per-session encryption" },
          { key: "kill_switch", label: "Kill Switch", desc: "Block on disconnect" },
        ].map(({ key, label, desc }) => {
          const checked = key === "obfuscation" ? (serverConfig.obfuscation?.enabled ?? false)
            : key === "crypto" ? (serverConfig.crypto?.enabled ?? false)
            : key === "kill_switch" ? (serverConfig.kill_switch?.enabled ?? false)
            : !!(serverConfig as any)[key];
          const onChange = key === "obfuscation" ? (v: boolean) => props.onNestServer("obfuscation", "enabled", v)
            : key === "crypto" ? (v: boolean) => props.onNestServer("crypto", "enabled", v)
            : key === "kill_switch" ? (v: boolean) => props.onNestServer("kill_switch", "enabled", v)
            : (v: boolean) => props.onUpdateServer(key, v);
          return (
            <label key={key} style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 10px", background: colors.cardBg, borderRadius: borderRadius.md, border: `1px solid #1a1a2a`, cursor: "pointer" }}>
              <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} style={{ accentColor: colors.accent }} />
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 12, fontWeight: 600, color: colors.text }}>{label}</div>
                  <div style={{ fontSize: 10, color: colors.textDim }}>{desc}</div>
                </div>
              </label>
          );
        })}
      </div>

      {serverConfig.obfuscation?.enabled && (
        <div style={{ ...subBlockStyle, borderLeft: `2px solid ${colors.accent}44`, marginTop: 2 }}>
          <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: colors.text, cursor: "pointer", marginBottom: 2 }}>
            <input type="checkbox" checked={serverConfig.obfuscation?.utls?.enabled ?? false} onChange={(e) => props.onNestServer2("obfuscation", "utls", "enabled", e.target.checked)} />
            uTLS (Chrome JA3 fingerprint)
          </label>
          {serverConfig.obfuscation?.utls?.enabled && (
            <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: colors.text, cursor: "pointer", margin: "0 0 4px 20px" }}>
              <input type="checkbox" checked={serverConfig.obfuscation?.utls?.fallback ?? true} onChange={(e) => props.onNestServer2("obfuscation", "utls", "fallback", e.target.checked)} />
              uTLS Fallback (crypto/tls on error)
            </label>
          )}
          <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: colors.text, cursor: "pointer", marginBottom: 4 }}>
            <input type="checkbox" checked={serverConfig.obfuscation?.padding?.enabled ?? true} onChange={(e) => props.onNestServer2("obfuscation", "padding", "enabled", e.target.checked)} />
            WS Padding (fixed packet size)
          </label>
          {(serverConfig.obfuscation?.padding?.enabled ?? true) && (
            <div style={{ marginLeft: 20 }}>
              <div style={fieldStyle()}>
                <span style={labelStyle()}>Padding Size</span>
                <input style={chipStyle} type="number" value={serverConfig.obfuscation?.padding?.size ?? 512} onChange={(e) => props.onNestServer2("obfuscation", "padding", "size", parseInt(e.target.value) || 512)} />
              </div>
            </div>
          )}
        </div>
      )}

      <div style={subTitleStyle}>Backoff Timing</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Min Backoff (s)</span>
          <input style={inputStyle} type="number" value={serverConfig.reconnect?.min_backoff_sec ?? 1} onChange={(e) => props.onNestServer("reconnect", "min_backoff_sec", parseInt(e.target.value) || 1)} />
        </div>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Max Backoff (s)</span>
          <input style={inputStyle} type="number" value={serverConfig.reconnect?.max_backoff_sec ?? 30} onChange={(e) => props.onNestServer("reconnect", "max_backoff_sec", parseInt(e.target.value) || 30)} />
        </div>
      </div>

      <div style={subTitleStyle}>Encryption</div>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>AES Key (hex)</span>
        <input style={{ ...inputStyle, opacity: serverConfig.crypto?.enabled ? 1 : 0.5 }} placeholder="32-byte hex key (required if AES enabled)" disabled={!serverConfig.crypto?.enabled} value={serverConfig.crypto?.key || ""} onChange={(e) => props.onNestServer("crypto", "key", e.target.value)} />
      </div>
    </>
  );

  const renderGlobal = () => (
    <>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>Log Level</span>
        <select style={selectStyle} value={globalConfig.log?.level || "info"} onChange={(e) => props.onNestGlobal("log", "level", e.target.value)}>
          <option value="debug">debug</option>
          <option value="info">info</option>
          <option value="warn">warn</option>
          <option value="error">error</option>
        </select>
      </div>

      <div style={subTitleStyle}>Proxy</div>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>Proxy Listen</span>
        <input style={inputStyle} value={globalConfig.proxy_listen || "127.0.0.1:2310"} onChange={(e) => props.onUpdateGlobal("proxy_listen", e.target.value)} />
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Proxy Username</span>
          <input style={inputStyle} placeholder="(optional)" value={globalConfig.proxy_auth?.username || ""} onChange={(e) => props.onNestGlobal("proxy_auth", "username", e.target.value)} />
        </div>
        <div style={fieldStyle()}>
          <span style={labelStyle()}>Proxy Password</span>
          <input style={inputStyle} type="password" placeholder="(optional)" value={globalConfig.proxy_auth?.password || ""} onChange={(e) => props.onNestGlobal("proxy_auth", "password", e.target.value)} />
        </div>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6, marginTop: 4 }}>
        <label style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 10px", background: colors.cardBg, borderRadius: borderRadius.md, border: `1px solid #1a1a2a`, cursor: "pointer" }}>
          <input type="checkbox" checked={!!globalConfig.system_proxy} onChange={(e) => props.onUpdateGlobal("system_proxy", e.target.checked)} style={{ accentColor: colors.accent }} />
          <div><div style={{ fontSize: 12, fontWeight: 600, color: colors.text }}>System Proxy</div><div style={{ fontSize: 10, color: colors.textDim }}>Set OS proxy settings</div></div>
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 10px", background: colors.cardBg, borderRadius: borderRadius.md, border: `1px solid #1a1a2a`, cursor: "pointer" }}>
          <input type="checkbox" checked={!!globalConfig.transparent} onChange={(e) => props.onUpdateGlobal("transparent", e.target.checked)} style={{ accentColor: colors.accent }} />
          <div><div style={{ fontSize: 12, fontWeight: 600, color: colors.text }}>Transparent Proxy</div><div style={{ fontSize: 10, color: colors.textDim }}>iptables redirect (Linux)</div></div>
        </label>
      </div>

      <div style={subTitleStyle}>DNS</div>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>DNS Proxy Listen</span>
        <input style={inputStyle} value={globalConfig.dns_proxy?.listen || "127.0.0.54:53"} onChange={(e) => props.onNestGlobal("dns_proxy", "listen", e.target.value)} />
      </div>
      <div style={fieldStyle()}>
        <span style={labelStyle()}>DNS Upstreams (Trusted Resolvers)</span>
        {(globalConfig.dns_proxy?.upstreams || ["1.1.1.1:53", "8.8.8.8:53"]).map((up, i) => (
          <div key={i} style={{ display: "flex", gap: 6, marginBottom: 4, alignItems: "center" }}>
            <input style={{ ...chipStyle, flex: 1 }} value={up} onChange={(e) => {
              const ups = [...(globalConfig.dns_proxy?.upstreams || ["1.1.1.1:53", "8.8.8.8:53"])];
              ups[i] = e.target.value;
              props.onNestGlobal("dns_proxy", "upstreams", ups);
            }} />
            <button style={{ padding: "2px 8px", background: colors.errorBg, border: `1px solid ${colors.errorBorder}`, borderRadius: borderRadius.sm, color: colors.error, cursor: "pointer", fontSize: 13 }}
              onClick={() => {
                const ups = [...(globalConfig.dns_proxy?.upstreams || ["1.1.1.1:53", "8.8.8.8:53"])];
                ups.splice(i, 1);
                props.onNestGlobal("dns_proxy", "upstreams", ups);
              }}>−</button>
          </div>
        ))}
        <button style={btnSmall} onClick={() => {
          const ups = [...(globalConfig.dns_proxy?.upstreams || ["1.1.1.1:53", "8.8.8.8:53"]), ""];
          props.onNestGlobal("dns_proxy", "upstreams", ups);
        }}>+ Add Upstream</button>
      </div>
    </>
  );

  const tabContent = [renderGeneral, renderTLS, renderRouting, renderAdvanced, renderGlobal];

  return (
    <div style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }}>
      <div style={tabBarStyle}>
        {TABS.map((name, i) => (
          <div key={name} style={tabStyle(i === activeTab)} onClick={() => setActiveTab(i)}
            onMouseEnter={(e) => { if (i !== activeTab) e.currentTarget.style.background = colors.cardBg; }}
            onMouseLeave={(e) => { if (i !== activeTab) e.currentTarget.style.background = "none"; }}
          >
            {name}
          </div>
        ))}
      </div>
      <div style={tabContentStyle}>
        {tabContent[activeTab]()}
      </div>
    </div>
  );
}
