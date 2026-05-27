package accesslog

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// newHandler builds the access-log http.Handler. Extracted from Plugin.Middleware
// to keep the plugin file focused on lifecycle / wiring.
func newHandler(log *zap.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		tc := extractTraceContext(r)

		rw := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rw, r)

		elapsed := time.Since(start).Milliseconds()
		status := rw.code
		if status == 0 {
			// Handler returned without calling WriteHeader or Write — net/http
			// treats this as 200 OK.
			status = http.StatusOK
		}

		// Field names deliberately match RoadRunner's built-in access-log
		// middleware (github.com/roadrunner-server/http v5, middleware/log.go)
		// so the parent package's foldHTTPRequest folds them into a GCP
		// `httpRequest` object unchanged. The trace/span_id/trace_sampled
		// fields are remapped by the encoder's remapTraceFields step.
		fields := make([]zap.Field, 0, 12)
		fields = append(fields,
			zap.Int("status", status),
			zap.String("method", r.Method),
			zap.String("URI", r.RequestURI),
			zap.String("URL", r.URL.String()),
			zap.String("remote_address", r.RemoteAddr),
			zap.Int64("read_bytes", r.ContentLength),
			zap.Int64("write_bytes", rw.written),
			zap.String("user_agent", r.UserAgent()),
			zap.String("referer", r.Referer()),
			zap.Int64("elapsed", elapsed),
		)
		if tc.traceID != "" {
			fields = append(fields, zap.String("trace", tc.traceID))
		}
		if tc.spanID != "" {
			fields = append(fields, zap.String("span_id", tc.spanID))
		}
		if tc.hasSampled {
			// remapTraceFields reads Integer (set by zap.Bool to 0 or 1) so
			// this round-trips correctly through the encoder.
			fields = append(fields, zap.Bool("trace_sampled", tc.sampled))
		}

		// Message string matches RoadRunner's `access_logs: true` entry so
		// the encoder's foldHTTPRequest path triggers without modification.
		log.Info("http access log", fields...)
	})
}

// responseRecorder is a small ResponseWriter wrapper that captures the
// response status code and the number of bytes written. It exposes an
// Unwrap() method so callers using http.NewResponseController (Go 1.20+)
// can reach the underlying writer for Flush/Hijack/SetWriteDeadline etc.
//
// We deliberately do NOT implement http.Flusher / http.Hijacker directly:
// doing so would falsely advertise those capabilities to handlers that
// type-assert against them, even when the wrapped writer lacks support.
// The Unwrap() approach defers to whatever the underlying writer actually
// implements.
type responseRecorder struct {
	http.ResponseWriter
	code    int
	written int64
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}
