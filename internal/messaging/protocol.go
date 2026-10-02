package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const TaskPrefix = "flowforge.tasks."
const ResultSubject = "flowforge.results"

type PublishFunc func(context.Context, string, string, []byte, string) error

type Task struct {
	ID            string          `json:"id"`
	ExecutionID   string          `json:"execution_id"`
	WorkflowID    string          `json:"workflow_id"`
	StepID        string          `json:"step_id"`
	TaskType      string          `json:"task_type"`
	Attempt       int             `json:"attempt"`
	Input         json.RawMessage `json:"input"`
	CorrelationID string          `json:"correlation_id"`
	DeadlineAt    time.Time       `json:"deadline_at"`
}
type Result struct {
	TaskID        string          `json:"task_id"`
	ExecutionID   string          `json:"execution_id"`
	StepID        string          `json:"step_id"`
	Attempt       int             `json:"attempt"`
	Output        json.RawMessage `json:"output,omitempty"`
	ErrorCode     string          `json:"error_code,omitempty"`
	Permanent     bool            `json:"permanent,omitempty"`
	CorrelationID string          `json:"correlation_id"`
}

func TaskID(executionID, stepID string, attempt int) string {
	return fmt.Sprintf("%s:%s:%d", executionID, stepID, attempt)
}
func (r Result) Validate() error {
	if r.ExecutionID == "" || len(r.ExecutionID) > 128 || r.StepID == "" || len(r.StepID) > 128 || r.Attempt < 1 || r.Attempt > 10 || r.TaskID != TaskID(r.ExecutionID, r.StepID, r.Attempt) || len(r.ErrorCode) > 128 || len(r.CorrelationID) > 128 {
		return fmt.Errorf("invalid result identity")
	}
	if r.ErrorCode == "" && (len(r.Output) == 0 || len(r.Output) > 1<<20 || !json.Valid(r.Output)) {
		return fmt.Errorf("invalid result output")
	}
	return nil
}
