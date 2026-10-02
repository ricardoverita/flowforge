# 0001: Use Go for core services

Status: Accepted

## Context

The API, scheduler, and worker need predictable process lifecycle management,
concurrency, explicit database transactions, and compact deployable artifacts.
The initial team benefits from one language across these processes.

## Decision

Use Go for the API, engine, worker example, and migration runner. Use standard
HTTP and structured logging facilities, pgx for SQL, and the maintained NATS
client. Wire dependencies in constructors rather than a DI framework.

## Consequences

Core processes share domain rules and message types. Failure handling remains
explicit. The console uses TypeScript separately, and cross-language workers
will depend on a versioned protocol rather than internal Go packages.

## Alternatives considered

Java/Kotlin offers mature orchestration libraries but a larger runtime and
framework surface. TypeScript would simplify language reuse with the UI, but
does not offer a clear benefit for this transaction-heavy core.
