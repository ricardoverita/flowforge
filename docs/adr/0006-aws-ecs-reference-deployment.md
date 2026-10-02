# 0006: AWS ECS Fargate reference deployment

Status: Accepted

## Context

The project needs a credible cloud reference without committing to Kubernetes
operations or deploying into an unknown AWS account. Stateless application
processes and durable infrastructure have different runtime needs.

## Decision

Use ECS Fargate for API, engine, and worker, with RDS PostgreSQL, private
networking, ECR, Secrets Manager, and CloudWatch. Implement real Terraform modules
and an environment configuration. Keep application services disabled until
images, schema, secrets, HTTPS ingress, and an external persistent NATS endpoint
are configured. Gate manual CD behind repository configuration and use OIDC.

## Consequences

The application runtime stays small. JetStream storage and availability require
a separate persistent deployment or managed service; Fargate ephemeral storage
does not meet that requirement. Applying the foundation costs money and requires
an operator-reviewed plan.

## Alternatives considered

EKS offers more scheduling control but adds a cluster without a current need.
Lambda does not match these long-running pollers naturally. EC2 remains a
possible host for persistent NATS rather than the default application runtime.
