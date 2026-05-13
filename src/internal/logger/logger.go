package logger

// @sk-task foundation#T3.1: zap logger with JSON output (AC-008)

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// @sk-task production-hardening#T2.3: structured audit logger (AC-010)
func Audit(logger *zap.Logger, level zapcore.Level, msg string, fields ...zap.Field) {
	ce := logger.Check(level, msg)
	if ce == nil {
		return
	}
	ce.Write(fields...)
}

// @sk-task foundation#T3.1: zap logger with JSON output (AC-008)
func New(level string) (*zap.Logger, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(lvl),
		Encoding:         "json",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
	}

	return cfg.Build()
}
