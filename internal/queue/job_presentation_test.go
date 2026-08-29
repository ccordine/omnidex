package queue

import (
	"errors"
	"strings"
	"testing"
)

func TestCurrentJobPresentationRejectsInvalidIdentityBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	var repository *Repository
	if _, err := repository.CurrentJobPresentation(t.Context(), 0); !errors.Is(err, ErrInvalidJobPresentation) {
		t.Fatalf("CurrentJobPresentation error=%v, want ErrInvalidJobPresentation", err)
	}
}

func TestCurrentJobPresentationSourceHasHardLatestEventAndStepBounds(t *testing.T) {
	t.Parallel()
	source := normalizedGenerationSource(t, "job_presentation.go")
	for _, required := range []string{
		"maxCurrentJobPresentationSteps+1",
		"maxCurrentJobProgressItems",
		"octet_length(c.value)",
		"s.generation = j.current_generation",
		"s.superseded_at_generation IS NULL",
		"c.key = 'event'",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("bounded current-generation presentation query lacks %q", required)
		}
	}
}
