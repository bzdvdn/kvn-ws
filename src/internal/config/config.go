package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/spf13/viper"
)

type LogConfig struct {
	Level string `json:"level" mapstructure:"level"`
}

// @sk-task production-hardening#T1.1: atomic config wrapper (AC-009)
type AtomicConfig[T any] struct {
	ptr atomic.Pointer[T]
}

func NewAtomicConfig[T any](cfg *T) *AtomicConfig[T] {
	a := &AtomicConfig[T]{}
	a.ptr.Store(cfg)
	return a
}

func (a *AtomicConfig[T]) Load() *T {
	return a.ptr.Load()
}

func (a *AtomicConfig[T]) Store(cfg *T) {
	a.ptr.Store(cfg)
}

// @sk-task foundation#T2.3: config loader with viper (AC-006, AC-007)
func load(path, prefix string, cfg interface{}) error {
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

// @sk-task production-readiness-gap#T1: check if secret key is set via environment (AC-001)
// @sk-task fix-critical-leaks#T4.3: prefix param instead of global (AC-012)
func secretFromEnv(prefix, key string) bool {
	envKey := prefix + "_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	return os.Getenv(envKey) != ""
}

// @sk-task production-readiness-gap#T1: warn when secrets are in config file (AC-001)
// @sk-task fix-critical-leaks#T4.3: prefix param instead of global (AC-012)
func warnSecretInFile(prefix string, keys []string) bool {
	anyInFile := false
	for _, key := range keys {
		if !secretFromEnv(prefix, key) {
			anyInFile = true
		}
	}
	return anyInFile
}

// @sk-task production-readiness-gap#T1: load tokens from env JSON var (AC-001)
func loadTokensFromEnvJSON(prefix string) ([]TokenCfg, bool) {
	envKey := prefix + "_AUTH_TOKENS_JSON"
	raw := os.Getenv(envKey)
	if raw == "" {
		return nil, false
	}
	var tokens []TokenCfg
	if err := json.Unmarshal([]byte(raw), &tokens); err != nil {
		return nil, false
	}
	for i := range tokens {
		if tokens[i].Name == "" {
			tokens[i].Name = fmt.Sprintf("token-%d", i)
		}
		if tokens[i].Secret == "" {
			tokens[i].Secret = tokens[i].Name
		}
	}
	return tokens, true
}
