package repositoryobjective

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSemanticPacketContainsOnlyQuestionCandidatesAndDeclarationEvidence(t *testing.T) {
	root := storageFixture(t)
	var captured SemanticGap
	selector := selectorFunc(func(_ context.Context, gap SemanticGap) (CandidateID, error) {
		captured = gap.Clone()
		return gap.Candidates[0].ID, nil
	})
	result, err := Run(t.Context(), Objective{
		ID: "objective.packet", Root: root, Question: "Which declaration owns durable storage?",
		Subject:    SubjectLookup{Kind: LookupName, Value: "Resolve"},
		Acceptance: fullAcceptance(),
	}, selector)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(captured)
	if err != nil {
		t.Fatal(err)
	}
	packet := string(raw)
	for _, forbidden := range []string{
		root, "go.mod", "cache/cache.go", "database/database.go", "TestResolve",
		"read_file", "grep", "run_tests", "tool", "action", "operation",
	} {
		if strings.Contains(packet, forbidden) {
			t.Errorf("semantic packet leaked forbidden content %q: %s", forbidden, packet)
		}
	}
	for _, candidate := range captured.Candidates {
		if strings.Contains(packet, result.Subject.Symbol.SymbolID) || strings.Contains(candidate.Summary, "/") {
			t.Fatalf("candidate exposed repository identity: %s", packet)
		}
	}
	if len(captured.Candidates) != 2 || len(captured.Evidence) != 2 {
		t.Fatalf("unexpected bounded packet: %#v", captured)
	}
}

func TestSelectorCannotMutateCodeHeldGapOrObjectiveAcceptance(t *testing.T) {
	acceptance := fullAcceptance()
	selector := selectorFunc(func(_ context.Context, gap SemanticGap) (CandidateID, error) {
		selected := gap.Candidates[0].ID
		gap.Question = "mutated"
		gap.Candidates[0].Summary = "mutated"
		gap.Candidates[0].EvidenceIDs[0] = "E99"
		gap.Evidence[0].Content = "mutated"
		return selected, nil
	})
	result, err := Run(t.Context(), Objective{
		ID: "objective.alias", Root: storageFixture(t), Question: "Which declaration owns durable storage?",
		Subject: SubjectLookup{Kind: LookupName, Value: "Resolve"}, Acceptance: acceptance,
	}, selector)
	if err != nil {
		t.Fatal(err)
	}
	acceptance[0] = AcceptanceApplicableTestsKnown
	result.Acceptance[0] = AcceptanceApplicableTestsKnown
	if result.Subject.Acceptance[0] != AcceptanceSubjectResolved ||
		result.Satisfied[0] != AcceptanceSubjectResolved {
		t.Fatalf("caller/result aliases changed accepted authority: %#v", result)
	}
}
