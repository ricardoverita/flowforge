# 0002: PostgreSQL as the source of truth

Status: Accepted

## Context

An execution must retain state and scheduled work after a process or broker
client restarts. Step order, version identity, and idempotency need database
constraints and atomic writes.

## Decision

Use relational PostgreSQL tables for definitions, executions, steps, history,
inbox, and outbox. Use explicit SQL through pgx. Store dynamic input, output, and
message payloads as `json`; no queries currently depend on payload indexes.
`json` preserves numeric tokens and avoids JSONB expansion invalidating the
domain's byte-size bounds. Keep important state in typed columns.

## Consequences

Transactions protect lifecycle changes and their publication intent. Recovery
reads current state rather than replaying an event log. Schema evolution and
retention require deliberate migrations and operational policies.

## Alternatives considered

Broker-only state lacks the relational invariants needed here. Event sourcing
would add replay/versioning work before it solves a requirement. JSONB can be
introduced selectively when real payload query needs appear.
