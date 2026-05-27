package accesslog

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// traceContext holds the trace-correlation values extracted from a request's
// headers. Empty fields mean the corresponding value was absent or malformed —
// callers should treat zero values as "not present" and skip emission rather
// than logging blanks.
type traceContext struct {
	traceID    string
	spanID     string
	sampled    bool
	hasSampled bool
}

// extractTraceContext inspects the request headers and returns the first
// trace context it can parse. Precedence: GCP-native X-Cloud-Trace-Context,
// then W3C traceparent. Returning the first match (rather than merging)
// keeps the semantics predictable when both headers happen to be present.
func extractTraceContext(r *http.Request) traceContext {
	if v := r.Header.Get("X-Cloud-Trace-Context"); v != "" {
		if tc, ok := parseCloudTraceContext(v); ok {
			return tc
		}
	}
	if v := r.Header.Get("traceparent"); v != "" {
		if tc, ok := parseTraceparent(v); ok {
			return tc
		}
	}
	return traceContext{}
}

// parseCloudTraceContext parses GCP's native trace header format:
//
//	TRACE_ID/SPAN_ID;o=SAMPLED
//
// TRACE_ID is a 32-character hex string. SPAN_ID is a decimal unsigned 64-bit
// integer — GCP's structured-log spanId field, however, expects a 16-character
// lowercase hex string, so we reformat. The optional `;o=N` suffix carries
// the trace-sampled flag (1 = sampled).
//
// Spec: https://cloud.google.com/trace/docs/setup#force-trace
func parseCloudTraceContext(v string) (traceContext, bool) {
	var tc traceContext

	// Split off the options suffix (`;o=...`) first so we can parse it
	// independently of the TRACE/SPAN portion.
	core, opts, hasOpts := strings.Cut(v, ";")

	traceID, spanPart, hasSlash := strings.Cut(core, "/")
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return tc, false
	}
	tc.traceID = traceID

	if hasSlash {
		spanPart = strings.TrimSpace(spanPart)
		if spanPart != "" {
			if n, err := strconv.ParseUint(spanPart, 10, 64); err == nil {
				tc.spanID = fmt.Sprintf("%016x", n)
			}
		}
	}

	if hasOpts {
		for _, kv := range strings.Split(opts, ";") {
			k, val, ok := strings.Cut(strings.TrimSpace(kv), "=")
			if !ok || k != "o" {
				continue
			}
			tc.hasSampled = true
			tc.sampled = strings.TrimSpace(val) == "1"
		}
	}

	return tc, true
}

// parseTraceparent parses a W3C traceparent header:
//
//	VERSION-TRACE_ID-PARENT_ID-FLAGS
//
// We only accept version `00` (the only version defined as of W3C Trace
// Context Level 2). TRACE_ID must be 32 hex chars, PARENT_ID 16 hex chars,
// and FLAGS 2 hex chars. Bit 0 of FLAGS is the sampled flag.
//
// Spec: https://www.w3.org/TR/trace-context/#traceparent-header
func parseTraceparent(v string) (traceContext, bool) {
	var tc traceContext

	parts := strings.Split(strings.TrimSpace(v), "-")
	if len(parts) != 4 {
		return tc, false
	}
	version, traceID, spanID, flagsHex := parts[0], parts[1], parts[2], parts[3]
	if version != "00" {
		return tc, false
	}
	if len(traceID) != 32 || !isAllHex(traceID) {
		return tc, false
	}
	if len(spanID) != 16 || !isAllHex(spanID) {
		return tc, false
	}
	if len(flagsHex) != 2 || !isAllHex(flagsHex) {
		return tc, false
	}

	tc.traceID = traceID
	tc.spanID = spanID

	flags, err := strconv.ParseUint(flagsHex, 16, 8)
	if err != nil {
		return tc, false
	}
	tc.hasSampled = true
	tc.sampled = (flags & 0x01) == 0x01

	return tc, true
}

func isAllHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
