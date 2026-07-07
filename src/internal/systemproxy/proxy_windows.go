//go:build windows

package systemproxy

import (
	"context"
	"fmt"
	"strings"
	"unsafe"

	"go.uber.org/zap"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	winRegPath             = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	interopSettingsChanged = 39
	interopRefresh         = 37
)

var (
	modWininet             = windows.NewLazySystemDLL("wininet.dll")
	procInternetSetOptionW = modWininet.NewProc("InternetSetOptionW")

	modWtsapi32           = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQueryUserToken = modWtsapi32.NewProc("WTSQueryUserToken")
)

// @sk-task system-proxy#T3.2: windows registry-based platform manager (AC-005)
type windowsManager struct {
	origEnable   *uint64
	origServer   string
	origOverride string
	userSID      string // "" = fallback to CURRENT_USER (LocalSystem)
}

func newPlatformManager() PlatformManager {
	return &windowsManager{}
}

// activeUserSID returns the SID string of the active console user.
// Returns "" if no interactive user is logged on or on error.
func activeUserSID() string {
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xFFFFFFFF {
		return ""
	}

	var token windows.Token
	r, _, _ := procWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&token)))
	if r == 0 {
		return ""
	}
	defer token.Close()

	var bufSize uint32
	windows.GetTokenInformation(token, windows.TokenUser, nil, 0, &bufSize)
	if bufSize == 0 {
		return ""
	}
	buf := make([]byte, bufSize)
	if err := windows.GetTokenInformation(token, windows.TokenUser, &buf[0], bufSize, &bufSize); err != nil {
		return ""
	}

	tokenUser := (*windows.Tokenuser)(unsafe.Pointer(&buf[0]))
	return tokenUser.User.Sid.String()
}

func (m *windowsManager) regRoot() (registry.Key, string) {
	if m.userSID != "" {
		return registry.USERS, m.userSID + `\` + winRegPath
	}
	return registry.CURRENT_USER, winRegPath
}

func (m *windowsManager) Set(ctx context.Context, logger *zap.Logger, addr string, noProxy string) error {
	// Try to target the active interactive user instead of LocalSystem
	if sid := activeUserSID(); sid != "" {
		m.userSID = sid
		logger.Debug("windows system proxy: targeting active user", zap.String("sid", sid))
	}

	root, path := m.regRoot()
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		logger.Warn("windows system proxy: cannot open registry", zap.Error(err))
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()

	// recovery: if orphaned proxy from a previous crash, clear it first
	curEnable, _, curEnableErr := k.GetIntegerValue("ProxyEnable")
	curServer, _, _ := k.GetStringValue("ProxyServer")
	if curEnableErr == nil && curEnable == 1 && curServer == addr {
		logger.Warn("windows system proxy: recovering orphaned proxy from crash",
			zap.String("orphaned", curServer),
		)
		_ = k.SetDWordValue("ProxyEnable", 0)
		_ = k.DeleteValue("ProxyServer")
		_ = k.DeleteValue("ProxyOverride")
		internetSetOption(interopSettingsChanged)
		internetSetOption(interopRefresh)
	}

	// save originals (after potential recovery)
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
	root, path := m.regRoot()
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
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
