# RoadRunner GCP Logger

Fork of [`roadrunner-server/logger`](https://docs.roadrunner.dev/logging-and-observability/logger)
that emits JSON in [GCP Cloud Logging structured format](https://cloud.google.com/logging/docs/structured-logging)
when running in `production` mode, and ships a companion HTTP access-log
middleware for trace correlation.

## What it changes

In `production` mode the JSON encoder uses GCP-recognized field names:

| Standard zap    | GCP-mode key      |
|-----------------|-------------------|
| `ts` (epoch)    | `time` (RFC3339Nano) |
| `level`         | `severity` (`DEBUG`/`INFO`/`WARNING`/`ERROR`/`CRITICAL`) |
| `msg`           | `message`         |
| `stacktrace`    | `stack_trace`     |

Other modes (`development`, `raw`, default) are untouched.

## HTTP access logs as `httpRequest`

Whenever an entry matches RoadRunner's built-in access-log message
(`"http log"` or `"http access log"`), the encoder folds the flat fields
(`method`, `status`, `URI`, `elapsed`, ...) into a nested `httpRequest`
object Cloud Logging recognizes as a first-class request summary.

## Trace correlation

Any entry carrying `trace`, `span_id`, or `trace_sampled` zap fields has them
rewritten to GCP's special LogEntry keys:

- `trace` → `logging.googleapis.com/trace` = `projects/<gcp_project>/traces/<trace-id>`
- `span_id` → `logging.googleapis.com/spanId`
- `trace_sampled` → `logging.googleapis.com/trace_sampled`

The wrapping is idempotent: a `trace` value that already starts with
`projects/` passes through unchanged.

Set the project via the new `gcp_project` config key on the `logs:` block:

```yaml
logs:
  mode: production
  gcp_project: "my-gcp-project"
```

PHP/OpenTelemetry-instrumented apps that already emit these fields get
correlation automatically — no further code changes needed.

## `accesslog` middleware

To get trace correlation on RoadRunner's HTTP access-log entries too, register
the bundled middleware in your RR binary:

```go
import "github.com/roadrunner-server/logger/v5/accesslog"

container.RegisterAll(
    &logger.Plugin{},
    &accesslog.Plugin{},
    // ... other plugins
)
```

Then in `.rr.yaml`:

```yaml
http:
  access_logs: false              # minimize the built-in entry
  middleware: ["gcp_access_log"]  # then ours emits the canonical one
```

The middleware reads either `X-Cloud-Trace-Context` (GCP-native) or W3C
`traceparent` headers, emits an `"http access log"` entry on the
`http_access` named-logger channel, and signals the encoder to drop the
built-in `"http log"` / `"http access log"` entries so nothing is duplicated.

## Upstream docs

For all other config — file rotation, channels, levels, line endings —
the upstream documentation still applies:
<https://docs.roadrunner.dev/logging-and-observability/logger>
