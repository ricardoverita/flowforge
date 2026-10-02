# Contributing

Keep changes focused on one behavior or architectural decision. Open an issue
before a broad change to execution semantics, messaging, or persistence. A bug
report should include the version, a minimal reproduction, expected behavior,
and relevant logs with credentials and customer payloads removed.

## Development

Use Go 1.27, Docker with Compose v2, and Node.js 24 for the console. Start the
local dependencies with `make compose-up`. `make build` builds all four Go
commands, including the migration runner. The database schema lives in
`migrations/`; add a new migration rather than editing an already released one.

Before opening a pull request:

```bash
make fmt-check
make vet
make test
make test-race
make lint
make security
```

Run `make tools` to install the pinned lint and vulnerability tools. Follow
[docs/testing.md](docs/testing.md) for integration tests. For frontend changes,
run `npm ci`, `npm run lint`, `npm run typecheck`, `npm test`, and `npm run build`
inside `web/`. For infrastructure changes, run `make terraform-check` and build
the affected Docker image.

## Changes and review

Domain code owns state transitions and invariants. Transport handlers should
decode requests, call application operations, and map results to HTTP. Keep SQL
and broker behavior in their adapters. Add an interface when it isolates an
actual boundary or makes a critical behavior testable.

Test failure cases that can corrupt durable state: duplicates, concurrency,
stale results, interrupted publication, and attempt exhaustion. Avoid tests
that simply restate getters or internal implementation details. Update OpenAPI
when a request, response, or HTTP contract changes. The specification is
embedded from `internal/api/openapi.yaml` and served at `/openapi.yaml`.

Use English for code, technical comments, documentation, and commits. Comments
should explain a non-obvious consistency rule or tradeoff. Commit messages can
use `feat:`, `fix:`, `test:`, `docs:`, or `chore:` followed by a concrete change.
Describe the problem, final behavior, and validation in the pull request.

An ADR is appropriate when a change creates a lasting architectural constraint,
not for every implementation detail. If a feature is incomplete, keep it out of
the capabilities list and describe it as planned work.

Contributions are provided under the repository's Apache License 2.0. Do not
submit credentials, real customer data, or code you cannot license under those
terms.
