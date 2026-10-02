# Observability

Every core process emits structured JSON logs and exposes `/metrics`, `/health`,
and `/ready` on its HTTP listener. The API publishes port 8080 locally; engine
8081 and worker 8082 remain on the Compose network. Readiness checks required
dependencies, while health indicates a live process.

Start the optional local tools:

```bash
make observability
```

Prometheus scrapes each process directly. Grafana provisions a Prometheus data
source and a FlowForge dashboard. Log in at http://localhost:3001 with username
`flowforge` and the generated password:

```bash
docker compose exec -T postgres cat /run/secrets/grafana_password
```

This command reads a local development credential; do not place its output in
an issue or a CI log. Prometheus is available at http://localhost:9090.

## Metrics

| Metric | Meaning |
| --- | --- |
| `workflow_executions_total` | Committed execution starts observed by an engine process |
| `workflow_execution_duration_seconds` | Started-to-terminal execution duration |
| `step_executions_total` | Applied completed or failed step attempts |
| `step_execution_duration_seconds` | Attempt duration through result or timeout processing |
| `step_failures_total` | Applied failures, including expired attempts |
| `worker_jobs_total` | Tasks whose result was published and whose task acknowledgement succeeded |

Duration instruments are histograms. The Prometheus exporter adds its standard
counter and histogram suffixes. Metric labels use task type or status rather
than execution IDs. Identifiers belong in logs and traces to avoid metric
cardinality growth.

Metrics describe observations by a process, not an authoritative count from
the database. A crash between commit and instrumentation can miss an increment,
and a task redelivery can produce another worker observation. Use durable state
and history for an execution's lifecycle.

## Traces and logs

HTTP requests, execution state operations, step handling, NATS publication and
consumption, and selected PostgreSQL operations have OpenTelemetry spans. W3C
trace context passes through outbox metadata and NATS headers. Correlation IDs
link API creation to task/result logs and execution records.

`make observability` sets `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318`
for container processes. For processes started directly on the host, use
`http://127.0.0.1:4318`. The collector exports a debug summary to its logs:

```bash
docker compose logs --tail=100 otel-collector
docker compose logs -f flowforge-api flowforge-engine example-worker
```

The local stack does not include Tempo, Jaeger, or another trace storage/query
backend. Adding one requires an explicit collector pipeline change. Request and
worker logs should contain IDs and stable error codes, without payloads,
credentials, or customer data.

Dashboards are a starting point. Queue latency, unpublished outbox depth,
retention growth, dead-letter inspection, replication health, and alert delivery
need further operational work before an externally operated deployment.
