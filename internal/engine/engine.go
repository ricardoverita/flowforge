package engine

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/messaging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Store exposes atomic workflow operations; transaction details belong to the persistence adapter.
type Store interface {
	AdvanceOne(context.Context, time.Time) (bool, error)
	ApplyResult(context.Context, messaging.Result, time.Time) (bool, error)
	PublishOne(context.Context, messaging.PublishFunc, time.Time) (bool, error)
}

type Engine struct {
	Store    Store
	Broker   *messaging.Broker
	Logger   *slog.Logger
	Interval time.Duration
}

func (e *Engine) Run(ctx context.Context) error {
	sub, err := e.Broker.Subscribe(messaging.ResultSubject, "flowforge-engine", "FLOWFORGE_RESULTS")
	if err != nil {
		return err
	}
	defer func() { _ = sub.Unsubscribe() }()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		e.poll(ctx, "scheduler", func(ctx context.Context) (bool, error) { return e.Store.AdvanceOne(ctx, time.Now().UTC()) })
	}()
	go func() {
		defer wg.Done()
		e.poll(ctx, "outbox", func(ctx context.Context) (bool, error) {
			return e.Store.PublishOne(ctx, e.Broker.Publish, time.Now().UTC())
		})
	}()
	defer wg.Wait()
	for ctx.Err() == nil {
		msgs, err := sub.Fetch(1, nats.MaxWait(time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			e.Logger.Error("result fetch failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(e.Interval):
			}
			continue
		}
		for _, msg := range msgs {
			e.handle(ctx, msg)
		}
	}
	return nil
}
func (e *Engine) poll(ctx context.Context, operation string, f func(context.Context) (bool, error)) {
	ticker := time.NewTicker(e.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for range 32 {
				worked, err := f(ctx)
				if err != nil {
					if ctx.Err() == nil {
						e.Logger.Error("engine operation failed", "operation", operation, "error_kind", fault.KindOf(err))
					}
					break
				}
				if !worked {
					break
				}
			}
		}
	}
}
func (e *Engine) handle(ctx context.Context, msg *nats.Msg) {
	ctx = messaging.MessageContext(ctx, msg)
	ctx, span := otel.Tracer("flowforge/messaging").Start(ctx, "nats.result", trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()
	var result messaging.Result
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		e.Logger.Warn("invalid result envelope discarded")
		_ = msg.Term()
		return
	}
	applied, err := e.Store.ApplyResult(ctx, result, time.Now().UTC())
	if err != nil {
		if fault.Is(err, fault.Permanent) || fault.Is(err, fault.NotFound) || fault.Is(err, fault.Validation) {
			e.Logger.Warn("invalid worker result discarded", "execution_id", result.ExecutionID, "step_id", result.StepID, "error_kind", fault.KindOf(err))
			_ = msg.Term()
			return
		}
		e.Logger.Error("result persistence failed", "execution_id", result.ExecutionID, "step_id", result.StepID, "error_kind", fault.KindOf(err))
		_ = msg.NakWithDelay(time.Second)
		return
	}
	if err = msg.AckSync(nats.Context(ctx)); err != nil {
		e.Logger.Warn("result acknowledgement failed", "execution_id", result.ExecutionID, "error", err)
		return
	}
	if applied {
		e.Logger.Info("step result committed", "execution_id", result.ExecutionID, "step_id", result.StepID, "attempt", result.Attempt, "correlation_id", result.CorrelationID, "trace_id", span.SpanContext().TraceID().String(), "failed", result.ErrorCode != "")
	}
}
