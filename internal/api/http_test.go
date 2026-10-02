package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ricardoverita/flowforge/internal/execution"
	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/workflow"
)

type stubStore struct {
	definition workflow.Definition
	snapshot   execution.Snapshot
	err        error
	replay     bool
	created    bool
	key        string
}

func (s *stubStore) CreateWorkflow(_ context.Context, d workflow.Definition) error {
	s.definition, s.created = d, true
	return s.err
}
func (s *stubStore) ListWorkflows(context.Context) ([]workflow.Definition, error) { return nil, s.err }
func (s *stubStore) GetWorkflow(context.Context, string) (workflow.Definition, error) {
	return s.definition, s.err
}
func (s *stubStore) CreateExecution(_ context.Context, e *execution.Execution, key string) (execution.Snapshot, bool, error) {
	s.key, s.created = key, true
	if s.snapshot.ID != "" {
		return s.snapshot, s.replay, s.err
	}
	s.snapshot = e.Snapshot()
	return s.snapshot, s.replay, s.err
}
func (s *stubStore) GetExecution(context.Context, string) (execution.Snapshot, error) {
	return s.snapshot, s.err
}
func (s *stubStore) ListEvents(context.Context, string) ([]execution.Event, error) { return nil, s.err }

func testHandler(store Store, ready func(context.Context) error) http.Handler {
	return New(store, ready, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func perform(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("X-Request-ID", "test-request-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestCreateWorkflowValidatesAndAppliesDefaults(t *testing.T) {
	store := &stubStore{}
	w := perform(testHandler(store, nil), http.MethodPost, "/api/v1/workflows", `{"name":"onboarding","version":1,"steps":[{"name":"verify_identity","task_type":"identity.verify"}]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
	}
	if !store.created || store.definition.Steps[0].MaxAttempts != 3 || store.definition.Steps[0].TimeoutSeconds != 30 {
		t.Fatalf("incorrect defaults: %+v", store.definition)
	}
	if w.Header().Get("Location") != "/api/v1/workflows/"+store.definition.ID || w.Header().Get("X-Request-ID") != "test-request-1" {
		t.Fatal("missing location or request ID")
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["created_at"] == nil || response["CreatedAt"] != nil {
		t.Fatalf("domain names leaked into DTO: %s", w.Body.String())
	}
}

func TestHTTPRejectsMalformedAndUnboundedBodies(t *testing.T) {
	tests := []struct {
		name, body, contentType string
		status                  int
	}{
		{"unknown field", `{"name":"wf","version":1,"extra":true}`, "application/json", 400},
		{"unknown nested field", `{"name":"wf","version":1,"steps":[{"name":"verify","task_type":"identity.verify","secret":true}]}`, "application/json", 400},
		{"multiple objects", `{} {}`, "application/json", 400},
		{"malformed", `{"name":`, "application/json", 400},
		{"invalid UTF-8", "{\"name\":\"\xff\"}", "application/json", 400},
		{"not object", `[]`, "application/json", 400},
		{"explicit zero attempts", `{"name":"wf","version":1,"steps":[{"name":"verify","task_type":"identity.verify","max_attempts":0}]}`, "application/json", 400},
		{"wildcard subject", `{"name":"wf","version":1,"steps":[{"name":"verify","task_type":"identity.*"}]}`, "application/json", 400},
		{"wrong media type", `{}`, "text/plain", 415},
		{"body limit", `{"name":"` + strings.Repeat("a", maxRequestBytes) + `"}`, "application/json", 413},
		{"trailing whitespace limit", `{}` + strings.Repeat(" ", maxRequestBytes), "application/json", 413},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &stubStore{}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", strings.NewReader(test.body))
			r.Header.Set("Content-Type", test.contentType)
			w := httptest.NewRecorder()
			testHandler(store, nil).ServeHTTP(w, r)
			if w.Code != test.status || store.created {
				t.Fatalf("expected %d without write; got %d created=%t body=%s", test.status, w.Code, store.created, w.Body.String())
			}
		})
	}
}

func TestFaultHTTPMappingDoesNotExposeInternalCauses(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"validation", fault.New(fault.Validation, "invalid_field", "Invalid field."), 400, "invalid_field"},
		{"not found", fault.New(fault.NotFound, "workflow_not_found", "Workflow was not found."), 404, "workflow_not_found"},
		{"conflict", fault.New(fault.Conflict, "workflow_exists", "Workflow already exists."), 409, "workflow_exists"},
		{"transient", fault.Wrap(fault.Transient, "db_timeout", "db password=secret", errors.New("password=secret")), 503, "temporarily_unavailable"},
		{"infrastructure", fault.Wrap(fault.Infrastructure, "db_failed", "password=secret", errors.New("password=secret")), 500, "internal_error"},
		{"permanent", fault.New(fault.Permanent, "state_corrupt", "password=secret"), 500, "internal_error"},
		{"untyped", errors.New("password=secret"), 500, "internal_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := perform(testHandler(&stubStore{err: test.err}, nil), http.MethodGet, "/api/v1/workflows", " ")
			if w.Code != test.status || !strings.Contains(w.Body.String(), test.code) {
				t.Fatalf("unexpected mapping: %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("internal cause leaked")
			}
			if !strings.Contains(w.Body.String(), "test-request-1") {
				t.Fatal("error response lacks request ID")
			}
		})
	}
}

func TestHealthAndReadinessHaveSeparateSemantics(t *testing.T) {
	checks := 0
	h := testHandler(&stubStore{}, func(ctx context.Context) error {
		checks++
		if _, ok := ctx.Deadline(); !ok {
			t.Error("readiness dependency call has no timeout")
		}
		return errors.New("database unavailable")
	})
	if w := perform(h, http.MethodGet, "/health", ""); w.Code != http.StatusOK || checks != 0 {
		t.Fatal("liveness checked dependencies")
	}
	if w := perform(h, http.MethodGet, "/ready", ""); w.Code != http.StatusServiceUnavailable || checks != 1 {
		t.Fatalf("readiness did not check dependencies: %d checks=%d", w.Code, checks)
	}
}

func TestCreateExecutionPreservesCorrelationAndReplay(t *testing.T) {
	d, err := workflow.NewDefinition("5716685c-72be-4b0a-aacf-7a57c0866a41", "onboarding", 1, "", []workflow.StepDefinition{{ID: "verify", Name: "verify", TaskType: "identity.verify", MaxAttempts: 3, TimeoutSeconds: 30}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, replay := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "replay"}[replay], func(t *testing.T) {
			store := &stubStore{definition: d, replay: replay}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/"+d.ID+"/executions", strings.NewReader(`{"input":{"customer":"example"}}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Request-ID", "customer-request-1")
			r.Header.Set("Idempotency-Key", "onboarding-1")
			w := httptest.NewRecorder()
			testHandler(store, nil).ServeHTTP(w, r)
			expected := http.StatusCreated
			if replay {
				expected = http.StatusOK
			}
			if w.Code != expected || store.key != "onboarding-1" || store.snapshot.CorrelationID != "customer-request-1" {
				t.Fatalf("invalid execution: %d %+v", w.Code, store.snapshot)
			}
			if replay && w.Header().Get("Idempotency-Replayed") != "true" {
				t.Fatal("missing replay header")
			}
		})
	}
}

func TestUnknownExecutionHistoryReturnsNotFound(t *testing.T) {
	w := perform(testHandler(&stubStore{err: fault.New(fault.NotFound, "execution_not_found", "Execution was not found.")}, nil), http.MethodGet, "/api/v1/executions/5716685c-72be-4b0a-aacf-7a57c0866a41/events", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown history returned %d", w.Code)
	}
}

func TestInvalidPathIDIsRejectedBeforePersistence(t *testing.T) {
	w := perform(testHandler(&stubStore{err: errors.New("must not reach store")}, nil), http.MethodGet, "/api/v1/executions/not-a-uuid", "")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "resource_id_invalid") {
		t.Fatalf("invalid ID reached persistence: %d %s", w.Code, w.Body.String())
	}
}

func TestListUsesEmptyArrayAndResponseHeaders(t *testing.T) {
	w := perform(testHandler(&stubStore{}, nil), http.MethodGet, "/api/v1/workflows", "")
	if strings.TrimSpace(w.Body.String()) != "[]" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid empty list or security headers: %s", w.Body.String())
	}
}

func TestDocumentationServesSpecificationAndRestrictedCSP(t *testing.T) {
	h := testHandler(&stubStore{}, nil)
	w := perform(h, http.MethodGet, "/openapi.yaml", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "/api/v1/executions/{id}/events:") {
		t.Fatal("specification is unavailable")
	}
	w = perform(h, http.MethodGet, "/docs", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self' https://unpkg.com") || strings.Contains(w.Header().Get("Content-Security-Policy"), "unsafe-inline") {
		t.Fatal("documentation CSP is invalid")
	}
	w = perform(h, http.MethodGet, "/docs/init.js", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "SwaggerUIBundle") {
		t.Fatal("documentation initializer is unavailable")
	}
}
