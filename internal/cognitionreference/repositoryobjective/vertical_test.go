package repositoryobjective

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func TestRunCompletesExactReadOnlyObjectiveWithoutInference(t *testing.T) {
	root := deliveryFixture(t)
	selector := selectorFunc(func(context.Context, SemanticGap) (CandidateID, error) {
		t.Fatal("deterministic exact lookup invoked semantic selector")
		return "", nil
	})
	result, err := Run(t.Context(), Objective{
		ID:         "objective.delivery-owner",
		Root:       root,
		Subject:    SubjectLookup{Kind: LookupQualifiedName, Value: "example.test/delivery.Dispatch"},
		Acceptance: fullAcceptance(),
	}, selector)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Complete || result.SelectorCalls != 0 || result.InferenceCalls != 0 {
		t.Fatalf("unexpected execution result: %#v", result)
	}
	if result.Subject.Authority != SubjectAuthorityDeterministic ||
		result.Subject.Symbol.QualifiedName != "example.test/delivery.Dispatch" {
		t.Fatalf("unexpected deterministic subject fact: %#v", result.Subject)
	}
	if got := qualifiedNames(result.DirectCalls); len(got) != 1 || got[0] != "example.test/delivery.nextWindow" {
		t.Fatalf("direct calls=%v", got)
	}
	if got := qualifiedNames(result.DirectCallers); len(got) != 1 || got[0] != "example.test/delivery.Run" {
		t.Fatalf("direct callers=%v", got)
	}
	if got := qualifiedNames(result.ApplicableTests); len(got) != 1 || got[0] != "example.test/delivery.TestDispatch" {
		t.Fatalf("applicable tests=%v", got)
	}
	if result.BeforeSnapshotID == "" || result.BeforeSnapshotID != result.AfterSnapshotID {
		t.Fatalf("repository authority changed: before=%q after=%q", result.BeforeSnapshotID, result.AfterSnapshotID)
	}
	requireExactAcceptance(t, result)
}

func TestRunCrossesOneSemanticSubjectGapThenCodeCompletes(t *testing.T) {
	root := storageFixture(t)
	const question = "Which declaration resolves values from durable storage rather than volatile process memory?"
	var captured SemanticGap
	selector := selectorFunc(func(_ context.Context, gap SemanticGap) (CandidateID, error) {
		captured = gap.Clone()
		for _, candidate := range gap.Candidates {
			for _, evidenceID := range candidate.EvidenceIDs {
				for _, evidence := range gap.Evidence {
					if evidence.ID == evidenceID && strings.Contains(evidence.Content, "durableRecord") {
						return candidate.ID, nil
					}
				}
			}
		}
		t.Fatal("semantic packet did not contain the distinguishing declaration evidence")
		return "", nil
	})
	acceptance := fullAcceptance()
	result, err := Run(t.Context(), Objective{
		ID: "objective.storage-owner", Root: root, Question: question,
		Subject:    SubjectLookup{Kind: LookupName, Value: "Resolve"},
		Acceptance: acceptance,
	}, selector)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Complete || result.SelectorCalls != 1 || result.InferenceCalls != 1 {
		t.Fatalf("unexpected execution result: %#v", result)
	}
	if result.Subject.Authority != SubjectAuthoritySemantic ||
		result.Subject.Symbol.QualifiedName != "example.test/storage/database.Resolve" ||
		result.Subject.GapID == "" || result.Subject.CandidateID == "" {
		t.Fatalf("unexpected semantic subject fact: %#v", result.Subject)
	}
	if captured.Question != question || captured.ObjectiveID != cognitionreference.ObjectiveID("objective.storage-owner") {
		t.Fatalf("gap did not bind exact question/objective: %#v", captured)
	}
	if got := qualifiedNames(result.DirectCalls); len(got) != 1 || got[0] != "example.test/storage/database.durableRecord" {
		t.Fatalf("code did not resume direct traversal: %v", got)
	}
	if got := qualifiedNames(result.ApplicableTests); len(got) != 1 || got[0] != "example.test/storage/database.TestResolve" {
		t.Fatalf("code did not resume test traversal: %v", got)
	}
	if result.BeforeSnapshotID != result.AfterSnapshotID {
		t.Fatalf("read-only objective mutated authority: before=%q after=%q", result.BeforeSnapshotID, result.AfterSnapshotID)
	}
	requireExactAcceptance(t, result)
}

func qualifiedNames(values []SymbolEvidence) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.QualifiedName
	}
	return result
}

func requireExactAcceptance(t *testing.T, result Result) {
	t.Helper()
	if len(result.Acceptance) != len(result.Satisfied) {
		t.Fatalf("acceptance=%v satisfied=%v", result.Acceptance, result.Satisfied)
	}
	for index := range result.Acceptance {
		if result.Acceptance[index] != result.Satisfied[index] {
			t.Fatalf("acceptance=%v satisfied=%v", result.Acceptance, result.Satisfied)
		}
	}
}
