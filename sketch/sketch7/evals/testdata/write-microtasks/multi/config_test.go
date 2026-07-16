package multi

import (
	"strings"
	"testing"
)

func TestRetryConfiguration(t *testing.T) {
	if RetryLimit != 3 {
		t.Fatalf("RetryLimit = %d, want 3", RetryLimit)
	}
	if !strings.Contains(RetryLabel(), "3") {
		t.Fatalf("RetryLabel() = %q, want it to name the limit", RetryLabel())
	}
}
