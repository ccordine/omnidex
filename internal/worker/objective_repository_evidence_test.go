package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestObjectiveRepositoryRelationCapsuleProjectsOnlySemanticFact(t *testing.T) {
	t.Parallel()
	pack := repositoryAcquisitionTestPack(t, "Explain which existing component owns dispatch.")
	from := pack.Symbols[0]
	to := from
	to.ID = "relation-target"
	to.Name = "DispatchTarget"
	pack.Symbols = append(pack.Symbols, to)
	pack.Relations = []repositoryretrieval.EvidenceRelation{{
		ID: "relation-provenance-sentinel", FromID: from.ID, ToID: to.ID,
		Kind: "calls", Origin: "origin-hidden-sentinel", Confidence: 0.731,
	}}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}

	capsules, err := repositoryEvidenceCapsules(pack, assemblyline.ArtifactIdentityProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	relation := capsules[len(capsules)-1]
	if relation.Capsule.Text != from.Name+" calls "+to.Name {
		t.Fatalf("relation capsule text=%q", relation.Capsule.Text)
	}
	for _, hidden := range []string{from.ID, to.ID, "origin-hidden-sentinel", "0.731", "from_id", "confidence"} {
		if strings.Contains(relation.Capsule.Text, hidden) {
			t.Fatalf("relation capsule exposed code-owned metadata %q: %s", hidden, relation.Capsule.Text)
		}
	}
	if !strings.Contains(relation.SourceRef, pack.ID+"#relation-provenance-sentinel") {
		t.Fatalf("relation provenance was not retained outside model text: %#v", relation)
	}
}

func TestObjectiveRepositoryEvidenceUsesCodeOwnedQueryAndOpaqueEvidenceSelection(t *testing.T) {
	t.Parallel()

	const requirement = "Which component owns dispatch?"
	const query = "Explain which existing component owns dispatch."
	queries := make([]string, 0, 1)
	relevanceCalls := 0
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		requirement,
		query,
		assemblyline.ArtifactIdentityProvenance{},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(exactRequirement string, evidence []objectiveEvidence) (
			assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error,
		) {
			relevanceCalls++
			if exactRequirement != requirement || len(evidence) != 1 || evidence[0].Capsule.ID != "R01" {
				t.Fatalf("relevance projection requirement=%q evidence=%#v", exactRequirement, evidence)
			}
			return assemblyline.RepositoryEvidenceRelevanceDecision{
				Schema: assemblyline.RepositoryEvidenceRelevanceSchemaV1, EvidenceIDs: []string{"R01"},
			}, objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || queries[0] != query ||
		relevanceCalls != 1 || result.ModelCalls != 1 ||
		len(result.Evidence) != 1 || result.Evidence[0].Capsule.ID != "R01" ||
		result.RepositoryCallLedger.relevanceReceipt != (objectiveStationReceipt{Calls: 1}) ||
		!result.RepositoryCallLedger.relevanceRecorded {
		t.Fatalf(
			"queries=%v relevance_calls=%d result=%#v",
			queries, relevanceCalls, result,
		)
	}
}

func TestObjectiveRepositoryEvidenceAcceptsFullyRestoredRelevanceRound(t *testing.T) {
	t.Parallel()
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		"Which component owns dispatch?",
		"Explain which existing component owns dispatch.",
		assemblyline.ArtifactIdentityProvenance{},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(_ string, evidence []objectiveEvidence) (
			assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error,
		) {
			return assemblyline.RepositoryEvidenceRelevanceDecision{
				Schema:      assemblyline.RepositoryEvidenceRelevanceSchemaV1,
				EvidenceIDs: []string{evidence[0].Capsule.ID},
			}, objectiveStationReceipt{Reused: true}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelCalls != 0 || !result.RepositoryCallLedger.relevanceRecorded ||
		result.RepositoryCallLedger.relevanceReceipt != (objectiveStationReceipt{Reused: true}) {
		t.Fatalf("result=%+v", result)
	}
}

func TestObjectiveRepositoryEvidenceDoesNotInventAnotherQueryAfterNoSelection(t *testing.T) {
	t.Parallel()

	buildCalls := 0
	_, err := acquireObjectiveRepositoryEvidenceClosure(
		"Which component owns dispatch?",
		"Explain which existing component owns dispatch.",
		assemblyline.ArtifactIdentityProvenance{},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			buildCalls++
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(string, []objectiveEvidence) (
			assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error,
		) {
			return assemblyline.RepositoryEvidenceRelevanceDecision{
				Schema:      assemblyline.RepositoryEvidenceRelevanceSchemaV1,
				EvidenceIDs: []string{},
			}, objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if !errors.Is(err, repositoryretrieval.ErrInsufficientEvidence) || buildCalls != 1 {
		t.Fatalf("error=%v build calls=%d", err, buildCalls)
	}
}

func TestObjectiveRepositoryEvidencePreservesCodeOwnedArtifactBindings(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		[]string{"internal/private/secret_owner.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		"What owns internal/private/secret_owner.go?",
		"What owns internal/private/secret_owner.go?",
		provenance,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(requirement string, evidence []objectiveEvidence) (
			assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error,
		) {
			if strings.Contains(requirement, "secret_owner.go") ||
				!strings.Contains(requirement, "ARTIFACT_1") {
				t.Fatalf("relevance received unredacted requirement %q", requirement)
			}
			return assemblyline.RepositoryEvidenceRelevanceDecision{
				Schema:      assemblyline.RepositoryEvidenceRelevanceSchemaV1,
				EvidenceIDs: []string{evidence[0].Capsule.ID},
			}, objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ArtifactIdentities) != 1 ||
		result.ArtifactIdentities[0].Value != "internal/private/secret_owner.go" {
		t.Fatalf("artifact bindings=%#v", result.ArtifactIdentities)
	}
	restored, err := assemblyline.RestoreArtifactIdentities(
		"ARTIFACT_1 owns the behavior.", result.ArtifactIdentities,
	)
	if err != nil {
		t.Fatal(err)
	}
	if restored != "internal/private/secret_owner.go owns the behavior." {
		t.Fatalf("restored=%q", restored)
	}
}
