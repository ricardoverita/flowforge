# 0004: Capability modules in a monorepo

Status: Accepted

## Context

The definition, execution, HTTP, and messaging contracts will evolve together.
Separate repositories or internal microservices would increase coordination
cost while the execution model is still small.

## Decision

Keep one Go module, one frontend directory, examples, deployment files, and docs
in one repository. Organize domain packages by workflow and execution capability.
Keep adapters and runtime plumbing in internal packages. Deploy API, engine, and
worker as separate processes from the shared codebase.

## Consequences

Changes can update the domain, SQL, protocol, and tests together. Internal APIs
can evolve without implying a public SDK. Modules must maintain clear dependency
direction; folder depth does not enforce architecture on its own.

## Alternatives considered

A horizontal controller/service/repository layout obscures domain ownership.
Independent services and repositories would impose distributed release contracts
before there is a need for independent ownership.
