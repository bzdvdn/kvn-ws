package config

// @sk-task foundation#T2.3: config loader with viper (AC-006, AC-007)

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type LogConfig struct {
	Level string `mapstructure:"level"`
}

// @sk-task foundation#T2.3: config loader with viper (AC-006, AC-007)
func load(path string, prefix string, cfg interface{}) error {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix(prefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config %s: %w", path, err)
	}
	if err := v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("unmarshal config %s: %w", path, err)
	}
	return nil
}
