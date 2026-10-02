package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ricardoverita/flowforge/internal/execution"
	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/messaging"
	"github.com/ricardoverita/flowforge/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
)

func RetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	base := time.Second * time.Duration(1<<uint(attempt-1))
	return base/2 + time.Duration(rand.Int64N(int64(base/2)+1))
}
func enqueue(ctx context.Context, tx pgx.Tx, e *execution.Execution, now time.Time) error {
	step, err := e.DispatchNext(now)
	if err != nil || step == nil {
		return err
	}
	s := e.Snapshot()
	task := messaging.Task{ID: messaging.TaskID(s.ID, step.ID, step.Attempt), ExecutionID: s.ID, WorkflowID: s.WorkflowID, StepID: step.ID, TaskType: step.TaskType, Attempt: step.Attempt, Input: step.Input, CorrelationID: s.CorrelationID, DeadlineAt: *step.DeadlineAt}
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	_, err = tx.Exec(ctx, `INSERT INTO outbox(id,execution_id,subject,payload,traceparent,created_at,available_at)VALUES($1,$2,$3,$4,$5,$6,$6)`, task.ID, s.ID, messaging.TaskPrefix+task.TaskType, payload, carrier.Get("traceparent"), now)
	if err != nil {
		return databaseError(err)
	}
	return event(ctx, tx, s.ID, "step.scheduled", step.ID, step.Attempt, now)
}

// AdvanceOne only locks an execution that has due work. The broker is never called in this transaction.
func (s *Store) AdvanceOne(ctx context.Context, now time.Time) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id, traceparent string
	err = tx.QueryRow(ctx, `SELECT e.id,e.traceparent FROM workflow_executions e WHERE e.status='pending' OR (e.status='running' AND EXISTS(SELECT 1 FROM step_executions s WHERE s.execution_id=e.id AND ((s.status='pending' AND NOT EXISTS(SELECT 1 FROM step_executions p WHERE p.execution_id=e.id AND p.position<s.position AND p.status<>'completed')) OR (s.status='retry_wait' AND s.retry_at<=$1) OR (s.status='running' AND s.deadline_at<=$1)))) ORDER BY e.created_at FOR UPDATE OF e SKIP LOCKED LIMIT 1`, now).Scan(&id, &traceparent)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, databaseError(err)
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{"traceparent": traceparent})
	ctx, span := otel.Tracer("flowforge/engine").Start(ctx, "engine.advance")
	defer span.End()
	snapshot, err := loadExecution(ctx, tx, id)
	if err != nil {
		return false, err
	}
	e, err := execution.Restore(snapshot)
	if err != nil {
		return false, err
	}
	span.SetAttributes(attribute.String("execution_id", id), attribute.String("workflow_id", snapshot.WorkflowID))
	started := snapshot.Status == execution.StatusPending
	if started {
		if err = e.Start(now); err != nil {
			return false, err
		}
		if err = event(ctx, tx, id, "execution.started", "", 0, now); err != nil {
			return false, err
		}
	}
	var timedOut *execution.StepSnapshot
	for _, st := range snapshot.Steps {
		if st.Status == execution.StepRunning && st.DeadlineAt != nil && !now.Before(*st.DeadlineAt) {
			retry := now.Add(RetryDelay(st.Attempt))
			if err = e.FailStep(st.ID, st.Attempt, "step_timeout", &retry, now); err != nil {
				return false, err
			}
			if err = event(ctx, tx, id, "step.timed_out", st.ID, st.Attempt, now); err != nil {
				return false, err
			}
			copy := st
			timedOut = &copy
		}
	}
	if err = enqueue(ctx, tx, e, now); err != nil {
		return false, err
	}
	if err = saveExecution(ctx, tx, e); err != nil {
		return false, err
	}
	if e.Snapshot().Status == execution.StatusFailed {
		if err = event(ctx, tx, id, "execution.failed", "", 0, now); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, databaseError(err)
	}
	if started {
		telemetry.Metrics.WorkflowExecutions.Add(ctx, 1)
	}
	if timedOut != nil {
		telemetry.ObserveStep(ctx, timedOut.TaskType, "failed", timedOut.StartedAt, now)
	}
	observeTerminal(ctx, snapshot, e.Snapshot(), now)
	return true, nil
}

