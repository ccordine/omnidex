package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func repositoryLedgerItem(id string, rank int64) model.LedgerItem {
	return model.LedgerItem{
		ID:         model.LedgerItemID(id),
		Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "turn-1", Revision: 1},
		Rationale:  "required by the objective", Rank: rank,
		Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewAgreed,
	}
}

func repositoryLedgerFixture() model.EngineeringLedger {
	return model.EngineeringLedger{ID: "proposal", Revision: 1, Tasks: []model.LedgerTask{{
		LedgerItem: repositoryLedgerItem("task", 2), Goal: "implement",
		Acceptance: []model.TaskAcceptance{{ID: "acceptance", Text: "verified", Verification: []string{"go test ./..."}}},
	}}}
}

func TestCodingLedgerMaterialMutationPreservesUnchangedAgreementsAndIdentity(t *testing.T) {
	ledger := repositoryLedgerFixture()
	additional := model.LedgerTask{LedgerItem: repositoryLedgerItem("additional", 5), Goal: "bounded"}
	additional.ReviewState = model.LedgerReviewProposed
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{Task: &additional}, Rationale: "owner asked",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Tasks[1].ID != "additional" {
		t.Fatal("stable identity was not preserved")
	}
	for _, record := range ledgerRepositoryRecords(ledger) {
		want := model.LedgerReviewAgreed
		if record.item.ID == "additional" {
			want = model.LedgerReviewProposed
		}
		if record.item.Disposition == model.LedgerDispositionActive && record.item.ReviewState != want {
			t.Fatalf("item %s review state = %s, want %s", record.item.ID, record.item.ReviewState, want)
		}
	}
}

func TestCodingLedgerFastRoutePreservesSupportedReasoningEffort(t *testing.T) {
	routes := []CodingLedgerRoute{
		{Role: "planner", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: "priority"},
		{Role: "executor", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: "priority"},
		{Role: "reviewer", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: "priority"},
	}
	if err := validateCodingLedgerRoutes(routes, 1); err != nil {
		t.Fatal(err)
	}
}

func TestCodingLedgerValidationRejectsExactDuplicateRecords(t *testing.T) {
	ledger := repositoryLedgerFixture()
	duplicate := ledger.Tasks[0]
	duplicate.ID = "task-copy"
	duplicate.Rank++
	ledger.Tasks = append(ledger.Tasks, duplicate)
	err := validateLedgerForRepository(ledger)
	if err == nil || !strings.Contains(err.Error(), "exact duplicate") {
		t.Fatalf("expected exact duplicate error, got %v", err)
	}
}

func TestCodingLedgerDraftValidationRejectsExactDuplicateRecords(t *testing.T) {
	ledger := repositoryLedgerFixture()
	copy := ledger.Tasks[0]
	copy.ID = "task-copy"
	copy.Rank++
	ledger.Tasks = append(ledger.Tasks, copy)
	if err := validateDraftLedgerForRepository(ledger); err != nil {
		t.Fatalf("fixture must remain a structurally valid draft: %v", err)
	}
	if err := validateCodingLedgerExactDuplicates(ledger); err == nil || !strings.Contains(err.Error(), "exact duplicate") {
		t.Fatalf("draft duplicate validation = %v", err)
	}
}

func TestCodingLedgerValidationDoesNotRequireAGlobalObjective(t *testing.T) {
	ledger := repositoryLedgerFixture()
	for _, objective := range []string{"", "Ship the approved task."} {
		ledger.Tasks[0].Objective = objective
		if err := validateLedgerForRepository(ledger); err != nil {
			t.Fatal(err)
		}
		if err := validateDraftLedgerForRepository(ledger); err != nil {
			t.Fatal(err)
		}
	}
}

func repositoryLedgerRoutes(revision int64) []CodingLedgerRoute {
	return []CodingLedgerRoute{
		{Role: "planner", ConfigurationRevision: revision, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "executor", ConfigurationRevision: revision, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "reviewer", ConfigurationRevision: revision, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
	}
}

func TestCodingLedgerTaskVerificationAmendmentResetsOnlyItsAgreement(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: repositoryLedgerItem("unrelated-task", 5), Goal: "unrelated"})
	amended := ledger.Tasks[0]
	amended.Acceptance = append([]model.TaskAcceptance(nil), amended.Acceptance...)
	amended.Acceptance[0].Verification = []string{"go test ./internal/queue"}
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: amended.ID, Item: &model.LedgerMutationItem{Task: &amended}, Rationale: "change required verification"}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Tasks[0].ReviewState != model.LedgerReviewProposed {
		t.Fatal("changed verification retained agreement")
	}
	if ledger.Tasks[1].ReviewState != model.LedgerReviewAgreed {
		t.Fatal("unrelated agreement changed")
	}
}
