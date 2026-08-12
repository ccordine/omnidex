package objectiveworkload

import (
	"errors"
	"testing"
)

func TestRunValidationRejectsOverlappingDuplicateCompiledLeaf(t *testing.T) {
	t.Parallel()
	authority, err := newAuthority("aaa")
	if err != nil {
		t.Fatal(err)
	}
	requirements := []Requirement{{
		ID: "R001", SourceQuote: "aa", Start: 0, End: 2,
		SHA256: digestBytes([]byte("aa")),
	}}
	workload := Workload{
		ID: compiledWorkloadIdentity(authority, requirements), Authority: authority,
		RootObjectiveID: "O000_root", Requirements: requirements,
		Objectives: expectedObjectives(requirements, ObjectivePending),
	}
	if err := validateWorkload(workload, true); !errors.Is(err, ErrInvalidGraph) {
		t.Fatalf("overlapping duplicate leaf error=%v", err)
	}
}
