package workflow

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ricardoverita/flowforge/internal/fault"
)

const (
	MaxSteps          = 100
	MaxAttempts       = 10
	MaxTimeoutSeconds = 3600
)

var taskTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

type Definition struct {
	ID          string
	Name        string
	Version     int
	Description string
	Steps       []StepDefinition
	CreatedAt   time.Time
}

type StepDefinition struct {
	ID             string
	Name           string
	TaskType       string
	MaxAttempts    int
	TimeoutSeconds int
}

func NewDefinition(id, name string, version int, description string, steps []StepDefinition, now time.Time) (Definition, error) {
	d := Definition{ID: id, Name: name, Version: version, Description: description, Steps: append([]StepDefinition(nil), steps...), CreatedAt: now.UTC()}
	if err := d.Validate(); err != nil {
		return Definition{}, err
	}
	return d, nil
}

func (d Definition) Validate() error {
	invalid := func(code, message string) error { return fault.New(fault.Validation, code, message) }
	if strings.TrimSpace(d.ID) == "" || len(d.ID) > 128 || strings.ContainsRune(d.ID, '\x00') {
		return invalid("workflow_id_invalid", "Workflow ID is required and must be at most 128 bytes.")
	}
	if strings.TrimSpace(d.Name) == "" || utf8.RuneCountInString(d.Name) > 100 || strings.ContainsRune(d.Name, '\x00') || !utf8.ValidString(d.Name) {
		return invalid("workflow_name_invalid", "Workflow name is required and must be at most 100 characters.")
	}
	if d.Version < 1 || d.Version > 1_000_000 {
		return invalid("workflow_version_invalid", "Workflow version must be between 1 and 1000000.")
	}
	if utf8.RuneCountInString(d.Description) > 2000 || strings.ContainsRune(d.Description, '\x00') || !utf8.ValidString(d.Description) {
		return invalid("workflow_description_invalid", "Workflow description must be at most 2000 characters.")
	}
	if d.CreatedAt.IsZero() {
		return invalid("workflow_time_invalid", "Workflow creation time is required.")
	}
	if len(d.Steps) == 0 || len(d.Steps) > MaxSteps {
		return invalid("workflow_steps_invalid", "A workflow must contain between 1 and 100 steps.")
	}
	ids := make(map[string]bool, len(d.Steps))
	names := make(map[string]bool, len(d.Steps))
	for _, step := range d.Steps {
		if strings.TrimSpace(step.ID) == "" || len(step.ID) > 128 || ids[step.ID] || strings.ContainsRune(step.ID, '\x00') {
			return invalid("step_id_invalid", "Step IDs must be present, unique, and at most 128 bytes.")
		}
		ids[step.ID] = true
		if strings.TrimSpace(step.Name) == "" || utf8.RuneCountInString(step.Name) > 100 || names[step.Name] || strings.ContainsRune(step.Name, '\x00') || !utf8.ValidString(step.Name) {
			return invalid("step_name_invalid", "Step names must be present, unique, and at most 100 characters.")
		}
		names[step.Name] = true
		if len(step.TaskType) > 128 || !taskTypePattern.MatchString(step.TaskType) {
			return invalid("task_type_invalid", "Task types must contain lowercase dot-separated tokens, such as identity.verify.")
		}
		if step.MaxAttempts < 1 || step.MaxAttempts > MaxAttempts {
			return invalid("step_attempts_invalid", "Step max_attempts must be between 1 and 10.")
		}
		if step.TimeoutSeconds < 1 || step.TimeoutSeconds > MaxTimeoutSeconds {
			return invalid("step_timeout_invalid", "Step timeout_seconds must be between 1 and 3600.")
		}
	}
	return nil
}
