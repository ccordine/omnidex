package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

func TestRatDoctrineFreezesBrainAndExperimentalConstants(t *testing.T) {
	sampling, err := cognitionpolicy.NewSamplingIdentity(32_768, 24_000, 1024)
	if err != nil {
		t.Fatal(err)
	}
	samplingSHA, err := sampling.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	fixed := validFixedExperiment()
	fixed.Brain.SamplingSHA256 = samplingSHA
	fixed.Brain.Sampling = sampling
	fixed.ContextCeilingBytes = 24_000
	first, err := NewRatGeneration("rat-generation-1", fixed, RuntimeCandidate{
		Version: "cognition-runtime.v1", SourceSHA256: strings.Repeat("c", 64),
		ExecutableSHA256: strings.Repeat("1", 64), MigrationsSHA256: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRatGeneration("rat-generation-2", fixed, RuntimeCandidate{
		Version: "cognition-runtime.v2", SourceSHA256: strings.Repeat("d", 64),
		ExecutableSHA256: strings.Repeat("2", 64), MigrationsSHA256: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireComparableRatGenerations(first, second); err != nil {
		t.Fatal(err)
	}
	changed := fixed
	changedSampling, err := cognitionpolicy.NewSamplingIdentity(32_769, 24_000, 1024)
	if err != nil {
		t.Fatal(err)
	}
	changedSHA, err := changedSampling.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	changed.Brain.NativeContextLimit = changedSampling.NativeContextLimit
	changed.Brain.Sampling = changedSampling
	changed.Brain.SamplingSHA256 = changedSHA
	if _, validationErr := changed.Brain.attestedBrain(); validationErr == nil {
		t.Fatal("changed native context retained the old provider attestation")
	}
	providerExpectation := changed.Brain.ProviderAttestation
	providerExpectation.NativeContextLimit = changedSampling.NativeContextLimit
	providerExpectation.AttestationSHA256 = ""
	changed.Brain.ProviderAttestation, err = llm.NewProviderIdentityAttestation(
		llm.ProviderIdentityExpectation{
			Backend: providerExpectation.Backend, BackendVersion: providerExpectation.BackendVersion,
			Model: providerExpectation.Model, Digest: providerExpectation.Digest,
			Quantization:       providerExpectation.Quantization,
			NativeContextLimit: providerExpectation.NativeContextLimit,
		},
		providerExpectation.BackendEvidence, providerExpectation.InstalledEvidence,
		providerExpectation.RunnerEvidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	third, err := NewRatGeneration("rat-generation-3", changed, RuntimeCandidate{
		Version: "cognition-runtime.v3", SourceSHA256: strings.Repeat("e", 64),
		ExecutableSHA256: strings.Repeat("3", 64), MigrationsSHA256: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireComparableRatGenerations(first, third); err == nil {
		t.Fatal("brain/context change was treated as a cognition-runtime experiment")
	}
}

func TestRatDoctrineRejectsUnfrozenOrIdenticalRuntime(t *testing.T) {
	if _, err := NewRatGeneration("rat-generation-1", FixedExperiment{}, RuntimeCandidate{}); err == nil {
		t.Fatal("empty experiment authority was accepted")
	}
	fixed := validFixedExperiment()
	generation, err := NewRatGeneration("rat-generation-1", fixed, RuntimeCandidate{
		Version: "runtime.v1", SourceSHA256: strings.Repeat("c", 64),
		ExecutableSHA256: strings.Repeat("1", 64), MigrationsSHA256: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireComparableRatGenerations(generation, generation); err == nil {
		t.Fatal("identical runtime was treated as an evolutionary comparison")
	}
}

func TestRatDoctrineRejectsTamperedFixedExperiment(t *testing.T) {
	generation, err := NewRatGeneration("rat-generation-1", validFixedExperiment(), RuntimeCandidate{
		Version: "runtime.v1", SourceSHA256: strings.Repeat("c", 64),
		ExecutableSHA256: strings.Repeat("1", 64), MigrationsSHA256: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := generation
	tampered.Fixed.ContextCeilingBytes++
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered frozen experiment retained its old digest")
	}
	if err := RequireComparableRatGenerations(generation, tampered); err == nil {
		t.Fatal("tampered generation was accepted as comparable")
	}
}

func validFixedExperiment() FixedExperiment {
	sampling, err := cognitionpolicy.NewSamplingIdentity(32_768, 24_576, 1024)
	if err != nil {
		panic(err)
	}
	ref, err := cognitionpolicy.NewBrainRef(
		"qwen", strings.Repeat("a", 64), "q4", "ollama", "1.0.0", "pending", sampling,
	)
	if err != nil {
		panic(err)
	}
	expected, err := ref.ProviderExpectation()
	if err != nil {
		panic(err)
	}
	provider, err := llm.NewProviderIdentityAttestation(expected, "fixture:backend", "fixture:installed", "fixture:runner")
	if err != nil {
		panic(err)
	}
	host, err := cognitionpolicy.AttestLocalHostHardware()
	if err != nil {
		panic(err)
	}
	ref, err = cognitionpolicy.NewBrainRef(
		"qwen", strings.Repeat("a", 64), "q4", "ollama", "1.0.0",
		"host-attestation:"+host.AttestationSHA256, sampling,
	)
	if err != nil {
		panic(err)
	}
	attested, err := cognitionpolicy.NewAttestedBrain(ref, provider, host)
	if err != nil {
		panic(err)
	}
	brain, err := brainFingerprintFromAttested(attested)
	if err != nil {
		panic(err)
	}
	return FixedExperiment{
		Brain:               brain,
		ContextCeilingBytes: 24_576, EnvironmentContractVersion: "environment.v1",
		EvaluatorVersion: "evaluator.v1", AuthorityPolicyVersion: "authority.v1",
		OracleIsolationVersion: "separate-process.v1",
	}
}
