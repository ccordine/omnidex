package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestSameCodingLedgerJSONPreservesInt64Precision(t *testing.T) {
	left := []byte(`{"rank":9007199254740992}`)
	right := []byte(`{"rank":9007199254740993}`)
	if sameCodingLedgerJSON(left, right) {
		t.Fatal("distinct int64 JSON values compared equal")
	}
	if !sameCodingLedgerJSON(left, []byte("{ \"rank\" : 9007199254740992 }")) {
		t.Fatal("equivalent JSON values compared different")
	}
}

func TestInitialCodingLedgerRequiresProposedReviewState(t *testing.T) {
	ledger := repositoryLedgerFixture()
	if err := validateInitialCodingLedgerReviewStates(ledger); err == nil {
		t.Fatal("pre-agreed initial item was accepted")
	}
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	ledger.Dependencies = []model.LedgerDependency{{
		LedgerItem: model.LedgerItem{
			ID: "dependency", Disposition: model.LedgerDispositionActive,
			ReviewState: model.LedgerReviewAgreed,
		},
	}}
	if err := validateInitialCodingLedgerReviewStates(ledger); err == nil {
		t.Fatal("pre-agreed initial dependency was accepted")
	}
	ledger.Dependencies[0].ReviewState = model.LedgerReviewProposed
	if err := validateInitialCodingLedgerReviewStates(ledger); err != nil {
		t.Fatal(err)
	}
}

func TestCodingLedgerRouteReplacementRequiresRepresentedMaterialMutation(t *testing.T) {
	current := repositoryLedgerRoutes(1)
	replacement := repositoryLedgerRoutes(1)
	replacement[0].Model = "replacement"
	outOfScope := model.OutOfScopeOwnerTurn("not represented")
	if err := validateCodingLedgerRouteMutation(current, replacement, 2, nil, outOfScope); err == nil {
		t.Fatal("out-of-scope zero-operation route replacement was accepted")
	}

	operation := []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationReview, TargetID: "task",
		ReviewState: model.LedgerReviewAgreed,
	}}
	represented := model.RepresentedOwnerTurn([]model.LedgerItemID{"task"})
	if err := validateCodingLedgerRouteMutation(current, replacement, 2, operation, represented); err == nil {
		t.Fatal("route replacement reused an old configuration revision")
	}
	for index := range replacement {
		replacement[index].ConfigurationRevision = 2
	}
	if err := validateCodingLedgerRouteMutation(current, replacement, 2, operation, represented); err != nil {
		t.Fatal(err)
	}
}

func TestInitialCodingLedgerRejectsMixedOwnerTurnDisposition(t *testing.T) {
	ledger := repositoryLedgerFixture()
	disposition := model.OwnerTurnDisposition{
		Statements: []model.OwnerStatementDisposition{{
			Kind: model.OwnerTurnOutOfScope, Reason: "not part of this ledger", ItemIDs: []model.LedgerItemID{"task"},
		}},
	}
	if err := validateOwnerTurnDisposition(ledger, disposition); err == nil {
		t.Fatal("out-of-scope disposition with represented item references was accepted")
	}
}

func TestCodingLedgerRequiredCheckResetUsesExactTaskBinding(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, model.LedgerAcceptanceCriterion{
		LedgerItem: repositoryLedgerItem("acceptance-two", 5), Outcome: "second behavior verified",
		RequirementIDs: []model.LedgerItemID{"requirement"}, TaskIDs: []model.LedgerItemID{"task-two"}, Required: true,
	})
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("task-two", 6), Goal: "implement second behavior",
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance-two"},
	})
	ledger.Checks = append(ledger.Checks, model.LedgerCheck{
		LedgerItem: repositoryLedgerItem("check-two", 7), Argv: []string{"go", "test", "./internal/model"},
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance-two"}, Required: true,
	})

	amended := ledger.Checks[0]
	amended.Argv = []string{"go", "test", "./internal/queue"}
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationAmend, TargetID: amended.ID,
		Item: &model.LedgerMutationItem{Check: &amended}, Rationale: "change first verification",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Tasks[0].ReviewState != model.LedgerReviewProposed {
		t.Fatalf("covered task review state = %s, want proposed", ledger.Tasks[0].ReviewState)
	}
	if ledger.Tasks[1].ReviewState != model.LedgerReviewAgreed {
		t.Fatalf("task sharing only the requirement changed to %s", ledger.Tasks[1].ReviewState)
	}
	if ledger.AcceptanceCriteria[1].ReviewState != model.LedgerReviewAgreed ||
		ledger.Checks[1].ReviewState != model.LedgerReviewAgreed {
		t.Fatal("unrelated verification scope changed")
	}
}

func TestCodingLedgerLiteralSearchPatternEscapesLikeMetacharacters(t *testing.T) {
	got := codingLedgerLiteralSearchPattern(`100%_\\Coverage`)
	want := `%100\%\_\\\\coverage%`
	if got != want {
		t.Fatalf("literal search pattern = %q, want %q", got, want)
	}
}
