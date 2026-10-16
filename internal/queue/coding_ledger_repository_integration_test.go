package queue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func repositoryLedgerItem(id string, rank int64) model.LedgerItem {
	return model.LedgerItem{
		ID:         model.LedgerItemID(id),
		Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "turn-1", Revision: 1},
		Rationale:  "required by the objective", Priority: model.LedgerPriorityNormal, Rank: rank,
		Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewAgreed,
	}
}

func repositoryLedgerFixture() model.EngineeringLedger {
	return model.EngineeringLedger{
		ID: "proposal", Revision: 1,
		Objectives:   []model.LedgerObjective{{LedgerItem: repositoryLedgerItem("objective", 0), Outcome: "ship"}},
		Requirements: []model.LedgerRequirement{{LedgerItem: repositoryLedgerItem("requirement", 1), Outcome: "works"}},
		Tasks: []model.LedgerTask{{
			LedgerItem: repositoryLedgerItem("task", 2), Goal: "implement",
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		}},
		AcceptanceCriteria: []model.LedgerAcceptanceCriterion{{
			LedgerItem: repositoryLedgerItem("acceptance", 3), Outcome: "verified",
			RequirementIDs: []model.LedgerItemID{"requirement"}, TaskIDs: []model.LedgerItemID{"task"}, Required: true,
		}},
		Checks: []model.LedgerCheck{{
			LedgerItem: repositoryLedgerItem("check", 4), Argv: []string{"go", "test", "./..."},
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"}, Required: true,
		}},
	}
}

func TestCodingLedgerMaterialMutationPreservesUnchangedAgreementsAndIdentity(t *testing.T) {
	ledger := repositoryLedgerFixture()
	constraint := model.LedgerConstraint{LedgerItem: repositoryLedgerItem("constraint", 5), Rule: "bounded"}
	constraint.ReviewState = model.LedgerReviewProposed
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{Constraint: &constraint}, Rationale: "owner asked",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Constraints[0].ID != "constraint" {
		t.Fatal("stable identity was not preserved")
	}
	for _, record := range ledgerRepositoryRecords(ledger) {
		want := model.LedgerReviewAgreed
		if record.item.ID == "constraint" {
			want = model.LedgerReviewProposed
		}
		if record.item.Disposition == model.LedgerDispositionActive && record.item.ReviewState != want {
			t.Fatalf("item %s review state = %s, want %s", record.item.ID, record.item.ReviewState, want)
		}
	}
}

func TestCodingLedgerDependencyChangeResetsOnlyAffectedTask(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("prerequisite", 6),
		Goal:       "prepare", RequirementIDs: []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	})
	dependency := model.LedgerDependency{
		LedgerItem: repositoryLedgerItem("dependency", 7),
		TaskID:     "task", PrerequisiteID: "prerequisite", Kind: model.LedgerDependencyRequires,
	}
	dependency.ReviewState = model.LedgerReviewProposed
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind:      model.LedgerMutationAdd,
		Item:      &model.LedgerMutationItem{Dependency: &dependency},
		Rationale: "task depends on preparation",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Tasks[0].ReviewState != model.LedgerReviewProposed {
		t.Fatal("dependent task agreement was not reset")
	}
	if ledger.Tasks[1].ReviewState != model.LedgerReviewAgreed ||
		ledger.Objectives[0].ReviewState != model.LedgerReviewAgreed {
		t.Fatal("dependency change reset unrelated agreements")
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

func TestCodingLedgerRemoveRestoreKeepsTombstone(t *testing.T) {
	ledger := repositoryLedgerFixture()
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationRemove, TargetID: "check", Rationale: "remove",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Checks[0].Disposition != model.LedgerDispositionTombstoned || ledger.Checks[0].ReviewState != "" {
		t.Fatal("remove did not retain a tombstone")
	}
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationRestore, TargetID: "check", Rationale: "restore",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Checks[0].ID != "check" || ledger.Checks[0].Disposition != model.LedgerDispositionActive ||
		ledger.Checks[0].ReviewState != model.LedgerReviewProposed {
		t.Fatal("restore did not preserve identity as a proposed active item")
	}
}

func TestCodingLedgerValidationRejectsExactDuplicateRecords(t *testing.T) {
	ledger := repositoryLedgerFixture()
	duplicate := ledger.Requirements[0]
	duplicate.ID = "requirement-copy"
	duplicate.Rank++
	ledger.Requirements = append(ledger.Requirements, duplicate)
	ledger.Tasks[0].RequirementIDs = append(ledger.Tasks[0].RequirementIDs, duplicate.ID)
	ledger.AcceptanceCriteria[0].RequirementIDs = append(ledger.AcceptanceCriteria[0].RequirementIDs, duplicate.ID)
	ledger.Checks[0].RequirementIDs = append(ledger.Checks[0].RequirementIDs, duplicate.ID)
	err := validateLedgerForRepository(ledger)
	if err == nil || !strings.Contains(err.Error(), "exact duplicate") {
		t.Fatalf("expected exact duplicate error, got %v", err)
	}
}

func TestCodingLedgerDraftValidationRejectsExactDuplicateRecords(t *testing.T) {
	ledger := repositoryLedgerFixture()
	copy := ledger.Requirements[0]
	copy.ID = "requirement-copy"
	copy.Rank++
	ledger.Requirements = append(ledger.Requirements, copy)
	if err := validateDraftLedgerForRepository(ledger); err != nil {
		t.Fatalf("fixture must remain a structurally valid draft: %v", err)
	}
	if err := validateCodingLedgerExactDuplicates(ledger); err == nil || !strings.Contains(err.Error(), "exact duplicate") {
		t.Fatalf("draft duplicate validation = %v", err)
	}
}

func TestCodingLedgerRepositoryTransactionallyFreezesExactRevision(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Ship the normalized ledger.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("delivery-task", 7), Goal: "deliver the integrated change",
		DeliveryObligation:     "publish the completed integration",
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	})
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, model.LedgerAcceptanceCriterion{
		LedgerItem: repositoryLedgerItem("overall-acceptance", 5),
		Outcome:    "integrated", RequirementIDs: []model.LedgerItemID{"requirement"}, Required: true,
	})
	ledger.Checks = append(ledger.Checks, model.LedgerCheck{
		LedgerItem:             repositoryLedgerItem("overall-check", 6),
		Argv:                   []string{"go", "test", "./..."},
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"overall-acceptance"}, Required: true,
	})
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	routes := []CodingLedgerRoute{
		{Role: "planner", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "executor", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "reviewer", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "ledger-create", OwnerTurnID: "owner-create",
		OwnerTurnText: "Ship the normalized ledger.",
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"objective", "requirement", "task", "delivery-task", "acceptance", "check", "overall-acceptance", "overall-check"},
		},
		Ledger: ledger, CoveragePassed: true, Routes: routes,
	})
	if err != nil {
		t.Fatal(err)
	}
	rank := int64(8)
	mutation := model.LedgerMutation{
		OperationID: "ledger-reorder", BaseRevision: created.Revision, OwnerTurnID: "owner-reorder",
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind:    model.OwnerTurnRepresented,
			ItemIDs: []model.LedgerItemID{"objective", "requirement", "task", "delivery-task", "acceptance", "check", "overall-acceptance", "overall-check"},
		},
		Operations: []model.LedgerMutationOperation{
			{Kind: model.LedgerMutationReview, TargetID: "objective", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReview, TargetID: "requirement", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReview, TargetID: "delivery-task", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReview, TargetID: "acceptance", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReview, TargetID: "check", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReview, TargetID: "overall-acceptance", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReview, TargetID: "overall-check", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank, Rationale: "owner reordered"},
		},
	}
	controlCommand := ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the task.", Mutation: mutation, CoveragePassed: true,
	}
	changed, err := repository.ApplyCodingLedgerControl(ctx, controlCommand)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.ApplyCodingLedgerControl(ctx, controlCommand)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Applied || replayed.Revision != changed.Revision {
		t.Fatalf("idempotent replay = %#v", replayed)
	}
	if _, err := repository.ApplyCodingLedgerMutation(ctx, controlCommand); err == nil {
		t.Fatal("control operation identity replayed as a semantic planning turn")
	}
	var ownerAction string
	if err := repository.pool.QueryRow(ctx, `
		SELECT action FROM coding_ledger_owner_turns
		WHERE ledger_id=$1 AND owner_turn_id=$2
	`, created.LedgerID, mutation.OwnerTurnID).Scan(&ownerAction); err != nil {
		t.Fatal(err)
	}
	if ownerAction != "planning_control" {
		t.Fatalf("direct owner action = %q, want planning_control", ownerAction)
	}
	planningTurns, err := repository.CodingLedgerPlanningTurns(ctx, created.LedgerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(planningTurns) != 1 || planningTurns[0].OwnerTurnID != "owner-create" {
		t.Fatalf("semantic planning turns include deterministic controls: %#v", planningTurns)
	}
	conflictingMutation := mutation
	conflictingMutation.Operations = append([]model.LedgerMutationOperation(nil), mutation.Operations...)
	conflictingRank := int64(9)
	conflictingMutation.Operations[len(conflictingMutation.Operations)-1].Rank = &conflictingRank
	if _, err := repository.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the task.",
		Mutation: conflictingMutation, CoveragePassed: true,
	}); err == nil {
		t.Fatal("operation identity conflict was accepted as an idempotent replay")
	}
	sameResultConflict := mutation
	sameResultConflict.Operations = append([]model.LedgerMutationOperation(nil), mutation.Operations...)
	sameResultConflict.Operations[0].Rationale = "different rationale with the same materialized result"
	if _, err := repository.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the task.",
		Mutation: sameResultConflict, CoveragePassed: true,
	}); err == nil {
		t.Fatal("operation identity accepted a different exact command with the same result")
	}
	if _, err := repository.ReadCodingLedger(ctx, created.LedgerID, CodingLedgerReadOptions{
		Revision: changed.Revision, Limit: -1,
	}); err == nil {
		t.Fatal("negative public read limit was accepted")
	}
	page, err := repository.ReadCodingLedger(ctx, created.LedgerID, CodingLedgerReadOptions{
		Revision: changed.Revision, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ledgerRepositoryRecords(page.Ledger))+len(page.Ledger.Dependencies) != 1 {
		t.Fatalf("bounded ledger page returned more than one record: %#v", page.Ledger)
	}
	detail, err := repository.CodingLedgerTaskDetail(ctx, created.LedgerID, changed.Revision, "task")
	if err != nil {
		t.Fatal(err)
	}
	hasCheck := false
	for _, check := range detail.Ledger.Checks {
		hasCheck = hasCheck || check.ID == "check"
	}
	if len(detail.Ledger.Tasks) != 1 || detail.Ledger.Tasks[0].ID != "task" ||
		len(detail.Ledger.Requirements) != 1 || detail.Ledger.Requirements[0].ID != "requirement" ||
		len(detail.Ledger.AcceptanceCriteria) != 1 || detail.Ledger.AcceptanceCriteria[0].ID != "acceptance" ||
		!hasCheck {
		t.Fatalf("task detail omitted linked ledger records: %#v", detail.Ledger)
	}
	diff, err := repository.CodingLedgerDiff(ctx, created.LedgerID, created.Revision, changed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Amended) == 0 {
		t.Fatalf("revision diff omitted changed records: %#v", diff)
	}
	auditID := repositoryReadyPlanningAudit(t, repository, created.LedgerID, changed.Revision, "ledger-freeze")
	frozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: changed.Revision, OperationID: "ledger-freeze",
		OwnerTurnID: "owner-freeze", OwnerTurnText: "Freeze this exact revision.", AuditID: auditID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if frozen.State != "frozen" || frozen.BaseRevision != changed.Revision {
		t.Fatalf("frozen projection = %#v", frozen)
	}
	if _, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: changed.Revision - 1, OperationID: "stale-freeze",
		OwnerTurnID: "owner-stale", OwnerTurnText: "Freeze stale.",
	}); err == nil {
		t.Fatal("stale freeze was accepted")
	}
}

