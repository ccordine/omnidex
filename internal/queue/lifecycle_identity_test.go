package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestLifecycleOperationIdentityIsAllocatedWithoutContent(t *testing.T) {
	first, err := NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("independent operations share an identity")
	}
	for _, id := range []LifecycleOperationID{first, second} {
		if _, err := ParseLifecycleOperationID(string(id)); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []string{"", "lifecycle_operation_", "lifecycle_operation_with space", "lifecycle_operation_" + strings.Repeat("a", 129)} {
		if _, err := ParseLifecycleOperationID(invalid); err == nil {
			t.Fatalf("accepted invalid identity %q", invalid)
		}
	}
}

func TestStepLifecycleIdentityUsesPersistedAttempt(t *testing.T) {
	authority := model.StepAttemptAuthority{JobID: 8, Generation: 2, StepID: 19, Attempt: 3, WorkerID: "worker-one"}
	for _, kind := range []LifecycleOperationKind{LifecycleCompleteStep, LifecycleFailStep} {
		id, err := NewStepLifecycleOperationID(authority, kind)
		if err != nil {
			t.Fatal(err)
		}
		want := "lifecycle_operation_step_19_attempt_3_" + string(kind)
		if string(id) != want {
			t.Fatalf("identity = %q, want %q", id, want)
		}
		retry, err := NewStepLifecycleOperationID(authority, kind)
		if err != nil || retry != id {
			t.Fatalf("retry changed identity: %q, %v", retry, err)
		}
		next := authority
		next.Attempt++
		nextID, err := NewStepLifecycleOperationID(next, kind)
		if err != nil || nextID == id {
			t.Fatalf("new attempt did not get its own identity: %q, %v", nextID, err)
		}
	}
	if _, err := NewStepLifecycleOperationID(authority, LifecycleCancelJob); err == nil {
		t.Fatal("accepted a non-step operation kind")
	}
	authority.Attempt = 0
	if _, err := NewStepLifecycleOperationID(authority, LifecycleCompleteStep); err == nil {
		t.Fatal("accepted an absent attempt")
	}
}
