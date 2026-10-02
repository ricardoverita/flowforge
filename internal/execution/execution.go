package execution

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/workflow"
)

type Status string
type StepStatus string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusWaiting   Status = "waiting"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"

	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepRetryWait StepStatus = "retry_wait"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepCancelled StepStatus = "cancelled"
)

type Snapshot struct {
	ID              string
	WorkflowID      string
	WorkflowVersion int
	Status          Status
	Input           json.RawMessage
	CorrelationID   string
	Revision        int64
	CreatedAt       time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	Steps           []StepSnapshot
}

type StepSnapshot struct {
	ID             string
	Name           string
	TaskType       string
	Position       int
	Status         StepStatus
	Attempt        int
	MaxAttempts    int
	TimeoutSeconds int
	Input          json.RawMessage
	Output         json.RawMessage
	ErrorCode      string
	StartedAt      *time.Time
	CompletedAt    *time.Time
	RetryAt        *time.Time
	DeadlineAt     *time.Time
}

type Event struct {
	ID          int64
	ExecutionID string
	Type        string
	StepID      string
	Attempt     int
	OccurredAt  time.Time
	Data        json.RawMessage
}

// Execution owns lifecycle mutations. Snapshots are copies suitable for persistence.
type Execution struct{ state Snapshot }

func New(id string, definition workflow.Definition, input json.RawMessage, correlation string, now time.Time) (*Execution, error) {
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" || len(id) > 128 || now.IsZero() || strings.ContainsRune(id, '\x00') {
		return nil, fault.New(fault.Validation, "execution_invalid", "Execution ID and creation time are required.")
	}
	if len(correlation) > 128 || strings.ContainsRune(correlation, '\x00') {
		return nil, fault.New(fault.Validation, "correlation_id_invalid", "Correlation ID must be at most 128 bytes.")
	}
	if !validObject(input) {
		return nil, fault.New(fault.Validation, "execution_input_invalid", "Execution input must be a JSON object no larger than 1 MiB.")
	}
	s := Snapshot{ID: id, WorkflowID: definition.ID, WorkflowVersion: definition.Version, Status: StatusPending, Input: append(json.RawMessage(nil), input...), CorrelationID: correlation, Revision: 1, CreatedAt: now.UTC()}
	for position, step := range definition.Steps {
		s.Steps = append(s.Steps, StepSnapshot{ID: step.ID, Name: step.Name, TaskType: step.TaskType, Position: position, Status: StepPending, MaxAttempts: step.MaxAttempts, TimeoutSeconds: step.TimeoutSeconds})
	}
	return &Execution{state: s}, nil
}