func TestCodingLedgerValidationRequiresOneActiveObjective(t *testing.T) {
	ledger := repositoryLedgerFixture()
	second := ledger.Objectives[0]
	second.ID = "objective-two"
	second.Outcome = "ship another contract"
	second.Rank = 10
	ledger.Objectives = append(ledger.Objectives, second)
	if err := validateLedgerForRepository(ledger); err == nil || !strings.Contains(err.Error(), "exactly one active objective") {
		t.Fatalf("multiple active objective validation = %v", err)
	}
}

func TestCodingLedgerQuestionAndDependencyLifecycleUsesRevisionAuthority(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Exercise normalized lifecycle.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	prerequisite := model.LedgerTask{
		LedgerItem: repositoryLedgerItem("prerequisite", 5), Goal: "prepare",
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	}
	dependency := model.LedgerDependency{
		LedgerItem: repositoryLedgerItem("dependency", 6), TaskID: "task",
		PrerequisiteID: "prerequisite", Kind: model.LedgerDependencyRequires,
	}
	withdrawnQuestion := model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("question-restore", 7), Kind: model.LedgerQuestion,
		Prompt: "Should this be restored?",
	}
	answeredQuestion := model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("question-answer", 8), Kind: model.LedgerQuestion,
		Prompt: "What is the answer?",
	}
	ledger.Tasks = append(ledger.Tasks, prerequisite)
	ledger.Dependencies = append(ledger.Dependencies, dependency)
	ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, withdrawnQuestion, answeredQuestion)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	for index := range ledger.Dependencies {
		ledger.Dependencies[index].ReviewState = model.LedgerReviewProposed
	}
	routes := []CodingLedgerRoute{
		{Role: "planner", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "executor", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "reviewer", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
	}
	allIDs := []model.LedgerItemID{
		"objective", "requirement", "task", "acceptance", "check",
		"prerequisite", "dependency", "question-restore", "question-answer",
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "lifecycle-create", OwnerTurnID: "lifecycle-create-turn",
		OwnerTurnText:        "Exercise normalized lifecycle.",
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: allIDs},
		Ledger:               ledger, CoveragePassed: true, Routes: routes,
	})
	if err != nil {
		t.Fatal(err)
	}

	apply := func(operationID, turnID string, base int64, operations []model.LedgerMutationOperation, ids []model.LedgerItemID) CodingLedgerRevision {
		t.Helper()
		result, applyErr := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
			LedgerID: created.LedgerID, OwnerTurnText: operationID,
			Mutation: model.LedgerMutation{
				OperationID: operationID, BaseRevision: base, OwnerTurnID: turnID,
				OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
				Operations:           operations,
			},
			CoveragePassed: true,
		})
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		return result
	}

	removedQuestion := apply("remove-question", "remove-question-turn", created.Revision,
		[]model.LedgerMutationOperation{{Kind: model.LedgerMutationRemove, TargetID: "question-restore", Rationale: "withdraw"}},
		[]model.LedgerItemID{"question-restore"})
	restoredQuestion := apply("restore-question", "restore-question-turn", removedQuestion.Revision,
		[]model.LedgerMutationOperation{{Kind: model.LedgerMutationRestore, TargetID: "question-restore", Rationale: "restore"}},
		[]model.LedgerItemID{"question-restore"})
	if restoredQuestion.Ledger.DecisionsAndQuestions[0].Disposition != model.LedgerDispositionActive {
		t.Fatal("restored question did not remain active")
	}

	answer := answeredQuestion
	answer.Answer = "Use the exact base revision."
	answer.ReviewState = model.LedgerReviewProposed
	answered := apply("answer-question", "answer-question-turn", restoredQuestion.Revision,
		[]model.LedgerMutationOperation{{
			Kind: model.LedgerMutationAmend, TargetID: "question-answer",
			Item: &model.LedgerMutationItem{DecisionQuestion: &answer}, Rationale: "answer",
		}}, []model.LedgerItemID{"question-answer"})
	var questionStatus, questionAnswer string
	if err := pool.QueryRow(ctx, `
		SELECT status,answer FROM coding_ledger_questions
		WHERE ledger_id=$1 AND question_id='question-answer'
	`, created.LedgerID).Scan(&questionStatus, &questionAnswer); err != nil {
		t.Fatal(err)
	}
	if questionStatus != "answered" || questionAnswer != answer.Answer {
		t.Fatalf("durable question = %s %q", questionStatus, questionAnswer)
	}

	removedDependency := apply("remove-dependency-endpoint", "remove-dependency-turn", answered.Revision,
		[]model.LedgerMutationOperation{
			{Kind: model.LedgerMutationRemove, TargetID: "dependency", Rationale: "remove edge"},
			{Kind: model.LedgerMutationRemove, TargetID: "prerequisite", Rationale: "remove endpoint"},
		}, []model.LedgerItemID{"dependency", "prerequisite"})
	projection, err := repository.ReadCodingLedger(ctx, created.LedgerID, CodingLedgerReadOptions{
		Revision: removedDependency.Revision, Limit: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Ledger.Dependencies) != 1 ||
		projection.Ledger.Dependencies[0].Disposition != model.LedgerDispositionTombstoned {
		t.Fatalf("dependency tombstone projection = %#v", projection.Ledger.Dependencies)
	}
	removedHistory, err := repository.CodingLedgerItemHistory(ctx, created.LedgerID, dependency.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	activeHistory, err := repository.CodingLedgerItemHistory(ctx, created.LedgerID, dependency.ID, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(removedHistory) != 1 || removedHistory[0].Disposition != model.LedgerDispositionTombstoned ||
		len(activeHistory) != 1 || activeHistory[0].Disposition != model.LedgerDispositionActive {
		t.Fatalf("dependency history pages = %#v / %#v", removedHistory, activeHistory)
	}
	if _, err := repository.CodingLedgerDiff(ctx, created.LedgerID, created.Revision, removedDependency.Revision+100); err == nil {
		t.Fatal("diff accepted a revision that does not exist")
	}
}

func TestCodingLedgerConcurrentInitialReplayIsSerialized(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Serialize initial proposal.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	command := CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "concurrent-create", OwnerTurnID: "concurrent-create-turn",
		OwnerTurnText: "Serialize initial proposal.",
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind:    model.OwnerTurnRepresented,
			ItemIDs: []model.LedgerItemID{"objective", "requirement", "task", "acceptance", "check"},
		},
		Ledger: ledger, CoveragePassed: true,
		Routes: []CodingLedgerRoute{
			{Role: "planner", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
			{Role: "executor", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
			{Role: "reviewer", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		},
	}
	type outcome struct {
		result CodingLedgerRevision
		err    error
	}
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			result, createErr := repository.CreateCodingLedger(ctx, command)
			outcomes <- outcome{result: result, err: createErr}
		}()
	}
	applied := 0
	for range 2 {
		value := <-outcomes
		if value.err != nil {
			t.Fatal(value.err)
		}
		if value.result.Applied {
			applied++
		}
	}
	if applied != 1 {
		t.Fatalf("concurrent initial applied count = %d, want 1", applied)
	}
}

func repositoryLedgerRoutes(revision int64) []CodingLedgerRoute {
	return []CodingLedgerRoute{
		{Role: "planner", ConfigurationRevision: revision, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "executor", ConfigurationRevision: revision, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "reviewer", ConfigurationRevision: revision, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
	}
}

func repositoryReadyPlanningAudit(t *testing.T, repository *Repository, ledgerID, revision int64, seed string) string {
	t.Helper()
	auditID := "audit-" + seed
	if err := repository.RecordCodingLedgerPlanningAudit(context.Background(), RecordCodingLedgerPlanningAuditCommand{
		LedgerID: ledgerID, AuditID: auditID, Revision: revision, Ready: true, Explanation: "fresh complete planning audit",
	}); err != nil {
		t.Fatal(err)
	}
	return auditID
}

func TestCodingLedgerRepositoryFindingRegressions(t *testing.T) {
	t.Run("out of scope replay and cross command reuse", func(t *testing.T) {
		_, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		job, err := repository.EnqueueCodingJob(ctx, "Record an excluded request.", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ledger := repositoryLedgerFixture()
		ids := make([]model.LedgerItemID, 0)
		for _, record := range ledgerRepositoryRecords(ledger) {
			setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
				item.ReviewState = model.LedgerReviewProposed
			})
			ids = append(ids, record.item.ID)
		}
		created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
			JobID: job.ID, OperationID: "finding-create", OwnerTurnID: "finding-create-turn",
			OwnerTurnText: "Record an excluded request.", Ledger: ledger, Routes: repositoryLedgerRoutes(1),
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
			CoveragePassed:       true,
		})
		if err != nil {
			t.Fatal(err)
		}
		command := ApplyCodingLedgerMutationCommand{
			LedgerID: created.LedgerID, OwnerTurnText: "Do not include deployment.",
			Mutation: model.LedgerMutation{
				OperationID: "out-of-scope", BaseRevision: created.Revision, OwnerTurnID: "out-of-scope-turn",
				OwnerTurnDisposition: model.OwnerTurnDisposition{Statements: []model.OwnerStatementDisposition{{Kind: model.OwnerTurnOutOfScope, Reason: "deployment is a separate request"}}},
			},
			CoveragePassed: true,
		}
		applied, err := repository.ApplyCodingLedgerMutation(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := repository.ApplyCodingLedgerMutation(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		if !applied.Applied || replayed.Applied || len(replayed.Ledger.Objectives) != len(applied.Ledger.Objectives) {
			t.Fatalf("out-of-scope apply/replay mismatch: applied=%#v replayed=%#v", applied, replayed)
		}
		if len(applied.OwnerTurnDispositions) != 1 || applied.OwnerTurnDispositions[0].Kind != model.OwnerTurnOutOfScope ||
			applied.OwnerTurnDispositions[0].Reason != "deployment is a separate request" {
			t.Fatalf("out-of-scope disposition is not visible in revision response: %#v", applied.OwnerTurnDispositions)
		}
		if applied.LatestDelta.FromRevision != created.Revision || applied.LatestDelta.ToRevision != applied.Revision ||
			len(applied.LatestDelta.Added)+len(applied.LatestDelta.Amended)+len(applied.LatestDelta.Removed)+len(applied.LatestDelta.Restored) != 0 {
			t.Fatalf("out-of-scope latest delta = %#v", applied.LatestDelta)
		}
		if _, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
			LedgerID: created.LedgerID, Revision: created.Revision,
			OperationID: "out-of-scope", OwnerTurnID: "out-of-scope-turn",
			OwnerTurnText: "Do not include deployment.",
		}); err == nil {
			t.Fatal("a proposed mutation receipt was replayed as a freeze receipt")
		}
	})

	t.Run("answered initial question and transient dependency tombstone", func(t *testing.T) {
		pool, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		job, err := repository.EnqueueCodingJob(ctx, "Preserve normalized history.", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ledger := repositoryLedgerFixture()
		prerequisite := model.LedgerTask{
			LedgerItem: repositoryLedgerItem("transient-prerequisite", 5), Goal: "prepare",
			RequirementIDs: []model.LedgerItemID{"requirement"}, AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		}
		question := model.LedgerDecisionQuestion{
			LedgerItem: repositoryLedgerItem("answered-initially", 6), Kind: model.LedgerQuestion,
			Prompt: "Which authority?", Answer: "The exact owner turn.",
		}
		ledger.Tasks = append(ledger.Tasks, prerequisite)
		ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, question)
		ids := make([]model.LedgerItemID, 0)
		for _, record := range ledgerRepositoryRecords(ledger) {
			setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
				item.ReviewState = model.LedgerReviewProposed
			})
			ids = append(ids, record.item.ID)
		}
		created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
			JobID: job.ID, OperationID: "history-create", OwnerTurnID: "history-create-turn",
			OwnerTurnText: "Preserve normalized history.", Ledger: ledger, Routes: repositoryLedgerRoutes(1),
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
			CoveragePassed:       true,
		})
		if err != nil {
			t.Fatal(err)
		}
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM coding_ledger_questions WHERE ledger_id=$1 AND question_id='answered-initially'`,
			created.LedgerID).Scan(&status); err != nil || status != "answered" {
			t.Fatalf("initial answered question status=%q err=%v", status, err)
		}
		dependency := model.LedgerDependency{
			LedgerItem: repositoryLedgerItem("transient-dependency", 7),
			TaskID:     "task", PrerequisiteID: prerequisite.ID, Kind: model.LedgerDependencyRequires,
		}
		dependency.ReviewState = model.LedgerReviewProposed
		changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
			LedgerID: created.LedgerID, OwnerTurnText: "Add and remove the transient dependency.",
			Mutation: model.LedgerMutation{
				OperationID: "transient-dependency-change", BaseRevision: created.Revision, OwnerTurnID: "transient-dependency-turn",
				OwnerTurnDisposition: model.OwnerTurnDisposition{
					Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{dependency.ID, prerequisite.ID},
				},
				Operations: []model.LedgerMutationOperation{
					{Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{Dependency: &dependency}, Rationale: "temporarily add"},
					{Kind: model.LedgerMutationRemove, TargetID: dependency.ID, Rationale: "remove the transient edge"},
					{Kind: model.LedgerMutationRemove, TargetID: prerequisite.ID, Rationale: "remove the transient endpoint"},
				},
			},
			CoveragePassed: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(changed.Ledger.Dependencies) != 1 ||
			changed.Ledger.Dependencies[0].ID != dependency.ID ||
			changed.Ledger.Dependencies[0].Disposition != model.LedgerDispositionTombstoned ||
			changed.Ledger.Dependencies[0].Rationale != dependency.Rationale {
			t.Fatalf("transient dependency tombstone was not retained exactly: %#v", changed.Ledger.Dependencies)
		}
		diff, err := repository.CodingLedgerDiff(ctx, created.LedgerID, created.Revision, changed.Revision)
		if err != nil {
			t.Fatal(err)
		}
		foundDependency := false
		for _, id := range diff.Added {
			foundDependency = foundDependency || id == dependency.ID
		}
		if !foundDependency {
			t.Fatalf("transient dependency identity missing from diff: %#v", diff)
		}
		history, err := repository.CodingLedgerItemHistory(ctx, created.LedgerID, dependency.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 1 || history[0].Disposition != model.LedgerDispositionTombstoned {
			t.Fatalf("transient dependency history was not retained: %#v", history)
		}
	})

	t.Run("large initial replay", func(t *testing.T) {
		_, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		job, err := repository.EnqueueCodingJob(ctx, "Retain every initial record.", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ledger := repositoryLedgerFixture()
		for index := 0; index < 205; index++ {
			ledger.Constraints = append(ledger.Constraints, model.LedgerConstraint{
				LedgerItem: repositoryLedgerItem(fmt.Sprintf("constraint-%03d", index), int64(index+10)),
				Rule:       fmt.Sprintf("bounded rule %03d", index),
			})
		}
		for _, record := range ledgerRepositoryRecords(ledger) {
			setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
				item.ReviewState = model.LedgerReviewProposed
			})
		}
		command := CreateCodingLedgerCommand{
			JobID: job.ID, OperationID: "large-create", OwnerTurnID: "large-create-turn",
			OwnerTurnText: "Retain every initial record.", Ledger: ledger, Routes: repositoryLedgerRoutes(1),
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"objective"},
			},
			CoveragePassed: true,
		}
		created, err := repository.CreateCodingLedger(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := repository.CreateCodingLedger(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		createdCount := len(ledgerRepositoryRecords(created.Ledger)) + len(created.Ledger.Dependencies)
		replayedCount := len(ledgerRepositoryRecords(replayed.Ledger)) + len(replayed.Ledger.Dependencies)
		if createdCount != 210 || replayedCount != createdCount || replayed.Applied {
			t.Fatalf("large initial apply/replay counts = %d/%d applied=%t", createdCount, replayedCount, replayed.Applied)
		}
	})

	t.Run("bounded diff", func(t *testing.T) {
		_, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		job, err := repository.EnqueueCodingJob(ctx, "Page revision differences.", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ledger := repositoryLedgerFixture()
		ids := make([]model.LedgerItemID, 0)
		for _, record := range ledgerRepositoryRecords(ledger) {
			setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
				item.ReviewState = model.LedgerReviewProposed
			})
			ids = append(ids, record.item.ID)
		}
		created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
			JobID: job.ID, OperationID: "diff-create", OwnerTurnID: "diff-create-turn",
			OwnerTurnText: "Page revision differences.", Ledger: ledger, Routes: repositoryLedgerRoutes(1),
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
			CoveragePassed:       true,
		})
		if err != nil {
			t.Fatal(err)
		}
		changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
			LedgerID: created.LedgerID, OwnerTurnText: "Agree two records.",
			Mutation: model.LedgerMutation{
				OperationID: "diff-change", BaseRevision: created.Revision, OwnerTurnID: "diff-change-turn",
				OwnerTurnDisposition: model.OwnerTurnDisposition{
					Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"objective", "requirement"},
				},
				Operations: []model.LedgerMutationOperation{
					{Kind: model.LedgerMutationReview, TargetID: "objective", ReviewState: model.LedgerReviewAgreed, Rationale: "agree"},
					{Kind: model.LedgerMutationReview, TargetID: "requirement", ReviewState: model.LedgerReviewAgreed, Rationale: "agree"},
				},
			},
			CoveragePassed: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		diff, err := repository.CodingLedgerDiff(ctx, created.LedgerID, created.Revision, changed.Revision, CodingLedgerPage{Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if !diff.HasMore || len(diff.Added)+len(diff.Amended)+len(diff.Removed)+len(diff.Restored) != 1 {
			t.Fatalf("diff page is not bounded: %#v", diff)
		}
	})
}

func TestCodingLedgerUnresolvedDispositionRejectsMaterialMutation(t *testing.T) {
	ledger := repositoryLedgerFixture()
	question := model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("question", 5),
		Kind:       model.LedgerQuestion,
		Prompt:     "Which behavior?",
	}
	ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, question)
	rank := int64(10)
	err := validateMutationOwnerTurnDisposition(ledger, ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank, Rationale: "change task",
	}}, model.OwnerTurnDisposition{Kind: model.OwnerTurnUnresolved, QuestionIDs: []model.LedgerItemID{"question"}})
	if err == nil || !strings.Contains(err.Error(), "cannot contain material mutations") {
		t.Fatalf("unresolved material mutation validation = %v", err)
	}
}

func TestCodingLedgerDecisionQuestionKindIsImmutable(t *testing.T) {
	ledger := repositoryLedgerFixture()
	question := model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("question", 5),
		Kind:       model.LedgerQuestion,
		Prompt:     "Which behavior?",
	}
	ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, question)
	decision := question
	decision.Kind = model.LedgerDecision
	decision.Answer = "Use the normalized ledger."
	err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationAmend, TargetID: question.ID,
		Item: &model.LedgerMutationItem{DecisionQuestion: &decision}, Rationale: "change kind",
	}})
	if err == nil || !strings.Contains(err.Error(), "same kind") {
		t.Fatalf("decision/question kind change validation = %v", err)
	}
}

func TestCodingLedgerFrozenProjectionRejectsMismatchedBindings(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Requirements = append(ledger.Requirements, model.LedgerRequirement{
		LedgerItem: repositoryLedgerItem("requirement-two", 5), Outcome: "second outcome",
	})
	ledger.Checks[0].RequirementIDs = []model.LedgerItemID{"requirement-two"}
	projection := CodingLedgerRevision{Ledger: ledger, Routes: repositoryLedgerRoutes(1)}
	if err := validateFrozenCodingAgentTasks(projection.Ledger); err == nil {
		t.Fatal("mismatched task, acceptance, and check bindings produced a frozen workload")
	}
}

func TestCodingLedgerDependencyPreservesMaximumRationale(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Preserve dependency rationale.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("prerequisite", 5), Goal: "prepare",
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	})
	dependency := model.LedgerDependency{
		LedgerItem: repositoryLedgerItem("dependency", 6), TaskID: "task",
		PrerequisiteID: "prerequisite", Kind: model.LedgerDependencyRequires,
	}
	dependency.Rationale = strings.Repeat("x", 8192)
	ledger.Dependencies = append(ledger.Dependencies, dependency)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	ledger.Dependencies[0].ReviewState = model.LedgerReviewProposed
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "boundary-create", OwnerTurnID: "boundary-turn",
		OwnerTurnText: "Preserve dependency rationale.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{
			"objective", "requirement", "task", "acceptance", "check", "prerequisite", "dependency",
		}},
		CoveragePassed: true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Ledger.Dependencies) != 1 || created.Ledger.Dependencies[0].Rationale != dependency.Rationale {
		t.Fatalf("maximum dependency rationale was not preserved: %d bytes", len(created.Ledger.Dependencies[0].Rationale))
	}
	removed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Remove the dependency.",
		Mutation: model.LedgerMutation{
			OperationID: "boundary-remove", BaseRevision: created.Revision, OwnerTurnID: "boundary-remove-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{dependency.ID},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationRemove, TargetID: dependency.ID, Rationale: "remove obsolete dependency",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Ledger.Dependencies) != 1 ||
		removed.Ledger.Dependencies[0].Disposition != model.LedgerDispositionTombstoned ||
		removed.Ledger.Dependencies[0].Rationale != dependency.Rationale {
		t.Fatal("maximum-size dependency tombstone did not retain its history")
	}
	history, err := repository.CodingLedgerItemHistory(ctx, created.LedgerID, dependency.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Rationale != dependency.Rationale ||
		history[0].Disposition != model.LedgerDispositionTombstoned {
		t.Fatalf("maximum-size dependency history = %#v", history)
	}
}

func TestCodingLedgerMutationReceiptFitsMaximumOriginDetail(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Mutate a maximum provenance item.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	const prefix = "owner_turn:"
	ledger.Objectives[0].Provenance.Reference = strings.Repeat("p", codingLedgerOriginDetailMaxBytes-len(prefix))
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "origin-boundary-create", OwnerTurnID: "origin-boundary-turn",
		OwnerTurnText: "Create the maximum provenance item.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind:    model.OwnerTurnRepresented,
			ItemIDs: []model.LedgerItemID{"objective", "requirement", "task", "acceptance", "check"},
		},
		CoveragePassed: true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	rank := int64(20)
	mutation := model.LedgerMutation{
		OperationID: "origin-boundary-reorder", BaseRevision: created.Revision, OwnerTurnID: "origin-boundary-reorder-turn",
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"objective"},
		},
		Operations: []model.LedgerMutationOperation{{
			Kind: model.LedgerMutationReorder, TargetID: "objective", Rank: &rank, Rationale: "move objective",
		}},
	}
	changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the objective.",
		Mutation: mutation, CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the objective.",
		Mutation: mutation, CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Applied || replayed.Revision != changed.Revision {
		t.Fatalf("maximum-origin replay = %#v", replayed)
	}
}

func TestCodingLedgerUnresolvedDispositionCanRaiseReferencedQuestion(t *testing.T) {
	base := repositoryLedgerFixture()
	question := model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("raised-question", 5),
		Kind:       model.LedgerQuestion,
		Prompt:     "Which exact behavior should apply?",
	}
	question.ReviewState = model.LedgerReviewProposed
	operation := model.LedgerMutationOperation{
		Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{DecisionQuestion: &question},
		Rationale: "the owner instruction requires clarification",
	}
	mutation := model.LedgerMutation{
		OperationID: "raise-question", BaseRevision: 1, OwnerTurnID: "raise-question-turn",
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind: model.OwnerTurnUnresolved, QuestionIDs: []model.LedgerItemID{question.ID},
		},
		Operations: []model.LedgerMutationOperation{operation},
	}
	if err := mutation.Validate(); err != nil {
		t.Fatal(err)
	}
	next := base
	if err := applyCodingLedgerOperations(&next, mutation.Operations); err != nil {
		t.Fatal(err)
	}
	if err := validateMutationOwnerTurnDisposition(base, next, mutation.Operations, mutation.OwnerTurnDisposition); err != nil {
		t.Fatal(err)
	}
}

func TestCodingLedgerRejectsRemoveAndRestoreInOneRevision(t *testing.T) {
	tests := []struct {
		name       string
		tombstoned bool
		operations []model.LedgerMutationOperation
	}{
		{
			name: "remove then restore",
			operations: []model.LedgerMutationOperation{
				{Kind: model.LedgerMutationRemove, TargetID: "check", Rationale: "remove"},
				{Kind: model.LedgerMutationRestore, TargetID: "check", Rationale: "restore"},
			},
		},
		{
			name:       "restore then remove",
			tombstoned: true,
			operations: []model.LedgerMutationOperation{
				{Kind: model.LedgerMutationRestore, TargetID: "check", Rationale: "restore"},
				{Kind: model.LedgerMutationRemove, TargetID: "check", Rationale: "remove"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger := repositoryLedgerFixture()
			if test.tombstoned {
				ledger.Checks[0].Disposition = model.LedgerDispositionTombstoned
				ledger.Checks[0].ReviewState = ""
			}
			before, err := json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
			err = applyCodingLedgerOperations(&ledger, test.operations)
			if err == nil || !strings.Contains(err.Error(), "removed and restored in one revision") {
				t.Fatalf("remove/restore validation = %v", err)
			}
			after, err := json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("rejected operations mutated ledger\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestCodingLedgerDependencyTombstonesRemainNormalizedAndRevisionBound(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Preserve high-fanout dependency history.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	const dependencyCount = 50
	for index := 0; index < dependencyCount; index++ {
		taskID := model.LedgerItemID(fmt.Sprintf("fanout-task-%02d", index))
		ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
			LedgerItem:             repositoryLedgerItem(string(taskID), int64(10+index)),
			Goal:                   fmt.Sprintf("prepare input %02d", index),
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		})
		dependency := model.LedgerDependency{
			LedgerItem:     repositoryLedgerItem(fmt.Sprintf("fanout-dependency-%02d", index), int64(100+index)),
			TaskID:         "task",
			PrerequisiteID: taskID,
			Kind:           model.LedgerDependencyRequires,
		}
		dependency.ReviewState = model.LedgerReviewProposed
		ledger.Dependencies = append(ledger.Dependencies, dependency)
	}
	ids := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger))+len(ledger.Dependencies))
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	for _, dependency := range ledger.Dependencies {
		ids = append(ids, dependency.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "fanout-create", OwnerTurnID: "fanout-create-turn",
		OwnerTurnText: "Preserve high-fanout dependency history.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}

	operations := make([]model.LedgerMutationOperation, 0, dependencyCount)
	removedIDs := make([]model.LedgerItemID, 0, dependencyCount)
	for _, dependency := range ledger.Dependencies {
		operations = append(operations, model.LedgerMutationOperation{
			Kind: model.LedgerMutationRemove, TargetID: dependency.ID, Rationale: "remove obsolete dependency",
		})
		removedIDs = append(removedIDs, dependency.ID)
	}
	removed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Remove the obsolete dependencies.",
		Mutation: model.LedgerMutation{
			OperationID: "fanout-remove", BaseRevision: created.Revision, OwnerTurnID: "fanout-remove-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: removedIDs},
			Operations:           operations,
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var liveEdges, dependencyVersions int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM coding_ledger_edges
		WHERE ledger_id=$1 AND revision=$2 AND edge_kind IN ('requires','blocks')
	`, created.LedgerID, removed.Revision).Scan(&liveEdges); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM coding_ledger_revision_items projection
		JOIN coding_ledger_items item USING(ledger_id,item_id)
		WHERE projection.ledger_id=$1 AND projection.revision=$2
		  AND item.item_id LIKE 'dependency.record.%'
		  AND projection.item_revision=$2
	`, created.LedgerID, removed.Revision).Scan(&dependencyVersions); err != nil {
		t.Fatal(err)
	}
	if liveEdges != 0 || dependencyVersions != dependencyCount {
		t.Fatalf("removed dependency storage live=%d versions=%d", liveEdges, dependencyVersions)
	}

	rank := int64(99)
	unrelated, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the objective.",
		Mutation: model.LedgerMutation{
			OperationID: "fanout-unrelated", BaseRevision: removed.Revision, OwnerTurnID: "fanout-unrelated-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"objective"},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReorder, TargetID: "objective", Rank: &rank, Rationale: "move the objective",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := repository.ReadCodingLedger(ctx, created.LedgerID, CodingLedgerReadOptions{
		Revision: unrelated.Revision, Limit: 1000, Kinds: []string{"dependency"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Ledger.Dependencies) != dependencyCount {
		t.Fatalf("dependency tombstone count after unrelated mutation = %d", len(projection.Ledger.Dependencies))
	}
	for _, dependency := range projection.Ledger.Dependencies {
		if dependency.Disposition != model.LedgerDispositionTombstoned {
			t.Fatalf("dependency %s disposition = %s", dependency.ID, dependency.Disposition)
		}
	}
}

func TestCodingLedgerFindingValidationGuards(t *testing.T) {
	t.Run("tombstoned dependency does not propagate agreement reset", func(t *testing.T) {
		ledger := repositoryLedgerFixture()
		prerequisite := model.LedgerTask{
			LedgerItem: repositoryLedgerItem("prior-prerequisite", 5), Goal: "prepare",
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		}
		dependency := model.LedgerDependency{
			LedgerItem:     repositoryLedgerItem("prior-dependency", 6),
			TaskID:         "task",
			PrerequisiteID: prerequisite.ID,
			Kind:           model.LedgerDependencyRequires,
		}
		ledger.Tasks = append(ledger.Tasks, prerequisite)
		ledger.Dependencies = append(ledger.Dependencies, dependency)
		if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
			Kind: model.LedgerMutationRemove, TargetID: dependency.ID, Rationale: "remove obsolete edge",
		}}); err != nil {
			t.Fatal(err)
		}
		setCodingLedgerItem(&ledger, "task", func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewAgreed
		})
		amended := prerequisite
		amended.Goal = "prepare exactly"
		if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
			Kind: model.LedgerMutationAmend, TargetID: prerequisite.ID,
			Item: &model.LedgerMutationItem{Task: &amended}, Rationale: "clarify prerequisite",
		}}); err != nil {
			t.Fatal(err)
		}
		if ledger.Tasks[0].ReviewState != model.LedgerReviewAgreed {
			t.Fatalf("unrelated dependent task agreement was reset to %q", ledger.Tasks[0].ReviewState)
		}
	})

	t.Run("tombstoned dependency cannot be reordered", func(t *testing.T) {
		ledger := repositoryLedgerFixture()
		prerequisite := model.LedgerTask{
			LedgerItem: repositoryLedgerItem("reorder-prerequisite", 5), Goal: "prepare",
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		}
		dependency := model.LedgerDependency{
			LedgerItem:     repositoryLedgerItem("reorder-dependency", 6),
			TaskID:         "task",
			PrerequisiteID: prerequisite.ID,
			Kind:           model.LedgerDependencyRequires,
		}
		ledger.Tasks = append(ledger.Tasks, prerequisite)
		ledger.Dependencies = append(ledger.Dependencies, dependency)
		if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
			Kind: model.LedgerMutationRemove, TargetID: dependency.ID, Rationale: "remove",
		}}); err != nil {
			t.Fatal(err)
		}
		rank := int64(20)
		err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
			Kind: model.LedgerMutationReorder, TargetID: dependency.ID, Rank: &rank, Rationale: "move",
		}})
		if err == nil || !strings.Contains(err.Error(), "is not active") {
			t.Fatalf("tombstoned dependency reorder validation = %v", err)
		}
	})

	t.Run("unresolved disposition requires active unanswered question", func(t *testing.T) {
		for _, test := range []struct {
			name     string
			question model.LedgerDecisionQuestion
		}{
			{
				name: "answered",
				question: model.LedgerDecisionQuestion{
					LedgerItem: repositoryLedgerItem("answered-question", 5),
					Kind:       model.LedgerQuestion, Prompt: "Which route?", Answer: "The frozen route.",
				},
			},
			{
				name: "tombstoned",
				question: model.LedgerDecisionQuestion{
					LedgerItem: func() model.LedgerItem {
						item := repositoryLedgerItem("tombstoned-question", 5)
						item.Disposition = model.LedgerDispositionTombstoned
						item.ReviewState = ""
						return item
					}(),
					Kind: model.LedgerQuestion, Prompt: "Which route?",
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				ledger := repositoryLedgerFixture()
				ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, test.question)
				err := validateMutationOwnerTurnDisposition(
					ledger, ledger, nil,
					model.OwnerTurnDisposition{
						Kind: model.OwnerTurnUnresolved, QuestionIDs: []model.LedgerItemID{test.question.ID},
					},
				)
				if err == nil || !strings.Contains(err.Error(), "not active and open") {
					t.Fatalf("closed question unresolved validation = %v", err)
				}
			})
		}
	})
}

func TestCodingLedgerDependencyAmendmentRetainsBothEndpointAuthorities(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Amend dependency authority.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	for index, id := range []model.LedgerItemID{"prerequisite-old", "prerequisite-new"} {
		ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
			LedgerItem:             repositoryLedgerItem(string(id), int64(5+index)),
			Goal:                   "prepare " + string(id),
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		})
	}
	dependency := model.LedgerDependency{
		LedgerItem:     repositoryLedgerItem("amended-dependency", 7),
		TaskID:         "task",
		PrerequisiteID: "prerequisite-old",
		Kind:           model.LedgerDependencyRequires,
	}
	ledger.Dependencies = append(ledger.Dependencies, dependency)
	ids := make([]model.LedgerItemID, 0)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	ledger.Dependencies[0].ReviewState = model.LedgerReviewProposed
	ids = append(ids, dependency.ID)
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "dependency-authority-create", OwnerTurnID: "dependency-authority-create-turn",
		OwnerTurnText: "Amend dependency authority.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	amendedDependency := dependency
	amendedDependency.PrerequisiteID = "prerequisite-new"
	amendedDependency.Rank = 9007199254740992
	amendedDependency.ReviewState = model.LedgerReviewProposed
	command := ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Use the new prerequisite.",
		Mutation: model.LedgerMutation{
			OperationID: "dependency-authority-amend", BaseRevision: created.Revision,
			OwnerTurnID: "dependency-authority-amend-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{dependency.ID},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationAmend, TargetID: dependency.ID,
				Item: &model.LedgerMutationItem{Dependency: &amendedDependency}, Rationale: "change prerequisite",
			}},
		},
		CoveragePassed: true,
	}
	applied, err := repository.ApplyCodingLedgerMutation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.ApplyCodingLedgerMutation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || replayed.Applied {
		t.Fatalf("dependency amendment apply/replay = %t/%t", applied.Applied, replayed.Applied)
	}
	rows, err := pool.Query(ctx, `
		SELECT item_id FROM coding_ledger_owner_turn_items
		WHERE ledger_id=$1 AND owner_turn_id=$2 ORDER BY item_id
	`, created.LedgerID, command.Mutation.OwnerTurnID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var represented []model.LedgerItemID
	for rows.Next() {
		var id model.LedgerItemID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		represented = append(represented, id)
	}
	want := []model.LedgerItemID{"prerequisite-new", "prerequisite-old", "task"}
	if !sameCodingLedgerItemIDs(represented, want) {
		t.Fatalf("dependency endpoint authorities = %v, want %v", represented, want)
	}

	precisionDependency := amendedDependency
	precisionDependency.Rank = 9007199254740993
	precisionCommand := ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the dependency exactly.",
		Mutation: model.LedgerMutation{
			OperationID: "dependency-precision-amend", BaseRevision: applied.Revision,
			OwnerTurnID: "dependency-precision-amend-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{dependency.ID},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationAmend, TargetID: dependency.ID,
				Item: &model.LedgerMutationItem{Dependency: &precisionDependency}, Rationale: "change exact rank",
			}},
		},
		CoveragePassed: true,
	}
	precisionApplied, err := repository.ApplyCodingLedgerMutation(ctx, precisionCommand)
	if err != nil {
		t.Fatal(err)
	}
	precisionReplayed, err := repository.ApplyCodingLedgerMutation(ctx, precisionCommand)
	if err != nil {
		t.Fatal(err)
	}
	if !precisionApplied.Applied || precisionReplayed.Applied ||
		len(precisionApplied.Ledger.Dependencies) != 1 ||
		precisionApplied.Ledger.Dependencies[0].Rank != precisionDependency.Rank {
		t.Fatalf("exact dependency rank apply/replay = %#v / %#v", precisionApplied, precisionReplayed)
	}
}

func TestCodingLedgerInitialJSONReplayNormalizesEmptyCollections(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Replay the exact JSON proposal.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	ledger.Constraints = []model.LedgerConstraint{}
	ledger.Groups = []model.LedgerGroup{}
	ledger.DecisionsAndQuestions = []model.LedgerDecisionQuestion{}
	ledger.Dependencies = []model.LedgerDependency{}
	ledger.Findings = []model.LedgerFinding{}
	raw, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var decoded model.EngineeringLedger
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	command := CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "json-replay-create", OwnerTurnID: "json-replay-turn",
		OwnerTurnText: "Replay the exact JSON proposal.", Ledger: decoded,
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind:    model.OwnerTurnRepresented,
			ItemIDs: []model.LedgerItemID{"objective", "requirement", "task", "acceptance", "check"},
		},
		CoveragePassed: true, Routes: repositoryLedgerRoutes(1),
	}
	created, err := repository.CreateCodingLedger(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.CreateCodingLedger(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Applied || replayed.Applied || replayed.Revision != created.Revision {
		t.Fatalf("JSON proposal apply/replay = %t/%t revisions=%d/%d", created.Applied, replayed.Applied, created.Revision, replayed.Revision)
	}
}

func TestCodingLedgerDependencyMetadataDoesNotEnterIndexedEdgeIdentity(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Preserve large dependency provenance.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("metadata-prerequisite", 5), Goal: "prepare",
		RequirementIDs: []model.LedgerItemID{"requirement"}, AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	})
	dependency := model.LedgerDependency{
		LedgerItem: repositoryLedgerItem("metadata-dependency", 6),
		TaskID:     "task", PrerequisiteID: "metadata-prerequisite", Kind: model.LedgerDependencyRequires,
	}
	dependency.Provenance.Reference = strings.Repeat("0123456789abcdef", 500)
	dependency.ReviewState = model.LedgerReviewProposed
	ledger.Dependencies = append(ledger.Dependencies, dependency)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	ids := []model.LedgerItemID{"objective", "requirement", "task", "acceptance", "check", "metadata-prerequisite", "metadata-dependency"}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "metadata-create", OwnerTurnID: "metadata-turn",
		OwnerTurnText: "Preserve large dependency provenance.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	var edgeID string
	if err := pool.QueryRow(ctx, `
		SELECT edge_id FROM coding_ledger_edges
		WHERE ledger_id=$1 AND revision=1 AND edge_kind='requires'
	`, created.LedgerID).Scan(&edgeID); err != nil {
		t.Fatal(err)
	}
	if edgeID != codingLedgerDependencyEdgeID(dependency.ID) {
		t.Fatalf("indexed edge identity is not the dependency namespace: %q", edgeID)
	}
	if got := created.Ledger.Dependencies[0].Provenance.Reference; got != dependency.Provenance.Reference {
		t.Fatalf("dependency provenance bytes=%d, want %d", len(got), len(dependency.Provenance.Reference))
	}
}

func TestCodingLedgerPlanningFindingCanBeRemovedAndRestored(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Restore a planning finding.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	finding := model.LedgerFinding{
		LedgerItem: repositoryLedgerItem("restorable-finding", 5),
		Kind:       model.LedgerFindingPlanning, Observation: "Needs owner review.",
		TargetIDs: []model.LedgerItemID{"task"},
	}
	finding.ReviewState = model.LedgerReviewProposed
	ledger.Findings = append(ledger.Findings, finding)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "finding-restore-create", OwnerTurnID: "finding-restore-create-turn",
		OwnerTurnText: "Restore a planning finding.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind:    model.OwnerTurnRepresented,
			ItemIDs: []model.LedgerItemID{"objective", "requirement", "task", "acceptance", "check", finding.ID},
		},
		CoveragePassed: true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	removed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Remove the finding.",
		Mutation: model.LedgerMutation{
			OperationID: "finding-remove", BaseRevision: created.Revision, OwnerTurnID: "finding-remove-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{finding.ID}},
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationRemove, TargetID: finding.ID, Rationale: "temporarily resolved"}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Restore the finding.",
		Mutation: model.LedgerMutation{
			OperationID: "finding-restore", BaseRevision: removed.Revision, OwnerTurnID: "finding-restore-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{finding.ID}},
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationRestore, TargetID: finding.ID, Rationale: "review remains necessary"}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Ledger.Findings) != 1 || restored.Ledger.Findings[0].ID != finding.ID ||
		restored.Ledger.Findings[0].Disposition != model.LedgerDispositionActive {
		t.Fatalf("restored finding = %#v", restored.Ledger.Findings)
	}
}

func TestCodingLedgerRejectsReservedDependencyRecordIdentity(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Objectives[0].ID = model.LedgerItemID(codingLedgerDependencyRecordPrefix + "user")
	if err := validateLedgerForRepository(ledger); err == nil || !strings.Contains(err.Error(), "reserved stable identity") {
		t.Fatalf("reserved item identity error = %v", err)
	}
}

func TestCodingLedgerInitialReplayPreservesDependencyDispositionIdentity(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Preserve exact dependency disposition identity.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("replay-prerequisite", 5), Goal: "prepare replay",
		RequirementIDs: []model.LedgerItemID{"requirement"}, AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	})
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	ledger.Dependencies = []model.LedgerDependency{
		{
			LedgerItem: repositoryLedgerItem("dependency-requires", 6),
			TaskID:     "task", PrerequisiteID: "replay-prerequisite", Kind: model.LedgerDependencyRequires,
		},
		{
			LedgerItem: repositoryLedgerItem("dependency-blocks", 7),
			TaskID:     "task", PrerequisiteID: "replay-prerequisite", Kind: model.LedgerDependencyBlocks,
		},
	}
	for index := range ledger.Dependencies {
		ledger.Dependencies[index].ReviewState = model.LedgerReviewProposed
	}
	command := CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "dependency-replay-create", OwnerTurnID: "dependency-replay-turn",
		OwnerTurnText: "Preserve exact dependency disposition identity.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"dependency-requires"},
		},
		CoveragePassed: true, Routes: repositoryLedgerRoutes(1),
	}
	if _, err := repository.CreateCodingLedger(ctx, command); err != nil {
		t.Fatal(err)
	}
	changed := command
	changed.OwnerTurnDisposition.ItemIDs = []model.LedgerItemID{"dependency-blocks"}
	if _, err := repository.CreateCodingLedger(ctx, changed); err == nil ||
		!strings.Contains(err.Error(), "different proposal") {
		t.Fatalf("changed dependency identity replay error = %v", err)
	}
}

func TestCodingLedgerTaskDetailIncludesLinkedGroup(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Read task group context.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	group := model.LedgerGroup{
		LedgerItem: repositoryLedgerItem("task-group", 5),
		Title:      "Task group",
	}
	group.ReviewState = model.LedgerReviewProposed
	ledger.Groups = append(ledger.Groups, group)
	ledger.Tasks[0].GroupID = group.ID
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "task-detail-group-create", OwnerTurnID: "task-detail-group-turn",
		OwnerTurnText: "Read task group context.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"task", group.ID},
		},
		CoveragePassed: true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repository.CodingLedgerTaskDetail(ctx, created.LedgerID, created.Revision, "task")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Ledger.Groups) != 1 || detail.Ledger.Groups[0].ID != group.ID {
		t.Fatalf("task detail groups = %#v", detail.Ledger.Groups)
	}
}

func TestCodingLedgerPersistsDecisionBindingsToTaskContextItems(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Persist decision bindings to task context.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Constraints = []model.LedgerConstraint{{
		LedgerItem: repositoryLedgerItem("constraint", 5), Rule: "preserve the public contract",
	}}
	ledger.Groups = []model.LedgerGroup{{
		LedgerItem: repositoryLedgerItem("group", 6), Title: "task-local subsystem",
	}}
	ledger.Tasks[0].ConstraintIDs = []model.LedgerItemID{"constraint"}
	ledger.Tasks[0].GroupID = "group"
	ledger.DecisionsAndQuestions = []model.LedgerDecisionQuestion{
		{
			LedgerItem: repositoryLedgerItem("constraint-decision", 7), Kind: model.LedgerDecision,
			Prompt: "Which contract?", Answer: "The existing public contract.",
			Affects: []model.LedgerItemID{"constraint"},
		},
		{
			LedgerItem: repositoryLedgerItem("group-decision", 8), Kind: model.LedgerDecision,
			Prompt: "Which subsystem?", Answer: "Only the task-local subsystem.",
			Affects: []model.LedgerItemID{"group"},
		},
	}
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	itemIDs := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger)))
	for _, record := range ledgerRepositoryRecords(ledger) {
		itemIDs = append(itemIDs, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "decision-binding-create", OwnerTurnID: "decision-binding-turn",
		OwnerTurnText: "Persist decision bindings to task context.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: itemIDs},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repository.CodingLedgerTaskDetail(ctx, created.LedgerID, created.Revision, "task")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Ledger.DecisionsAndQuestions) != 2 {
		t.Fatalf("task decision bindings = %#v", detail.Ledger.DecisionsAndQuestions)
	}
}

func TestCodingAgentTaskFromLedgerDetailKeepsOnlySelectedTaskContext(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Tasks[0].DeliveryObligation = "deliver the verified implementation"
	ledger.Tasks = ledger.Tasks[:1]
	ledger.Dependencies = []model.LedgerDependency{{
		LedgerItem: repositoryLedgerItem("dependency", 5), TaskID: "task",
		PrerequisiteID: "prerequisite", Kind: model.LedgerDependencyRequires,
	}}
	detail := CodingLedgerRevision{LedgerID: 1, Revision: 1, Ledger: ledger}
	task, err := codingAgentTaskFromLedgerDetail(detail)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "task" || !strings.Contains(task.Goal, "deliver the verified implementation") ||
		len(task.Acceptance) != 1 || task.Acceptance[0] != "verified" ||
		len(task.Dependencies) != 1 || task.Dependencies[0] != "prerequisite" ||
		len(task.Checks) != 1 || task.Checks[0].ID != "check" ||
		len(task.ReadFiles)+len(task.WriteFiles)+len(task.DeleteFiles) != 0 {
		t.Fatalf("isolated task context = %#v", task)
	}
}

func TestCodingAgentTaskFromLedgerIncludesOnlyBoundGuidance(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Constraints = []model.LedgerConstraint{{
		LedgerItem: repositoryLedgerItem("constraint", 5), Rule: "preserve the public contract",
	}}
	ledger.Groups = []model.LedgerGroup{{
		LedgerItem: repositoryLedgerItem("group", 6), Title: "task-local subsystem",
	}}
	ledger.Tasks[0].ConstraintIDs = []model.LedgerItemID{"constraint"}
	ledger.Tasks[0].GroupID = "group"
	ledger.DecisionsAndQuestions = []model.LedgerDecisionQuestion{
		{
			LedgerItem: repositoryLedgerItem("bound-decision", 7), Kind: model.LedgerDecision,
			Prompt: "Which contract?", Answer: "The existing public contract.",
			Affects: []model.LedgerItemID{"constraint"},
		},
		{
			LedgerItem: repositoryLedgerItem("sibling-decision", 8), Kind: model.LedgerDecision,
			Prompt: "Which sibling behavior?", Answer: "Unrelated behavior.",
			Affects: []model.LedgerItemID{"other-task"},
		},
	}
	task, err := codingAgentTaskFromLedger(ledger, "task")
	if err != nil {
		t.Fatal(err)
	}
	kinds := make(map[string]string)
	for _, guidance := range task.Guidance {
		kinds[guidance.ID] = guidance.Kind
	}
	for id, kind := range map[string]string{
		"requirement":    "requirement",
		"constraint":     "constraint",
		"group":          "group",
		"bound-decision": "decision",
	} {
		if kinds[id] != kind {
			t.Fatalf("task guidance omitted %s %s: %#v", kind, id, task.Guidance)
		}
	}
	if _, exists := kinds["sibling-decision"]; exists {
		t.Fatalf("task guidance included a sibling decision: %#v", task.Guidance)
	}
}

func TestNextIncompleteCodingLedgerTaskSelectsOneTaskAtATime(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Execute one durable ledger task at a time.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "task-ledger-selector-planner")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("claim normalized planning: %#v, %v", planner, err)
	}

	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("later-task", 5), Goal: "implement the later task",
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	})
	ledger.AcceptanceCriteria[0].TaskIDs = append(ledger.AcceptanceCriteria[0].TaskIDs, "later-task")
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, model.LedgerAcceptanceCriterion{
		LedgerItem: repositoryLedgerItem("overall-acceptance", 6), Outcome: "the integrated task set is verified",
		RequirementIDs: []model.LedgerItemID{"requirement"}, Required: true,
	})
	ledger.Checks = append(ledger.Checks, model.LedgerCheck{
		LedgerItem: repositoryLedgerItem("overall-check", 7), Argv: []string{"go", "test", "./..."},
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"overall-acceptance"}, Required: true,
	})
	itemIDs := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger)))
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		itemIDs = append(itemIDs, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "task-selector-create", OwnerTurnID: "task-selector-create-turn",
		OwnerTurnText: "Execute one durable ledger task at a time.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: itemIDs},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	reviews := make([]model.LedgerMutationOperation, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		reviews = append(reviews, model.LedgerMutationOperation{
			Kind: model.LedgerMutationReview, TargetID: itemID,
			ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed",
		})
	}
	agreed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Agree to the exact task ledger.",
		Mutation: model.LedgerMutation{
			OperationID: "task-selector-agree", BaseRevision: created.Revision,
			OwnerTurnID:          "task-selector-agree-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: itemIDs},
			Operations:           reviews,
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, planner.Authority, created.LedgerID, agreed.Revision); err != nil {
		t.Fatal(err)
	}
	auditID := repositoryReadyPlanningAudit(t, repository, created.LedgerID, agreed.Revision, "task-selector-freeze")
	frozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: agreed.Revision, OperationID: "task-selector-freeze",
		OwnerTurnID: "task-selector-freeze-turn", OwnerTurnText: "Freeze the task selector fixture.", AuditID: auditID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteCodingLedgerReview(ctx, job.ID, created.LedgerID, frozen.Revision); err != nil {
		t.Fatal(err)
	}
	executor, err := repository.ClaimNextStep(ctx, "task-ledger-selector-executor")
	if err != nil || executor == nil || executor.Step.Action != "v3_coding" {
		t.Fatalf("claim normalized execution: %#v, %v", executor, err)
	}

	first, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.ID != "task" || first.Goal != "implement" || len(first.Acceptance) != 1 ||
		len(first.Checks) != 1 || first.Checks[0].ID != "check" {
		t.Fatalf("first selected task = %#v", first)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *first)
	second, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if second == nil || second.ID != "later-task" || second.Goal != "implement the later task" {
		t.Fatalf("second selected task = %#v", second)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *second)
	remaining, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || remaining != nil {
		t.Fatalf("remaining task = %#v, %v", remaining, err)
	}
}

func TestCodingLedgerTaskDetailUsesExactActiveBindings(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Read only exact active task bindings.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("other-task", 5), Goal: "implement another outcome",
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"other-acceptance"},
	})
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, model.LedgerAcceptanceCriterion{
		LedgerItem: repositoryLedgerItem("other-acceptance", 6), Outcome: "another outcome is verified",
		RequirementIDs: []model.LedgerItemID{"requirement"}, TaskIDs: []model.LedgerItemID{"other-task"}, Required: true,
	})
	ledger.Checks = append(ledger.Checks,
		model.LedgerCheck{
			LedgerItem: repositoryLedgerItem("other-check", 7), Argv: []string{"go", "test", "./other"},
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"other-acceptance"}, Required: true,
		},
		model.LedgerCheck{
			LedgerItem: repositoryLedgerItem("removed-check", 8), Argv: []string{"go", "test", "./removed"},
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"}, Required: true,
		},
	)
	ledger.Dependencies = append(ledger.Dependencies, model.LedgerDependency{
		LedgerItem: repositoryLedgerItem("removed-dependency", 9),
		TaskID:     "task", PrerequisiteID: "other-task", Kind: model.LedgerDependencyRequires,
	})
	ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("removed-decision", 10), Kind: model.LedgerDecision,
		Prompt: "Use removed context?", Answer: "No.", Affects: []model.LedgerItemID{"task"},
	})
	ledger.Findings = append(ledger.Findings, model.LedgerFinding{
		LedgerItem: repositoryLedgerItem("removed-finding", 11), Kind: model.LedgerFindingPlanning,
		Observation: "Removed planning context.", TargetIDs: []model.LedgerItemID{"task"},
	})
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			if item.Disposition == model.LedgerDispositionActive {
				item.ReviewState = model.LedgerReviewProposed
			}
		})
	}
	for index := range ledger.Dependencies {
		ledger.Dependencies[index].ReviewState = model.LedgerReviewProposed
	}
	for _, id := range []model.LedgerItemID{
		"removed-check", "removed-dependency", "removed-decision", "removed-finding",
	} {
		setCodingLedgerItem(&ledger, id, func(item *model.LedgerItem) {
			item.Disposition = model.LedgerDispositionTombstoned
			item.ReviewState = ""
		})
	}
	ids := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger))+len(ledger.Dependencies))
	for _, record := range ledgerRepositoryRecords(ledger) {
		if record.item.Disposition == model.LedgerDispositionActive {
			ids = append(ids, record.item.ID)
		}
	}
	for _, dependency := range ledger.Dependencies {
		if dependency.Disposition == model.LedgerDispositionActive {
			ids = append(ids, dependency.ID)
		}
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "task-detail-bindings-create", OwnerTurnID: "task-detail-bindings-turn",
		OwnerTurnText: "Read only exact active task bindings.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repository.CodingLedgerTaskDetail(ctx, created.LedgerID, created.Revision, "task")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Ledger.Checks) != 1 || detail.Ledger.Checks[0].ID != "check" {
		t.Fatalf("task detail checks = %#v", detail.Ledger.Checks)
	}
	if len(detail.Ledger.Dependencies) != 0 ||
		len(detail.Ledger.DecisionsAndQuestions) != 0 ||
		len(detail.Ledger.Findings) != 0 {
		t.Fatalf("task detail included removed related records: dependencies=%#v decisions=%#v findings=%#v",
			detail.Ledger.Dependencies, detail.Ledger.DecisionsAndQuestions, detail.Ledger.Findings)
	}
}

func TestCodingLedgerSearchTreatsLikeMetacharactersLiterally(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Search literal ledger text.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Objectives[0].Outcome = "deliver 100% coverage"
	ledger.Requirements[0].Outcome = "deliver 1000 coverage"
	ids := make([]model.LedgerItemID, 0)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "literal-search-create", OwnerTurnID: "literal-search-turn",
		OwnerTurnText: "Search literal ledger text.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := repository.ReadCodingLedger(ctx, created.LedgerID, CodingLedgerReadOptions{
		Revision: created.Revision, Search: "100% coverage",
	})
	if err != nil {
		t.Fatal(err)
	}
	records := ledgerRepositoryRecords(page.Ledger)
	if len(records) != 1 || records[0].item.ID != "objective" {
		t.Fatalf("literal wildcard search returned %#v", page.Ledger)
	}
}

func TestCodingLedgerChangedRoutesCanBeRefrozenWithExactConfigurationAuthority(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Refreeze changed routes.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks[0].DeliveryObligation = "deliver the task"
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("route-delivery-task", 7), Goal: "deliver the integrated change",
		DeliveryObligation:     "publish the completed integration",
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	})
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, model.LedgerAcceptanceCriterion{
		LedgerItem: repositoryLedgerItem("route-overall-acceptance", 5),
		Outcome:    "integrated", RequirementIDs: []model.LedgerItemID{"requirement"}, Required: true,
	})
	ledger.Checks = append(ledger.Checks, model.LedgerCheck{
		LedgerItem:             repositoryLedgerItem("route-overall-check", 6),
		Argv:                   []string{"go", "test", "./..."},
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"route-overall-acceptance"}, Required: true,
	})
	ids := make([]model.LedgerItemID, 0)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "route-refreeze-create", OwnerTurnID: "route-second-freeze-turn:sign_off",
		OwnerTurnText: "Refreeze changed routes.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	operations := make([]model.LedgerMutationOperation, 0, len(ids))
	for _, id := range ids {
		operations = append(operations, model.LedgerMutationOperation{
			Kind: model.LedgerMutationReview, TargetID: id,
			ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed",
		})
	}
	agreed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Agree to the initial ledger.",
		Mutation: model.LedgerMutation{
			OperationID: "route-refreeze-agree", BaseRevision: created.Revision,
			OwnerTurnID:          "route-refreeze-agree-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
			Operations:           operations,
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstAudit := repositoryReadyPlanningAudit(t, repository, created.LedgerID, agreed.Revision, "route-first-freeze")
	firstFrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: agreed.Revision, OperationID: "route-first-freeze",
		OwnerTurnID: "route-first-freeze-turn", OwnerTurnText: "Freeze the initial routes.", AuditID: firstAudit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE coding_ledgers
		SET state='revising',frozen_revision=NULL,updated_at=clock_timestamp()
		WHERE id=$1 AND current_revision=$2
	`, created.LedgerID, firstFrozen.Revision); err != nil {
		t.Fatal(err)
	}
	replacement := repositoryLedgerRoutes(firstFrozen.Revision + 1)
	for index := range replacement {
		replacement[index].ReasoningEffort = "xhigh"
	}
	changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Use the replacement fast routes.",
		Mutation: model.LedgerMutation{
			OperationID: "route-replacement", BaseRevision: firstFrozen.Revision,
			OwnerTurnID: "route-replacement-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"task"},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReview, TargetID: "task",
				ReviewState: model.LedgerReviewAgreed, Rationale: "approve replacement route authority",
			}},
		},
		CoveragePassed: true, Routes: replacement,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondAudit := repositoryReadyPlanningAudit(t, repository, created.LedgerID, changed.Revision, "route-second-freeze")
	refrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: changed.Revision, OperationID: "route-second-freeze",
		OwnerTurnID: "route-second-freeze-turn", OwnerTurnText: "Freeze the replacement routes.", AuditID: secondAudit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if refrozen.State != "frozen" || refrozen.BaseRevision != changed.Revision {
		t.Fatalf("replacement route freeze = %#v", refrozen)
	}
	var action, disposition string
	if err := pool.QueryRow(ctx, `
		SELECT turn_record.action,disposition.disposition
		FROM coding_ledger_owner_turns turn_record
		JOIN coding_ledger_owner_turn_dispositions disposition
		  USING (ledger_id,owner_turn_id)
		WHERE turn_record.ledger_id=$1 AND turn_record.owner_turn_id=$2
	`, created.LedgerID, "route-second-freeze-turn").Scan(&action, &disposition); err != nil {
		t.Fatal(err)
	}
	if action != "execution_configuration" || disposition != "represented" {
		t.Fatalf("replacement route authority = %s/%s", action, disposition)
	}
}

