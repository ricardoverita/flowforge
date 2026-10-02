CREATE TABLE workflow_definitions (
 id text PRIMARY KEY, name text NOT NULL, version integer NOT NULL CHECK (version > 0),
 description text NOT NULL, created_at timestamptz NOT NULL,
 UNIQUE(name, version), UNIQUE(id, version)
);
CREATE TABLE workflow_steps (
 workflow_id text NOT NULL REFERENCES workflow_definitions(id), id text NOT NULL,
 position integer NOT NULL CHECK(position >= 0), name text NOT NULL, task_type text NOT NULL,
 max_attempts integer NOT NULL CHECK(max_attempts BETWEEN 1 AND 10),
 timeout_seconds integer NOT NULL CHECK(timeout_seconds BETWEEN 1 AND 3600),
 PRIMARY KEY(workflow_id, id), UNIQUE(workflow_id, position), UNIQUE(workflow_id, name)
);
CREATE TABLE workflow_executions (
 id text PRIMARY KEY, workflow_id text NOT NULL, workflow_version integer NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','running','waiting','completed','failed','cancelled')),
 input json NOT NULL CHECK(json_typeof(input) = 'object'), correlation_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision > 0), created_at timestamptz NOT NULL,
 started_at timestamptz, completed_at timestamptz,
 idempotency_key text UNIQUE, request_hash text NOT NULL, traceparent text NOT NULL DEFAULT '',
 FOREIGN KEY(workflow_id, workflow_version) REFERENCES workflow_definitions(id, version),
 CHECK((status IN ('completed','failed','cancelled')) = (completed_at IS NOT NULL))
);
CREATE INDEX executions_active ON workflow_executions(created_at) WHERE status IN ('pending','running');
CREATE TABLE step_executions (
 execution_id text NOT NULL REFERENCES workflow_executions(id), id text NOT NULL,
 position integer NOT NULL CHECK(position >= 0), name text NOT NULL, task_type text NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','running','retry_wait','completed','failed','cancelled')),
 attempt integer NOT NULL CHECK(attempt >= 0), max_attempts integer NOT NULL CHECK(max_attempts BETWEEN 1 AND 10),
 timeout_seconds integer NOT NULL CHECK(timeout_seconds BETWEEN 1 AND 3600),
 input json, output json, error_code text NOT NULL DEFAULT '',
 started_at timestamptz, completed_at timestamptz, retry_at timestamptz, deadline_at timestamptz,
 PRIMARY KEY(execution_id,id), UNIQUE(execution_id,position), CHECK(attempt <= max_attempts),
 CHECK(status <> 'running' OR (attempt > 0 AND deadline_at IS NOT NULL AND started_at IS NOT NULL)),
 CHECK(status <> 'retry_wait' OR (retry_at IS NOT NULL AND attempt < max_attempts)),
 CHECK(status <> 'completed' OR (output IS NOT NULL AND completed_at IS NOT NULL))
);
CREATE INDEX steps_ready ON step_executions(retry_at) WHERE status IN ('pending','retry_wait');
CREATE INDEX steps_deadline ON step_executions(deadline_at) WHERE status = 'running';
CREATE TABLE workflow_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 execution_id text NOT NULL REFERENCES workflow_executions(id), type text NOT NULL,
 step_id text NOT NULL DEFAULT '', attempt integer NOT NULL DEFAULT 0,
 occurred_at timestamptz NOT NULL, data json NOT NULL DEFAULT '{}'
);
CREATE INDEX events_execution ON workflow_events(execution_id,id);
CREATE TABLE outbox (
 id text PRIMARY KEY, execution_id text NOT NULL REFERENCES workflow_executions(id),
 subject text NOT NULL, payload json NOT NULL, traceparent text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL, available_at timestamptz NOT NULL,
 published_at timestamptz, attempts integer NOT NULL DEFAULT 0
);
CREATE INDEX outbox_pending ON outbox(available_at,created_at) WHERE published_at IS NULL;
CREATE TABLE result_inbox (
 task_id text PRIMARY KEY, execution_id text NOT NULL REFERENCES workflow_executions(id),
 received_at timestamptz NOT NULL
);
