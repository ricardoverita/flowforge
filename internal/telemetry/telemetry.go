package telemetry

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Instruments struct {
	WorkflowExecutions metric.Int64Counter
	WorkflowDuration   metric.Float64Histogram
	StepExecutions     metric.Int64Counter
	StepDuration       metric.Float64Histogram
	StepFailures       metric.Int64Counter
	WorkerJobs         metric.Int64Counter
}

var Metrics Instruments

func init() {
	m := otel.Meter("github.com/ricardoverita/flowforge")
	Metrics.WorkflowExecutions, _ = m.Int64Counter("workflow_executions")
	Metrics.WorkflowDuration, _ = m.Float64Histogram("workflow_execution_duration_seconds")
	Metrics.StepExecutions, _ = m.Int64Counter("step_executions")
	Metrics.StepDuration, _ = m.Float64Histogram("step_execution_duration_seconds")
	Metrics.StepFailures, _ = m.Int64Counter("step_failures")
	Metrics.WorkerJobs, _ = m.Int64Counter("worker_jobs")
}

func Setup(ctx context.Context, service, endpoint string) (http.Handler, func(context.Context) error, error) {
	registry := prometheus.NewRegistry()
	reader, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		return nil, nil, err
	}
	res := resource.NewWithAttributes("", attribute.String("service.name", service))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithResource(res))
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if endpoint != "" {
		exporter, e := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
		if e != nil {
			return nil, nil, e
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	tp := sdktrace.NewTracerProvider(options...)
	otel.SetMeterProvider(mp)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	m := mp.Meter("github.com/ricardoverita/flowforge")
	Metrics.WorkflowExecutions, err = m.Int64Counter("workflow_executions", metric.WithDescription("Committed workflow starts observed by this process"))
	if err != nil {
		return nil, nil, err
	}
	Metrics.WorkflowDuration, err = m.Float64Histogram("workflow_execution_duration_seconds", metric.WithUnit("s"))
	if err != nil {
		return nil, nil, err
	}
	Metrics.StepExecutions, err = m.Int64Counter("step_executions")
	if err != nil {
		return nil, nil, err
	}
	Metrics.StepDuration, err = m.Float64Histogram("step_execution_duration_seconds", metric.WithUnit("s"))
	if err != nil {
		return nil, nil, err
	}
	Metrics.StepFailures, err = m.Int64Counter("step_failures")
	if err != nil {
		return nil, nil, err
	}
	Metrics.WorkerJobs, err = m.Int64Counter("worker_jobs")
	if err != nil {
		return nil, nil, err
	}
	shutdown := func(ctx context.Context) error { return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx)) }
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), shutdown, nil
}

func ObserveStep(ctx context.Context, taskType, status string, started *time.Time, now time.Time) {
	attrs := metric.WithAttributes(attribute.String("task_type", taskType), attribute.String("status", status))
	Metrics.StepExecutions.Add(ctx, 1, attrs)
	if started != nil {
		Metrics.StepDuration.Record(ctx, now.Sub(*started).Seconds(), attrs)
	}
	if status == "failed" {
		Metrics.StepFailures.Add(ctx, 1, attrs)
	}
}
