# Architecture decision records

These records describe decisions accepted for the initial release. Change a
decision with a new record and link the superseded one rather than silently
rewriting its rationale.

| Record | Decision |
| --- | --- |
| [0001](0001-use-go-for-core-services.md) | Go for core processes |
| [0002](0002-postgresql-as-source-of-truth.md) | PostgreSQL for durable workflow state |
| [0003](0003-nats-jetstream-for-messaging.md) | JetStream for task and result transport |
| [0004](0004-monorepo-architecture.md) | Capability modules in a single repository |
| [0005](0005-docker-compose-for-local-development.md) | Compose for the local runtime |
| [0006](0006-aws-ecs-reference-deployment.md) | ECS Fargate as the application cloud reference |
| [0007](0007-at-least-once-and-idempotent-processing.md) | At-least-once delivery and business idempotency |
| [0008](0008-transactional-outbox.md) | Outbox for state-to-message consistency |
| [0009](0009-postgresql-concurrency.md) | Database row claims and execution revisions |
| [0010](0010-trusted-worker-boundary.md) | Explicit local trust and deployment boundaries |
