package worker

import (
	"errors"
	"testing"
)

func TestSemanticCandidateExhaustedErrorReportsAttemptsPrecisely(t *testing.T) {
	t.Parallel()
	err := (&semanticCandidateExhaustedError{
		Subject:  "requirement",
		Attempts: 3,
		Err:      errors.New("duplicate leaf"),
	}).Error()
	const want = "requirement candidate failed after 3 bounded attempts: duplicate leaf"
	if err != want {
		t.Fatalf("error=%q, want %q", err, want)
	}
}
