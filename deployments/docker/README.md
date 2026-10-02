# Local runtime

```sh
docker compose up --build -d --wait
```

The short-lived `secrets` container generates random PostgreSQL, NATS and Grafana credentials into Docker named volumes. PostgreSQL administrative credentials are mounted only into the database container; the application role has no superuser, role-creation or database-creation privileges. It also initializes data-volume ownership. Every long-running service runs without root, drops Linux capabilities and uses a read-only root filesystem. Ports bind to `127.0.0.1`; the local API can additionally require a bearer token by setting `API_TOKEN` outside Git.

Secrets remain in the named volume across restarts. To read the generated Grafana password locally:

```sh
docker compose exec -T postgres cat /run/secrets/grafana_password
```

Use username `flowforge` at `http://localhost:3001`. Anyone with Docker access can already read container secrets; do not reuse development credentials elsewhere.

## Optional profiles

```sh
make ui             # Next.js at http://localhost:3000
make observability  # Prometheus at :9090, Grafana at :3001, OTLP HTTP at :4318
```

`make observability` recreates application containers with the collector endpoint configured. The collector batches traces and writes a basic trace summary to its logs; Prometheus scrapes the services' `/metrics` endpoints directly. The provisioned Grafana dashboard shows execution counts, failures and latency. Traces do not have a searchable storage backend in this first version.

```sh
docker compose logs -f flowforge-api flowforge-engine example-worker
docker compose logs -f otel-collector
make compose-down
```

`compose-down` preserves named volumes. `docker compose --profile ui --profile observability down -v` deletes the local database, JetStream data, credentials and metrics history; use it only to intentionally reset the environment. Do not remove only the secret volume while retaining the initialized database: regenerated credentials would no longer match PostgreSQL.

## Host integration tests

The test database must be isolated from your demo data. CI starts a separately named Compose project with generated credentials and destroys it after testing. Tests use `TEST_DATABASE_URL`, `TEST_NATS_URL` and `TEST_NATS_TOKEN`. Supply the generated token separately from the NATS URL. If another Compose stack uses the default ports, supply distinct `POSTGRES_PORT` and `NATS_PORT` values.
