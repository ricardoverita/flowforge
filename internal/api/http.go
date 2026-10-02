package api

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ricardoverita/flowforge/internal/fault"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

const maxRequestBytes = 1 << 20

//go:embed openapi.yaml
var openAPISpec []byte

type handler struct {
	app    *application
	ready  func(context.Context) error
	logger *slog.Logger
}

type requestIDKey struct{}

func New(store Store, ready func(context.Context) error, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &handler{app: newApplication(store), ready: ready, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/workflows", h.createWorkflow)
	mux.HandleFunc("GET /api/v1/workflows", h.listWorkflows)
	mux.HandleFunc("GET /api/v1/workflows/{id}", h.getWorkflow)
	mux.HandleFunc("POST /api/v1/workflows/{id}/executions", h.createExecution)
	mux.HandleFunc("GET /api/v1/executions/{id}", h.getExecution)
	mux.HandleFunc("GET /api/v1/executions/{id}/events", h.listEvents)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /ready", h.readiness)
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openAPISpec)
	})
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' https://unpkg.com; style-src https://unpkg.com; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'")
		_, _ = io.WriteString(w, swaggerPage)
	})
	// The initializer is served from the same origin so the CSP does not need inline scripts.
	mux.HandleFunc("GET /docs/init.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = io.WriteString(w, `window.onload = function () { SwaggerUIBundle({url: '/openapi.yaml', dom_id: '#swagger-ui', persistAuthorization: false}); };`)
	})
	return otelhttp.NewHandler(h.middleware(mux), "flowforge.http")
}

func (h *handler) createWorkflow(w http.ResponseWriter, r *http.Request) {
	var dto workflowRequest
	if err := decodeJSON(w, r, &dto); err != nil {
		h.writeError(w, r, err)
		return
	}
	command := createWorkflowCommand{Name: dto.Name, Version: dto.Version, Description: dto.Description}
	for _, step := range dto.Steps {
		command.Steps = append(command.Steps, createStepCommand(step))
	}
	definition, err := h.app.createWorkflow(r.Context(), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/workflows/"+definition.ID)
	writeJSON(w, http.StatusCreated, workflowDTO(definition))
}

func (h *handler) listWorkflows(w http.ResponseWriter, r *http.Request) {
	definitions, err := h.app.store.ListWorkflows(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	items := make([]workflowResponse, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, workflowDTO(definition))
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) getWorkflow(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	definition, err := h.app.store.GetWorkflow(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, workflowDTO(definition))
}

func (h *handler) createExecution(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var dto executionRequest
	if err := decodeJSON(w, r, &dto); err != nil {
		h.writeError(w, r, err)
		return
	}
	correlation, _ := r.Context().Value(requestIDKey{}).(string)
	snapshot, replay, err := h.app.createExecution(r.Context(), id, dto.Input, correlation, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/executions/"+snapshot.ID)
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, status, executionDTO(snapshot))
}

func (h *handler) getExecution(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	snapshot, err := h.app.store.GetExecution(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, executionDTO(snapshot))
}

func (h *handler) listEvents(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	// Checking the parent prevents an unknown execution from appearing as an empty history.
	if _, err := h.app.store.GetExecution(r.Context(), id); err != nil {
		h.writeError(w, r, err)
		return
	}
	events, err := h.app.store.ListEvents(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	items := make([]eventResponse, 0, len(events))
	for _, event := range events {
		items = append(items, eventResponse(event))
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.ready != nil {
		if err := h.ready(ctx); err != nil {
			h.writeError(w, r, fault.Wrap(fault.Transient, "dependencies_unavailable", "Required dependencies are unavailable.", err))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return fault.New(fault.Validation, "content_type_invalid", "Content-Type must be application/json.")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fault.New(fault.Validation, "request_too_large", "Request body must be no larger than 1 MiB.")
		}
		return fault.New(fault.Validation, "request_body_unreadable", "Request body could not be read.")
	}
	if !utf8.Valid(body) {
		return fault.New(fault.Validation, "request_json_invalid", "Request body must contain valid UTF-8 JSON.")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fault.New(fault.Validation, "request_json_invalid", "Request body must contain one valid JSON object with only documented fields.")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fault.New(fault.Validation, "request_json_invalid", "Request body must contain exactly one JSON object.")
	}
	return nil
}

func pathID(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); len(id) != 36 || err != nil {
		return "", fault.New(fault.Validation, "resource_id_invalid", "Resource ID must be a UUID.")
	}
	return id, nil
}

func (h *handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "The request could not be completed."
	var typed *fault.Error
	if errors.As(err, &typed) {
		switch typed.Kind {
		case fault.Validation:
			status, code, message = http.StatusBadRequest, typed.Code, typed.Message
		case fault.NotFound:
			status, code, message = http.StatusNotFound, typed.Code, typed.Message
		case fault.Conflict:
			status, code, message = http.StatusConflict, typed.Code, typed.Message
		case fault.Transient:
			status, code, message = http.StatusServiceUnavailable, "temporarily_unavailable", "The service is temporarily unavailable."
		}
	}
	if code == "request_too_large" {
		status = http.StatusRequestEntityTooLarge
	}
	if code == "content_type_invalid" {
		status = http.StatusUnsupportedMediaType
	}
	requestID, _ := r.Context().Value(requestIDKey{}).(string)
	if status >= 500 {
		// Internal causes and dynamic payloads may contain secrets; log only stable classifications.
		h.logger.ErrorContext(r.Context(), "request failed", "request_id", requestID, "error_kind", fault.KindOf(err), "error_code", code)
	}
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "1")
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *handler) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if !validHeaderValue(requestID, false) {
			requestID = uuid.NewString()
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID))
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		started := time.Now()
		recorder := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if recovered := recover(); recovered != nil {
				h.writeError(recorder, r, fault.New(fault.Infrastructure, "handler_panic", "Request handler failed."))
			}
			traceID := trace.SpanContextFromContext(r.Context()).TraceID().String()
			h.logger.InfoContext(r.Context(), "http request", "method", r.Method, "route", r.Pattern, "status", recorder.status, "duration_ms", time.Since(started).Milliseconds(), "request_id", requestID, "trace_id", traceID)
		}()
		next.ServeHTTP(recorder, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = status, true
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *statusWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

const swaggerPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>FlowForge API</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.29.0/swagger-ui.css"></head>
<body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5.29.0/swagger-ui-bundle.js"></script><script src="/docs/init.js"></script></body></html>`
