package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/llm"
)

func reserveAcceptedDecisionWithoutAction(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) (CognitionRuntimeSnapshotRecord, cognition.CoordinatorStep) {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context, CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	schema := fixture.Start.ActionCatalog.Schemas[0]
	request, err := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{})
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID, Action: request,
		EvidenceRefs: []cognition.EvidenceRef{}, ExpectedEffect: "Expose bounded public state.",
	}
	response, _, err := cognitionJSON(decision)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: string(response)}, cognitionTestBrain(),
		cognitionGuardProjectionLoader{repository: fixture.Repository},
		CognitionPolicyCallJournal{Repository: fixture.Repository},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		fixture.Start.Root.ID, fixture.Start.Root.CompletionCheck,
		prepared.Prepared.Snapshot.CurrentRevision(), cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := cognition.NewCoordinator(policy)
	if err != nil {
		t.Fatal(err)
	}
	step, err := coordinator.Step(
		fixture.Context, prepared.Prepared.Snapshot, completion,
		prepared.Prepared.CompletionEvidenceRefs,
	)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, step
}

func cognitionTestBrain() cognitionpolicy.AttestedBrain {
	return cognitionTestBrainWithCPU("c")
}

func cognitionTestBrainWithCPU(character string) cognitionpolicy.AttestedBrain {
	sampling, err := cognitionpolicy.NewSamplingIdentity(
		1_000_000, cognitionpolicy.MaxEnvelopeBytes, 4*1024,
	)
	if err != nil {
		panic(err)
	}
	brain, err := cognitionpolicy.NewBrainRef(
		"model:test", strings.Repeat("b", 64), "q4_k_m",
		"test-backend", "1.0.0", "test-hardware", sampling,
	)
	if err != nil {
		panic(err)
	}
	expected, err := brain.ProviderExpectation()
	if err != nil {
		panic(err)
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "queue-test:/version", "queue-test:/installed", "queue-test:/runner",
	)
	if err != nil {
		panic(err)
	}
	host, err := cognitionpolicy.NewHostHardwareAttestation(
		"linux", "amd64", 8, strings.Repeat(character, 64), strings.Repeat("d", 64),
	)
	if err != nil {
		panic(err)
	}
	attested, err := cognitionpolicy.NewAttestedBrain(brain, attestation, host)
	if err != nil {
		panic(err)
	}
	return attested
}

func recoveryRefForTest(ref cognitionruntime.AcceptedDecisionRecoveryRef) *cognitionruntime.AcceptedDecisionRecoveryRef {
	return &ref
}

func cognitionDecisionPointer(value cognition.CognitionDecision) *cognition.CognitionDecision {
	copy := value.Clone()
	return &copy
}

func assertAcceptedRecoveryCounts(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	calls, recoveries, actions int,
) {
	t.Helper()
	var gotCalls, gotRecoveries, gotActions int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT
		  (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		  (SELECT COUNT(*) FROM cognition_accepted_decision_recoveries WHERE episode_id=$1),
		  (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&gotCalls, &gotRecoveries, &gotActions); err != nil {
		t.Fatal(err)
	}
	if gotCalls != calls || gotRecoveries != recoveries || gotActions != actions {
		t.Fatalf("durable calls/recoveries/actions=%d/%d/%d want %d/%d/%d",
			gotCalls, gotRecoveries, gotActions, calls, recoveries, actions)
	}
}
