package main

import "testing"

func TestScreenGenerateEndpoint(t *testing.T) {
	if got := screenGenerateEndpoint("http://localhost:11434"); got != "http://localhost:11434/api/generate" {
		t.Fatalf("screenGenerateEndpoint mismatch got=%q", got)
	}
	if got := screenGenerateEndpoint("http://localhost:11434/api/generate"); got != "http://localhost:11434/api/generate" {
		t.Fatalf("screenGenerateEndpoint passthrough mismatch got=%q", got)
	}
}
