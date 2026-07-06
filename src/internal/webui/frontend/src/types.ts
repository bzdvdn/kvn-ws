export type Status = "disconnected" | "connecting" | "connected" | "error";

export interface SourceRule {
  geoip?: string;
  geosite?: string;
  cidr?: string;
  url?: string;
}

export interface ClientConfig {
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
  tunnel_timeout?: number;
  proxy_listen?: string;
  // @sk-task kvn-web-config-update#T1.1: proxy_connections field (AC-005, AC-006)
  proxy_connections?: number;
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
    geoip_path?: string;
    geoip_url?: string;
    geosite_path?: string;
    geosite_url?: string;
    source_ttl_hours?: number;
    include_sources?: SourceRule[];
    exclude_sources?: SourceRule[];
    dns_cache?: { enabled?: boolean; ttl?: number };
  };
  reconnect?: { min_backoff_sec?: number; max_backoff_sec?: number };
  system_proxy?: boolean;
  transparent?: boolean;
  dns_proxy?: { listen?: string; upstream?: string; upstreams?: string[] };
}

export interface ServerEntry {
  name: string;
  server?: string;
  auth?: { token?: string };
  transport?: string;
  [key: string]: any;
}

export interface ServersResponse {
  active_server: string;
  servers: ServerEntry[];
}

export interface LogEntry {
  line: string;
  level: string;
  action?: number;
  ip?: string;
  ts?: string;
}

export interface MetricSnapshot {
  tx_bytes: number;
  rx_bytes: number;
  latency_ms: number;
  uptime_s: number;
  tx_speed: number;
  rx_speed: number;
  reconnects: number;
}

export const ACTION_MAP: Record<string, string> = {
  "0": "none",
  "1": "server",
  "2": "direct",
};
