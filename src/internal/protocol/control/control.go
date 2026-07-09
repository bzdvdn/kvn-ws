package control

import "time"

// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task production-hardening#T4.1: control frame types (AC-002)
const (
	DefaultPingInterval = 25 * time.Second
	DefaultPongTimeout  = 120 * time.Second
	DefaultPingLimit    = 3
)