func TestAgentsReplanCarriesOnlyTheExactAcceptedTaskContract(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Preserve exact accepted work across replanning.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "accepted-carry-planner")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("claim planning step: %#v, %v", planner, err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, "Preserve the exact accepted task.")
	firstFrozen := freezeCodingAgentPlanFixture(t, repository, job, fixture)
	executor, err := repository.ClaimNextStep(ctx, "accepted-carry-executor")
	if err != nil || executor == nil || executor.Step.Action != "v3_coding" {
		t.Fatalf("claim execution step: %#v, %v", executor, err)
	}
	task, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || task == nil {
		t.Fatalf("select first task: %#v, %v", task, err)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *task)

	root, identity := codingPlanWorkspaceAuthority(t, job)
	replanned, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "accepted-carry-replan", job.ID),
		JobID:       job.ID, Feedback: "Keep the exact approved task contract.",
		ObservedLedgerRevision: firstFrozen.Revision,
		WorkspaceRoot:          root, WorkspaceIdentity: identity,
	})
	if err != nil || !replanned.Applied {
		t.Fatalf("replan accepted task: %#v, %v", replanned, err)
	}
	var state string
	var frozenRevision *int64
	if err := pool.QueryRow(ctx, `
		SELECT state,frozen_revision FROM coding_ledgers WHERE id=$1
	`, firstFrozen.LedgerID).Scan(&state, &frozenRevision); err != nil {
		t.Fatal(err)
	}
	if state != "revising" || frozenRevision != nil {
		t.Fatalf("ledger after replan = %s/%v", state, frozenRevision)
	}

	current, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: firstFrozen.LedgerID, OwnerTurnText: "Keep the exact approved task contract.",
		Mutation: model.LedgerMutation{
			OperationID: "accepted-carry-confirm", BaseRevision: firstFrozen.Revision,
			OwnerTurnID: "accepted-carry-confirm-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{model.LedgerItemID(task.ID)},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReview, TargetID: model.LedgerItemID(task.ID),
				ReviewState: model.LedgerReviewAgreed, Rationale: "the exact contract remains approved",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	auditID := repositoryReadyPlanningAudit(t, repository, current.LedgerID, current.Revision, "accepted-carry-refreeze")
	refrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: current.LedgerID, Revision: current.Revision,
		OperationID: "accepted-carry-refreeze", OwnerTurnID: "accepted-carry-refreeze-turn",
		OwnerTurnText: "Freeze the unchanged approved task contract.", AuditID: auditID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var carriedState string
	if err := pool.QueryRow(ctx, `
		SELECT state FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3
	`, refrozen.LedgerID, refrozen.Revision, task.ID).Scan(&carriedState); err != nil {
		t.Fatal(err)
	}
	if carriedState != "accepted" {
		t.Fatalf("unchanged accepted task reopened as %q", carriedState)
	}

	changedReplan, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "accepted-contract-change-replan", job.ID),
		JobID:       job.ID, Feedback: "Replace the approved task with a materially changed contract.",
		ObservedLedgerRevision: refrozen.Revision,
		WorkspaceRoot:          root, WorkspaceIdentity: identity,
	})
	if err != nil || !changedReplan.Applied {
		t.Fatalf("replan changed task: %#v, %v", changedReplan, err)
	}
	amendedTask := refrozen.Ledger.Tasks[0]
	amendedTask.Goal = "Implement the materially changed task contract."
	amendedTask.ReviewState = model.LedgerReviewProposed
	changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: refrozen.LedgerID, OwnerTurnText: "Replace the approved task with a materially changed contract.",
		Mutation: model.LedgerMutation{
			OperationID: "accepted-contract-change", BaseRevision: refrozen.Revision,
			OwnerTurnID: "accepted-contract-change-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{amendedTask.ID},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationAmend, TargetID: amendedTask.ID,
				Item: &model.LedgerMutationItem{Task: &amendedTask}, Rationale: "the task contract changed",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	agreedChange, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: changed.LedgerID, OwnerTurnText: "Approve the changed task contract.",
		Mutation: model.LedgerMutation{
			OperationID: "accepted-contract-change-agree", BaseRevision: changed.Revision,
			OwnerTurnID: "accepted-contract-change-agree-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{amendedTask.ID},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReview, TargetID: amendedTask.ID,
				ReviewState: model.LedgerReviewAgreed, Rationale: "approve the changed contract",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	changedAuditID := repositoryReadyPlanningAudit(t, repository, agreedChange.LedgerID, agreedChange.Revision, "accepted-contract-change-freeze")
	changedFrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: agreedChange.LedgerID, Revision: agreedChange.Revision,
		OperationID: "accepted-contract-change-freeze", OwnerTurnID: "accepted-contract-change-freeze-turn",
		OwnerTurnText: "Freeze the changed approved task contract.", AuditID: changedAuditID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var changedState string
	if err := pool.QueryRow(ctx, `
		SELECT state FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3
	`, changedFrozen.LedgerID, changedFrozen.Revision, task.ID).Scan(&changedState); err != nil {
		t.Fatal(err)
	}
	if changedState != "ready" {
		t.Fatalf("changed accepted task did not reopen as ready: %q", changedState)
	}

	cancelled, err := repository.CancelJob(ctx, CancelJobCommand{
		OperationID: codingPlanOperationID(t, "accepted-carry-cancel", job.ID),
		JobID:       job.ID, Reason: "Stop the remaining job without erasing accepted work.",
		WorkspaceRoot: root, WorkspaceIdentity: identity,
	})
	if err != nil || !cancelled.Applied {
		t.Fatalf("cancel accepted ledger: %#v, %v", cancelled, err)
	}
	var ledgerState, preservedState string
	if err := pool.QueryRow(ctx, `SELECT state FROM coding_ledgers WHERE id=$1`, refrozen.LedgerID).Scan(&ledgerState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT state FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3
	`, refrozen.LedgerID, refrozen.Revision, task.ID).Scan(&preservedState); err != nil {
		t.Fatal(err)
	}
	var cancelledState string
	if err := pool.QueryRow(ctx, `
		SELECT state FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3
	`, changedFrozen.LedgerID, changedFrozen.Revision, task.ID).Scan(&cancelledState); err != nil {
		t.Fatal(err)
	}
	if ledgerState != "cancelled" || preservedState != "accepted" || cancelledState != "cancelled" {
		t.Fatalf("cancellation states: ledger=%s accepted_history=%s unfinished=%s", ledgerState, preservedState, cancelledState)
	}
}

func TestCodingLedgerDependencyEdgeIdentityIsNamespaced(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Persist colliding logical edge identities.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	prerequisite := model.LedgerTask{
		LedgerItem: repositoryLedgerItem("prerequisite", 5), Goal: "prepare",
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
	}
	dependency := model.LedgerDependency{
		LedgerItem: repositoryLedgerItem("task:req:0", 6),
		TaskID:     "task", PrerequisiteID: prerequisite.ID, Kind: model.LedgerDependencyRequires,
	}
	ledger.Tasks = append(ledger.Tasks, prerequisite)
	ledger.Dependencies = append(ledger.Dependencies, dependency)
	ids := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger))+1)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	ledger.Dependencies[0].ReviewState = model.LedgerReviewProposed
	ids = append(ids, dependency.ID)

	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "edge-namespace-create", OwnerTurnID: "edge-namespace-turn",
		OwnerTurnText: "Persist colliding logical edge identities.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	var edgeCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM coding_ledger_edges
		WHERE ledger_id=$1 AND revision=$2
		  AND edge_id IN ($3,$4)
	`, created.LedgerID, created.Revision,
		codingLedgerDerivedEdgePrefix+"task:req:0", codingLedgerDependencyEdgeID(dependency.ID)).Scan(&edgeCount); err != nil {
		t.Fatal(err)
	}
	if edgeCount != 2 {
		t.Fatalf("namespaced edge count = %d, want 2", edgeCount)
	}
}

func TestCodingLedgerUnchangedRemovedCheckRetainsRemovalVersion(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Preserve an optional check tombstone.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Checks = append(ledger.Checks, model.LedgerCheck{
		LedgerItem:             repositoryLedgerItem("optional-check", 5),
		Argv:                   []string{"go", "test", "./..."},
		RequirementIDs:         []model.LedgerItemID{"requirement"},
		AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		Required:               false,
	})
	ids := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger)))
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "check-tombstone-create", OwnerTurnID: "check-tombstone-create-turn",
		OwnerTurnText: "Preserve an optional check tombstone.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	removed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Remove the optional check.",
		Mutation: model.LedgerMutation{
			OperationID: "remove-optional-check", BaseRevision: created.Revision, OwnerTurnID: "remove-optional-check-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"optional-check"},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationRemove, TargetID: "optional-check", Rationale: "no longer needed",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rank := int64(20)
	changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Reorder the task.",
		Mutation: model.LedgerMutation{
			OperationID: "reorder-after-check-removal", BaseRevision: removed.Revision, OwnerTurnID: "reorder-after-check-removal-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"task"},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank, Rationale: "move the task",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var versionCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM coding_ledger_item_versions
		WHERE ledger_id=$1 AND item_id='optional-check'
	`, created.LedgerID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	var itemRevision int64
	var removedByTurnID string
	if err := pool.QueryRow(ctx, `
		SELECT projection.item_revision,version.removed_by_turn_id
		FROM coding_ledger_revision_items projection
		JOIN coding_ledger_item_versions version
		  ON (version.ledger_id,version.item_id,version.revision)=
		     (projection.ledger_id,projection.item_id,projection.item_revision)
		WHERE projection.ledger_id=$1 AND projection.revision=$2
		  AND projection.item_id='optional-check'
	`, created.LedgerID, changed.Revision).Scan(&itemRevision, &removedByTurnID); err != nil {
		t.Fatal(err)
	}
	if versionCount != 2 || itemRevision != removed.Revision || removedByTurnID != "remove-optional-check-turn" {
		t.Fatalf("removed check versions=%d projected_revision=%d removed_by=%q; want 2, %d, remove-optional-check-turn",
			versionCount, itemRevision, removedByTurnID, removed.Revision)
	}
}

func TestCodingLedgerRequiredCheckAmendmentResetsVerificationScope(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
		LedgerItem: repositoryLedgerItem("unrelated-task", 5),
		Goal:       "unrelated", RequirementIDs: []model.LedgerItemID{},
		AcceptanceCriterionIDs: []model.LedgerItemID{},
	})
	amended := ledger.Checks[0]
	amended.Argv = []string{"go", "test", "./internal/queue"}
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationAmend, TargetID: amended.ID,
		Item: &model.LedgerMutationItem{Check: &amended}, Rationale: "change required verification",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Checks[0].ReviewState != model.LedgerReviewProposed ||
		ledger.AcceptanceCriteria[0].ReviewState != model.LedgerReviewProposed ||
		ledger.Tasks[0].ReviewState != model.LedgerReviewProposed {
		t.Fatalf("required verification scope was not reset: check=%s criterion=%s task=%s",
			ledger.Checks[0].ReviewState, ledger.AcceptanceCriteria[0].ReviewState, ledger.Tasks[0].ReviewState)
	}
	if ledger.Requirements[0].ReviewState != model.LedgerReviewAgreed ||
		ledger.Tasks[1].ReviewState != model.LedgerReviewAgreed {
		t.Fatalf("unrelated agreements changed: requirement=%s task=%s",
			ledger.Requirements[0].ReviewState, ledger.Tasks[1].ReviewState)
	}
}

func TestCodingLedgerTaskDetailIncludesExplicitBlockers(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Read explicit task blockers.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	question := model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("task-question", 5), Kind: model.LedgerQuestion,
		Prompt: "Which behavior?", Affects: []model.LedgerItemID{"task"},
	}
	decision := model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("task-decision", 6), Kind: model.LedgerDecision,
		Prompt: "Use the exact behavior?", Answer: "Yes.", Affects: []model.LedgerItemID{"task"},
	}
	finding := model.LedgerFinding{
		LedgerItem: repositoryLedgerItem("task-finding", 7), Kind: model.LedgerFindingPlanning,
		Observation: "The task has an unresolved discrepancy.", TargetIDs: []model.LedgerItemID{"task"},
	}
	ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, question, decision)
	ledger.Findings = append(ledger.Findings, finding)
	ids := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger)))
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "task-detail-blockers-create", OwnerTurnID: "task-detail-blockers-turn",
		OwnerTurnText: "Read explicit task blockers.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repository.CodingLedgerTaskDetail(ctx, created.LedgerID, created.Revision, "task")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Ledger.DecisionsAndQuestions) != 2 ||
		detail.Ledger.DecisionsAndQuestions[0].ID != question.ID ||
		detail.Ledger.DecisionsAndQuestions[1].ID != decision.ID {
		t.Fatalf("task detail decisions/questions = %#v", detail.Ledger.DecisionsAndQuestions)
	}
	if len(detail.Ledger.Findings) != 1 || detail.Ledger.Findings[0].ID != finding.ID {
		t.Fatalf("task detail findings = %#v", detail.Ledger.Findings)
	}
}

func TestCodingLedgerGeneratedAnswerTurnUsesReservedNamespace(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Keep generated answer turns separate.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const answeringTurnID = "answer-question-turn"
	question := model.LedgerDecisionQuestion{
		LedgerItem: repositoryLedgerItem("new-answered-question", 5), Kind: model.LedgerQuestion,
		Prompt: "Which authority?", Answer: "The exact owner turn.",
	}
	digest := sha256.Sum256([]byte(answeringTurnID + "\x00" + string(question.ID)))
	legacyGeneratedID := fmt.Sprintf("%s:answer:%x", answeringTurnID, digest[:8])
	ledger := repositoryLedgerFixture()
	ids := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger)))
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "answer-namespace-create", OwnerTurnID: legacyGeneratedID,
		OwnerTurnText: "Keep generated answer turns separate.", Ledger: ledger,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
		CoveragePassed:       true, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	question.ReviewState = model.LedgerReviewProposed
	applied, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Add the answered question.",
		Mutation: model.LedgerMutation{
			OperationID: "add-answered-question", BaseRevision: created.Revision, OwnerTurnID: answeringTurnID,
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{question.ID},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{DecisionQuestion: &question},
				Rationale: "record the answered question",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var answeredBy string
	if err := pool.QueryRow(ctx, `
		SELECT answered_by_turn_id FROM coding_ledger_questions
		WHERE ledger_id=$1 AND question_id=$2
	`, created.LedgerID, question.ID).Scan(&answeredBy); err != nil {
		t.Fatal(err)
	}
	if applied.Revision != created.Revision+1 || answeredBy == legacyGeneratedID ||
		!strings.HasPrefix(answeredBy, codingLedgerInternalOwnerTurnPrefix) {
		t.Fatalf("generated answer turn = %q, revision=%d", answeredBy, applied.Revision)
	}
}
