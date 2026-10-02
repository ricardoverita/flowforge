# AWS reference infrastructure

This configuration defines a VPC across two availability zones, private application and database subnets, security groups, RDS PostgreSQL, an ECS Fargate cluster, ECR repositories, Secrets Manager secret metadata, CloudWatch logs and a CPU alarm. Fargate keeps the runtime small without introducing Kubernetes control-plane operations.

The development environment defaults to `enable_services = false`. It can provision the foundation, but it does not start applications until image digests, secrets, TLS, NATS and the database schema exist. No cloud deployment is performed by the repository's CI. **Applying even the foundation incurs AWS charges**: RDS and private interface endpoints are billable; enabling NAT or the ALB adds more charges.

## Validate locally

Terraform 1.13 or newer is required. Validation downloads the AWS provider and does not need AWS credentials:

```sh
terraform -chdir=deployments/terraform fmt -check -recursive
terraform -chdir=deployments/terraform/environments/dev init -backend=false
terraform -chdir=deployments/terraform/environments/dev validate
```

## State and environments

The S3 backend is configured separately. Bootstrap an encrypted, versioned S3 bucket with public access blocked and tightly scoped IAM access; state is sensitive even when it does not contain secret values. Copy `environments/dev/backend.hcl.example` to a private backend configuration and replace the bucket name. Authenticate with a short-lived AWS session, then run:

```sh
terraform -chdir=deployments/terraform/environments/dev init -backend-config=backend.hcl
terraform -chdir=deployments/terraform/environments/dev plan -out=dev.tfplan
```

S3 native locking uses `use_lockfile = true`. The state role needs object access to this environment's state key and the `.tflock` object, including delete permission on the lock file. See [HashiCorp's S3 backend documentation](https://developer.hashicorp.com/terraform/language/backend/s3).

Use a separate configuration directory and state key for each future environment. Development uses a single-AZ RDS instance and an optional single NAT gateway; production needs a deliberate availability, backup, egress and cost review. Do not share development state with production. The database has deletion protection and a required final snapshot; removing it requires an explicit configuration change.

## Secrets and database privileges

RDS generates its master password through Secrets Manager. Terraform stores only secret metadata and ARNs, not passwords or tokens. The master account is reserved for schema and role administration.

The security module creates two empty secret containers. Populate their values outside Terraform, using an approved secret-management channel:

- Database application secret: JSON containing `username` and `password` for a dedicated PostgreSQL role.
- Runtime secret: JSON containing `nats_token` and a nonempty `api_token`.

Create a database role restricted to the FlowForge database and schema. Apply migrations with a schema-owner role, then grant the runtime role only the required table and sequence privileges. For the initial one-off migrate task, temporarily point the application secret at the schema-owner credentials; switch it to the runtime role and start new task revisions before enabling services. Do not inject the RDS master secret into steady-state services. Rotation of ECS environment secrets requires replacing tasks. See [AWS's ECS secret injection guidance](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/secrets-envvar-secrets-manager.html).

The reference uses PostgreSQL TLS with `sslmode=require`, which encrypts transport. Deployments that require authenticated server identity should configure `verify-full` with an RDS CA bundle in their runtime image. Public ingress is limited to explicitly supplied CIDRs on an HTTPS listener; application tasks and RDS have no public IPs.

## NATS is a separate persistent dependency

Fargate ephemeral disks are unsuitable for a durable JetStream deployment. This reference accepts an existing TLS NATS endpoint. Operate a three-node JetStream cluster on persistent hosts, or use a managed NATS service; configure stream replication, account isolation, backups and monitoring for that service. The local Compose cluster is a single-node development instance, not a cloud durability design.

Supply a `tls://` endpoint reachable from the private subnets and populate `nats_token` in the runtime secret. Private connectivity can use routing or peering configured outside this module. For a public external NATS endpoint, enable the optional NAT gateway. Task egress initially permits outbound traffic; restrict it to the actual endpoint and AWS network ranges for production.

## Application bootstrap order

1. Provision the foundation after reviewing an authenticated Terraform plan.
2. Build and publish `api`, `engine`, `worker` and `migrate` images to their ECR repositories. Configure `images` with digest references, not mutable tags.
3. Populate secrets, prepare the PostgreSQL role and supply the NATS endpoint. Configure the secret ARNs. Apply to create task definitions while keeping `enable_services = false`.
4. Run the migration task once in the private task subnets with the task security group and public IPs disabled. Confirm it stops successfully; migrations must finish before the engine starts. Rotate to runtime database privileges.
5. Supply an ACM certificate, allowed API client CIDRs, and set `schema_initialized = true` and `enable_services = true`. Review and apply the resulting plan.

The configuration exports the migration task definition ARN, subnet IDs and task security group. Fargate services use rolling deployments with container readiness checks and deployment rollback. The API target group checks `/ready`.

## Opt-in delivery workflow

`.github/workflows/deploy-ecs.yml` supports manual dispatch and reusable invocation. It is skipped unless the repository variable `ENABLE_AWS_CD` is `true` and the caller runs on `main`. Configure a protected GitHub environment with `AWS_REGION` and `AWS_DEPLOY_ROLE_ARN`; use required reviewers where appropriate. The workflow builds the selected Go image, pushes the commit tag to ECR, resolves its digest, registers a new ECS task revision, updates the service and waits for stability. It does not apply Terraform or run schema migrations.

Use GitHub OIDC rather than access keys. Restrict the AWS role trust to this repository's protected environment (`repo:ricardoverita/flowforge:environment:dev` for the development environment), audience `sts.amazonaws.com`, and grant only:

- ECR authorization and image upload access to the selected repository.
- ECS describe/register/update access for the intended cluster and services.
- `iam:PassRole` for the existing task and execution roles, conditioned on `ecs-tasks.amazonaws.com`.

Record the deployed image digests in environment configuration before the next Terraform apply; otherwise Terraform will reconcile services to its configured task definitions. Review rollout status and application readiness after deployment. The immutable ECR tag may already exist on a rerun of the same commit; publish a new commit or reuse the previously published digest deliberately.

AWS OIDC setup is described in [GitHub's AWS OIDC guide](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-aws).

## Operational scope

CloudWatch receives structured service logs. The CPU alarm has no notification target until an operator configures one. Local OpenTelemetry, Prometheus and Grafana are usable through Compose; an AWS trace exporter and metrics collector, autoscaling policies, production dashboards, disaster recovery exercises and a multi-AZ database are follow-up deployment work. Do not treat this development reference as a production service guarantee.
