package webui

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

func (s *Server) cfgPath() string {
	return filepath.Join(s.configDir, "config.yaml")
}

// legacyV2CfgPath is read once during migration to the unified config.
func (s *Server) legacyV2CfgPath() string {
	return filepath.Join(s.configDir, "v2", "config.yaml")
}

func (s *Server) loadWebUIConfig() (*config.WebUIConfig, error) {
	root := s.cfgPath()
	if _, err := os.Stat(root); err == nil {
		cfg, err := config.LoadWebUIConfig(root)
		if err != nil {
			return nil, err
		}
		if hasV2Server(cfg) {
			filtered := *cfg
			filtered.Servers = nil
			for i := range cfg.Servers {
				entry := &cfg.Servers[i]
				if entry.Protocol == "v2" {
					filtered.Servers = append(filtered.Servers, *entry)
				}
			}
			if len(filtered.Servers) != len(cfg.Servers) {
				if original, readErr := os.ReadFile(root); readErr == nil { // #nosec G304 -- root is the trusted config path
					if _, backupErr := os.Stat(root + ".v1.bak"); errors.Is(backupErr, os.ErrNotExist) {
						_ = os.WriteFile(root+".v1.bak", original, 0o600) // #nosec G703 -- root is the trusted config path
					}
				}
				if err := config.SaveWebUIConfig(root, &filtered); err != nil {
					return nil, err
				}
			}
			return &filtered, nil
		}
		// A pre-2.0 v1 config is preserved but is not shown in the v2 UI.
		if err := os.Rename(root, root+".v1.bak"); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if _, err := os.Stat(s.legacyV2CfgPath()); err == nil {
		cfg, err := config.LoadWebUIConfigOptional(s.legacyV2CfgPath())
		if err != nil {
			return nil, err
		}
		if len(cfg.Servers) > 0 {
			if err := config.SaveWebUIConfig(root, cfg); err != nil {
				return nil, err
			}
		}
		return cfg, nil
	}
	return &config.WebUIConfig{}, nil
}

func hasV2Server(cfg *config.WebUIConfig) bool {
	for i := range cfg.Servers {
		if cfg.Servers[i].Protocol == "v2" {
			return true
		}
	}
	return false
}

func (s *Server) saveWebUIConfig(cfg *config.WebUIConfig) error {
	return config.SaveWebUIConfig(s.cfgPath(), cfg)
}

func (s *Server) applyDefaults(cfg *config.ClientConfig) {
	if cfg.Mode == "" {
		cfg.Mode = "tun"
	}
	if cfg.ClientID == "" {
		cfg.ClientID = config.NewClientID()
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1200
	}
	config.SetClientDefaults(cfg)
	if cfg.UTLS == nil {
		cfg.UTLS = &config.UTLSConfig{}
	}
	cfg.UTLS.ApplyDefaults()
	if cfg.Stealth == nil {
		cfg.Stealth = &config.V2StealthConfig{}
	}
	cfg.Stealth.ApplyDefaults()
	cfg.Protocol = "v2"
}
