package webui

import (
	"bytes"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// @sk-test webui-log-level-filter#T1.1: SSE gets all levels even when syslog is filtered (AC-003)
func TestUILogSplitChannels(t *testing.T) {
	var sse []LogEntry
	sseCore := &uiLogCore{pushLog: func(le LogEntry) { sse = append(sse, le) }}

	var buf bytes.Buffer
	atomicLevel := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	encoder := zapcore.NewJSONEncoder(zap.NewDevelopmentEncoderConfig())
	stdoutCore := zapcore.NewCore(encoder, zapcore.AddSync(&buf), atomicLevel)

	log := zap.New(zapcore.NewTee(sseCore, stdoutCore))
	defer log.Sync()

	log.Debug("dbg line")
	log.Info("info line")
	log.Warn("warn line")
	log.Error("error line")

	levels := []string{}
	for _, e := range sse {
		levels = append(levels, e.Level)
	}
	if len(sse) != 4 {
		t.Fatalf("SSE: expected 4 entries (all levels), got %d: %+v", len(sse), levels)
	}
	seen := map[string]bool{}
	for _, e := range sse {
		seen[e.Level] = true
	}
	for _, lvl := range []string{"debug", "info", "warn", "error"} {
		if !seen[lvl] {
			t.Errorf("SSE: missing level %s, got %v", lvl, levels)
		}
	}

	out := buf.String()
	if strings.Contains(out, "dbg line") {
		t.Fatalf("stdout/syslog: debug leaked through info filter: %s", out)
	}
	for _, want := range []string{"info line", "warn line", "error line"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout/syslog: missing %q", want)
		}
	}
}

// @sk-test webui-log-level-filter#T1.1: debug-level syslog keeps debug (AC-003)
func TestUILogSplitChannelsDebug(t *testing.T) {
	var sse []LogEntry
	sseCore := &uiLogCore{pushLog: func(le LogEntry) { sse = append(sse, le) }}

	var buf bytes.Buffer
	atomicLevel := zap.NewAtomicLevelAt(zapcore.DebugLevel)
	encoder := zapcore.NewJSONEncoder(zap.NewDevelopmentEncoderConfig())
	stdoutCore := zapcore.NewCore(encoder, zapcore.AddSync(&buf), atomicLevel)

	log := zap.New(zapcore.NewTee(sseCore, stdoutCore))
	defer log.Sync()

	log.Debug("dbg line")
	_ = log.Sync()

	if len(sse) != 1 || sse[0].Level != "debug" {
		t.Fatalf("SSE: expected 1 debug entry, got %+v", sse)
	}
	if !strings.Contains(buf.String(), "dbg line") {
		t.Fatalf("stdout/syslog: debug missing at debug level: %s", buf.String())
	}
}
