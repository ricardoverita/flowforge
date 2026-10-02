package messaging

import (
	"encoding/json"
	"testing"
)

func TestResultRejectsMismatchedAttemptIdentity(t *testing.T) {
	r := Result{TaskID: TaskID("ex", "step", 1), ExecutionID: "ex", StepID: "step", Attempt: 2, Output: json.RawMessage(`{}`)}
	if r.Validate() == nil {
		t.Fatal("mismatched attempt accepted")
	}
	r.TaskID = TaskID("ex", "step", 2)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}