func Restore(s Snapshot) (*Execution, error) {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.WorkflowID) == "" || s.WorkflowVersion < 1 || s.Revision < 1 || s.CreatedAt.IsZero() || len(s.Steps) == 0 || len(s.Steps) > workflow.MaxSteps || !validObject(s.Input) {
		return nil, fault.New(fault.Permanent, "execution_snapshot_invalid", "Stored execution violates domain invariants.")
	}
	if !validStatus(s.Status) || (terminal(s.Status) != (s.CompletedAt != nil)) || (s.Status != StatusPending && s.Status != StatusCancelled && s.StartedAt == nil) {
		return nil, fault.New(fault.Permanent, "execution_status_invalid", "Stored execution lifecycle is invalid.")
	}
	if len(s.CorrelationID) > 128 || (s.Status == StatusPending && s.StartedAt != nil) || zeroTimePointer(s.StartedAt) || zeroTimePointer(s.CompletedAt) {
		return nil, fault.New(fault.Permanent, "execution_metadata_invalid", "Stored execution metadata is invalid.")
	}
	seen := make(map[string]bool, len(s.Steps))
	active := false
	failed, cancelled, unfinished := false, false, false
	definitionSteps := make([]workflow.StepDefinition, 0, len(s.Steps))
	for position, step := range s.Steps {
		if step.ID == "" || seen[step.ID] || step.Position != position || !validStepStatus(step.Status) || step.Attempt < 0 || step.Attempt > step.MaxAttempts || step.MaxAttempts < 1 || step.MaxAttempts > workflow.MaxAttempts || step.TimeoutSeconds < 1 || step.TimeoutSeconds > workflow.MaxTimeoutSeconds {
			return nil, fault.New(fault.Permanent, "step_snapshot_invalid", "Stored step violates domain invariants.")
		}
		seen[step.ID] = true
		definitionSteps = append(definitionSteps, workflow.StepDefinition{ID: step.ID, Name: step.Name, TaskType: step.TaskType, MaxAttempts: step.MaxAttempts, TimeoutSeconds: step.TimeoutSeconds})
		if len(step.ErrorCode) > 128 || zeroTimePointer(step.StartedAt) || zeroTimePointer(step.CompletedAt) || zeroTimePointer(step.RetryAt) || zeroTimePointer(step.DeadlineAt) {
			return nil, fault.New(fault.Permanent, "step_metadata_invalid", "Stored step metadata is invalid.")
		}
		if step.Status == StepPending && (step.Attempt != 0 || step.StartedAt != nil || step.CompletedAt != nil || step.RetryAt != nil || step.DeadlineAt != nil || len(step.Input) != 0 || len(step.Output) != 0 || step.ErrorCode != "") {
			return nil, fault.New(fault.Permanent, "step_pending_invalid", "A pending step cannot contain attempt state.")
		}
		if step.Status == StepRunning && (step.Attempt < 1 || step.StartedAt == nil || step.DeadlineAt == nil) {
			return nil, fault.New(fault.Permanent, "step_running_invalid", "Stored running step has no attempt or deadline.")
		}
		if step.Status == StepRetryWait && (step.RetryAt == nil || step.Attempt < 1 || step.Attempt >= step.MaxAttempts) {
			return nil, fault.New(fault.Permanent, "step_retry_invalid", "Stored retry has no remaining attempt or retry time.")
		}
		if step.Status == StepCompleted && (step.CompletedAt == nil || step.Attempt < 1 || !validJSON(step.Output)) {
			return nil, fault.New(fault.Permanent, "step_completed_invalid", "Stored completed step has no valid result.")
		}
		if (step.Status == StepRunning || step.Status == StepRetryWait || step.Status == StepCompleted || step.Status == StepFailed) && (step.StartedAt == nil || !validJSON(step.Input)) {
			return nil, fault.New(fault.Permanent, "step_attempt_invalid", "An attempted step requires a start time and valid input.")
		}
		if step.Status == StepRunning && (step.CompletedAt != nil || step.RetryAt != nil || len(step.Output) != 0 || step.ErrorCode != "") {
			return nil, fault.New(fault.Permanent, "step_running_invalid", "A running step cannot contain a result or retry timer.")
		}
		if step.Status != StepRunning && step.DeadlineAt != nil || step.Status != StepRetryWait && step.RetryAt != nil {
			return nil, fault.New(fault.Permanent, "step_timer_invalid", "Stored step timers do not match its lifecycle.")
		}
		if step.Status == StepRetryWait && (step.CompletedAt != nil || step.ErrorCode == "" || len(step.Output) != 0) {
			return nil, fault.New(fault.Permanent, "step_retry_invalid", "A retrying step requires a failure without a terminal result.")
		}
		if step.Status == StepFailed && (step.CompletedAt == nil || step.Attempt < 1 || step.ErrorCode == "" || len(step.Output) != 0) {
			return nil, fault.New(fault.Permanent, "step_failed_invalid", "A failed step requires a terminal failure.")
		}
		if step.Status == StepCancelled && step.CompletedAt == nil {
			return nil, fault.New(fault.Permanent, "step_cancelled_invalid", "A cancelled step requires a completion time.")
		}
		if step.Status == StepCompleted && active {
			return nil, fault.New(fault.Permanent, "step_order_invalid", "Stored sequential execution contains an out-of-order result.")
		}
		if step.Status != StepCompleted {
			if active && (step.Status == StepRunning || step.Status == StepRetryWait || step.Status == StepFailed) {
				return nil, fault.New(fault.Permanent, "step_order_invalid", "Only the first unfinished step may be active.")
			}
			active = true
		}
		if s.Status == StatusCompleted && step.Status != StepCompleted {
			return nil, fault.New(fault.Permanent, "execution_completed_invalid", "A completed execution must have completed every step.")
		}
		if terminal(s.Status) && (step.Status == StepRunning || step.Status == StepRetryWait) {
			return nil, fault.New(fault.Permanent, "execution_terminal_invalid", "A terminal execution cannot contain active work.")
		}
		if s.Status == StatusPending && step.Status != StepPending || s.Status == StatusWaiting && step.Status == StepRunning {
			return nil, fault.New(fault.Permanent, "execution_step_status_invalid", "Stored step and execution lifecycles disagree.")
		}
		failed = failed || step.Status == StepFailed
		cancelled = cancelled || step.Status == StepCancelled
		unfinished = unfinished || step.Status != StepCompleted
	}
	if s.Status == StatusFailed && !failed || s.Status == StatusCancelled && !cancelled || s.Status != StatusFailed && failed || s.Status != StatusCancelled && cancelled || (s.Status == StatusRunning || s.Status == StatusWaiting) && !unfinished {
		return nil, fault.New(fault.Permanent, "execution_step_status_invalid", "Stored step and execution lifecycles disagree.")
	}
	if _, err := workflow.NewDefinition(s.WorkflowID, "restored execution", s.WorkflowVersion, "", definitionSteps, s.CreatedAt); err != nil {
		return nil, fault.Wrap(fault.Permanent, "step_definition_invalid", "Stored step definitions are invalid.", err)
	}
	return &Execution{state: cloneSnapshot(s)}, nil
}

