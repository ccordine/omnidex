package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestSemanticUncertaintyContractsExhaustivelyCoverWorkKinds(t *testing.T) {
	kinds := AllWorkKinds()
	if len(kinds) == 0 {
		t.Fatal("closed work-kind registry is empty")
	}
	seenKinds := make(map[WorkKind]struct{}, len(kinds))
	seenIDs := make(map[string]WorkKind, len(kinds))
	seenDigests := make(map[string]WorkKind, len(kinds))
	seenQuestions := make(map[string]WorkKind, len(kinds))
	for _, kind := range kinds {
		if _, duplicate := seenKinds[kind]; duplicate {
			t.Fatalf("work kind %q is duplicated", kind)
		}
		seenKinds[kind] = struct{}{}
		contract, err := SemanticUncertaintyContractForWorkKind(kind)
		if err != nil {
			t.Fatalf("resolve %q: %v", kind, err)
		}
		if contract.WorkKind != kind {
			t.Fatalf("%q resolved contract for %q", kind, contract.WorkKind)
		}
		if err := contract.Validate(); err != nil {
			t.Fatalf("validate %q: %v", kind, err)
		}
		if previous, duplicate := seenIDs[contract.ID]; duplicate {
			t.Fatalf("%q and %q share contract ID %q", previous, kind, contract.ID)
		}
		seenIDs[contract.ID] = kind
		if previous, duplicate := seenQuestions[contract.ExactQuestion]; duplicate {
			t.Fatalf("%q and %q share exact question %q", previous, kind, contract.ExactQuestion)
		}
		seenQuestions[contract.ExactQuestion] = kind
		digest, err := contract.Digest()
		if err != nil {
			t.Fatalf("digest %q: %v", kind, err)
		}
		if len(digest) != 64 {
			t.Fatalf("%q digest has %d bytes", kind, len(digest))
		}
		if previous, duplicate := seenDigests[digest]; duplicate {
			t.Fatalf("%q and %q share digest %q", previous, kind, digest)
		}
		seenDigests[digest] = kind
	}
}

func TestSemanticUncertaintyContractLookupHasNoFallback(t *testing.T) {
	if _, err := SemanticUncertaintyContractForWorkKind(WorkKind("unknown")); err == nil {
		t.Fatal("unknown work kind resolved a semantic uncertainty contract")
	}
}

