package platform

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ricardoverita/flowforge/internal/api"
	"github.com/ricardoverita/flowforge/internal/engine"
	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/messaging"
	"github.com/ricardoverita/flowforge/internal/postgres"
	"github.com/ricardoverita/flowforge/internal/telemetry"
	"github.com/ricardoverita/flowforge/internal/worker"
)

func Main(service string) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(); err != nil {
			os.Exit(1)
		}
		return
	}
	if err := run(service, logger); err != nil {
		var classified *fault.Error
		if errors.As(err, &classified) {
			logger.Error("service stopped", "service", service, "error_kind", classified.Kind, "error_code", classified.Code)
		} else {
			logger.Error("service stopped", "service", service, "error", err)
		}
		os.Exit(1)
	}
}
func run(service string, logger *slog.Logger) error {
	c, err := Load(service)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	metrics, shutdown, err := telemetry.Setup(startup, c.ServiceName, c.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer func() {
		flush, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if e := shutdown(flush); e != nil {
			logger.Warn("telemetry flush failed", "error", e)
		}
	}()
	var store *postgres.Store
	if service != "worker" {
		store, err = postgres.Open(startup, c.DatabaseURL)
		if err != nil {
			return fault.Wrap(fault.Infrastructure, "database_startup_failed", "Database startup failed.", err)
		}
		defer store.Close()
	}
	if service == "migrate" {
		return store.Migrate(ctx, "migrations")
	}
	var broker *messaging.Broker
	if service != "api" {
		broker, err = messaging.Open(c.NATSURL, c.NATSToken, c.ServiceName, logger)
		if err != nil {
			return fault.Wrap(fault.Infrastructure, "broker_startup_failed", "Broker startup failed.", err)
		}
		defer broker.Close()
	}
	ready := func(probe context.Context) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if store != nil {
			if e := store.Ready(probe); e != nil {
				return e
			}
			var version int
			if e := store.Pool.QueryRow(probe, `SELECT COUNT(*) FROM schema_migrations`).Scan(&version); e != nil || version < 1 {
				return fmt.Errorf("database migrations unavailable")
			}
		}
		if broker != nil {
			return broker.Ready(probe)
		}
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics)
	if service == "api" {
		mux.Handle("/", Authenticate(api.New(store, ready, logger), c.APIToken))
	} else {
		mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
		mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
			probe, done := context.WithTimeout(r.Context(), 2*time.Second)
			defer done()
			if ctx.Err() != nil || ready(probe) != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		})
	}
	server := &http.Server{Addr: c.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	runctx, halt := context.WithCancel(ctx)
	defer halt()
	errs := make(chan error, 2)
	active := 0
	if service == "engine" {
		active++
		go func() {
			errs <- (&engine.Engine{Store: store, Broker: broker, Logger: logger, Interval: c.PollInterval}).Run(runctx)
		}()
	}
	if service == "worker" {
		active++
		go func() {
			errs <- (&worker.Worker{Broker: broker, Logger: logger, Delay: c.WorkerDelay, FailTask: c.WorkerFailTask, FailAttempts: c.WorkerFailAttempts}).Run(runctx)
		}()
	}
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errs <- err
	}()
	active++
	logger.Info("service started", "service", c.ServiceName, "http_addr", c.HTTPAddr)
	var cause error
	select {
	case <-ctx.Done():
	case cause = <-errs:
		active--
	}
	halt()
	shutdownCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	if err = server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		cause = errors.Join(cause, err)
	}
	for active > 0 {
		select {
		case e := <-errs:
			cause = errors.Join(cause, e)
			active--
		case <-shutdownCtx.Done():
			return errors.Join(cause, shutdownCtx.Err())
		}
	}
	logger.Info("service stopped cleanly", "service", c.ServiceName)
	return cause
}
func Authenticate(next http.Handler, token string) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/docs" || r.URL.Path == "/docs/init.js" || r.URL.Path == "/openapi.yaml" {
			next.ServeHTTP(w, r)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			requestID := r.Header.Get("X-Request-ID")
			valid := len(requestID) > 0 && len(requestID) <= 128
			for _, b := range []byte(requestID) {
				if b < 33 || b > 126 {
					valid = false
					break
				}
			}
			if !valid {
				requestID = uuid.NewString()
			}
			w.Header().Set("X-Request-ID", requestID)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprintf(w, `{"error":{"code":"unauthorized","message":"Authentication is required.","request_id":%q}}`, requestID)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func healthcheck() error {
	addr := env("HTTP_ADDR", ":8080")
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/ready")
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness status %d", response.StatusCode)
	}
	return nil
}