func (e *Execution) Snapshot() Snapshot { return cloneSnapshot(e.state) }

func (e *Execution) Start(now time.Time) error { return e.Transition(StatusRunning, now) }

func (e *Execution) Transition(next Status, now time.Time) error {
	current := e.state.Status
	allowed := current == StatusPending && (next == StatusRunning || next == StatusCancelled) || current == StatusRunning && (next == StatusWaiting || next == StatusCompleted || next == StatusFailed || next == StatusCancelled) || current == StatusWaiting && (next == StatusRunning || next == StatusCancelled)
	if !allowed || now.IsZero() {
		return fault.New(fault.Conflict, "execution_transition_invalid", "The execution cannot make this state transition.")
	}
	if next == StatusCompleted {
		for _, step := range e.state.Steps {
			if step.Status != StepCompleted {
				return fault.New(fault.Conflict, "execution_incomplete", "Every step must complete before the execution can complete.")
			}
		}
	}
	if next == StatusWaiting {
		for _, step := range e.state.Steps {
			if step.Status == StepRunning {
				return fault.New(fault.Conflict, "execution_has_running_step", "The execution cannot wait while a step is running.")
			}
		}
	}
	if next == StatusFailed {
		failed := false
		for _, step := range e.state.Steps {
			if step.Status == StepFailed {
				failed = true
			}
		}
		if !failed {
			return fault.New(fault.Conflict, "execution_has_no_failure", "A failed step is required to fail the execution.")
		}
	}
	at := now.UTC()
	e.state.Status = next
	if next == StatusRunning && e.state.StartedAt == nil {
		e.state.StartedAt = &at
	}
	if terminal(next) {
		e.state.CompletedAt = &at
	}
	if next == StatusCancelled {
		for i := range e.state.Steps {
			step := &e.state.Steps[i]
			if step.Status != StepCompleted && step.Status != StepFailed {
				step.Status = StepCancelled
				step.CompletedAt = timeCopy(&at)
				step.RetryAt, step.DeadlineAt = nil, nil
			}
		}
	}
	return nil
}

func (e *Execution) DispatchNext(now time.Time) (*StepSnapshot, error) {
	if now.IsZero() {
		return nil, fault.New(fault.Validation, "dispatch_time_invalid", "Dispatch time is required.")
	}
	if e.state.Status != StatusRunning {
		return nil, nil
	}
	for i := range e.state.Steps {
		step := &e.state.Steps[i]
		if step.Status == StepCompleted {
			continue
		}
		if step.Status != StepPending && step.Status != StepRetryWait {
			return nil, nil
		}
		if step.Status == StepRetryWait && (step.RetryAt == nil || now.Before(*step.RetryAt)) {
			return nil, nil
		}
		at, deadline := now.UTC(), now.UTC().Add(time.Duration(step.TimeoutSeconds)*time.Second)
		step.Status, step.Attempt, step.ErrorCode = StepRunning, step.Attempt+1, ""
		step.StartedAt, step.DeadlineAt, step.RetryAt, step.CompletedAt = &at, &deadline, nil, nil
		if i == 0 {
			step.Input = append(json.RawMessage(nil), e.state.Input...)
		} else {
			step.Input = append(json.RawMessage(nil), e.state.Steps[i-1].Output...)
		}
		result := cloneStep(*step)
		return &result, nil
	}
	return nil, nil
}

