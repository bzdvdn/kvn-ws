// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task production-hardening#T4.1: control frame types (AC-002)
package control

import "time"

const (
	DefaultPingInterval = 30 * time.Second
	DefaultPongTimeout  = 30 * time.Second
	DefaultPingLimit    = 3
)
