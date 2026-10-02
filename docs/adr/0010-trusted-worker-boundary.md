# 0010: Explicit trusted-worker and operator boundaries

Status: Accepted

## Context

Attempt IDs protect consistency but do not authenticate a worker. A console
that holds an API credential can grant its authority to every console visitor.
The first release has no user or tenant model.

## Decision

Treat the default stack as a trusted local environment, bind ports to loopback,
generate broker/database credentials, and support optional API bearer access.
Keep console API credentials server-side. Document subject-scoped worker
credentials, authenticated console access, TLS, private networking, and separate
database privileges as requirements for an external deployment.

## Consequences

Local setup remains usable without inventing OAuth or multi-tenancy. Operators
must not mistake the shared token or attempt fencing for complete authorization.
The current broker credential is broad and the console has no login.

## Alternatives considered

Full OAuth and workload identity would expand this iteration substantially.
Silently treating valid message IDs as worker authentication would leave a
misleading security boundary.
