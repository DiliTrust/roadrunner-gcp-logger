package logger

import (
	"encoding/json"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const testMsg = "hi"

func encoderForTest() zapcore.Encoder {
	return newGCPEncoder(zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "severity",
		NameKey:        encKeyLogger,
		MessageKey:     encKeyMessage,
		StacktraceKey:  "stack_trace",
		LineEnding:     "\n",
		EncodeLevel:    GCPSeverityEncoder,
		EncodeTime:     gcpRFC3339NanoTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})
}

func encodeOne(t *testing.T, enc zapcore.Encoder, entry zapcore.Entry, fields []zapcore.Field) (string, map[string]any) {
	t.Helper()
	buf, err := enc.EncodeEntry(entry, fields)
	if err != nil {
		t.Fatalf("EncodeEntry: %v", err)
	}
	raw := buf.String()
	if raw == "" {
		return "", nil
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimRight(raw, "\n")), &got); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return raw, got
}

func TestRemapTraceFields_WrapsWithProject(t *testing.T) {
	t.Cleanup(func() { gcpProject.Store("") })
	SetGCPProject("my-proj")

	_, got := encodeOne(t, encoderForTest(),
		zapcore.Entry{Level: zapcore.InfoLevel, Message: testMsg},
		[]zapcore.Field{zap.String("trace", "abc123")},
	)
	if got["logging.googleapis.com/trace"] != "projects/my-proj/traces/abc123" {
		t.Errorf("trace = %v, want projects/my-proj/traces/abc123", got["logging.googleapis.com/trace"])
	}
	if _, hasOld := got["trace"]; hasOld {
		t.Errorf("raw `trace` field should have been removed")
	}
}

func TestRemapTraceFields_IdempotentWhenPrewrapped(t *testing.T) {
	t.Cleanup(func() { gcpProject.Store("") })
	SetGCPProject("my-proj")

	_, got := encodeOne(t, encoderForTest(),
		zapcore.Entry{Level: zapcore.InfoLevel, Message: testMsg},
		[]zapcore.Field{zap.String("trace", "projects/other/traces/xyz")},
	)
	if got["logging.googleapis.com/trace"] != "projects/other/traces/xyz" {
		t.Errorf("trace = %v, want unmodified", got["logging.googleapis.com/trace"])
	}
}

func TestRemapTraceFields_NoProjectLeavesUnwrapped(t *testing.T) {
	t.Cleanup(func() { gcpProject.Store("") })
	SetGCPProject("")

	_, got := encodeOne(t, encoderForTest(),
		zapcore.Entry{Level: zapcore.InfoLevel, Message: testMsg},
		[]zapcore.Field{zap.String("trace", "abc123")},
	)
	if got["logging.googleapis.com/trace"] != "abc123" {
		t.Errorf("trace = %v, want abc123", got["logging.googleapis.com/trace"])
	}
}

func TestRemapTraceFields_SpanAndSampled(t *testing.T) {
	_, got := encodeOne(t, encoderForTest(),
		zapcore.Entry{Level: zapcore.InfoLevel, Message: testMsg},
		[]zapcore.Field{
			zap.String("span_id", "00f067aa0ba902b7"),
			zap.Bool("trace_sampled", true),
		},
	)
	if got["logging.googleapis.com/spanId"] != "00f067aa0ba902b7" {
		t.Errorf("spanId = %v", got["logging.googleapis.com/spanId"])
	}
	if got["logging.googleapis.com/trace_sampled"] != true {
		t.Errorf("trace_sampled = %v", got["logging.googleapis.com/trace_sampled"])
	}
}

func TestEncodeEntry_DropsBuiltInAccessLogWhenReplaced(t *testing.T) {
	t.Cleanup(func() { accessLogReplaced.Store(false) })
	MarkAccessLogReplaced()

	raw, _ := encodeOne(t, encoderForTest(),
		zapcore.Entry{
			Level:      zapcore.InfoLevel,
			LoggerName: httpLoggerName,
			Message:    httpLogMessage,
		},
		[]zapcore.Field{zap.String("method", "GET")},
	)
	if raw != "" {
		t.Errorf("expected dropped entry, got %q", raw)
	}
}

func TestEncodeEntry_KeepsBuiltInAccessLogWhenNotReplaced(t *testing.T) {
	// flag defaults to false; the entry should pass through and be folded.
	_, got := encodeOne(t, encoderForTest(),
		zapcore.Entry{
			Level:      zapcore.InfoLevel,
			LoggerName: httpLoggerName,
			Message:    httpLogMessage,
		},
		[]zapcore.Field{zap.String("method", "GET")},
	)
	if got == nil {
		t.Fatal("entry was dropped")
	}
	req, ok := got["httpRequest"].(map[string]any)
	if !ok {
		t.Fatalf("httpRequest missing: %v", got)
	}
	if req["requestMethod"] != "GET" {
		t.Errorf("requestMethod = %v", req["requestMethod"])
	}
}

func TestEncodeEntry_DoesNotDropMiddlewareAccessLog(t *testing.T) {
	t.Cleanup(func() { accessLogReplaced.Store(false) })
	MarkAccessLogReplaced()

	_, got := encodeOne(t, encoderForTest(),
		zapcore.Entry{
			Level:      zapcore.InfoLevel,
			LoggerName: "http_access",
			Message:    httpAccessLogMessage,
		},
		[]zapcore.Field{zap.String("method", "POST")},
	)
	if got == nil {
		t.Fatal("our middleware's entry was incorrectly dropped")
	}
}
