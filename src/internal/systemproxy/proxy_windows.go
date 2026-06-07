//go:build windows

package systemproxy

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	winRegPath          = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	interopSettingsChanged = 39
	interopRefresh         = 37
)

var (
	modWininet             = windows.NewLazySystemDLL("wininet.dll")
	procInternetSetOptionW = modWininet.NewProc("InternetSetOptionW")
)

// @sk-task system-proxy#T3.2: windows registry-based platform manager (AC-005)
type windowsManager struct {
	origEnable   *uint64
	origServer   string
	origOverride string
}

func newPlatformManager() PlatformManager {
	return &windowsManager{}
}

func (m *windowsManager) Set(ctx context.Context, logger *zap.Logger, addr string, noProxy string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, winRegPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		logger.Warn("windows system proxy: cannot open registry", zap.Error(err))
		return nil
	}
	defer k.Close()

	// save originals
	enableVal, _, err := k.GetIntegerValue("ProxyEnable")
	if err == nil {
		m.origEnable = &enableVal
	}
	m.origServer, _, _ = k.GetStringValue("ProxyServer")
	m.origOverride, _, _ = k.GetStringValue("ProxyOverride")

	// set new values
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("set ProxyEnable: %w", err)
	}
	if err := k.SetStringValue("ProxyServer", addr); err != nil {
		return fmt.Errorf("set ProxyServer: %w", err)
	}
	if noProxy != "" {
		if err := k.SetStringValue("ProxyOverride", noProxyToWindowsOverride(noProxy)); err != nil {
			logger.Warn("windows system proxy: set ProxyOverride failed", zap.Error(err))
		}
	}

	// notify all applications that proxy settings changed
	internetSetOption(interopSettingsChanged)
	internetSetOption(interopRefresh)

	logger.Info("windows system proxy set",
		zap.String("addr", addr),
	)
	return nil
}

func (m *windowsManager) Restore(ctx context.Context, logger *zap.Logger) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, winRegPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		logger.Warn("windows system proxy restore: cannot open registry", zap.Error(err))
		return nil
	}
	defer k.Close()

	if m.origEnable != nil {
		_ = k.SetDWordValue("ProxyEnable", uint32(*m.origEnable))
	} else {
		_ = k.DeleteValue("ProxyEnable")
	}

	if m.origServer != "" {
		_ = k.SetStringValue("ProxyServer", m.origServer)
	} else {
		_ = k.DeleteValue("ProxyServer")
	}

	if m.origOverride != "" {
		_ = k.SetStringValue("ProxyOverride", m.origOverride)
	} else {
		_ = k.DeleteValue("ProxyOverride")
	}

	internetSetOption(interopSettingsChanged)
	internetSetOption(interopRefresh)

	logger.Info("windows system proxy restored")
	return nil
}

func internetSetOption(option uint32) {
	_, _, _ = procInternetSetOptionW.Call(0, uintptr(option), 0, 0)
}

// @sk-task system-proxy#T3.2: convert NO_PROXY comma-separated to Windows semicolon-separated (AC-005)
func noProxyToWindowsOverride(noProxy string) string {
	if noProxy == "" {
		return ""
	}
	parts := strings.Split(noProxy, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return strings.Join(parts, ";")
}
