package cognition

import (
	"errors"
	"testing"
)

func TestRuntimeSnapshotHashBindsAttemptAndContextProjection(t *testing.T) {
	t.Parallel()
	snapshot, _, _ := testRuntimeSnapshot(t)

	changedAttempt := snapshot.Attempt()
	changedAttempt.Attempt++
	withAttempt, err := NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), changedAttempt, snapshot.ContextProjection(),
		snapshot.Budget(), snapshot.EvidenceRefs(),
	)
	if err != nil {
		t.Fatal(err)
	}
	changedProjection := snapshot.ContextProjection()
	changedProjection.WorkingSetVersion++
	withProjection, err := NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), changedProjection,
		snapshot.Budget(), snapshot.EvidenceRefs(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SHA256() == withAttempt.SHA256() || snapshot.SHA256() == withProjection.SHA256() ||
		withAttempt.SHA256() == withProjection.SHA256() {
		t.Fatal("runtime snapshot hash omitted an authority identity")
	}
}

func TestContextProjectionReferenceRequiresEveryExactIdentity(t *testing.T) {
	t.Parallel()
	valid := testContextProjectionRef()
	if err := valid.Validate(); err != nil {
		t.Fatalf("validate reference: %v", err)
	}
	mutations := map[string]func(*ContextProjectionRef){
		"ID":                  func(ref *ContextProjectionRef) { ref.ID = "" },
		"hash":                func(ref *ContextProjectionRef) { ref.SHA256 = "bad" },
		"working-set ID":      func(ref *ContextProjectionRef) { ref.WorkingSetID = "" },
		"working-set version": func(ref *ContextProjectionRef) { ref.WorkingSetVersion = 0 },
		"renderer version":    func(ref *ContextProjectionRef) { ref.RendererVersion = "" },
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			invalid := valid
			mutate(&invalid)
			if err := invalid.Validate(); !errors.Is(err, ErrInvalidContextProjection) {
				t.Fatalf("error = %v, want ErrInvalidContextProjection", err)
			}
		})
	}
}
