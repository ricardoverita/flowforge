# Deployment strategy

The repository has an executable local environment and a cloud reference
foundation. These serve different purposes. Compose demonstrates the execution
path and restart behavior. Terraform supplies real AWS application and database
resources but leaves account configuration and persistent NATS to the operator.

## Local runtime

```bash
docker compose up --build -d --wait
docker compose --profile ui --profile observability ps
docker compose logs --tail=100 flowforge-api flowforge-engine example-worker
docker compose --profile ui --profile observability down
```

The default stack includes a credential initializer and migration runner before
the core processes start. Named volumes preserve data. Stopping containers does
not delete the volumes; deleting volumes removes executions, broker data, and
the generated local credentials together.

Application images use multi-stage builds and non-root runtime users. Compose
applies read-only application filesystems, temporary scratch mounts, dropped
capabilities, resource limits, and health checks. The local PostgreSQL connection
uses unencrypted transport inside the private Docker network; the cloud reference
uses TLS.

## AWS reference

Read [the Terraform guide](../deployments/terraform/README.md) before planning
changes. The configuration separates network, security, database, compute, and
observability modules and uses an encrypted S3 remote backend with native state
locking once configured. The dev environment does not start application services
by default.

Supply immutable ECR image digests, database and runtime secrets, a reachable
TLS JetStream service with persistent storage, initialized schema, and HTTPS
ingress before enabling ECS services. Keep migration privileges separate from
the runtime database role. Do not inject the RDS master credential into steady
state application tasks.

JetStream cannot be treated as another stateless Fargate container. A production
deployment requires persistent nodes or a managed broker, with a reviewed
replication and backup strategy. The repository does not create that service.

## CI and opt-in CD

The automatic CI checks source formatting, lint, unit/race/integration tests,
builds, dependencies, and Terraform configuration. A separate workflow verifies
container builds without publishing images.

`.github/workflows/deploy-ecs.yml` is a manual/reusable delivery path gated by
`ENABLE_AWS_CD=true` and `main`. It obtains a short-lived AWS session through
GitHub OIDC, builds a selected image, pushes it to ECR, registers a digest-based
task revision, updates the selected ECS service, and waits for stability.
Environment configuration supplies the role, region, cluster, repository, and
service details.

The workflow does not apply Terraform or run migrations. Future automated
delivery should connect a successful CI revision to reviewed infrastructure and
schema changes, then rolling deployment and readiness verification. Record the
deployed image digests in Terraform configuration to prevent a later apply from
restoring an older task revision.

Protect the GitHub environment, scope the role trust to this repository and
environment, and grant only the ECR/ECS operations and role passing needed for
the selected application. Long-lived AWS access keys are not part of the design.
