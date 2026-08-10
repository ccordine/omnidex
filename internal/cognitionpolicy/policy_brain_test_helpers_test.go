package cognitionpolicy

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func policyTestBrain() BrainRef {
	sampling, err := NewSamplingIdentity(1_000_000, MaxEnvelopeBytes, 4*1024)
	if err != nil {
		panic(err)
	}
	brain, err := NewBrainRef(
		"model:test", strings.Repeat("b", 64), "q4_k_m",
		llm.ExactPreparedProviderBackend, llm.ExactPreparedProviderVersion,
		"test-hardware", sampling,
	)
	if err != nil {
		panic(err)
	}
	return brain
}

func policyTestAttestedBrain() AttestedBrain {
	return policyAttestBrain(policyTestBrain())
}

func policyAttestBrain(brain BrainRef) AttestedBrain {
	expected, err := brain.ProviderExpectation()
	if err != nil {
		panic(err)
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "test:/version", "test:/installed", "test:/runner",
	)
	if err != nil {
		panic(err)
	}
	request, err := BootstrapProviderIdentityRequest(brain)
	if err != nil {
		panic(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC), attestation,
		request.ChallengeSHA256,
	)
	if err != nil {
		panic(err)
	}
	host, err := AttestLocalHostHardware()
	if err != nil {
		panic(err)
	}
	attested, err := NewAttestedBrain(brain, attestation, observed.Observation, host)
	if err != nil {
		panic(err)
	}
	return attested
}

func policyTestObservedProviderIdentity(
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

func policyTestFailedProviderIdentityGeneration(
	attempt CallAttempt,
) llm.PreparedGeneration {
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil {
		panic(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		attempt.ProviderAttestation, strings.Repeat("a", 64),
	)
	if err != nil {
		panic(err)
	}
	operations := observed.Evidence.Clone().Operations
	operations[0], err = llm.NewProviderIdentityOperationEvidence(
		operations[0].Operation, operations[0].Method, operations[0].Endpoint,
		true, operations[0].Request, 503, llm.ProviderIdentityHTTPError,
		true, llm.NewProviderContentEncodingEvidence(nil, false),
		[]byte(`{"error":"unavailable"}`),
	)
	if err != nil {
		panic(err)
	}
	for index := 1; index < len(operations); index++ {
		operations[index], err = llm.NewProviderIdentityOperationEvidence(
			operations[index].Operation, operations[index].Method, operations[index].Endpoint,
			false, operations[index].Request, 0, llm.ProviderIdentityNotDispatched,
			false, llm.ProviderContentEncodingEvidence{}, nil,
		)
		if err != nil {
			panic(err)
		}
	}
	evidence, err := llm.NewProviderIdentityEvidence(operations)
	if err != nil {
		panic(err)
	}
	if err := evidence.ValidateRequests(llm.ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}); err != nil {
		panic(err)
	}
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, ProviderIdentityEvidence: evidence,
	}
}

func refreshPolicyTestSampling(brain *BrainRef) {
	brain.Sampling.NativeContextLimit = brain.NativeContextLimit
	brain.Sampling.ContextCeilingBytes = brain.ContextCeilingBytes
	sha, err := brain.Sampling.SHA256()
	if err != nil {
		panic(err)
	}
	brain.SamplingSHA256 = sha
}
