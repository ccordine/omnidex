package queue

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
		cognitionGuardActivationAuthority(t, fixture),
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
	return cognitionTestBrainBootstrapWithCPU("c").AttestedBrain
}

func cognitionTestBrainWithCPU(character string) cognitionpolicy.AttestedBrain {
	return cognitionTestBrainBootstrapWithCPU(character).AttestedBrain
}

func cognitionTestBrainBootstrap() cognitionpolicy.BrainBootstrap {
	return cognitionTestBrainBootstrapWithCPU("c")
}

func cognitionTestBrainBootstrapWithCPU(character string) cognitionpolicy.BrainBootstrap {
	sampling, err := cognitionpolicy.NewSamplingIdentity(
		1_000_000, cognitionpolicy.MaxEnvelopeBytes, 4*1024,
	)
	if err != nil {
		panic(err)
	}
	brain, err := cognitionpolicy.NewBrainRef(
		"model:test", strings.Repeat("b", 64), "q4_k_m",
		llm.ExactPreparedProviderBackend, llm.ExactPreparedProviderVersion,
		"test-hardware", sampling,
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
	bootstrap, err := cognitionpolicy.BootstrapProviderIdentityRequest(brain)
	if err != nil {
		panic(err)
	}
	observed, err := queueTestObservedProviderIdentity(
		time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC), attestation,
		bootstrap.ChallengeSHA256,
	)
	if err != nil {
		panic(err)
	}
	host, err := cognitionpolicy.AttestLocalHostHardware()
	if err != nil {
		panic(err)
	}
	if character != "c" {
		host, err = cognitionpolicy.NewHostHardwareAttestation(
			host.OS, host.Architecture, host.LogicalCPUs,
			strings.Repeat(character, 64), host.AcceleratorIdentitySHA256,
		)
		if err != nil {
			panic(err)
		}
	}
	attested, err := cognitionpolicy.NewAttestedBrain(brain, attestation, observed.Observation, host)
	if err != nil {
		panic(err)
	}
	result, err := cognitionpolicy.NewBrainBootstrap(attested, observed.Evidence)
	if err != nil {
		panic(err)
	}
	return result
}

func queueTestObservedProviderIdentity(
	observedAt time.Time,
	attestation llm.ProviderIdentityAttestation,
	challenge string,
) (llm.ObservedProviderIdentity, error) {
	selection := llm.ProviderIdentitySelection{
		Model: attestation.Model, NativeContextLimit: attestation.NativeContextLimit,
	}
	showRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	installed := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q}}]}`,
		attestation.Model, attestation.Model, attestation.Digest, attestation.Quantization,
	))
	show := []byte(`{"model_info":{"general.architecture":"qwen35",` +
		`"tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35",` +
		`"tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,` +
		`"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,` +
		`"tokenizer.ggml.merges":null}}`)
	runner := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q},"context_length":%d}]}`,
		attestation.Model, attestation.Model, attestation.Digest, attestation.Quantization,
		attestation.NativeContextLimit,
	))
	evidence, err := llm.NewSuccessfulProviderIdentityEvidence(
		[]byte(fmt.Sprintf(`{"version":%q}`, attestation.BackendVersion)), installed,
		showRequest, show, preloadRequest, []byte(`{"done":true}`), runner,
	)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	return llm.NewObservedProviderIdentity(observedAt, attestation, evidence, challenge)
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
