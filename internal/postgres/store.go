package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoverita/flowforge/internal/execution"
	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/workflow"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type Store struct{ Pool *pgxpool.Pool }
type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func Open(ctx context.Context, url string) (*Store, error) {
	c, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fault.New(fault.Validation, "database_config_invalid", "Database configuration is invalid.")
	}
	c.MaxConns = 10
	c.MinConns = 1
	c.MaxConnLifetime = time.Hour
	c.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	c.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "15000"
	pool, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, err
	}
	s := &Store{Pool: pool}
	if err = s.Ready(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Ready(ctx context.Context) error { return s.Pool.Ping(ctx) }
func (s *Store) Close()                          { s.Pool.Close() }
func databaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fault.New(fault.NotFound, "not_found", "The requested resource was not found.")
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" {
			return fault.Wrap(fault.Conflict, "already_exists", "The resource already exists.", err)
		}
		if p.Code == "40001" || p.Code == "40P01" {
			return fault.Wrap(fault.Transient, "database_retry", "Please retry the request.", err)
		}
	}
	return fault.Wrap(fault.Infrastructure, "database_unavailable", "The service is temporarily unavailable.", err)
}
func (s *Store) CreateWorkflow(ctx context.Context, d workflow.Definition) error {
	ctx, span := otel.Tracer("flowforge/postgres").Start(ctx, "postgres.create_workflow")
	defer span.End()
	if err := d.Validate(); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO workflow_definitions(id,name,version,description,created_at) VALUES($1,$2,$3,$4,$5)`, d.ID, d.Name, d.Version, d.Description, d.CreatedAt)
	if err != nil {
		return databaseError(err)
	}
	for i, step := range d.Steps {
		_, err = tx.Exec(ctx, `INSERT INTO workflow_steps(workflow_id,id,position,name,task_type,max_attempts,timeout_seconds) VALUES($1,$2,$3,$4,$5,$6,$7)`, d.ID, step.ID, i, step.Name, step.TaskType, step.MaxAttempts, step.TimeoutSeconds)
		if err != nil {
			return databaseError(err)
		}
	}
	return databaseError(tx.Commit(ctx))
}
func (s *Store) GetWorkflow(ctx context.Context, id string) (workflow.Definition, error) {
	return loadWorkflow(ctx, s.Pool, id)
}
func loadWorkflow(ctx context.Context, q querier, id string) (workflow.Definition, error) {
	var d workflow.Definition
	err := q.QueryRow(ctx, `SELECT id,name,version,description,created_at FROM workflow_definitions WHERE id=$1`, id).Scan(&d.ID, &d.Name, &d.Version, &d.Description, &d.CreatedAt)
	if err != nil {
		return d, databaseError(err)
	}
	rows, err := q.Query(ctx, `SELECT id,name,task_type,max_attempts,timeout_seconds FROM workflow_steps WHERE workflow_id=$1 ORDER BY position`, id)
	if err != nil {
		return d, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var st workflow.StepDefinition
		if err = rows.Scan(&st.ID, &st.Name, &st.TaskType, &st.MaxAttempts, &st.TimeoutSeconds); err != nil {
			return d, databaseError(err)
		}
		d.Steps = append(d.Steps, st)
	}
	return d, databaseError(rows.Err())
}
func (s *Store) ListWorkflows(ctx context.Context) ([]workflow.Definition, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id FROM workflow_definitions ORDER BY created_at,id LIMIT 1000`)
	if err != nil {
		return nil, databaseError(err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, databaseError(err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, databaseError(err)
	}
	defs := make([]workflow.Definition, 0, len(ids))
	for _, id := range ids {
		d, e := s.GetWorkflow(ctx, id)
		if e != nil {
			return nil, e
		}
		defs = append(defs, d)
	}
	return defs, nil
}
func requestHash(s execution.Snapshot) string {
	var input any
	decoder := json.NewDecoder(bytes.NewReader(s.Input))
	decoder.UseNumber()
	_ = decoder.Decode(&input)
	canonical, _ := json.Marshal(input)
	h := sha256.Sum256(append(append([]byte(s.WorkflowID), 0), canonical...))
	return hex.EncodeToString(h[:])
}
func (s *Store) CreateExecution(ctx context.Context, e *execution.Execution, key string) (execution.Snapshot, bool, error) {
	ctx, span := otel.Tracer("flowforge/postgres").Start(ctx, "postgres.create_execution")
	defer span.End()
	snapshot := e.Snapshot()
	hash := requestHash(snapshot)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return snapshot, false, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var nullableKey any
	if key != "" {
		nullableKey = key
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	tag, err := tx.Exec(ctx, `INSERT INTO workflow_executions(id,workflow_id,workflow_version,status,input,correlation_id,revision,created_at,idempotency_key,request_hash,traceparent) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(idempotency_key) DO NOTHING`, snapshot.ID, snapshot.WorkflowID, snapshot.WorkflowVersion, snapshot.Status, snapshot.Input, snapshot.CorrelationID, snapshot.Revision, snapshot.CreatedAt, nullableKey, hash, carrier.Get("traceparent"))
	if err != nil {
		return snapshot, false, databaseError(err)
	}
	if tag.RowsAffected() == 0 {
		var id, storedHash string
		err = tx.QueryRow(ctx, `SELECT id,request_hash FROM workflow_executions WHERE idempotency_key=$1`, key).Scan(&id, &storedHash)
		if err != nil {
			return snapshot, false, databaseError(err)
		}
		if storedHash != hash {
			return snapshot, false, fault.New(fault.Conflict, "idempotency_conflict", "The idempotency key was already used for a different request.")
		}
		out, e := loadExecution(ctx, tx, id)
		return out, true, e
	}
	for _, step := range snapshot.Steps {
		_, err = tx.Exec(ctx, `INSERT INTO step_executions(execution_id,id,position,name,task_type,status,attempt,max_attempts,timeout_seconds) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, snapshot.ID, step.ID, step.Position, step.Name, step.TaskType, step.Status, step.Attempt, step.MaxAttempts, step.TimeoutSeconds)
		if err != nil {
			return snapshot, false, databaseError(err)
		}
	}
	if err = event(ctx, tx, snapshot.ID, "execution.created", "", 0, snapshot.CreatedAt); err != nil {
		return snapshot, false, err
	}
	return snapshot, false, databaseError(tx.Commit(ctx))
}
func (s *Store) GetExecution(ctx context.Context, id string) (execution.Snapshot, error) {
	ctx, span := otel.Tracer("flowforge/postgres").Start(ctx, "postgres.get_execution")
	defer span.End()
	// A repeatable-read snapshot prevents a reader observing parent and steps from different commits.
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return execution.Snapshot{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	out, err := loadExecution(ctx, tx, id)
	if err != nil {
		return out, err
	}
	return out, databaseError(tx.Commit(ctx))
}
func loadExecution(ctx context.Context, q querier, id string) (execution.Snapshot, error) {
	var s execution.Snapshot
	err := q.QueryRow(ctx, `SELECT id,workflow_id,workflow_version,status,input,correlation_id,revision,created_at,started_at,completed_at FROM workflow_executions WHERE id=$1`, id).Scan(&s.ID, &s.WorkflowID, &s.WorkflowVersion, &s.Status, &s.Input, &s.CorrelationID, &s.Revision, &s.CreatedAt, &s.StartedAt, &s.CompletedAt)
	if err != nil {
		return s, databaseError(err)
	}
	rows, err := q.Query(ctx, `SELECT id,name,task_type,position,status,attempt,max_attempts,timeout_seconds,input,output,error_code,started_at,completed_at,retry_at,deadline_at FROM step_executions WHERE execution_id=$1 ORDER BY position`, id)
	if err != nil {
		return s, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var st execution.StepSnapshot
		err = rows.Scan(&st.ID, &st.Name, &st.TaskType, &st.Position, &st.Status, &st.Attempt, &st.MaxAttempts, &st.TimeoutSeconds, &st.Input, &st.Output, &st.ErrorCode, &st.StartedAt, &st.CompletedAt, &st.RetryAt, &st.DeadlineAt)
		if err != nil {
			return s, databaseError(err)
		}
		s.Steps = append(s.Steps, st)
	}
	return s, databaseError(rows.Err())
}
func saveExecution(ctx context.Context, tx pgx.Tx, e *execution.Execution) error {
	s := e.Snapshot()
	tag, err := tx.Exec(ctx, `UPDATE workflow_executions SET status=$2,revision=revision+1,started_at=$3,completed_at=$4 WHERE id=$1 AND revision=$5`, s.ID, s.Status, s.StartedAt, s.CompletedAt, s.Revision)
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return fault.New(fault.Conflict, "revision_conflict", "The execution changed concurrently.")
	}
	for _, st := range s.Steps {
		_, err = tx.Exec(ctx, `UPDATE step_executions SET status=$3,attempt=$4,input=$5,output=$6,error_code=$7,started_at=$8,completed_at=$9,retry_at=$10,deadline_at=$11 WHERE execution_id=$1 AND id=$2`, s.ID, st.ID, st.Status, st.Attempt, nullableJSON(st.Input), nullableJSON(st.Output), st.ErrorCode, st.StartedAt, st.CompletedAt, st.RetryAt, st.DeadlineAt)
		if err != nil {
			return databaseError(err)
		}
	}
	return nil
}
func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}
func event(ctx context.Context, tx pgx.Tx, id, kind, step string, attempt int, now time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO workflow_events(execution_id,type,step_id,attempt,occurred_at) VALUES($1,$2,$3,$4,$5)`, id, kind, step, attempt, now)
	return databaseError(err)
}
func (s *Store) ListEvents(ctx context.Context, id string) ([]execution.Event, error) {
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_executions WHERE id=$1)`, id).Scan(&exists); err != nil {
		return nil, databaseError(err)
	}
	if !exists {
		return nil, fault.New(fault.NotFound, "execution_not_found", "The execution was not found.")
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,execution_id,type,step_id,attempt,occurred_at,data FROM workflow_events WHERE execution_id=$1 ORDER BY id LIMIT 10000`, id)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	events := []execution.Event{}
	for rows.Next() {
		var e execution.Event
		if err = rows.Scan(&e.ID, &e.ExecutionID, &e.Type, &e.StepID, &e.Attempt, &e.OccurredAt, &e.Data); err != nil {
			return nil, databaseError(err)
		}
		events = append(events, e)
	}
	return events, databaseError(rows.Err())
}

func (s *Store) Migrate(ctx context.Context, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no migrations in %s", dir)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// DDL can wait on disk and locks longer than ordinary application queries.
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout = '120s'; SET LOCAL lock_timeout = '30s'`); err != nil {
		return err
	}
	// Serialize deploy-time migration runners without external distributed locks.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7409271)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	for _, file := range files {
		data, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		hash := sha256.Sum256(data)
		checksum := hex.EncodeToString(hash[:])
		name := filepath.Base(file)
		var stored string
		e = tx.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, name).Scan(&stored)
		if e == nil {
			if stored != checksum {
				return fmt.Errorf("migration %s checksum changed", name)
			}
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, string(data)); e != nil {
			return fmt.Errorf("apply %s: %w", name, e)
		}
		if _, e = tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum)VALUES($1,$2)`, name, checksum); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
