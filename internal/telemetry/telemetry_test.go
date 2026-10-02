package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestSetupExportsTracesToSignalPath(t *testing.T) {
	previousMeter, previousTracer := otel.GetMeterProvider(), otel.GetTracerProvider()
	previousPropagator, previousMetrics := otel.GetTextMapPropagator(), Metrics
	t.Cleanup(func() {
		otel.SetMeterProvider(previousMeter)
		otel.SetTracerProvider(previousTracer)
		otel.SetTextMapPropagator(previousPropagator)
		Metrics = previousMetrics
	})
	for _, tc := range []struct {
		name, basePath, tracePath string
	}{
		{"base endpoint", "", "/v1/traces"},
		{"trailing slash", "/", "/v1/traces"},
		{"proxy prefix", "/otel", "/otel/v1/traces"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan string, 1)
			collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-protobuf" {
					t.Errorf("unexpected export request: method=%s, content type=%s", r.Method, r.Header.Get("Content-Type"))
				}
				body, err := io.ReadAll(r.Body)
				if err != nil || len(body) == 0 {
					t.Errorf("missing trace payload: size=%d, error=%v", len(body), err)
				}
				received <- r.URL.Path
				w.Header().Set("Content-Type", "application/x-protobuf")
				w.WriteHeader(http.StatusOK)
			}))
			defer collector.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, shutdown, err := Setup(ctx, "flowforge-test", collector.URL+tc.basePath)
			if err != nil {
				t.Fatal(err)
			}
			_, span := otel.Tracer("flowforge-test").Start(ctx, "workflow.execution")
			span.End()
			if err = shutdown(ctx); err != nil {
				t.Fatal("flush trace export:", err)
			}
			select {
			case path := <-received:
				if path != tc.tracePath {
					t.Fatalf("trace path=%q, want %q", path, tc.tracePath)
				}
			case <-ctx.Done():
				t.Fatal("collector received no trace export")
			}
		})
	}
}
