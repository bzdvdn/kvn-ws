package config

// @sk-task foundation#T2.3: config loader with viper (AC-006, AC-007)

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/spf13/viper"
)

type LogConfig struct {
	Level string `mapstructure:"level"`
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
