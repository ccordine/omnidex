package queue

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCodingLedgerRepositoryTransactionallyFreezesExactRevision(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Ship the normalized ledger.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: repositoryLedgerItem("delivery-task", 7), Goal: "deliver the integrated change", Acceptance: []model.TaskAcceptance{{ID: "overall-acceptance", Text: "integrated", Verification: []string{"go test ./..."}}}})
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
		OwnerTurnDisposition: model.RepresentedOwnerTurn(
			[]model.LedgerItemID{"task", "delivery-task"}),

		Ledger: ledger, Routes: routes,
	})
	if err != nil {
		t.Fatal(err)
	}
	rank := int64(8)
	mutation := model.LedgerMutation{
		OperationID: "ledger-reorder", BaseRevision: created.Revision, OwnerTurnID: "owner-reorder",
		OwnerTurnDisposition: model.RepresentedOwnerTurn(

			[]model.LedgerItemID{"task", "delivery-task"}),

		Operations: []model.LedgerMutationOperation{
			{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReview, TargetID: "delivery-task", ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed"},
			{Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank, Rationale: "owner reordered"},
		},
	}
	controlCommand := ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the task.", Mutation: mutation,
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
	conflictingMutation := mutation
	conflictingMutation.Operations = append([]model.LedgerMutationOperation(nil), mutation.Operations...)
	conflictingRank := int64(9)
	conflictingMutation.Operations[len(conflictingMutation.Operations)-1].Rank = &conflictingRank
	if _, err := repository.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the task.",
		Mutation: conflictingMutation,
	}); err == nil {
		t.Fatal("operation identity conflict was accepted as an idempotent replay")
	}
	sameResultConflict := mutation
	sameResultConflict.Operations = append([]model.LedgerMutationOperation(nil), mutation.Operations...)
	sameResultConflict.Operations[0].Rationale = "different rationale with the same materialized result"
	if _, err := repository.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the task.",
		Mutation: sameResultConflict,
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
	if len(ledgerRepositoryRecords(page.Ledger)) != 1 {
		t.Fatalf("bounded ledger page returned more than one record: %#v", page.Ledger)
	}
	detail, err := repository.CodingLedgerTaskDetail(ctx, created.LedgerID, changed.Revision, "task")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Ledger.Tasks) != 1 || detail.Ledger.Tasks[0].ID != "task" || !reflect.DeepEqual(detail.Ledger.Tasks[0].Acceptance, ledger.Tasks[0].Acceptance) {
		t.Fatalf("task detail lost exact acceptance: %#v", detail.Ledger)
	}
	diff, err := repository.CodingLedgerDiff(ctx, created.LedgerID, created.Revision, changed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Amended) == 0 {
		t.Fatalf("revision diff omitted changed records: %#v", diff)
	}
	// Reordering invalidates the task's prior agreement. Only a fresh owner
	// agreement can authorize the changed exact revision.
	if _, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: created.LedgerID, Revision: changed.Revision, OperationID: "unagreed-reorder-freeze", OwnerTurnID: "unagreed-reorder-owner", OwnerTurnText: "Freeze before renewed agreement."}); err == nil {
		t.Fatal("reordered task bypassed owner agreement")
	}
	changed, err = repository.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Agree to the reordered task.", Mutation: model.LedgerMutation{OperationID: "reordered-task-agreement", OwnerTurnID: "reordered-task-owner", BaseRevision: changed.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed}}}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: changed.Revision, OperationID: "ledger-freeze",
		OwnerTurnID: "owner-freeze", OwnerTurnText: "Freeze this exact revision.",
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
		OwnerTurnDisposition: model.RepresentedOwnerTurn(

			[]model.LedgerItemID{"task"}),

		Ledger: ledger,
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
	ledger.Tasks[0].Provenance.Reference = strings.Repeat("p", 8192-len(prefix))
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "origin-boundary-create", OwnerTurnID: "origin-boundary-turn",
		OwnerTurnText: "Create the maximum provenance item.", Ledger: ledger,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(

			[]model.LedgerItemID{"task"}),

		Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	rank := int64(20)
	mutation := model.LedgerMutation{
		OperationID: "origin-boundary-reorder", BaseRevision: created.Revision, OwnerTurnID: "origin-boundary-reorder-turn",
		OwnerTurnDisposition: model.RepresentedOwnerTurn(
			[]model.LedgerItemID{"task"}),

		Operations: []model.LedgerMutationOperation{{
			Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank, Rationale: "move objective",
		}},
	}
	changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the objective.",
		Mutation: mutation,
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move the objective.",
		Mutation: mutation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Applied || replayed.Revision != changed.Revision {
		t.Fatalf("maximum-origin replay = %#v", replayed)
	}
}
