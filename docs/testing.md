# Testing

Unit tests run without PostgreSQL or NATS. They cover state transitions, workflow
validation, sequential step policies, worker simulation, API transport, error
classification, and configuration. Race tests exercise those packages with the
Go race detector.

```bash
make test
make test-race
go tool cover -func=coverage.out
```

Integration tests are compiled with the `integration` build tag and use real
PostgreSQL and JetStream. They must run against disposable dependencies: tests
create and clear durable state, including broker streams. The test setup applies
the repository's migrations.

`make test-integration` starts an isolated Compose project, obtains generated
credentials, runs tagged race tests, and removes only that project's containers
and volumes. It defaults to ports 5433 and 4223, so the regular local stack can
remain running:

```bash
make test-integration
TEST_POSTGRES_PORT=5434 TEST_NATS_PORT=4224 make test-integration
```

When `TEST_DATABASE_URL` and `TEST_NATS_URL` are already set, the Make target uses
those dependencies instead; set `TEST_NATS_TOKEN` when the broker requires it.
The caller owns cleanup in that mode. Do not use a shared database with valuable
data. Local credentials are not intended for logs, shell tracing, or committed
files.

The critical integration behaviors are state and publication atomicity,
repeated execution requests, duplicate/stale results, multiple scheduler claims,
retry scheduling, timeout recovery, and durable broker publication/consumption.
Consult the integration test names for the scenarios actually asserted by the
current revision. Coverage is a diagnostic tool, not a release target.

## Frontend

Use Node.js 24 and run from `web/`:

```bash
npm ci
npm run lint
npm run typecheck
npm test
npm run build
npm audit --audit-level=high
```

Transport tests verify no-store reads, server-side credentials, idempotency
headers, bounded error messages, and network failures. Input tests cover invalid
workflow policies and route identifiers. Rendering tests cover visible lifecycle
labels and UTC timestamps. These are focused unit tests; browser end-to-end
coverage is a future addition.

## End-to-end demonstration

```bash
docker compose up --build -d --wait
./scripts/demo.sh
```

The demo creates or reuses the fixture definition, verifies a repeated creation
request returns one execution, observes its steps, and reads history. It exits
nonzero if the API is unavailable, idempotency differs, the workflow fails, or
the execution does not finish within `DEMO_TIMEOUT` (120 seconds by default).
