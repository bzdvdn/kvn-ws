package webui

import (
	"go.uber.org/zap/zapcore"
)

// @sk-task kvn-web-redesign#T1.2: forward client logs to the web UI via SSE (AC-013)
// uiLogCore forwards every log entry (all levels) to the web UI log stream.
// It never writes to stdout/syslog — the filtered sink is the core it is
// composed with via zapcore.NewTee.
type uiLogCore struct {
	pushLog func(LogEntry)
}

func (c *uiLogCore) Enabled(level zapcore.Level) bool {
	return true
}

func (c *uiLogCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry { //nolint:gocritic // matches zapcore.Core interface
	return ce.AddCore(entry, c)
}

func (c *uiLogCore) Write(entry zapcore.Entry, fields []zapcore.Field) error { //nolint:gocritic // matches zapcore.Core interface
	var action int
	var ip string
	for _, f := range fields {
		switch f.Key {
		case "action":
			if f.Type == zapcore.Int64Type {
				action = int(f.Integer)
			}
		case "ip":
			if f.Type == zapcore.StringType {
				ip = f.String
			}
		}
	}
	le := LogEntry{
		Line:   entry.Message,
		Level:  entry.Level.String(),
		TS:     entry.Time.Format("2006-01-02T15:04:05.000Z0700"),
		Action: action,
		IP:     ip,
	}
	c.pushLog(le)
	return nil
}

func (c *uiLogCore) Sync() error {
	return nil
}

func (c *uiLogCore) With(fields []zapcore.Field) zapcore.Core {
	return c
}