func (s *Store) ApplyResult(ctx context.Context, r messaging.Result, now time.Time) (bool, error) {
	if err := r.Validate(); err != nil {
		return false, fault.Wrap(fault.Permanent, "result_invalid", "Worker result is invalid.", err)
	}
	ctx, span := otel.Tracer("flowforge/engine").Start(ctx, "engine.apply_result")
	defer span.End()
	span.SetAttributes(attribute.String("execution_id", r.ExecutionID), attribute.String("step_id", r.StepID))
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM workflow_executions WHERE id=$1 FOR UPDATE`, r.ExecutionID).Scan(&id)
	if err != nil {
		return false, databaseError(err)
	}
	tag, err := tx.Exec(ctx, `INSERT INTO result_inbox(task_id,execution_id,received_at)VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, r.TaskID, r.ExecutionID, now)
	if err != nil {
		return false, databaseError(err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	snapshot, err := loadExecution(ctx, tx, id)
	if err != nil {
		return false, err
	}
	var current *execution.StepSnapshot
	for _, st := range snapshot.Steps {
		if st.ID == r.StepID {
			copy := st
			current = &copy
			break
		}
	}
	// Late results are fenced by the durable attempt and deadline, not the broker deduplication window.
	if current == nil || snapshot.Status != execution.StatusRunning || current.Status != execution.StepRunning || current.Attempt != r.Attempt || current.DeadlineAt == nil || !now.Before(*current.DeadlineAt) {
		return false, databaseError(tx.Commit(ctx))
	}
	e, err := execution.Restore(snapshot)
	if err != nil {
		return false, err
	}
	status := "completed"
	if r.ErrorCode == "" {
		err = e.SucceedStep(r.StepID, r.Attempt, r.Output, now)
	} else {
		status = "failed"
		var retry *time.Time
		if !r.Permanent {
			at := now.Add(RetryDelay(r.Attempt))
			retry = &at
		}
		err = e.FailStep(r.StepID, r.Attempt, r.ErrorCode, retry, now)
	}
	if err != nil {
		return false, err
	}
	if err = event(ctx, tx, id, "step."+status, r.StepID, r.Attempt, now); err != nil {
		return false, err
	}
	if err = enqueue(ctx, tx, e, now); err != nil {
		return false, err
	}
	if err = saveExecution(ctx, tx, e); err != nil {
		return false, err
	}
	final := e.Snapshot()
	if final.Status == execution.StatusCompleted || final.Status == execution.StatusFailed {
		if err = event(ctx, tx, id, "execution."+string(final.Status), "", 0, now); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, databaseError(err)
	}
	telemetry.ObserveStep(ctx, current.TaskType, status, current.StartedAt, now)
	observeTerminal(ctx, snapshot, final, now)
	return true, nil
}
func observeTerminal(ctx context.Context, before, after execution.Snapshot, now time.Time) {
	if before.Status != after.Status && (after.Status == execution.StatusCompleted || after.Status == execution.StatusFailed) && after.StartedAt != nil {
		telemetry.Metrics.WorkflowDuration.Record(ctx, now.Sub(*after.StartedAt).Seconds(), metric.WithAttributes(attribute.String("status", string(after.Status))))
	}
}

// A bounded publish is held under the row lock. A crash after PUBACK and before commit republishes the same ID.
func (s *Store) PublishOne(ctx context.Context, publish messaging.PublishFunc, now time.Time) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id, subject, traceparent string
	var payload []byte
	var attempts int
	err = tx.QueryRow(ctx, `SELECT id,subject,payload,traceparent,attempts FROM outbox WHERE published_at IS NULL AND available_at<=$1 ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`, now).Scan(&id, &subject, &payload, &traceparent, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, databaseError(err)
	}
	pubctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	publishErr := publish(pubctx, subject, id, payload, traceparent)
	if publishErr != nil {
		_, err = tx.Exec(ctx, `UPDATE outbox SET attempts=attempts+1,available_at=$2 WHERE id=$1`, id, now.Add(RetryDelay(attempts+1)))
	} else {
		_, err = tx.Exec(ctx, `UPDATE outbox SET published_at=$2 WHERE id=$1`, id, now)
	}
	if err != nil {
		return false, databaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return false, databaseError(err)
	}
	return true, publishErr
}
