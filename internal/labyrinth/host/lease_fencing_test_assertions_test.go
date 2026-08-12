package host

import (
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func fencedHostBrainBootstrap(t *testing.T) cognitionpolicy.BrainBootstrap {
	t.Helper()
	host, err := cognitionpolicy.NewHostHardwareAttestation(
		"linux", "amd64", 1, fencedHostTestDigest, fencedHostTestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	sampling, err := cognitionpolicy.NewSamplingIdentity(32_768, 16_384, 256)
	if err != nil {
		t.Fatal(err)
	}
	brain, err := cognitionpolicy.NewBrainRef(
		"host-fence-model", fencedHostTestDigest, "Q4",
		llm.ExactPreparedProviderBackend, llm.ExactPreparedProviderVersion,
		"host-attestation:"+host.AttestationSHA256, sampling,
	)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := brain.ProviderExpectation()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := llm.NewProviderIdentityAttestation(
		expected, "backend-version", "installed-model", "runner-allocation",
	)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := cognitionpolicy.BootstrapProviderIdentityRequest(brain)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := fencedHostProviderEvidence(provider)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := llm.NewObservedProviderIdentity(
		time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC), provider,
		evidence, bootstrap.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	attested, err := cognitionpolicy.NewAttestedBrain(brain, provider, observation.Observation, host)
	if err != nil {
		t.Fatal(err)
	}
	result, err := cognitionpolicy.NewBrainBootstrap(attested, observation.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func fencedHostProviderEvidence(
	attestation llm.ProviderIdentityAttestation,
) (llm.ProviderIdentityEvidence, error) {
	selection := llm.ProviderIdentitySelection{
		Model: attestation.Model, NativeContextLimit: attestation.NativeContextLimit,
	}
	tokenizerRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	version := []byte(fmt.Sprintf(`{"version":%q}`, attestation.BackendVersion))
	installed := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"parent_model":"","format":"gguf","family":"qwen3","families":["qwen3"],"parameter_size":"9B","quantization_level":%q}}]}`,
		attestation.Model, attestation.Model, attestation.Digest, attestation.Quantization,
	))
	tokenizer := []byte(`{"model_info":{"general.architecture":"qwen35",` +
		`"tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35",` +
		`"tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,` +
		`"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,` +
		`"tokenizer.ggml.merges":null}}`)
	runner := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"size_vram":1,"digest":%q,"details":{"parent_model":"","format":"gguf","family":"qwen3","families":["qwen3"],"parameter_size":"9B","quantization_level":%q},"context_length":%d}]}`,
		attestation.Model, attestation.Model, attestation.Digest,
		attestation.Quantization, attestation.NativeContextLimit,
	))
	return llm.NewSuccessfulProviderIdentityEvidence(
		version, installed, tokenizerRequest, tokenizer,
		preloadRequest, []byte(`{"done":true}`), runner,
	)
}

func stepAuthority(actor cognition.AttemptRef) model.StepAttemptAuthority {
	return model.StepAttemptAuthority{
		JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
		Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
	}
}

func attemptRef(authority model.StepAttemptAuthority) cognition.AttemptRef {
	return cognition.AttemptRef{
		JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		Attempt: uint64(authority.Attempt), WorkerID: authority.WorkerID,
	}
}

func fencedWitnessAction(
	t *testing.T,
	fixture fencedHostFixture,
	started cognition.Transition,
	actor cognition.AttemptRef,
) cognition.RegisteredAction {
	t.Helper()
	witness := fixture.witness[0]
	schema, exists := fixture.scenario.Catalog().Schema(witness.Request.Kind)
	if !exists {
		t.Fatalf("witness schema %q is absent", witness.Request.Kind)
	}
	var evidence []cognition.EvidenceRef
	if schema.EvidencePolicy == cognition.EvidenceRequired {
		evidence = make([]cognition.EvidenceRef, len(started.Observations))
		for index, observation := range started.Observations {
			evidence[index] = observation.EvidenceRef()
		}
	}
	action, err := cognition.NewRegisteredAction(
		"lease-fenced-action", actor, schema, witness.Request, evidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func assertHostHead(
	t *testing.T,
	fixture fencedHostFixture,
	want cognition.WorldRevision,
	wantReceipts int,
) {
	t.Helper()
	receipt, err := fixture.store.Episode(t.Context(), fixture.episode)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := fixture.pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM `+fixture.store.relation("action_receipts")+` WHERE episode_id=$1
	`, fixture.episode.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if receipt.Current != want || count != wantReceipts {
		t.Fatalf("host head/receipts=%+v/%d want=%+v/%d", receipt.Current, count, want, wantReceipts)
	}
}
