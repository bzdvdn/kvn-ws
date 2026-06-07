//go:build darwin

package systemproxy

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// @sk-task system-proxy#T3.1: macos platform manager using networksetup (AC-004)
type darwinManager struct {
	iface           string
	origWebEnabled  bool
	origWebServer   string
	origWebPort     string
	origSecureWebEnabled bool
	origSecureWebServer  string
	origSecureWebPort    string
}

func newPlatformManager() PlatformManager {
	return &darwinManager{}
}

func (m *darwinManager) Set(ctx context.Context, logger *zap.Logger, addr string, noProxy string) error {
	iface, err := activeNetworkService()
	if err != nil {
		logger.Warn("macOS system proxy: cannot determine active network service", zap.Error(err))
		return nil
	}
	m.iface = iface

	host, port, err := splitHostPort(addr)
	if err != nil {
		logger.Warn("macOS system proxy: invalid addr", zap.Error(err))
		return nil
	}

	// save original web proxy
	m.origWebEnabled, m.origWebServer, m.origWebPort = getProxySettings(iface, "-getwebproxy")

	// save original secure web proxy
	m.origSecureWebEnabled, m.origSecureWebServer, m.origSecureWebPort = getProxySettings(iface, "-getsecurewebproxy")

	// set web proxy
	if err := exec.CommandContext(ctx, "networksetup", "-setwebproxy", iface, host, port).Run(); err != nil {
		logger.Warn("macOS: -setwebproxy failed", zap.Error(err))
	}
	if err := exec.CommandContext(ctx, "networksetup", "-setwebproxystate", iface, "on").Run(); err != nil {
		logger.Warn("macOS: -setwebproxystate on failed", zap.Error(err))
	}

	// set secure web proxy
	if err := exec.CommandContext(ctx, "networksetup", "-setsecurewebproxy", iface, host, port).Run(); err != nil {
		logger.Warn("macOS: -setsecurewebproxy failed", zap.Error(err))
	}
	if err := exec.CommandContext(ctx, "networksetup", "-setsecurewebproxystate", iface, "on").Run(); err != nil {
		logger.Warn("macOS: -setsecurewebproxystate on failed", zap.Error(err))
	}

	logger.Info("macOS system proxy set",
		zap.String("interface", iface),
		zap.String("addr", addr),
	)
	return nil
}

func (m *darwinManager) Restore(ctx context.Context, logger *zap.Logger) error {
	if m.iface == "" {
		return nil
	}

	if m.origWebEnabled {
		_ = exec.CommandContext(ctx, "networksetup", "-setwebproxy", m.iface, m.origWebServer, m.origWebPort).Run()
		_ = exec.CommandContext(ctx, "networksetup", "-setwebproxystate", m.iface, "on").Run()
	} else {
		_ = exec.CommandContext(ctx, "networksetup", "-setwebproxystate", m.iface, "off").Run()
	}

	if m.origSecureWebEnabled {
		_ = exec.CommandContext(ctx, "networksetup", "-setsecurewebproxy", m.iface, m.origSecureWebServer, m.origSecureWebPort).Run()
		_ = exec.CommandContext(ctx, "networksetup", "-setsecurewebproxystate", m.iface, "on").Run()
	} else {
		_ = exec.CommandContext(ctx, "networksetup", "-setsecurewebproxystate", m.iface, "off").Run()
	}

	logger.Info("macOS system proxy restored", zap.String("interface", m.iface))
	return nil
}

// @sk-task system-proxy#T3.1: find first active network service (AC-004)
func activeNetworkService() (string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return "", fmt.Errorf("list network services: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		if strings.HasSuffix(line, "(Inactive)") {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("no active network service found")
}

// @sk-task system-proxy#T3.1: parse proxy settings via networksetup -getwebproxy/-getsecurewebproxy (AC-004)
func getProxySettings(iface, cmdFlag string) (enabled bool, server string, port string) {
	out, err := exec.Command("networksetup", cmdFlag, iface).Output()
	if err != nil {
		return false, "", ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Enabled:") {
			enabled = strings.TrimSpace(strings.TrimPrefix(line, "Enabled:")) == "Yes"
		} else if strings.HasPrefix(line, "Server:") {
			server = strings.TrimSpace(strings.TrimPrefix(line, "Server:"))
		} else if strings.HasPrefix(line, "Port:") {
			port = strings.TrimSpace(strings.TrimPrefix(line, "Port:"))
		}
	}
	return
}

func splitHostPort(addr string) (host, port string, err error) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return addr, "80", nil
	}
	return addr[:idx], addr[idx+1:], nil
}
