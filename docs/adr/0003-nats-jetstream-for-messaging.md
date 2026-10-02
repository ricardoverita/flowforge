# 0003: NATS JetStream for messaging

Status: Accepted

## Context

Workers should consume durable tasks independently of API availability. The
transport needs acknowledged publication, durable consumers, redelivery, and a
small local footprint.

## Decision

Use file-backed JetStream streams with work-queue retention. Tasks use
`flowforge.tasks.<task_type>`; results use `flowforge.results`. Pull consumers
have explicit acknowledgements. A task's attempt ID becomes `Nats-Msg-Id`.
The broker does not own workflow state.

## Consequences

Worker replicas can share a durable consumer. Subject tokens provide a future
routing and permission boundary. Duplicate publication and delivery remain
possible. The single-node local broker does not provide replicated availability,
and production requires persistent NATS infrastructure.

## Alternatives considered

Core NATS has no durable delivery for this workload. Kafka's partitioned log and
operations are unnecessary for the initial work queue. PostgreSQL-only dispatch
would couple external workers to the control-plane database.
