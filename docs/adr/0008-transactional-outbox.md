# 0008: Transactional outbox for task publication

Status: Accepted

## Context

Recording a step transition and publishing its next task are two writes to
different systems. A crash between them can strand a workflow or publish work
that never became durable.

## Decision

Insert the next task's outbox row in the transaction that records step state and
history. A relay claims an outbox row, performs a publish bounded to three
seconds, waits for PUBACK, and then marks the row published. A failed publish
stores its next retry time. Repeated publication uses the same task ID.

## Consequences

Committed scheduling intent survives process failure and broker unavailability.
The relay holds an outbox row lock during its bounded network call, a deliberate
small implementation tradeoff. Duplicate delivery remains part of the contract.
Retention and batching are follow-up work.

## Alternatives considered

Publishing before commit can create phantom work; publishing after commit without
an outbox can lose work. A distributed transaction across broker and database
adds coordination without removing the worker's external side-effect problem.
