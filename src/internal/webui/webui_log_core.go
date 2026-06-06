package webui

import (
	"go.uber.org/zap/zapcore"
)

type uiLogCore struct {
	zapcore.Core
	pushLog func(LogEntry)
}

func (c *uiLogCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
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
	return c.Core.Write(entry, fields)
}

func (c *uiLogCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if entry.Level >= zapcore.DebugLevel {
		return ce.AddCore(entry, c)
	}
	return ce
}
