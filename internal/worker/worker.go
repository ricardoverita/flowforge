package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ricardoverita/flowforge/internal/messaging"
	"github.com/ricardoverita/flowforge/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type Worker struct {
	Broker       *messaging.Broker
	Logger       *slog.Logger
	Delay        time.Duration
	FailTask     string
	FailAttempts int
}

func (w *Worker) Run(ctx context.Context) error {
	sub, err := w.Broker.Subscribe(messaging.TaskPrefix+">", "fintech-example-worker", "FLOWFORGE_TASKS")
	if err != nil {
		return err
	}
	defer func() { _ = sub.Unsubscribe() }()
	for ctx.Err() == nil {
		msgs, e := sub.Fetch(1, nats.MaxWait(time.Second))
		if e != nil {
			if errors.Is(e, nats.ErrTimeout) {
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			w.Logger.Error("task fetch failed", "error", e)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}
		for _, msg := range msgs {
			w.handle(ctx, msg)
		}
	}
	return nil
}
func (w *Worker) handle(ctx context.Context, msg *nats.Msg) {
	ctx = messaging.MessageContext(ctx, msg)
	ctx, span := otel.Tracer("flowforge/worker").Start(ctx, "worker.execute", trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()
	var task messaging.Task
	if err := json.Unmarshal(msg.Data, &task); err != nil || task.ID != messaging.TaskID(task.ExecutionID, task.StepID, task.Attempt) || task.ExecutionID == "" || task.StepID == "" || task.Attempt < 1 || task.Attempt > 10 || task.DeadlineAt.IsZero() || !json.Valid(task.Input) || msg.Subject != messaging.TaskPrefix+task.TaskType {
		w.Logger.Warn("invalid task envelope discarded")
		_ = msg.Term()
		return
	}
	span.SetAttributes(attribute.String("execution_id", task.ExecutionID), attribute.String("step_id", task.StepID), attribute.String("task_type", task.TaskType))
	result := messaging.Result{TaskID: task.ID, ExecutionID: task.ExecutionID, StepID: task.StepID, Attempt: task.Attempt, CorrelationID: task.CorrelationID}
	if !time.Now().Before(task.DeadlineAt) {
		result.ErrorCode = "step_timeout"
	} else {
		timer := time.NewTimer(w.Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			_ = msg.Nak()
			return
		case <-timer.C:
		}
		if task.TaskType == w.FailTask && task.Attempt <= w.FailAttempts {
			result.ErrorCode = "simulated_transient_failure"
		} else {
			result.Output, result.ErrorCode = Simulate(task)
			result.Permanent = result.ErrorCode != ""
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		_ = msg.Term()
		return
	}
	pubctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err = w.Broker.Publish(pubctx, messaging.ResultSubject, "result:"+task.ID, data, ""); err != nil {
		w.Logger.Error("result publish failed", "execution_id", task.ExecutionID, "step_id", task.StepID, "error", err)
		_ = msg.NakWithDelay(time.Second)
		return
	}
	if err = msg.AckSync(nats.Context(ctx)); err != nil {
		w.Logger.Warn("task acknowledgement failed", "execution_id", task.ExecutionID, "error", err)
		return
	}
	telemetry.Metrics.WorkerJobs.Add(ctx, 1, metric.WithAttributes(attribute.String("task_type", task.TaskType)))
	w.Logger.Info("task handled", "workflow_id", task.WorkflowID, "execution_id", task.ExecutionID, "step_id", task.StepID, "attempt", task.Attempt, "correlation_id", task.CorrelationID, "trace_id", span.SpanContext().TraceID().String(), "worker", "fintech-example", "failed", result.ErrorCode != "")
}

// The example has no external effects. Real handlers must persist the execution/step business idempotency key.
func Simulate(task messaging.Task) (json.RawMessage, string) {
	var input map[string]json.RawMessage
	if err := json.Unmarshal(task.Input, &input); err != nil || input == nil {
		return nil, "unsupported_input"
	}
	values := map[string]any{"identity.verify": true, "risk.check": "low", "account.create": "account_" + task.ExecutionID, "notification.send": true}
	keys := map[string]string{"identity.verify": "identity_verified", "risk.check": "risk_level", "account.create": "account_id", "notification.send": "notification_sent"}
	value, ok := values[task.TaskType]
	if !ok {
		return nil, "unsupported_task_type"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "simulation_failed"
	}
	input[keys[task.TaskType]] = raw
	out, err := json.Marshal(input)
	if err != nil {
		return nil, "simulation_failed"
	}
	if len(out) > 1<<20 {
		return nil, "output_too_large"
	}
	return out, ""
}