func TestApplicationIntentSemanticUncertaintyVersionsAreExact(t *testing.T) {
	t.Parallel()
	for _, kind := range []WorkKind{
		WorkApplicationProductContext,
		WorkApplicationRequirementCoverage,
		WorkApplicationRequirement,
	} {
		current, err := SemanticUncertaintyContractForPortableRenderer(
			PortableRendererV8, kind,
		)
		if err != nil {
			t.Fatal(err)
		}
		wantCurrentVersion := ".v2"
		if kind == WorkApplicationRequirement {
			wantCurrentVersion = ".v3"
		}
		if !strings.HasSuffix(current.ID, wantCurrentVersion) {
			t.Fatalf("current %s contract=%+v", kind, current)
		}
		if kind == WorkApplicationProductContext {
			if !strings.Contains(current.ExactQuestion, "product or domain identity") ||
				!strings.Contains(current.SingleResult, "excludes software requirements") {
				t.Fatalf("current %s contract is not identity-only: %+v", kind, current)
			}
		} else {
			if strings.Contains(current.RequiredInformation, "product context") {
				t.Fatalf("current %s contract retains redundant product context: %+v", kind, current)
			}
			if !strings.Contains(current.ExactQuestion, "task-local runtime implementation requirement") {
				t.Fatalf("current %s contract is not source-task-local: %+v", kind, current)
			}
			if kind == WorkApplicationRequirement &&
				!strings.Contains(current.RequiredInformation, ApplicationRequirementRemains) {
				t.Fatalf("current requirement contract lacks bound coverage authority: %+v", current)
			}
		}
		for _, renderer := range []string{
			HistoricalPortableRendererV7,
		} {
			historical, err := SemanticUncertaintyContractForPortableRenderer(
				renderer, kind,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(historical.ID, ".v2") {
				t.Fatalf("historical %s/%s contract is not V2: %+v", renderer, kind, historical)
			}
			if kind == WorkApplicationRequirement {
				if historical == current || strings.Contains(
					historical.RequiredInformation, ApplicationRequirementRemains,
				) {
					t.Fatalf("historical %s requirement contract was reinterpreted: %+v", renderer, historical)
				}
			} else if historical != current {
				t.Fatalf("historical %s/%s contract drifted from V2: %+v", renderer, kind, historical)
			}
		}
		for _, renderer := range []string{
			HistoricalPortableRendererV5,
			HistoricalPortableRendererV6,
		} {
			historical, err := SemanticUncertaintyContractForPortableRenderer(
				renderer, kind,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(historical.ID, ".v1") {
				t.Fatalf("historical %s/%s contract=%+v", renderer, kind, historical)
			}
			if kind == WorkApplicationProductContext {
				if historical.ExactQuestion != "What concise product context is explicitly established by the software request?" ||
					historical.SingleResult != "One concise product-context text leaf." {
					t.Fatalf("historical %s/%s product contract drifted: %+v", renderer, kind, historical)
				}
			} else if !strings.Contains(historical.RequiredInformation, "product context") {
				t.Fatalf("historical %s/%s contract lost product context: %+v", renderer, kind, historical)
			}
			if historical == current {
				t.Fatalf("historical %s contract equals current V2 contract", kind)
			}
			if _, err := historical.Digest(); err != nil {
				t.Fatalf("digest historical %s/%s contract: %v", renderer, kind, err)
			}
		}
	}
}

func TestSemanticUncertaintyRendererLookupRejectsUnknownHistory(t *testing.T) {
	t.Parallel()
	if _, err := SemanticUncertaintyContractForPortableRenderer(
		"omnidex.render-portable-job.v4", WorkApplicationRequirement,
	); err == nil {
		t.Fatal("unregistered renderer resolved semantic uncertainty authority")
	}
}

func TestRendererV8OnlyRequirementRefinementContractsRejectHistoricalRenderers(
	t *testing.T,
) {
	t.Parallel()
	for _, kind := range []WorkKind{
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateSplit,
		WorkApplicationRequirementCandidateSplitCorrection,
	} {
		current, err := SemanticUncertaintyContractForPortableRenderer(
			PortableRendererV8, kind,
		)
		if err != nil || !strings.HasSuffix(current.ID, ".v1") {
			t.Fatalf("current %s contract=%+v error=%v", kind, current, err)
		}
		for _, renderer := range []string{
			HistoricalPortableRendererV7,
			HistoricalPortableRendererV6,
			HistoricalPortableRendererV5,
		} {
			if _, err := SemanticUncertaintyContractForPortableRenderer(
				renderer, kind,
			); err == nil {
				t.Fatalf("historical renderer %s accepted V8-only work kind %s", renderer, kind)
			}
		}
	}
}

func TestSemanticUncertaintyContractRejectsBlankFivePartAnswer(t *testing.T) {
	contract := mustSemanticUncertaintyContract(t, WorkApplicationClassify)
	mutations := []func(*SemanticUncertaintyContract){
		func(value *SemanticUncertaintyContract) { value.ExactQuestion = "" },
		func(value *SemanticUncertaintyContract) { value.DeterministicLimitation = "" },
		func(value *SemanticUncertaintyContract) { value.RequiredInformation = "" },
		func(value *SemanticUncertaintyContract) { value.SingleResult = "" },
		func(value *SemanticUncertaintyContract) { value.DeterministicConsumer = "" },
	}
	for index, mutate := range mutations {
		candidate := contract
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("blank five-part answer %d was accepted", index)
		}
	}
}

func TestSemanticUncertaintyContractRejectsGeneralAuthorityLanguage(t *testing.T) {
	contract := mustSemanticUncertaintyContract(t, WorkApplicationClassify)
	for _, forbidden := range []string{
		"Which agent should decide?",
		"The worker must choose a value.",
		"A tool selection inventory.",
		"One control plane value.",
		"A workflow decision consumes the result.",
	} {
		candidate := contract
		candidate.RequiredInformation = forbidden
		if strings.HasSuffix(forbidden, "?") {
			candidate.ExactQuestion = forbidden
			candidate.RequiredInformation = contract.RequiredInformation
		}
		if err := candidate.Validate(); err == nil {
			t.Fatalf("forbidden authority language %q was accepted", forbidden)
		}
	}
}

func TestSemanticUncertaintyContractEnforcesOneResponsibility(t *testing.T) {
	contract := mustSemanticUncertaintyContract(t, WorkApplicationClassify)
	for name, mutate := range map[string]func(*SemanticUncertaintyContract){
		"two questions": func(value *SemanticUncertaintyContract) {
			value.ExactQuestion = "Which surface applies? Which format applies?"
		},
		"compound question": func(value *SemanticUncertaintyContract) {
			value.ExactQuestion = "Which surface and format apply?"
		},
		"compound result": func(value *SemanticUncertaintyContract) {
			value.SingleResult = "One surface and one format."
		},
		"non-single result": func(value *SemanticUncertaintyContract) {
			value.SingleResult = "A registered surface."
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := contract
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("compound semantic responsibility was accepted")
			}
		})
	}
}

func TestSemanticUncertaintyContractDigestRejectsMutation(t *testing.T) {
	contract := mustSemanticUncertaintyContract(t, WorkApplicationClassify)
	first, err := contract.Digest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := contract.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("contract digest changed: %q != %q", first, second)
	}
	contract.ID = "locally-mutated"
	if _, err := contract.Digest(); err == nil {
		t.Fatal("locally mutated contract was digested")
	}
}

func TestSemanticUncertaintyRegistryDigestIsStable(t *testing.T) {
	hash := sha256.New()
	for _, kind := range AllWorkKinds() {
		contract := mustSemanticUncertaintyContract(t, kind)
		digest, err := contract.Digest()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = hash.Write([]byte(kind))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(digest))
		_, _ = hash.Write([]byte{0})
	}
	got := hex.EncodeToString(hash.Sum(nil))
	const want = "cc703c7c88c1d39945cc3a3a539bc0661e76f01cf1dd9b11e9d615dbaeaf1a45"
	if got != want {
		t.Fatalf("semantic uncertainty registry digest changed: got %s want %s", got, want)
	}
}

func mustSemanticUncertaintyContract(
	t *testing.T,
	kind WorkKind,
) SemanticUncertaintyContract {
	t.Helper()
	contract, err := SemanticUncertaintyContractForWorkKind(kind)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}
