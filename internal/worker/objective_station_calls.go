package worker

import "fmt"

// Dispatch counts measure usage, not permission to consume a semantic value.
// Code-resolved choices and retained results both require zero provider calls.
func validateObjectiveLeafCallCount(label string, calls int) error {
	return validateObjectiveCallCount(label, calls, exactSemanticLeafCalls)
}

func validateObjectiveCallCount(label string, calls, maximum int) error {
	if maximum < 0 {
		return fmt.Errorf("%s has invalid maximum call budget %d", label, maximum)
	}
	if calls < 0 || calls > maximum {
		return fmt.Errorf("%s reported %d calls outside 0..%d", label, calls, maximum)
	}
	return nil
}
