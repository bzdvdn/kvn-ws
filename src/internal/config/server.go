package config

// @sk-task foundation#T2.3: server config struct (AC-007)
type ServerConfig struct {
	Listen  string       `mapstructure:"listen"`
	TLS     TLSCfg       `mapstructure:"tls"`
	Network NetworkCfg   `mapstructure:"network"`
	Session SessionCfg   `mapstructure:"session"`
	Auth    ServerAuth   `mapstructure:"auth"`
	Logging LogConfig    `mapstructure:"logging"`
}

type TLSCfg struct {
	Cert       string `mapstructure:"cert"`
	Key        string `mapstructure:"key"`
	MinVersion string `mapstructure:"min_version"`
}

type NetworkCfg struct {
	PoolIPv4 PoolCfg `mapstructure:"pool_ipv4"`
}

type PoolCfg struct {
	Subnet     string `mapstructure:"subnet"`
	Gateway    string `mapstructure:"gateway"`
	RangeStart string `mapstructure:"range_start"`
	RangeEnd   string `mapstructure:"range_end"`
}

type SessionCfg struct {
	MaxClients     int `mapstructure:"max_clients"`
	IdleTimeoutSec int `mapstructure:"idle_timeout_sec"`
}

type ServerAuth struct {
	Mode   string   `mapstructure:"mode"`
	Tokens []string `mapstructure:"tokens"`
}

func LoadServerConfig(path string) (*ServerConfig, error) {
	cfg := &ServerConfig{}
	if err := load(path, "KVN_SERVER", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
