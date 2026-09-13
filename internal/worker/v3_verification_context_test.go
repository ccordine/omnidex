package worker

import (
	"context"
	"testing"
)

func TestVerificationEvidencePersistenceSurvivesCommandCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	persistence, stop := directCodingVerificationEvidenceContext(parent)
	defer stop()
	if err := persistence.Err(); err != nil {
		t.Fatalf("bounded evidence persistence inherited command cancellation: %v", err)
	}
}

func containsExactString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func sameExactStrings(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}
