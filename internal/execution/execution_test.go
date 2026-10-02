package execution

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/workflow"
)

var testTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func newTestExecution(t *testing.T) *Execution {
	t.Helper()
	d, err := workflow.NewDefinition("workflow-1", "onboarding", 1, "", []workflow.StepDefinition{
		{ID: "verify", Name: "verify", TaskType: "identity.verify", MaxAttempts: 2, TimeoutSeconds: 30},
		{ID: "account", Name: "account", TaskType: "account.create", MaxAttempts: 2, TimeoutSeconds: 30},
	}, testTime)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New("execution-1", d, json.RawMessage(`{"customer":"example"}`), "request-1", testTime)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestSequentialLifecycle(t *testing.T) {
	e := newTestExecution(t)
	if next, err := e.DispatchNext(testTime); err != nil || next != nil {
		t.Fatalf("pending execution dispatched: %v %v", next, err)
	}
	if err := e.Start(testTime); err != nil {
		t.Fatal(err)
	}
	first, err := e.DispatchNext(testTime)
	if err != nil || first == nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if first.ID != "verify" || first.Attempt != 1 || !first.DeadlineAt.Equal(testTime.Add(30*time.Second)) {
		t.Fatalf("invalid attempt: %+v", first)
	}
	if next, err := e.DispatchNext(testTime); err != nil || next != nil {
		t.Fatal("dispatched concurrent sequential work")
	}
	if err := e.SucceedStep(first.ID, first.Attempt, json.RawMessage(`{"verified":true}`), testTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	second, err := e.DispatchNext(testTime.Add(2 * time.Second))
	if err != nil || second == nil || second.ID != "account" || string(second.Input) != `{"verified":true}` {
		t.Fatalf("output did not flow to next step: %+v %v", second, err)
	}
	if err := e.SucceedStep(second.ID, second.Attempt, json.RawMessage(`{"account":"created"}`), testTime.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	snapshot := e.Snapshot()
	if snapshot.Status != StatusCompleted || snapshot.CompletedAt == nil || snapshot.StartedAt == nil {
		t.Fatalf("execution did not complete: %+v", snapshot)
	}
	if _, err := Restore(snapshot); err != nil {
		t.Fatalf("cannot restore completed execution: %v", err)
	}
}

func TestRetryAndStaleResult(t *testing.T) {
	e := newTestExecution(t)
	if err := e.Start(testTime); err != nil {
		t.Fatal(err)
	}
	step, err := e.DispatchNext(testTime)
	if err != nil {
		t.Fatal(err)
	}
	retryAt := testTime.Add(5 * time.Second)
	if err := e.FailStep(step.ID, 1, "provider_unavailable", &retryAt, testTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	beforeDuplicate := e.Snapshot()
	if err := e.FailStep(step.ID, 1, "provider_unavailable", &retryAt, testTime.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeDuplicate, e.Snapshot()) {
		t.Fatal("duplicate failure mutated retry state")
	}
	if _, err := Restore(e.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if next, err := e.DispatchNext(retryAt.Add(-time.Nanosecond)); err != nil || next != nil {
		t.Fatal("retry dispatched before its timer")
	}
	secondAttempt, err := e.DispatchNext(retryAt)
	if err != nil || secondAttempt == nil || secondAttempt.Attempt != 2 {
		t.Fatalf("retry missing: %+v %v", secondAttempt, err)
	}
	if err := e.SucceedStep(step.ID, 1, json.RawMessage(`{}`), retryAt); !fault.Is(err, fault.Conflict) {
		t.Fatalf("stale success accepted: %v", err)
	}
	if err := e.FailStep(step.ID, 1, "timeout", nil, retryAt); !fault.Is(err, fault.Conflict) {
		t.Fatalf("stale failure accepted: %v", err)
	}
	nextRetry := retryAt.Add(time.Second)
	if err := e.FailStep(step.ID, 2, "provider_unavailable", &nextRetry, retryAt); err != nil {
		t.Fatal(err)
	}
	if e.Snapshot().Status != StatusFailed || e.Snapshot().Steps[0].Status != StepFailed {
		t.Fatal("attempt exhaustion did not fail execution")
	}
	if _, err := Restore(e.Snapshot()); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateSuccessDoesNotOverwriteResult(t *testing.T) {
	e := newTestExecution(t)
	if err := e.Start(testTime); err != nil {
		t.Fatal(err)
	}
	step, err := e.DispatchNext(testTime)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SucceedStep(step.ID, 1, json.RawMessage(`{"ok":true}`), testTime); err != nil {
		t.Fatal(err)
	}
	before := e.Snapshot()
	if err := e.SucceedStep(step.ID, 1, json.RawMessage(`{"ok":false}`), testTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatal("duplicate success overwrote accepted output")
	}
}

func TestLifecycleTransitions(t *testing.T) {
	states := []Status{StatusPending, StatusRunning, StatusWaiting, StatusCompleted, StatusFailed, StatusCancelled}
	allowed := map[[2]Status]bool{
		{StatusPending, StatusRunning}: true, {StatusPending, StatusCancelled}: true,
		{StatusRunning, StatusWaiting}: true, {StatusRunning, StatusCancelled}: true,
		{StatusWaiting, StatusRunning}: true, {StatusWaiting, StatusCancelled}: true,
	}
	for _, from := range states {
		for _, to := range states {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				e := newTestExecution(t)
				e.state.Status = from
				before := e.Snapshot()
				err := e.Transition(to, testTime)
				if allowed[[2]Status{from, to}] {
					if err != nil || e.Snapshot().Status != to {
						t.Fatalf("valid transition rejected: %v", err)
					}
				} else {
					if !fault.Is(err, fault.Conflict) {
						t.Fatalf("invalid transition accepted: %v", err)
					}
					if !reflect.DeepEqual(before, e.Snapshot()) {
						t.Fatal("rejected transition mutated state")
					}
				}
			})
		}
	}
}

func TestCancelPreventsLateResults(t *testing.T) {
	e := newTestExecution(t)
	if err := e.Start(testTime); err != nil {
		t.Fatal(err)
	}
	step, err := e.DispatchNext(testTime)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Transition(StatusWaiting, testTime); !fault.Is(err, fault.Conflict) {
		t.Fatalf("waiting accepted active work: %v", err)
	}
	if err := e.Transition(StatusCancelled, testTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, item := range e.Snapshot().Steps {
		if item.Status != StepCancelled || item.CompletedAt == nil || item.DeadlineAt != nil {
			t.Fatalf("uncancelled work: %+v", item)
		}
	}
	if err := e.SucceedStep(step.ID, 1, json.RawMessage(`{}`), testTime.Add(2*time.Second)); !fault.Is(err, fault.Conflict) {
		t.Fatalf("late result accepted: %v", err)
	}
	if _, err := Restore(e.Snapshot()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotsDoNotExposeMutableState(t *testing.T) {
	e := newTestExecution(t)
	if err := e.Start(testTime); err != nil {
		t.Fatal(err)
	}
	step, err := e.DispatchNext(testTime)
	if err != nil {
		t.Fatal(err)
	}
	step.Input[0] = 'x'
	*step.DeadlineAt = testTime
	snapshot := e.Snapshot()
	snapshot.Input[0] = 'x'
	snapshot.Steps[0].Status = StepCompleted
	*snapshot.StartedAt = time.Time{}
	actual := e.Snapshot()
	if actual.Input[0] != '{' || actual.Steps[0].Input[0] != '{' || actual.Steps[0].Status != StepRunning || actual.StartedAt.IsZero() || actual.Steps[0].DeadlineAt.Equal(testTime) {
		t.Fatal("snapshot mutation reached the domain")
	}
	if _, err := Restore(actual); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreRejectsCorruptState(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func(*Snapshot)
	}{
		{"unknown status", func(s *Snapshot) { s.Status = "invented" }},
		{"zero revision", func(s *Snapshot) { s.Revision = 0 }},
		{"wrong position", func(s *Snapshot) { s.Steps[0].Position = 1 }},
		{"duplicate step", func(s *Snapshot) { s.Steps[1].ID = s.Steps[0].ID }},
		{"unknown step status", func(s *Snapshot) { s.Steps[0].Status = "invented" }},
		{"running without deadline", func(s *Snapshot) { s.Steps[0].Status = StepRunning }},
		{"invalid input", func(s *Snapshot) { s.Input = json.RawMessage(`[]`) }},
		{"pending with previous attempt", func(s *Snapshot) { s.Steps[0].Attempt = 1 }},
		{"pending with start time", func(s *Snapshot) { s.StartedAt = &testTime }},
		{"pending with completed step", func(s *Snapshot) {
			s.Steps[0].Status = StepCompleted
			s.Steps[0].Attempt = 1
			s.Steps[0].Input, s.Steps[0].Output = json.RawMessage(`{}`), json.RawMessage(`{}`)
			s.Steps[0].StartedAt, s.Steps[0].CompletedAt = &testTime, &testTime
		}},
		{"failed without failed step", func(s *Snapshot) { s.Status = StatusFailed; s.StartedAt = &testTime; s.CompletedAt = &testTime }},
		{"wrong task subject", func(s *Snapshot) { s.Steps[0].TaskType = "invalid.*" }},
		{"incomplete completed execution", func(s *Snapshot) { s.Status = StatusCompleted; s.StartedAt = &testTime; s.CompletedAt = &testTime }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newTestExecution(t).Snapshot()
			test.corrupt(&s)
			if _, err := Restore(s); !fault.Is(err, fault.Permanent) {
				t.Fatalf("corrupt state restored: %v", err)
			}
		})
	}
}

func TestInvalidResultsLeaveRunningAttemptUntouched(t *testing.T) {
	e := newTestExecution(t)
	if err := e.Start(testTime); err != nil {
		t.Fatal(err)
	}
	step, err := e.DispatchNext(testTime)
	if err != nil {
		t.Fatal(err)
	}
	before := e.Snapshot()
	if err := e.SucceedStep(step.ID, 1, json.RawMessage(`{"broken":`), testTime); !fault.Is(err, fault.Validation) {
		t.Fatalf("malformed output accepted: %v", err)
	}
	if !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatal("invalid success mutated attempt")
	}
	past := testTime.Add(-time.Second)
	if err := e.FailStep(step.ID, 1, "retryable", &past, testTime); !fault.Is(err, fault.Validation) {
		t.Fatalf("past retry accepted: %v", err)
	}
	if !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatal("invalid retry mutated attempt")
	}
	if err := e.FailStep(step.ID, 1, "bad\x00code", nil, testTime); !fault.Is(err, fault.Validation) {
		t.Fatalf("NUL failure code accepted: %v", err)
	}
	if !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatal("invalid error code mutated attempt")
	}
}

func TestPendingCancellationCanBeRestored(t *testing.T) {
	e := newTestExecution(t)
	if err := e.Transition(StatusCancelled, testTime); err != nil {
		t.Fatal(err)
	}
	if e.Snapshot().StartedAt != nil {
		t.Fatal("cancelled pending execution acquired a start time")
	}
	if _, err := Restore(e.Snapshot()); err != nil {
		t.Fatal(err)
	}
}
