# Initial scope and design rationale

The first iteration proves the path from a versioned definition to a durable
sequential execution, through a real database and broker, to a completed history.
It also establishes failure recovery and the development checks needed to
evolve that path safely.

## Choices that constrain this release

The control plane is a modular codebase with separately deployable processes,
not a fleet of internal business services. Workflow and execution have small
domain modules. PostgreSQL and NATS isolate real infrastructure boundaries.
There are no generic factories, strategies, DI containers, or repository layers
whose only purpose would be to display a pattern.

The proposed outbox is justified immediately: completing a step must both persist
its result and make the next task deliverable. The adapter coordinates those
writes in one transaction. Event history records transitions, but the engine
restores relational snapshots rather than replaying events. This is not event
sourcing or a separate CQRS architecture.

Retries and deadlines are included because crash recovery without persisted
scheduling would leave an execution stranded. Waiting, approval, parallel joins,
compensation, and cancellation require additional commands and invariants and
remain future work. Domain states alone do not constitute an implemented feature.

The frontend is a small operator console. It uses the API's actual endpoints and
lookup capabilities. It does not introduce an execution-list endpoint, a visual
builder, or another execution engine.

## Infrastructure scope

Docker Compose includes PostgreSQL, persistent JetStream, API, engine, and worker.
Observability and the web console are optional profiles. Local credentials are
generated rather than embedded in Compose. Local single-node storage demonstrates
restart recovery, not availability across host loss.

Terraform establishes an AWS reference with network, security, RDS, ECS/ECR,
Secrets Manager, and logging resources. Persistent NATS must be supplied through
a separate deployment with storage and availability appropriate to its needs.
ECS Fargate is the initial application runtime; Kubernetes would add cluster
operations without resolving a current requirement.

CI runs automatically. CD is explicitly gated and requires account-specific
OIDC roles, registry settings, secrets, and deployment variables. No cloud apply
is part of the default workflow.

## Included behavior

Definition creation/list/detail; execution creation/detail/history; sequential
task dispatch; durable step results; bounded retry and timeout scheduling;
request idempotency; duplicate-message handling; structured logging; OpenTelemetry
spans and metrics; graceful shutdown; readiness checks; domain, API, adapter, and
integration tests; OpenAPI; container builds; and infrastructure validation.

## Deferred behavior

External event waiting, general timers, branching, parallel execution, joins,
compensation, human approval, cancellation commands, an operator dead-letter
queue, real service adapters, tenant isolation, user authentication, SDKs, and
AI-specific workers. Failed attempts and terminal executions are observable
through durable state and history; automatic recovery of arbitrary business
side effects is outside this iteration.
