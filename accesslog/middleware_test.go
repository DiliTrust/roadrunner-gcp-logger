package accesslog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, recorded := observer.New(zapcore.InfoLevel)
	return zap.New(core), recorded
}

func TestMiddleware_EmitsAccessLogWithTrace(t *testing.T) {
	log, recorded := newObservedLogger()

	handler := newHandler(log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/users?q=1", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Cloud-Trace-Context", "abcdef0123456789abcdef0123456789/123;o=1")
	req.Header.Set("User-Agent", "curl/8")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = res.Body.Close()

	entries := recorded.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Message != "http access log" {
		t.Errorf("message=%q", e.Message)
	}

	got := e.ContextMap()
	if got["method"] != "GET" {
		t.Errorf("method=%v", got["method"])
	}
	if got["status"] != int64(200) {
		t.Errorf("status=%v (%T)", got["status"], got["status"])
	}
	if !strings.HasSuffix(got["URI"].(string), "/api/users?q=1") {
		t.Errorf("URI=%v", got["URI"])
	}
	if got["user_agent"] != "curl/8" {
		t.Errorf("user_agent=%v", got["user_agent"])
	}
	if got["trace"] != "abcdef0123456789abcdef0123456789" {
		t.Errorf("trace=%v", got["trace"])
	}
	if got["span_id"] != "000000000000007b" {
		t.Errorf("span_id=%v", got["span_id"])
	}
	if got["trace_sampled"] != true {
		t.Errorf("trace_sampled=%v", got["trace_sampled"])
	}
}

func TestMiddleware_NoTraceHeaderOmitsTraceFields(t *testing.T) {
	log, recorded := newObservedLogger()

	handler := newHandler(log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = res.Body.Close()

	entries := recorded.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0].ContextMap()
	if _, has := got["trace"]; has {
		t.Errorf("trace field should be absent")
	}
	if _, has := got["span_id"]; has {
		t.Errorf("span_id field should be absent")
	}
	if _, has := got["trace_sampled"]; has {
		t.Errorf("trace_sampled field should be absent")
	}
	if got["status"] != int64(http.StatusTeapot) {
		t.Errorf("status=%v", got["status"])
	}
}
