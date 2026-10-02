package postgres

import (
	"testing"
	"time"
)

func TestRetryDelayIsBoundedAndCapped(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		for range 20 {
			delay := RetryDelay(attempt)
			n := attempt
			if n < 1 {
				n = 1
			}
			if n > 6 {
				n = 6
			}
			base := time.Second * time.Duration(1<<uint(n-1))
			if delay < base/2 || delay > base {
				t.Fatalf("attempt %d delay %s outside [%s,%s]", attempt, delay, base/2, base)
			}
		}
	}
}
