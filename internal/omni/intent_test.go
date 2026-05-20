package omni

import "testing"

func TestClassifyIntentDirectCommandRequest(t *testing.T) {
	result := ClassifyIntent("run pwd")

	if result.Classification != IntentExecution {
		t.Fatalf("classification = %s, want %s", result.Classification, IntentExecution)
	}
	if result.Confidence < intentConfidenceThreshold {
		t.Fatalf("confidence = %.2f, want >= %.2f", result.Confidence, intentConfidenceThreshold)
	}
}

func TestClassifyIntentDefaultsNonEmptyPromptsToExecution(t *testing.T) {
	result := ClassifyIntent("how do I run tests?")

	if result.Classification != IntentExecution {
		t.Fatalf("classification = %s, want execution", result.Classification)
	}
}
