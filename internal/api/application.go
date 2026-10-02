package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/ricardoverita/flowforge/internal/execution"
	"github.com/ricardoverita/flowforge/internal/fault"
	"github.com/ricardoverita/flowforge/internal/workflow"
)

// Store is the transactional persistence boundary needed by the API use cases.
type Store interface {
	CreateWorkflow(context.Context, workflow.Definition) error
	ListWorkflows(context.Context) ([]workflow.Definition, error)
	GetWorkflow(context.Context, string) (workflow.Definition, error)
	CreateExecution(context.Context, *execution.Execution, string) (execution.Snapshot, bool, error)
	GetExecution(context.Context, string) (execution.Snapshot, error)
	ListEvents(context.Context, string) ([]execution.Event, error)
}

type application struct {
	store Store
	now   func() time.Time
	id    func() string
}

type createWorkflowCommand struct {
	Name        string
	Version     int
	Description string
	Steps       []createStepCommand
}

type createStepCommand struct {
	Name           string
	TaskType       string
	MaxAttempts    *int
	TimeoutSeconds *int
}

func newApplication(store Store) *application {
	return &application{store: store, now: time.Now, id: uuid.NewString}
}

func (a *application) createWorkflow(ctx context.Context, command createWorkflowCommand) (workflow.Definition, error) {
	steps := make([]workflow.StepDefinition, 0, len(command.Steps))
	for _, item := range command.Steps {
		attempts, timeout := 3, 30
		if item.MaxAttempts != nil {
			attempts = *item.MaxAttempts
		}
		if item.TimeoutSeconds != nil {
			timeout = *item.TimeoutSeconds
		}
		steps = append(steps, workflow.StepDefinition{ID: a.id(), Name: item.Name, TaskType: item.TaskType, MaxAttempts: attempts, TimeoutSeconds: timeout})
	}
	definition, err := workflow.NewDefinition(a.id(), command.Name, command.Version, command.Description, steps, a.now())
	if err != nil {
		return workflow.Definition{}, err
	}
	if err := a.store.CreateWorkflow(ctx, definition); err != nil {
		return workflow.Definition{}, err
	}
	return definition, nil
}

func (a *application) createExecution(ctx context.Context, workflowID string, input json.RawMessage, correlation, idempotencyKey string) (execution.Snapshot, bool, error) {
	if !validHeaderValue(idempotencyKey, true) {
		return execution.Snapshot{}, false, fault.New(fault.Validation, "idempotency_key_invalid", "Idempotency-Key must contain at most 128 visible ASCII characters.")
	}
	definition, err := a.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return execution.Snapshot{}, false, err
	}
	run, err := execution.New(a.id(), definition, input, correlation, a.now())
	if err != nil {
		return execution.Snapshot{}, false, err
	}
	return a.store.CreateExecution(ctx, run, idempotencyKey)
}

func validHeaderValue(value string, emptyAllowed bool) bool {
	if len(value) > 128 || (!emptyAllowed && value == "") {
		return false
	}
	for _, b := range []byte(value) {
		if b < 33 || b > 126 {
			return false
		}
	}
	return true
}
