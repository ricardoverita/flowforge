# 0009: PostgreSQL row claims and execution revisions

Status: Accepted

## Context

Multiple engines may find the same due execution or receive related results.
Sequential step order requires one authoritative transition at a time.

## Decision

Claim scheduler and relay rows with `FOR UPDATE SKIP LOCKED`. Lock an execution
before applying a result. Persist with an execution revision comparison and
increment. Use unique inbox and API idempotency constraints for repeated inputs.
Keep state, history, and outbox changes in the same transaction.

Revalidate due work after acquiring the execution lock. Under Read Committed,
the candidate query can retain an older child-row snapshot while observing a
newer locked execution row. A poll that makes no transition must not increment
the revision or append history.

## Consequences

Engine replicas can process different executions without a separate locking
service. A hot execution remains serialized, which matches the current
sequential model. Parallel execution will need explicit join invariants rather
than removing those locks casually.

## Alternatives considered

Redis locks would introduce lease and failure semantics outside the source of
truth. Optimistic writes alone would cause avoidable scheduling contention.
Process-local locks do not protect against another engine instance.
