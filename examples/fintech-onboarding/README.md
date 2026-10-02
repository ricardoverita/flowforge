# Customer onboarding

This example exercises the complete execution path with four simulated tasks:

```text
verify_identity → risk_check → create_account → send_notification
```

The example worker adds `identity_verified`, `risk_level`, `account_id`, and
`notification_sent` to the payload as it passes through the workflow. No real
identity checks, accounts, or notifications are created. A step receives the
previous step's output; the first step receives the execution input.

From the repository root:

```bash
docker compose up --build -d --wait
./scripts/demo.sh
```

The script requires Python 3, registers the definition if it does not exist,
starts an execution, verifies API idempotency, and prints progress until the
execution finishes. It is safe to repeat: definitions are reused and each run
uses a fresh execution key. Set `FLOWFORGE_DEMO_KEY` to reuse one execution.
When API authentication is enabled, provide `API_TOKEN` to the script.

To observe a retry:

```bash
WORKER_FAIL_TASK=risk.check WORKER_FAIL_ATTEMPTS=1 \
  docker compose up -d --force-recreate example-worker
./scripts/demo.sh
docker compose logs --tail=50 flowforge-engine example-worker
```

`risk_check` fails on its first attempt and then succeeds. The engine persists
the retry time with exponential backoff and jitter. Setting `WORKER_FAIL_ATTEMPTS=3`
exhausts this definition's three attempts and fails the execution. Restore the
default worker afterward:

```bash
WORKER_FAIL_TASK= WORKER_FAIL_ATTEMPTS=0 docker compose up -d --force-recreate example-worker
```

For a timeout experiment, register a new definition version with
`timeout_seconds: 1` and start a worker with `WORKER_DELAY=2s`. Late results do
not advance the execution; the engine checks the active attempt and its deadline.

`workflow.json` is a `POST /api/v1/workflows` request and `input.json` is a
`POST /api/v1/workflows/{id}/executions` request. The API assigns definition and
step IDs. To perform each request manually, open the local API explorer at
http://localhost:8080/docs.
