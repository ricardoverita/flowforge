package workflow

import (
	"strings"
	"testing"
	"time"

	"github.com/ricardoverita/flowforge/internal/fault"
)

func TestDefinitionValidation(t *testing.T) {
	step := StepDefinition{ID: "step-1", Name: "verify_identity", TaskType: "identity.verify", MaxAttempts: 3, TimeoutSeconds: 30}
	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{"empty ID", func(d *Definition) { d.ID = "" }},
		{"empty name", func(d *Definition) { d.Name = "  " }},
		{"long name", func(d *Definition) { d.Name = strings.Repeat("a", 101) }},
		{"invalid version", func(d *Definition) { d.Version = 0 }},
		{"long description", func(d *Definition) { d.Description = strings.Repeat("a", 2001) }},
		{"NUL in name", func(d *Definition) { d.Name = "workflow\x00name" }},
		{"NUL in description", func(d *Definition) { d.Description = "description\x00" }},
		{"NUL in step name", func(d *Definition) { d.Steps[0].Name = "verify\x00" }},
		{"no steps", func(d *Definition) { d.Steps = nil }},
		{"too many steps", func(d *Definition) { d.Steps = make([]StepDefinition, MaxSteps+1) }},
		{"duplicate ID", func(d *Definition) {
			d.Steps = append(d.Steps, StepDefinition{ID: step.ID, Name: "other", TaskType: "risk.check", MaxAttempts: 1, TimeoutSeconds: 1})
		}},
		{"duplicate name", func(d *Definition) {
			d.Steps = append(d.Steps, StepDefinition{ID: "step-2", Name: step.Name, TaskType: "risk.check", MaxAttempts: 1, TimeoutSeconds: 1})
		}},
		{"wildcard task", func(d *Definition) { d.Steps[0].TaskType = "identity.*" }},
		{"uppercase task", func(d *Definition) { d.Steps[0].TaskType = "Identity.verify" }},
		{"empty task token", func(d *Definition) { d.Steps[0].TaskType = "identity..verify" }},
		{"single task token", func(d *Definition) { d.Steps[0].TaskType = "verify" }},
		{"zero attempts", func(d *Definition) { d.Steps[0].MaxAttempts = 0 }},
		{"too many attempts", func(d *Definition) { d.Steps[0].MaxAttempts = MaxAttempts + 1 }},
		{"zero timeout", func(d *Definition) { d.Steps[0].TimeoutSeconds = 0 }},
		{"long timeout", func(d *Definition) { d.Steps[0].TimeoutSeconds = MaxTimeoutSeconds + 1 }},
		{"no timestamp", func(d *Definition) { d.CreatedAt = time.Time{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Definition{ID: "wf-1", Name: "onboarding", Version: 1, Steps: []StepDefinition{step}, CreatedAt: time.Now()}
			test.mutate(&d)
			if err := d.Validate(); !fault.Is(err, fault.Validation) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestDefinitionCopiesStepSlice(t *testing.T) {
	steps := []StepDefinition{{ID: "step-1", Name: "verify", TaskType: "identity.verify", MaxAttempts: 1, TimeoutSeconds: 1}}
	d, err := NewDefinition("wf-1", "onboarding", 1, "", steps, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	steps[0].TaskType = "changed.task"
	if d.Steps[0].TaskType != "identity.verify" {
		t.Fatal("definition retained the caller's mutable slice")
	}
}
