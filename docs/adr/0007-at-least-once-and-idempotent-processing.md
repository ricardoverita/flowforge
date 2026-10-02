# 0007: At-least-once and idempotent processing

Status: Accepted

## Context

A process can commit a side effect and fail before acknowledging a message.
Broker duplicate windows cannot protect an external business operation forever.
Retries also intentionally create new task attempts.

## Decision

Assume at-least-once publication and delivery. Use durable API idempotency keys,
a result inbox unique on task ID, and active-attempt/deadline checks. Worker
results publish before task acknowledgement; engine state commits before result
acknowledgement. Require business effects to use an execution/step key across
all retry attempts.

## Consequences

Duplicate and stale results do not repeat state transitions. A real worker still
needs its own durable side-effect idempotency, reconciliation, or compensation.
The deterministic example has no external effects. No end-to-end exactly-once
guarantee is claimed.

## Alternatives considered

Acknowledging first risks lost work. In-memory duplicate tracking is lost on
restart. Relying only on `Nats-Msg-Id` ties correctness to a bounded broker window.
