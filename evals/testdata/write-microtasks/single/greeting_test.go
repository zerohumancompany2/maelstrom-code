package single

import "testing"

func TestGreeting(t *testing.T) {
	if got := Greeting(); got != "hello from maelstrom" {
		t.Fatalf("Greeting() = %q, want %q", got, "hello from maelstrom")
	}
}
