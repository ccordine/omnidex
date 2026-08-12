package cognitionpolicy

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
)

func TestPolicyRequiresClientWriterBrainAndProjection(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	client := &policyTestClient{}
	journal := &policyTestCallJournal{}
	loader := newPolicyTestProjectionLoader(projection)
	brain := policyTestAttestedBrain()
	activation := policyTestDefaultProviderProcessActivation(brain)
	if _, err := New(nil, brain, activation, loader, journal); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil client error = %v, want ErrInvalidConfig", err)
	}
	if _, err := New(client, brain, activation, loader, nil); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil journal error = %v, want ErrInvalidConfig", err)
	}
	if _, err := New(client, brain, activation, nil, journal); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil projection loader error = %v, want ErrInvalidConfig", err)
	}
	invalidBrain := policyTestAttestedBrain()
	invalidBrain.Ref.Model = ""
	if _, err := New(client, invalidBrain, activation, loader, journal); !errors.Is(err, ErrInvalidBrain) {
		t.Fatalf("invalid brain error = %v, want ErrInvalidBrain", err)
	}
	invalidProjection := projection
	invalidProjection.RenderedSHA256 = "bad"
	invalidLoader := newPolicyTestProjectionLoader(invalidProjection)
	snapshot, _ := policyTestSnapshot(t, projection)
	policy, err := New(client, brain, activation, invalidLoader, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("invalid projection error = %v, want ErrInvalidProjection", err)
	}
}

func TestPolicyRejectsProjectionBeyondBoundBrainLimitsBeforeModelCall(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	client, journal := &policyTestClient{}, &policyTestCallJournal{}

	byteLimited := policyTestBrain()
	byteLimited.ContextCeilingBytes = projection.RenderedBytes - 1
	refreshPolicyTestSampling(&byteLimited)
	attested := policyAttestBrain(byteLimited)
	policy, err := New(
		client, attested, policyTestDefaultProviderProcessActivation(attested),
		newPolicyTestProjectionLoader(projection), journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshotForProjection(t, projection)); !errors.Is(err, ErrInvalidBrain) {
		t.Fatalf("byte ceiling error = %v, want ErrInvalidBrain", err)
	}
	if client.generateCalls != 0 {
		t.Fatal("invalid projection limits reached the model")
	}
}

func TestPolicyOwnsProjectionCopy(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	brain := policyTestAttestedBrain()
	policy, err := New(
		client, brain, policyTestDefaultProviderProcessActivation(brain),
		newPolicyTestProjectionLoader(projection), &policyTestCallJournal{},
	)
	if err != nil {
		t.Fatal(err)
	}
	projection.Selected[0].ContentSHA256 = "mutated"
	if _, err := policy.Decide(context.Background(), snapshot); err != nil {
		t.Fatalf("caller mutation reached bound policy projection: %v", err)
	}
}

func snapshotForProjection(t *testing.T, projection contextbuilder.Projection) cognition.RuntimeSnapshot {
	t.Helper()
	snapshot, _ := policyTestSnapshot(t, projection)
	return snapshot
}
