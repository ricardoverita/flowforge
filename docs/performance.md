# Performance measurement plan

No throughput or availability claim is made for this release. The first engine
uses bounded poll batches and one-message pulls per process. SQL row locking
serializes changes to an execution, while separate executions can advance on
different engine instances.

Before tuning, use a dedicated environment with representative database storage,
broker persistence, payload sizes, and a deterministic worker delay. Record
service versions, instance counts, CPU/memory bounds, and connection limits for
every result.

Measure:

| Measurement | Start and end |
| --- | --- |
| Execution creation throughput | Accepted API creation requests per second, with and without idempotent replays |
| Step throughput | Committed successful/failed attempt transitions per second |
| Dispatch latency | Durable task scheduling to worker receipt |
| Execution latency | Execution creation to terminal state, reported as p50, p95, and p99 |
| Recovery latency | Dependency restoration or engine restart to resumed durable progress |

Run a ramp test for independent workflows, then a sustained load with retries
and broker/database interruptions. Include duplicate publications and concurrent
engine replicas. Track request errors, DB pool waits, lock contention, outbox
age/depth, broker pending messages, disk growth, and resource saturation.

The durable history provides correctness checks after each run: one active step
per sequential execution, no duplicate transitions for the same task, immutable
workflow version references, and no stale result advancing an expired attempt.
Evaluate retention and cleanup costs as part of sustained tests.

Only then consider relay batching, worker concurrency, query/index changes, or
scheduler partitioning. A microbenchmark that bypasses PostgreSQL and JetStream
would not establish orchestration throughput.
