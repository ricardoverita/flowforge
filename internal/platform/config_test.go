package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func clearConfig(t *testing.T) {
	t.Helper()
	for _, key := range []string{"DATABASE_URL", "DATABASE_PASSWORD", "DATABASE_PASSWORD_FILE", "NATS_TOKEN", "NATS_TOKEN_FILE", "NATS_URL", "API_TOKEN", "HTTP_ADDR", "POLL_INTERVAL", "WORKER_DELAY", "WORKER_FAIL_ATTEMPTS", "OTEL_EXPORTER_OTLP_ENDPOINT"} {
		t.Setenv(key, "")
	}
}
func TestConfigRequiresDatabaseAndRejectsInvalidDurations(t *testing.T) {
	clearConfig(t)
	if _, err := Load("api"); err == nil {
		t.Fatal("missing database accepted")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/flowforge")
	t.Setenv("POLL_INTERVAL", "0s")
	if _, err := Load("api"); err == nil {
		t.Fatal("invalid duration accepted")
	}
}
func TestConfigReadsSecretFilesWithoutRequiringWorkerDatabase(t *testing.T) {
	clearConfig(t)
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("random-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NATS_TOKEN_FILE", path)
	c, err := Load("worker")
	if err != nil {
		t.Fatal(err)
	}
	if c.NATSToken != "random-token" || c.HTTPAddr != ":8082" {
		t.Fatal("secret or default mismatch")
	}
	t.Setenv("NATS_TOKEN", "second-token")
	if _, err = Load("worker"); err == nil {
		t.Fatal("ambiguous secret sources accepted")
	}
}
func TestTokenAuthentication(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	handler := Authenticate(next, "test-token")
	for _, test := range []struct {
		path, token string
		status      int
	}{{"/health", "", 204}, {"/ready", "", 204}, {"/api/v1/workflows", "", 401}, {"/api/v1/workflows", "Bearer wrong", 401}, {"/api/v1/workflows", "Bearer test-token", 204}} {
		t.Run(test.path+test.token, func(t *testing.T) {
			r := httptest.NewRequestWithContext(context.Background(), "GET", test.path, nil)
			r.Header.Set("Authorization", test.token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("got %d", w.Code)
			}
		})
	}
}

func TestConfigDoesNotReadCredentialsForUnrelatedServices(t *testing.T) {
	clearConfig(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/flowforge")
	t.Setenv("NATS_TOKEN_FILE", "/missing/nats-secret")
	for _, service := range []string{"api", "migrate"} {
		if _, err := Load(service); err != nil {
			t.Fatalf("%s read unrelated broker credential: %v", service, err)
		}
	}
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_PASSWORD_FILE", "/missing/db-secret")
	t.Setenv("NATS_TOKEN_FILE", "")
	if _, err := Load("worker"); err != nil {
		t.Fatalf("worker read unrelated database credential: %v", err)
	}
}