func (e *Execution) SucceedStep(stepID string, attempt int, output json.RawMessage, now time.Time) error {
	step, err := e.findStep(stepID)
	if err != nil {
		return err
	}
	if step.Status == StepCompleted && step.Attempt == attempt {
		return nil
	}
	if e.state.Status != StatusRunning || step.Status != StepRunning || step.Attempt != attempt {
		return fault.New(fault.Conflict, "step_result_stale", "The result does not match the running step attempt.")
	}
	if !validJSON(output) || now.IsZero() {
		return fault.New(fault.Validation, "step_result_invalid", "Step output must be valid JSON no larger than 1 MiB.")
	}
	at := now.UTC()
	step.Status, step.Output, step.ErrorCode = StepCompleted, append(json.RawMessage(nil), output...), ""
	step.CompletedAt, step.DeadlineAt, step.RetryAt = &at, nil, nil
	for _, candidate := range e.state.Steps {
		if candidate.Status != StepCompleted {
			return nil
		}
	}
	return e.Transition(StatusCompleted, now)
}

func (e *Execution) FailStep(stepID string, attempt int, code string, retryAt *time.Time, now time.Time) error {
	step, err := e.findStep(stepID)
	if err != nil {
		return err
	}
	if (step.Status == StepRetryWait || step.Status == StepFailed) && step.Attempt == attempt {
		return nil
	}
	if e.state.Status != StatusRunning || step.Status != StepRunning || step.Attempt != attempt {
		return fault.New(fault.Conflict, "step_result_stale", "The result does not match the running step attempt.")
	}
	if strings.TrimSpace(code) == "" || len(code) > 128 || strings.ContainsRune(code, '\x00') || now.IsZero() || (retryAt != nil && retryAt.Before(now)) {
		return fault.New(fault.Validation, "step_failure_invalid", "A bounded error code and a valid retry time are required.")
	}
	step.ErrorCode, step.DeadlineAt = code, nil
	if retryAt != nil && step.Attempt < step.MaxAttempts {
		step.Status, step.RetryAt = StepRetryWait, timeCopy(retryAt)
		return nil
	}
	at := now.UTC()
	step.Status, step.CompletedAt, step.RetryAt = StepFailed, &at, nil
	return e.Transition(StatusFailed, now)
}

func (e *Execution) findStep(id string) (*StepSnapshot, error) {
	for i := range e.state.Steps {
		if e.state.Steps[i].ID == id {
			return &e.state.Steps[i], nil
		}
	}
	return nil, fault.New(fault.NotFound, "step_not_found", "Step was not found in this execution.")
}

func validObject(raw json.RawMessage) bool {
	return validJSON(raw) && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{"))
}
func validJSON(raw json.RawMessage) bool {
	return len(raw) > 0 && len(raw) <= 1<<20 && utf8.Valid(raw) && json.Valid(raw)
}
func validStatus(s Status) bool {
	return s == StatusPending || s == StatusRunning || s == StatusWaiting || terminal(s)
}
func terminal(s Status) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}
func validStepStatus(s StepStatus) bool {
	return s == StepPending || s == StepRunning || s == StepRetryWait || s == StepCompleted || s == StepFailed || s == StepCancelled
}
func timeCopy(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
func zeroTimePointer(value *time.Time) bool { return value != nil && value.IsZero() }
func cloneStep(s StepSnapshot) StepSnapshot {
	s.Input, s.Output = append(json.RawMessage(nil), s.Input...), append(json.RawMessage(nil), s.Output...)
	s.StartedAt, s.CompletedAt, s.RetryAt, s.DeadlineAt = timeCopy(s.StartedAt), timeCopy(s.CompletedAt), timeCopy(s.RetryAt), timeCopy(s.DeadlineAt)
	return s
}
func cloneSnapshot(s Snapshot) Snapshot {
	s.Input = append(json.RawMessage(nil), s.Input...)
	s.StartedAt, s.CompletedAt = timeCopy(s.StartedAt), timeCopy(s.CompletedAt)
	steps := make([]StepSnapshot, len(s.Steps))
	for i, step := range s.Steps {
		steps[i] = cloneStep(step)
	}
	s.Steps = steps
	return s
}
