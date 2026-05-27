package logger

import (
	"strings"

	"github.com/fatih/color"
	"go.uber.org/zap/zapcore"
)

// GCPSeverityEncoder maps zap log levels to GCP Cloud Logging severity strings.
// See: https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity
func GCPSeverityEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch level {
	case zapcore.DebugLevel:
		enc.AppendString("DEBUG")
	case zapcore.InfoLevel:
		enc.AppendString("INFO")
	case zapcore.WarnLevel:
		enc.AppendString("WARNING")
	case zapcore.ErrorLevel:
		enc.AppendString("ERROR")
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		enc.AppendString("CRITICAL")
	case zapcore.InvalidLevel:
		enc.AppendString("DEFAULT")
	default:
		enc.AppendString("DEFAULT")
	}
}

// ColoredLevelEncoder colorizes log levels.
func ColoredLevelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch level {
	case zapcore.DebugLevel:
		enc.AppendString(color.HiWhiteString(level.CapitalString()))
	case zapcore.InfoLevel:
		enc.AppendString(color.HiCyanString(level.CapitalString()))
	case zapcore.WarnLevel:
		enc.AppendString(color.HiYellowString(level.CapitalString()))
	case zapcore.ErrorLevel, zapcore.DPanicLevel:
		enc.AppendString(color.HiRedString(level.CapitalString()))
	case zapcore.PanicLevel, zapcore.FatalLevel, zapcore.InvalidLevel:
		enc.AppendString(color.HiMagentaString(level.CapitalString()))
	}
}

// ColoredNameEncoder colorizes service names.
func ColoredNameEncoder(s string, enc zapcore.PrimitiveArrayEncoder) {
	if len(s) < 12 {
		s += strings.Repeat(" ", 12-len(s))
	}

	enc.AppendString(color.HiGreenString(s))
}
