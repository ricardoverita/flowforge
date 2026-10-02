# Security policy

FlowForge is an early development release. Security fixes target the current
`main` branch; there is no supported production release line yet.

## Reporting a vulnerability

Use GitHub's **Report a vulnerability** option on the repository's Security tab
to send a private report when private reporting is enabled. If that option is
unavailable, contact [the maintainer](https://github.com/ricardoverita) to arrange
a private channel. Do not disclose exploit details, credentials, or affected
customer payloads in a public issue. A report should describe the affected
revision, impact, and a minimal reproduction using synthetic data.

There is no guaranteed response time at this stage. Acknowledged reports will
be tracked privately until a fix and disclosure plan are agreed.

## Deployment boundary

The default Compose stack is for local development and publishes ports on
loopback. PostgreSQL, NATS, and Grafana credentials are generated in a Docker
volume rather than checked into the repository. API bearer authentication is
optional through `API_TOKEN`; it provides one shared credential and does not
provide user authorization, tenant isolation, or audit attribution.

The console has no independent login. If its server is configured with
`FLOWFORGE_API_TOKEN`, anyone who can reach the console can use that credential
through its actions. Protect both API and console with an authenticated gateway
and TLS before exposing them beyond a trusted development environment.

Workers and engines share a trusted broker boundary in the local stack. A
broker credential allows result publication; result IDs and attempts provide
consistency checks, not proof that an authorized worker executed a business
operation. Production worker credentials need subject-level publish/subscribe
permissions and separate administration privileges.

HTTP request bodies are bounded, inputs are validated, errors hide internal
causes, containers run non-root where applicable, and CI checks dependency
vulnerabilities. These controls do not replace workload identity, encryption,
backups, operational retention, or review of a real business integration.

Execution input and worker output are persisted payloads. Use synthetic data in
the example, and design retention, access controls, and redaction before storing
personal or sensitive information. See
[the security architecture](docs/architecture/security.md) for the remaining
production requirements.
