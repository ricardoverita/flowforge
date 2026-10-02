package postgres

import (
	"encoding/json"
	"github.com/ricardoverita/flowforge/internal/execution"
	"testing"
)

func TestIdempotencyHashPreservesLargeNumbers(t *testing.T) {
	a := execution.Snapshot{WorkflowID: "workflow", Input: json.RawMessage(`{"n":9007199254740992}`)}
	b := a
	b.Input = json.RawMessage(`{"n":9007199254740993}`)
	if requestHash(a) == requestHash(b) {
		t.Fatal("different large integers collide")
	}
}
