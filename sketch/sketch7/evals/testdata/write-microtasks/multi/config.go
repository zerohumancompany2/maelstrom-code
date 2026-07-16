package multi

// RetryLimit is the number of transient provider retries.
const RetryLimit = 0

// RetryLabel is rendered in diagnostics.
func RetryLabel() string {
	return "retries disabled"
}
