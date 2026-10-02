# Security boundaries

FlowForge coordinates work; it does not establish the identity or authorization
of every participant. The local stack is a trusted development environment.
Consistency checks protect state against duplicates and stale messages, but an
untrusted publisher with result-subject access can still forge a valid result.

## API and console

`API_TOKEN` enables a shared bearer credential on API routes. This is useful for
an internal development deployment, but it is not per-user authorization, tenant
isolation, or OAuth. Use a TLS gateway and restrict network access. Health and
readiness endpoints describe service availability rather than application data.

The console calls the API from the server. `FLOWFORGE_API_TOKEN` stays outside
the browser, but console users can invoke the server's workflow and execution
actions. Exposing the console exposes that authority. A real deployment needs
operator authentication and authorization before the console is internet-facing.

The API caps request bodies at 1 MiB, rejects unknown DTO fields, validates
workflow policies and JSON payloads, uses server timeouts, emits request IDs, and
maps internal infrastructure failures to bounded public errors. Dynamic payloads
and credentials must not be added to structured logs.

## Broker

Compose generates one NATS token shared by the broker, engine, and worker, and
keeps the broker port on loopback. Only those processes mount that credential;
the API uses PostgreSQL without broker access. Engine and worker can administer
streams and publish tasks and results in this development stack.
Production credentials should separate API/engine, worker task types, and stream
administration, with subject-level permissions. Use TLS and private networking.
The result envelope's task ID, step ID, attempt, and deadline are consistency
fences, not cryptographic worker authentication.

Invalid messages are terminated and logged with bounded context. There is no
operator dead-letter workflow yet. Decide how invalid envelopes will be retained
or inspected without exposing payloads before adding external workers.

## Database and runtime

Database application, database administration, broker, and Grafana credentials
use separate named volumes mounted only by the processes that require them.
The worker has no database credential and the API has no broker credential.
The initial Compose database role owns the development schema. A production
deployment should separate migration ownership from application privileges and
restrict the runtime role to required tables and operations. RDS credentials
belong in Secrets Manager; Terraform state must be treated as sensitive even
when values are marked sensitive.

Application images use non-root users and minimal runtime stages. Compose uses
read-only filesystems for application processes, drops capabilities, and applies
resource bounds. The one-shot local credential initializer needs volume ownership
permissions; it is not a long-running application process.

Persisted inputs and outputs can contain sensitive data even when logs do not.
Add payload retention, access controls, backup encryption, rotation procedures,
and audit attribution for a real service. Dependency scans help detect known
vulnerabilities; they do not prove the security of deployment configuration.
