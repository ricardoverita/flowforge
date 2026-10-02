//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoverita/flowforge/internal/execution"
	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/messaging"
	"github.com/ricardoverita/flowforge/internal/workflow"
)

func integrationStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required; run make test-integration")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + uuid.New().String()[:8]
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); _, _ = admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); admin.Close() })
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "../../migrations")
	if err = s.Migrate(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx, dir); err != nil {
		t.Fatal("migration replay:", err)
	}
	return s
}
func fixture(t *testing.T, s *Store, steps int) (workflow.Definition, execution.Snapshot) {
	t.Helper()
	now := time.Now().UTC()
	defs := []workflow.StepDefinition{}
	for i := range steps {
		defs = append(defs, workflow.StepDefinition{ID: fmt.Sprintf("step_%d", i), Name: fmt.Sprintf("step_%d", i), TaskType: "identity.verify", MaxAttempts: 3, TimeoutSeconds: 1})
	}
	d, err := workflow.NewDefinition(uuid.NewString(), "onboarding", 1, "", defs, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateWorkflow(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	e, err := execution.New(uuid.NewString(), d, json.RawMessage(`{"customer":"demo"}`), "request_1", now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := s.CreateExecution(context.Background(), e, "")
	if err != nil {
		t.Fatal(err)
	}
	return d, snapshot
}
func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestWorkflowPersistenceAndVersionConflict(t *testing.T) {
	s := integrationStore(t)
	d, _ := fixture(t, s, 2)
	loaded, err := s.GetWorkflow(context.Background(), d.ID)
	if err != nil || len(loaded.Steps) != 2 || loaded.Name != d.Name {
		t.Fatal(loaded, err)
	}
	d.ID = uuid.NewString()
	if err = s.CreateWorkflow(context.Background(), d); !fault.Is(err, fault.Conflict) {
		t.Fatal("expected version conflict", err)
	}
}
func TestConcurrentIdempotentCreation(t *testing.T) {
	s := integrationStore(t)
	d, _ := fixture(t, s, 1)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make(chan string, 16)
	errs := make(chan error, 16)
	for range 16 {
		wg.Go(func() {
			e, err := execution.New(uuid.NewString(), d, json.RawMessage(`{"a":1,"b":2}`), "req", time.Now())
			if err != nil {
				errs <- err
				return
			}
			snapshot, _, err := s.CreateExecution(ctx, e, "same-key")
			if err != nil {
				errs <- err
				return
			}
			ids <- snapshot.ID
		})
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("same key created different executions")
		}
	}
	e, _ := execution.New(uuid.NewString(), d, json.RawMessage(`{"b":2,"a":1}`), "different-request", time.Now())
	out, replayed, err := s.CreateExecution(ctx, e, "same-key")
	if err != nil || !replayed || out.ID != first {
		t.Fatal("canonical replay failed", err)
	}
	e, _ = execution.New(uuid.NewString(), d, json.RawMessage(`{"a":2}`), "req", time.Now())
	if _, _, err = s.CreateExecution(ctx, e, "same-key"); !fault.Is(err, fault.Conflict) {
		t.Fatal("different request must conflict", err)
	}
	if count(t, s, "workflow_executions") != 2 {
		t.Fatal("duplicate execution persisted")
	}
}
func TestConcurrentSchedulingDispatchesOnce(t *testing.T) {
	s := integrationStore(t)
	_, ex := fixture(t, s, 1)
	ctx := context.Background()
	now := ex.CreatedAt.Add(time.Millisecond)
	var done atomic.Int64
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			ok, err := s.AdvanceOne(ctx, now)
			if err != nil {
				t.Error(err)
			}
			if ok {
				done.Add(1)
			}
		})
	}
	wg.Wait()
	if done.Load() != 1 || count(t, s, "outbox") != 1 {
		t.Fatal("concurrent scheduling duplicated work")
	}
}
func TestResultCommitSchedulesNextAndDeduplicates(t *testing.T) {
	s := integrationStore(t)
	_, ex := fixture(t, s, 2)
	ctx := context.Background()
	now := ex.CreatedAt.Add(time.Millisecond)
	if _, err := s.AdvanceOne(ctx, now); err != nil {
		t.Fatal(err)
	}
	r := messaging.Result{TaskID: messaging.TaskID(ex.ID, "step_0", 1), ExecutionID: ex.ID, StepID: "step_0", Attempt: 1, Output: json.RawMessage(`{"verified":true}`)}
	if ok, err := s.ApplyResult(ctx, r, now.Add(time.Millisecond)); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := s.ApplyResult(ctx, r, now.Add(2*time.Millisecond)); err != nil || ok {
		t.Fatal("duplicate applied", ok, err)
	}
	loaded, err := s.GetExecution(ctx, ex.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Steps[0].Status != execution.StepCompleted || loaded.Steps[1].Status != execution.StepRunning || count(t, s, "outbox") != 2 || count(t, s, "result_inbox") != 1 {
		t.Fatal("result and next task were not atomic")
	}
	r.StepID = "step_1"
	r.TaskID = messaging.TaskID(ex.ID, r.StepID, 1)
	if _, err = s.ApplyResult(ctx, r, now.Add(3*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	loaded, err = s.GetExecution(ctx, ex.ID)
	if err != nil || loaded.Status != execution.StatusCompleted || loaded.CompletedAt == nil {
		t.Fatal("workflow did not complete", err)
	}
	events, err := s.ListEvents(ctx, ex.ID)
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].Type != "execution.completed" {
		t.Fatal("missing completion event")
	}
}
func TestTimeoutRecoveryFencesLateResults(t *testing.T) {
	s := integrationStore(t)
	_, ex := fixture(t, s, 1)
	ctx := context.Background()
	now := ex.CreatedAt.Add(time.Millisecond)
	_, err := s.AdvanceOne(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdvanceOne(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	loaded, _ := s.GetExecution(ctx, ex.ID)
	if loaded.Steps[0].Status != execution.StepRetryWait {
		t.Fatal("timeout did not schedule retry")
	}
	r := messaging.Result{TaskID: messaging.TaskID(ex.ID, "step_0", 1), ExecutionID: ex.ID, StepID: "step_0", Attempt: 1, Output: json.RawMessage(`{}`)}
	if ok, err := s.ApplyResult(ctx, r, now.Add(2*time.Second)); err != nil || ok {
		t.Fatal("late result changed state", err)
	}
	if _, err = s.AdvanceOne(ctx, *loaded.Steps[0].RetryAt); err != nil {
		t.Fatal(err)
	}
	r.Attempt = 2
	r.TaskID = messaging.TaskID(ex.ID, r.StepID, 2)
	if ok, err := s.ApplyResult(ctx, r, loaded.Steps[0].RetryAt.Add(time.Millisecond)); err != nil || !ok {
		t.Fatal("retry did not complete", err)
	}
}
func TestPermanentFailureTerminatesWithoutRetry(t *testing.T) {
	s := integrationStore(t)
	_, ex := fixture(t, s, 2)
	ctx := context.Background()
	now := ex.CreatedAt.Add(time.Millisecond)
	_, _ = s.AdvanceOne(ctx, now)
	r := messaging.Result{TaskID: messaging.TaskID(ex.ID, "step_0", 1), ExecutionID: ex.ID, StepID: "step_0", Attempt: 1, ErrorCode: "denied", Permanent: true}
	if _, err := s.ApplyResult(ctx, r, now.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	loaded, _ := s.GetExecution(ctx, ex.ID)
	if loaded.Status != execution.StatusFailed || count(t, s, "outbox") != 1 {
		t.Fatal("permanent failure scheduled more work")
	}
}
func TestOutboxSurvivesPublisherFailure(t *testing.T) {
	s := integrationStore(t)
	_, ex := fixture(t, s, 1)
	ctx := context.Background()
	now := ex.CreatedAt.Add(time.Millisecond)
	_, _ = s.AdvanceOne(ctx, now)
	var firstID string
	failed := func(_ context.Context, _, id string, _ []byte, _ string) error {
		firstID = id
		return errors.New("broker unavailable")
	}
	if ok, err := s.PublishOne(ctx, failed, now); err == nil || !ok {
		t.Fatal("publish error missing")
	}
	var published bool
	if err := s.Pool.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM outbox`).Scan(&published); err != nil || published {
		t.Fatal("failed publish marked sent", err)
	}
	succeeded := func(_ context.Context, _, id string, _ []byte, _ string) error {
		if id != firstID {
			t.Fatal("retry changed publication identity")
		}
		return nil
	}
	if ok, err := s.PublishOne(ctx, succeeded, now.Add(time.Minute)); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := s.PublishOne(ctx, succeeded, now.Add(time.Minute)); err != nil || ok {
		t.Fatal("published row selected twice", err)
	}
}
