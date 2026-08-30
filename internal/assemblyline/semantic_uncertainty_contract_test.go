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
		current, err := SemanticUncertaintyContractForWorkKind(kind)
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
				!strings.Contains(current.RequiredInformation, "excluded non-runtime candidates") {
				t.Fatalf("current %s contract lacks excluded candidate authority: %+v", kind, current)
			}
			if kind == WorkApplicationRequirement &&
				!strings.Contains(current.RequiredInformation, ApplicationRequirementRemains) {
				t.Fatalf("current requirement contract lacks bound coverage authority: %+v", current)
			}
		}
	}
}

func TestRequirementRefinementSemanticUncertaintyContractsAreV1(
	t *testing.T,
) {
	t.Parallel()
	for _, kind := range []WorkKind{
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateOutcomeRelation,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidateResultRelationGrounding,
		WorkApplicationRequirementCandidateResultRelationCorrection,
		WorkApplicationRequirementCandidateSplit,
		WorkApplicationRequirementCandidateSplitCorrection,
	} {
		current, err := SemanticUncertaintyContractForWorkKind(kind)
		if err != nil || !strings.HasSuffix(current.ID, ".v1") {
			t.Fatalf("current %s contract=%+v error=%v", kind, current, err)
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
	const want = "7d5475947d2b63d2a864b15fec659509a0225986f47fb85ab3a054d37b9cf231"
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
