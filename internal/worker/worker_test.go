package worker

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ricardoverita/flowforge/internal/messaging"
)

func TestSimulationPreservesInputAndIsIdempotent(t *testing.T) {
	task := messaging.Task{ExecutionID: "execution_1", TaskType: "account.create", Input: json.RawMessage(`{"customer_ref":"demo"}`)}
	first, code := Simulate(task)
	if code != "" {
		t.Fatal(code)
	}
	second, code := Simulate(task)
	if code != "" || !bytes.Equal(first, second) {
		t.Fatal("duplicate simulation changed output")
	}
	var out map[string]any
	if err := json.Unmarshal(first, &out); err != nil {
		t.Fatal(err)
	}
	if out["customer_ref"] != "demo" || out["account_id"] != "account_execution_1" {
		t.Fatal("lost input or unstable account identity")
	}
}
func TestUnsupportedSimulationFailsPermanently(t *testing.T) {
	_, code := Simulate(messaging.Task{TaskType: "money.transfer", Input: json.RawMessage(`{}`)})
	if code != "unsupported_task_type" {
		t.Fatal(code)
	}
}
