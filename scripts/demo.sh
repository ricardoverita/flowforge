#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec python3 - "$project_root" <<'PY'
import json
import os
import pathlib
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

root = pathlib.Path(sys.argv[1])
base = os.environ.get("FLOWFORGE_API_URL", "http://localhost:8080").rstrip("/")
token = os.environ.get("API_TOKEN", "")
timeout = int(os.environ.get("DEMO_TIMEOUT", "120"))
if timeout < 1 or timeout > 3600:
    raise SystemExit("DEMO_TIMEOUT must be between 1 and 3600 seconds.")

def request(method, path, body=None, key=None):
    headers = {"Accept": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    if key:
        headers["Idempotency-Key"] = key
    data = None
    if body is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=10) as response:
        return json.load(response)

try:
    readiness_deadline = time.monotonic() + 60
    while True:
        try:
            request("GET", "/ready")
            break
        except (urllib.error.URLError, TimeoutError):
            if time.monotonic() >= readiness_deadline:
                raise SystemExit("The API is not ready. Start the Compose environment and check its logs.")
            time.sleep(1)

    fixture = json.loads((root / "examples/fintech-onboarding/workflow.json").read_text())
    workflows = request("GET", "/api/v1/workflows")
    definition = next((w for w in workflows if w["name"] == fixture["name"] and w["version"] == fixture["version"]), None)
    if definition is None:
        try:
            definition = request("POST", "/api/v1/workflows", fixture)
        except urllib.error.HTTPError as error:
            if error.code != 409:
                raise
            workflows = request("GET", "/api/v1/workflows")
            definition = next(w for w in workflows if w["name"] == fixture["name"] and w["version"] == fixture["version"])
    observed = [{key: step[key] for key in ("name", "task_type", "max_attempts", "timeout_seconds")} for step in definition["steps"]]
    if observed != fixture["steps"]:
        raise SystemExit("customer-onboarding v1 exists with different steps. Use a new workflow version.")

    workflow_id = urllib.parse.quote(definition["id"], safe="")
    input_body = json.loads((root / "examples/fintech-onboarding/input.json").read_text())
    key = os.environ.get("FLOWFORGE_DEMO_KEY", "demo-" + str(uuid.uuid4()))
    execution = request("POST", "/api/v1/workflows/" + workflow_id + "/executions", input_body, key)
    repeated = request("POST", "/api/v1/workflows/" + workflow_id + "/executions", input_body, key)
    if repeated["id"] != execution["id"]:
        raise SystemExit("Idempotency verification failed: the repeated request returned a different execution.")
    execution_id = urllib.parse.quote(execution["id"], safe="")
    print("Workflow execution " + execution["id"], flush=True)
    print("Idempotency verified: repeated request returned the same execution.", flush=True)
    previous = None
    deadline = time.monotonic() + timeout
    while True:
        execution = request("GET", "/api/v1/executions/" + execution_id)
        state = (execution["status"], tuple((s["status"], s["attempt"]) for s in execution["steps"]))
        if state != previous:
            print("\nExecution: " + execution["status"], flush=True)
            for step in execution["steps"]:
                print("  {:<22} {:<12} attempt {}/{}".format(step["name"], step["status"], step["attempt"], step["max_attempts"]), flush=True)
            previous = state
        if execution["status"] in ("completed", "failed", "cancelled"):
            break
        if time.monotonic() >= deadline:
            raise SystemExit("Execution is still active. Inspect /api/v1/executions/" + execution_id)
        time.sleep(0.5)
    events = request("GET", "/api/v1/executions/" + execution_id + "/events")
    print("\nHistory: {} events".format(len(events)), flush=True)
    print("API: " + base + "/api/v1/executions/" + execution_id, flush=True)
    if execution["status"] != "completed":
        raise SystemExit("Workflow ended with status: " + execution["status"])
except urllib.error.HTTPError as error:
    raise SystemExit("API request failed with HTTP {}. Inspect the API logs or /docs.".format(error.code)) from None
except (urllib.error.URLError, TimeoutError) as error:
    raise SystemExit("Cannot reach the API at " + base + ". Check the running services.") from None
PY
