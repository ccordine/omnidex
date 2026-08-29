package main

import "testing"

func TestFeature001(t *testing.T) {
	result := Feature001(TaskInput{Arguments: []string{"run"}, StandardInput: ""}, CapabilityResults{})
	if result.Error != "" {
		t.Fatalf("Feature001 returned error: %s", result.Error)
	}
	if result.Output != "Hello, Omnidex." {
		t.Errorf("Expected output 'Hello, Omnidex.', got '%s'", result.Output)
	}
}
