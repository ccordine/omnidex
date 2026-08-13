package worker

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialists"
)

type countingSkillEmbeddingClient struct{ calls atomic.Int64 }

func (client *countingSkillEmbeddingClient) Embedding(context.Context, string) ([]float64, error) {
	client.calls.Add(1)
	return []float64{0.1, 0.2}, nil
}

func TestLearnedCodingSkillIdentityIsCodeOwned(t *testing.T) {
	t.Parallel()

	input := codingSkillNeed{
		LocalContext: "interactive browser tool",
		Need:         "support pressure-sensitive pointer interaction",
	}
	id := learnedCodingSkillID(input)
	if !strings.HasPrefix(id, "learned_") || strings.Contains(id, "pointer") {
		t.Fatalf("learned skill identity is not opaque and code-owned: %q", id)
	}
}

func TestCodingProgramAllowsNoLearnedSkillBinding(t *testing.T) {
	t.Parallel()

	requirements := []assemblyline.Requirement{{ID: "requirement_001", SourceQuote: "filter visible records"}}
	if err := validateDirectCodingSkillBindings(requirements, nil); err != nil {
		t.Fatalf("optional learned skill binding rejected: %v", err)
	}
}

func TestEmptyActiveSkillRegistryMakesNoEmbeddingOrSelectionCall(t *testing.T) {
	ctx, repository, _ := openRepositoryTestDatabase(t)
	embeddings := &countingSkillEmbeddingClient{}
	service := &Service{
		repo: repository, embeddings: embeddings,
		embeddingProvider: "test-provider", embeddingModel: "test-model",
	}
	session := &directCodingSession{runtime: &nativeRuntimeV3{svc: service, ctx: ctx}}
	bindings, err := session.bindRequirementSkills("one local context", []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "one exact local need"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 0 {
		t.Fatalf("empty registry returned bindings=%v", bindings)
	}
	if calls := embeddings.calls.Load(); calls != 0 {
		t.Fatalf("empty registry made %d embedding calls", calls)
	}
}

func TestCodingProgramRejectsPendingLearnedSkillBinding(t *testing.T) {
	t.Parallel()

	requirements := []assemblyline.Requirement{{ID: "requirement_001", SourceQuote: "filter visible records"}}
	version := validDirectCodingSkillVersion(t, specialists.SkillStatus("candidate"))
	err := validateDirectCodingSkillBindings(requirements, map[string]directCodingSkillBinding{
		requirements[0].ID: {
			RequirementID: requirements[0].ID,
			Procedure:     version.Spec.Instructions,
			Version:       version,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("pending binding error=%v, want active-only failure", err)
	}
}

func TestCodingContractDoesNotRequireLearnedSkillEnrichment(t *testing.T) {
	t.Parallel()

	requirement := assemblyline.Requirement{ID: "requirement_001", SourceQuote: "filter visible records"}
	contract := genericBrowserFeatureContract(
		requirement,
		nil,
		nil,
		assemblyline.ApplicationSpecification{ProductQuote: "a bounded browser tool"},
	)
	if !strings.Contains(contract, "Exact feature: "+requirement.SourceQuote) {
		t.Fatalf("contract lost exact requirement: %q", contract)
	}
	if strings.Contains(strings.ToLower(contract), "validated procedure") {
		t.Fatalf("contract invented a required learned procedure: %q", contract)
	}
}

func validDirectCodingSkillVersion(t *testing.T, status specialists.SkillStatus) specialists.SkillVersion {
	t.Helper()
	spec, err := specialists.SpecWithSchemaDocuments(specialists.Spec{
		ID:           "learned_0123456789abcdef0123456789abcdef",
		Purpose:      "Local context: a bounded browser tool\nLocal need: filter visible records",
		Instructions: "Apply one bounded filter.",
	}, json.RawMessage(`{"type":"object","additionalProperties":false}`),
		json.RawMessage(`{"type":"object","additionalProperties":false}`))
	if err != nil {
		t.Fatal(err)
	}
	jobID := int64(41)
	hash, err := specialists.SkillContentHash(spec, specialists.SkillKindCodeProcedure)
	if err != nil {
		t.Fatal(err)
	}
	return specialists.SkillVersion{
		Spec: spec, Version: 1, Status: status, Source: specialists.SkillSourceLearned,
		Kind: specialists.SkillKindCodeProcedure, CreatedByJobID: &jobID, ContentSHA256: hash,
	}
}
