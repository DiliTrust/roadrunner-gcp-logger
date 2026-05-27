// Package accesslog provides a RoadRunner HTTP middleware that emits per-request
// access logs enriched with GCP trace correlation fields. It is intended to
// replace RoadRunner's built-in access-log middleware when running on Google
// Cloud Platform; entries emitted by this middleware are recognized by the
// parent logger plugin's GCP encoder and folded into Cloud Logging's
// `httpRequest` structured field with `logging.googleapis.com/trace`
// correlation attached.
package accesslog

import (
	"context"
	"net/http"

	"github.com/roadrunner-server/logger/v5"
	"go.uber.org/zap"
)

// PluginName is the Endure plugin name and the identifier users put in
// RoadRunner's `http.middleware` list to enable this middleware.
const PluginName = "gcp_access_log"

// loggerChannel is the named-logger channel this middleware writes to. Users
// can target it via `logs.channels.http_access` for per-channel overrides.
// The channel is deliberately distinct from RoadRunner's built-in `http`
// channel so that the parent package's encoder filter — which suppresses
// duplicate built-in access-log entries — does not also drop ours.
const loggerChannel = "http_access"

// Plugin is the RoadRunner middleware plugin. It depends on the logger plugin
// and signals to the encoder (via logger.MarkAccessLogReplaced) that the
// built-in access-log entries should be dropped to prevent duplicates.
type Plugin struct {
	log *zap.Logger
}

// Init is invoked by Endure. The Logger dependency is satisfied by the
// parent logger plugin's Provides hook.
func (p *Plugin) Init(log logger.Logger) error {
	p.log = log.NamedLogger(loggerChannel)
	logger.MarkAccessLogReplaced()
	return nil
}

func (p *Plugin) Serve() chan error {
	return make(chan error, 1)
}

func (p *Plugin) Stop(context.Context) error {
	if p.log != nil {
		_ = p.log.Sync()
	}
	return nil
}

// Name returns the plugin name. Required by Endure and by RoadRunner's HTTP
// middleware registry.
func (p *Plugin) Name() string {
	return PluginName
}

// Middleware satisfies the common.Middleware interface that RoadRunner's HTTP
// plugin consumes. See github.com/roadrunner-server/http v5 common/interfaces.go.
func (p *Plugin) Middleware(next http.Handler) http.Handler {
	return newHandler(p.log, next)
}
