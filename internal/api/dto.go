package api

import (
	"encoding/json"
	"time"

	"github.com/ricardoverita/flowforge/internal/execution"
	"github.com/ricardoverita/flowforge/internal/workflow"
)

type workflowRequest struct {
	Name        string        `json:"name"`
	Version     int           `json:"version"`
	Description string        `json:"description"`
	Steps       []stepRequest `json:"steps"`
}

type stepRequest struct {
	Name           string `json:"name"`
	TaskType       string `json:"task_type"`
	MaxAttempts    *int   `json:"max_attempts"`
	TimeoutSeconds *int   `json:"timeout_seconds"`
}

type executionRequest struct {
	Input json.RawMessage `json:"input"`
}

type workflowResponse struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Version     int                      `json:"version"`
	Description string                   `json:"description"`
	Steps       []stepDefinitionResponse `json:"steps"`
	CreatedAt   time.Time                `json:"created_at"`
}

type stepDefinitionResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	TaskType       string `json:"task_type"`
	MaxAttempts    int    `json:"max_attempts"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type executionResponse struct {
	ID              string                  `json:"id"`
	WorkflowID      string                  `json:"workflow_id"`
	WorkflowVersion int                     `json:"workflow_version"`
	Status          execution.Status        `json:"status"`
	Input           json.RawMessage         `json:"input"`
	CorrelationID   string                  `json:"correlation_id"`
	Revision        int64                   `json:"revision"`
	CreatedAt       time.Time               `json:"created_at"`
	StartedAt       *time.Time              `json:"started_at"`
	CompletedAt     *time.Time              `json:"completed_at"`
	Steps           []stepExecutionResponse `json:"steps"`
}

type stepExecutionResponse struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	TaskType       string               `json:"task_type"`
	Position       int                  `json:"position"`
	Status         execution.StepStatus `json:"status"`
	Attempt        int                  `json:"attempt"`
	MaxAttempts    int                  `json:"max_attempts"`
	TimeoutSeconds int                  `json:"timeout_seconds"`
	Input          json.RawMessage      `json:"input"`
	Output         json.RawMessage      `json:"output"`
	ErrorCode      string               `json:"error_code"`
	StartedAt      *time.Time           `json:"started_at"`
	CompletedAt    *time.Time           `json:"completed_at"`
	RetryAt        *time.Time           `json:"retry_at"`
	DeadlineAt     *time.Time           `json:"deadline_at"`
}

type eventResponse struct {
	ID          int64           `json:"id"`
	ExecutionID string          `json:"execution_id"`
	Type        string          `json:"type"`
	StepID      string          `json:"step_id"`
	Attempt     int             `json:"attempt"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Data        json.RawMessage `json:"data"`
}

func workflowDTO(d workflow.Definition) workflowResponse {
	steps := make([]stepDefinitionResponse, 0, len(d.Steps))
	for _, s := range d.Steps {
		steps = append(steps, stepDefinitionResponse(s))
	}
	return workflowResponse{d.ID, d.Name, d.Version, d.Description, steps, d.CreatedAt}
}

func executionDTO(d execution.Snapshot) executionResponse {
	steps := make([]stepExecutionResponse, 0, len(d.Steps))
	for _, s := range d.Steps {
		steps = append(steps, stepExecutionResponse(s))
	}
	return executionResponse{d.ID, d.WorkflowID, d.WorkflowVersion, d.Status, d.Input, d.CorrelationID, d.Revision, d.CreatedAt, d.StartedAt, d.CompletedAt, steps}
}
