package fault

import (
	"errors"
	"fmt"
	"testing"
)

func TestClassificationSurvivesWrappingWithoutExposingCause(t *testing.T) {
	cause := errors.New("database password=secret")
	err := fmt.Errorf("persist execution: %w", Wrap(Transient, "database_retry", "Retry the request.", cause))
	if KindOf(err) != Transient || CodeOf(err) != "database_retry" || !Is(err, Transient) {
		t.Fatal("classification was lost during wrapping")
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is not inspectable")
	}
	if KindOf(cause) != Infrastructure || CodeOf(cause) != "internal_error" {
		t.Fatal("unknown errors must have a safe fallback")
	}
}
