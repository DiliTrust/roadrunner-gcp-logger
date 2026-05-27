package logger

import (
	"fmt"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// Message strings emitted by RoadRunner's HTTP plugin access-log middleware
// (github.com/roadrunner-server/http v5, middleware/log.go). Used to detect
// which entries need their flat HTTP fields folded into a GCP-recognized
// `httpRequest` object.
const (
	httpLogMessage       = "http log"
	httpAccessLogMessage = "http access log"

	// httpLoggerName is the logger name used by RR's HTTP plugin
	// (cfg.UnmarshalKey(http.PluginName, ...) -> log.NamedLogger("http")).
	// Entries from this logger are candidates for suppression when our
	// accesslog middleware is active. Our middleware emits via
	// NamedLogger("http_access") so its entries are never filtered.
	httpLoggerName = "http"

	// GCP Cloud Logging special field keys that link an entry to Cloud Trace.
	// https://cloud.google.com/logging/docs/structured-logging#special-payload-fields
	gcpTraceKey        = "logging.googleapis.com/trace"
	gcpSpanKey         = "logging.googleapis.com/spanId"
	gcpTraceSampledKey = "logging.googleapis.com/trace_sampled"
)

// gcpProject holds the GCP project ID used to wrap raw trace IDs into the
// `projects/<id>/traces/<trace>` form Cloud Logging expects. Package-level
// because the encoder is registered process-wide via zap.RegisterEncoder
// and cannot receive per-instance config. Set by Plugin.Init.
//
//nolint:gochecknoglobals // process-wide encoder state; zap encoders are registered globally.
var gcpProject atomic.Value // string

// accessLogReplaced is flipped to true by the accesslog middleware plugin's
// Init. When set, the encoder drops entries from the built-in HTTP access-log
// middleware ("http log" / "http access log" on the "http" named logger) to
// prevent duplicate Cloud Logging ingest.
//
//nolint:gochecknoglobals // process-wide encoder state; coupled to a singleton encoder registration.
var accessLogReplaced atomic.Bool

// emptyBufferPool supplies the zero-byte buffer the encoder returns when it
// drops an entry. zapcore.Core requires a non-nil *buffer.Buffer; a zero-byte
// one causes the Core's writer to receive no bytes.
//
//nolint:gochecknoglobals // buffer pool reuse; idiomatic for zap encoders (zap itself does the same).
var emptyBufferPool = buffer.NewPool()

// SetGCPProject configures the GCP project ID used to format the
// `logging.googleapis.com/trace` field. Safe to call from plugin Init.
// An empty value leaves trace IDs unwrapped (still indexed by Cloud Logging
// but not linked to a Cloud Trace span).
func SetGCPProject(project string) {
	gcpProject.Store(project)
}

// MarkAccessLogReplaced signals that a custom access-log middleware is active.
// The encoder will drop entries from RoadRunner's built-in HTTP access-log
// middleware so they aren't ingested twice.
func MarkAccessLogReplaced() {
	accessLogReplaced.Store(true)
}

// init registers "gcp" as a zap encoding so it can be selected via the
// standard `encoding:` config field, in addition to being applied
// automatically by the production mode. zap.RegisterEncoder is the
// documented API for adding custom encodings and must run before any
// zap.Config.Build() call — so an init() is the right hook.
//
//nolint:gochecknoinits // required by zap.RegisterEncoder contract; runs before BuildLogger.
func init() {
	_ = zap.RegisterEncoder(encodingGCP, func(cfg zapcore.EncoderConfig) (zapcore.Encoder, error) {
		return newGCPEncoder(cfg), nil
	})
}

// gcpEncoder wraps the JSON encoder to (1) translate trace/span_id/trace_sampled
// fields into GCP Cloud Logging's special keys, (2) fold RoadRunner HTTP
// access-log entries into the `httpRequest` structured field, and (3) drop
// duplicate built-in access-log entries when a custom access-log middleware
// is active.
type gcpEncoder struct {
	zapcore.Encoder
}

func newGCPEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &gcpEncoder{Encoder: zapcore.NewJSONEncoder(cfg)}
}

func (e *gcpEncoder) Clone() zapcore.Encoder {
	return &gcpEncoder{Encoder: e.Encoder.Clone()}
}

func (e *gcpEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	if accessLogReplaced.Load() &&
		entry.LoggerName == httpLoggerName &&
		(entry.Message == httpLogMessage || entry.Message == httpAccessLogMessage) {
		// Drop: our accesslog middleware emits the canonical entry instead.
		return emptyBufferPool.Get(), nil
	}

	fields = remapTraceFields(fields)
	if entry.Message == httpLogMessage || entry.Message == httpAccessLogMessage {
		fields = foldHTTPRequest(fields)
	}
	return e.Encoder.EncodeEntry(entry, fields)
}

// remapTraceFields rewrites zap fields named `trace`, `span_id`, and
// `trace_sampled` into GCP Cloud Logging's special LogEntry keys.
//
// `trace` is wrapped as `projects/<project>/traces/<trace-id>` unless the
// caller already provided the wrapped form (idempotent — supports apps that
// pre-format, e.g. OTel exporters).
func remapTraceFields(fields []zapcore.Field) []zapcore.Field {
	hasTrace := false
	for _, f := range fields {
		if f.Key == "trace" || f.Key == "span_id" || f.Key == "trace_sampled" {
			hasTrace = true
			break
		}
	}
	if !hasTrace {
		return fields
	}

	out := make([]zapcore.Field, 0, len(fields))
	for _, f := range fields {
		switch f.Key {
		case "trace":
			v := f.String
			if v == "" {
				continue
			}
			if !strings.HasPrefix(v, "projects/") {
				if proj, _ := gcpProject.Load().(string); proj != "" {
					v = "projects/" + proj + "/traces/" + v
				}
			}
			out = append(out, zap.String(gcpTraceKey, v))
		case "span_id":
			if f.String == "" {
				continue
			}
			out = append(out, zap.String(gcpSpanKey, f.String))
		case "trace_sampled":
			out = append(out, zap.Bool(gcpTraceSampledKey, f.Integer == 1))
		default:
			out = append(out, f)
		}
	}
	return out
}

// foldHTTPRequest extracts the RoadRunner HTTP plugin access-log fields and
// folds them into a single `httpRequest` field whose shape matches GCP's
// LogEntry.httpRequest schema.
//
// See: https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#HttpRequest
func foldHTTPRequest(fields []zapcore.Field) []zapcore.Field {
	req := make(map[string]any, 10)
	out := make([]zapcore.Field, 0, len(fields)+1)

	var uri, url string

	for _, f := range fields {
		switch f.Key {
		case "method":
			req["requestMethod"] = f.String
		case "URI":
			uri = f.String
		case "URL":
			url = f.String
		case "status":
			req["status"] = f.Integer
		case "write_bytes":
			req["responseSize"] = fmt.Sprintf("%d", f.Integer)
		case "read_bytes":
			req["requestSize"] = fmt.Sprintf("%d", f.Integer)
		case "user_agent":
			req["userAgent"] = f.String
		case "referer":
			req["referer"] = f.String
		case "remote_address":
			req["remoteIp"] = f.String
		case "elapsed":
			req["latency"] = fmt.Sprintf("%.3fs", float64(f.Integer)/1000.0)
		case "start", "request_time", "time_local":
			// Redundant with the entry-level `time` field.
		default:
			out = append(out, f)
		}
	}

	if uri != "" {
		req["requestUrl"] = uri
	} else if url != "" {
		req["requestUrl"] = url
	}

	if len(req) == 0 {
		return fields
	}
	return append(out, zap.Any("httpRequest", req))
}
