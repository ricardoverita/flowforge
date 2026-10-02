# 0005: Docker Compose for local development

Status: Accepted

## Context

The smallest useful environment needs PostgreSQL, persistent JetStream, API,
engine, and a worker. A contributor should be able to exercise the whole path
without configuring cloud infrastructure.

## Decision

Use Compose with dependency health checks, a one-shot migration process, durable
volumes, loopback ports, generated local credentials, and bounded container
resources. Keep UI and observability in optional profiles. Production images
have build and runtime stages with non-root application users.

## Consequences

`docker compose up --build` creates a usable local environment. Data survives
container recreation. Local volumes and single-node services do not provide
disaster recovery or a production availability model.

## Alternatives considered

Installing every service directly on the host adds setup drift. Kubernetes and
Helm introduce cluster operations that do not address this local requirement.
