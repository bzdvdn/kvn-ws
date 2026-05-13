package config

// @sk-task foundation#T2.3: client config struct (AC-006)
type ClientConfig struct {
	Server        string    `mapstructure:"server"`
	Auth          AuthCfg   `mapstructure:"auth"`
	MTU           int       `mapstructure:"mtu"`
	IPv6          bool      `mapstructure:"ipv6"`
	AutoReconnect bool      `mapstructure:"auto_reconnect"`
	Log           LogConfig `mapstructure:"log"`
}

type AuthCfg struct {
	Token string `mapstructure:"token"`
}

func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg := &ClientConfig{}
	if err := load(path, "KVN_CLIENT", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
