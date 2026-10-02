# Consistency and recovery

The database records what the engine has decided. The broker distributes tasks
and results that may be repeated. Every result must be checked against durable
execution state; a broker acknowledgement is not a workflow transaction.

## Creating an execution

The API stores the execution, ordered step snapshots, and creation event in one
transaction. When `Idempotency-Key` is present, the request's workflow and payload
fingerprint share a durable unique key. The same request returns the existing
execution; a conflicting payload or workflow returns HTTP 409. A key is not a
substitute for workflow version identity. Keys have no expiration policy yet.

The scheduler discovers pending executions from PostgreSQL. An API process can
exit immediately after commit without losing the scheduling intent.

## Scheduling and publishing

The scheduler locks a due execution with `FOR UPDATE SKIP LOCKED`, restores its
aggregate, validates the next transition, and writes the step state, event, and
outbox task atomically. Other engine instances skip claimed executions and work
on different rows.

The relay claims one outbox row with the same lock convention. A bounded publish
waits for JetStream PUBACK, then records `published_at`. Unlike the execution
transaction, this small relay transaction holds an outbox lock across the network
call. The three-second publish bound limits that cost. A failed publish records
its next retry time; a crash after PUBACK but before commit republishes the same
task ID.

Task identity is `execution_id:step_id:attempt`. That ID is also `Nats-Msg-Id`.
JetStream's duplicate window reduces repeated writes, while correctness comes
from database idempotency and attempt checks after that window expires.

## Worker results

The worker waits for a result PUBACK before acknowledging its task. A crash in
between can execute the task again. The bundled simulation has no external
effects and returns deterministic results for the same task.

The engine locks the execution, inserts the result task ID into an inbox with a
unique constraint, and verifies the active step, attempt, and deadline. It
commits the result, history, and next outbox task before acknowledging the broker
message. A duplicate inbox insertion makes no state change. A late or stale
result is recorded as received and cannot advance the workflow.

```mermaid
sequenceDiagram
    participant Engine
    participant DB as PostgreSQL
    participant Broker as JetStream
    participant Worker
    Engine->>DB: Lock execution; persist step + event + outbox
    DB-->>Engine: Commit
    Engine->>Broker: Publish task with stable task ID
    Broker-->>Engine: PUBACK
    Engine->>DB: Mark outbox published
    Broker->>Worker: Deliver task
    Worker->>Broker: Publish result
    Broker-->>Worker: PUBACK
    Worker->>Broker: ACK task
    Broker->>Engine: Deliver result
    Engine->>DB: Inbox + state + event + next task
    DB-->>Engine: Commit
    Engine->>Broker: ACK result
```

## Retries and timeouts

Transient failures persist `retry_at` and `retry_wait`. Backoff grows
exponentially, with jitter between half and all of the bounded base delay. The
base caps at 32 seconds. A due retry advances the attempt number and creates a
new task ID. A permanent failure or attempt exhaustion fails the execution.

Every running step has a persisted deadline. A scheduler restart still detects
expired attempts and applies the same failure/retry rules. Stale results do not
resurrect an expired attempt. The scheduler's clock and database timestamp
comparisons therefore require synchronized system clocks in a distributed
deployment.

## External effects and limits

A production worker must persist a business idempotency key based on
`execution_id:step_id`, shared across retry attempts. Attempt IDs distinguish
engine tasks; they must not cause the same account, payment, or notification to
be created again on a retry. Commit that business key with the side effect when
possible, or use the downstream service's idempotency mechanism. If neither is
possible, define reconciliation or compensation explicitly.

Execution revisions guard stale persistence with compare-and-swap while row
locks serialize active changes. No external distributed lock is required. The
initial worker and engine handle one pulled message at a time per process;
larger worker concurrency and relay batching need measurement before tuning.

Published outbox, inbox, history, and completed executions currently accumulate.
Retention and archival are not implemented. Local storage is single-node, and
recovery from infrastructure loss requires backups and broker replication.

References: [PostgreSQL locking clauses](https://www.postgresql.org/docs/current/sql-select.html#SQL-FOR-UPDATE-SHARE)
and [JetStream pull consumers](https://docs.nats.io/learn/jetstream/pull-consumers).
