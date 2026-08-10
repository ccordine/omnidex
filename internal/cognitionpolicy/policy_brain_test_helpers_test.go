package cognitionpolicy

import (
	"strings"

	"github.com/gryph/omnidex/internal/llm"
)

func policyTestBrain() BrainRef {
	sampling, err := NewSamplingIdentity(1_000_000, MaxEnvelopeBytes, 4*1024)
	if err != nil {
		panic(err)
	}
	brain, err := NewBrainRef(
		"model:test", strings.Repeat("b", 64), "q4_k_m",
		"test-backend", "1.0.0", "test-hardware", sampling,
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
	host, err := AttestLocalHostHardware()
	if err != nil {
		panic(err)
	}
	attested, err := NewAttestedBrain(brain, attestation, host)
	if err != nil {
		panic(err)
	}
	return attested
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
