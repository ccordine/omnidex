package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshCodingPlanRepository(t *testing.T) (*pgxpool.Pool, *Repository) {
	t.Helper()
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for isolated PostgreSQL coding-plan coverage")
	}
	schema := "omnidex_coding_plan_test_" + codingPlanTestNonce(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatalf("install fresh coding-plan schema %q: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop coding-plan test schema %q: %v", schema, err)
		}
		pool.Close()
	})
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatalf("freeze coding-plan model authority: %v", err)
	}
	return pool, New(pool, authority)
}

type codingAgentLedgerFixture struct {
	authority model.StepAttemptAuthority
	created   CodingLedgerRevision
	itemIDs   []model.LedgerItemID
	goal      string
}

func codingAgentPlanReviewFixture(t *testing.T, repository *Repository, planner *model.ClaimedStep, goal string) (codingAgentLedgerFixture, CodingPlanLeafWrite) {
	t.Helper()
	ledger := repositoryLedgerFixture()
	ledger.Objectives[0].Outcome = goal
	ledger.Requirements[0].Outcome = goal
	ledger.Tasks[0].Goal = goal
	ledger.Tasks[0].WriteFiles = []string{"agent-fixture.txt"}
	ledger.AcceptanceCriteria[0].Outcome = goal
	ledger.Checks[0].Argv = []string{"true"}
	itemIDs := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger)))
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		itemIDs = append(itemIDs, record.item.ID)
	}
	configuration, err := codingagent.ConfigurationFromMetadata(planner.Job.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	routes := []CodingLedgerRoute{
		{Role: "planner", ConfigurationRevision: 1, Provider: configuration.Planner.Provider, Model: configuration.Planner.Model, ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "executor", ConfigurationRevision: 1, Provider: configuration.Executor.Provider, Model: configuration.Executor.Model, ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "reviewer", ConfigurationRevision: 1, Provider: configuration.Reviewer.Provider, Model: configuration.Reviewer.Model, ReasoningEffort: "medium", ServiceTier: "priority"},
	}
	seed := fmt.Sprintf("agent-%d-%d", planner.Job.ID, planner.Authority.Generation)
	created, err := repository.CreateCodingLedger(context.Background(), CreateCodingLedgerCommand{
		JobID: planner.Job.ID, OperationID: seed + "-create", OwnerTurnID: seed + "-create-turn",
		OwnerTurnText: goal, Ledger: ledger, CoveragePassed: true, Routes: routes,
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: itemIDs},
	})
	if err != nil {
		t.Fatal(err)
	}
	leaf := codingAgentPlanLeaf(t, goal, model.CodingPlanDecisionPending, planner.Authority.Generation)
	return codingAgentLedgerFixture{authority: planner.Authority, created: created, itemIDs: itemIDs, goal: goal}, leaf
}

func codingAgentPlanLeaf(t *testing.T, statement string, decision model.CodingPlanDecision, originGeneration int64) CodingPlanLeafWrite {
	t.Helper()
	id, err := model.NewCodingPlanLeafID()
	if err != nil {
		t.Fatalf("construct agent plan leaf identity: %v", err)
	}
	receipt := assemblyline.ApplicationRequirementCandidateResultRelationResult{Schema: assemblyline.ApplicationRequirementCandidateResultRelationSchemaV1, Relation: assemblyline.ApplicationRequirementNoDerivedResult}
	return CodingPlanLeafWrite{Leaf: model.CodingPlanLeaf{ID: id, Statement: statement, Decision: decision}, DecisionOriginGeneration: originGeneration, ResultRelation: &receipt}
}

func freezeCodingAgentPlanFixture(t *testing.T, repository *Repository, job model.Job, fixture codingAgentLedgerFixture) CodingLedgerRevision {
	t.Helper()
	ctx := context.Background()
	seed := fmt.Sprintf("agent-%d-%d", job.ID, fixture.authority.Generation)
	operations := make([]model.LedgerMutationOperation, 0, len(fixture.itemIDs))
	for _, itemID := range fixture.itemIDs {
		operations = append(operations, model.LedgerMutationOperation{
			Kind: model.LedgerMutationReview, TargetID: itemID,
			ReviewState: model.LedgerReviewAgreed, Rationale: "fixture owner agreed",
		})
	}
	agreed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: fixture.created.LedgerID, OwnerTurnText: fixture.goal, CoveragePassed: true,
		Mutation: model.LedgerMutation{
			OperationID: seed + "-agree", BaseRevision: fixture.created.Revision,
			OwnerTurnID:          seed + "-agree-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: fixture.itemIDs},
			Operations:           operations,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, fixture.authority, agreed.LedgerID, agreed.Revision); err != nil {
		t.Fatal(err)
	}
	auditID := repositoryReadyPlanningAudit(t, repository, agreed.LedgerID, agreed.Revision, seed)
	frozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: agreed.LedgerID, Revision: agreed.Revision, OperationID: seed + "-freeze",
		OwnerTurnID: seed + "-freeze-turn", OwnerTurnText: fixture.goal, AuditID: auditID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteCodingLedgerReview(ctx, job.ID, frozen.LedgerID, frozen.Revision); err != nil {
		t.Fatal(err)
	}
	return frozen
}

func storeCodingPlanFixture(
	t *testing.T,
	repository *Repository,
	leaves []CodingPlanLeafWrite,
) (model.Job, *model.ClaimedStep, model.CodingPlan) {
	t.Helper()
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Build the exact coding-plan fixture.", t.TempDir())
	if err != nil {
		t.Fatalf("enqueue coding-plan fixture: %v", err)
	}
	claim, err := repository.ClaimNextStep(ctx, "coding-plan-review-fixture")
	if err != nil {
		t.Fatalf("claim coding-plan fixture: %v", err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Step.Action != "v3_coding_plan" {
		t.Fatalf("coding-plan fixture claim = %#v", claim)
	}
	plan, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{
		Authority: claim.Authority,

		Leaves: leaves,
	})
	if err != nil {
		t.Fatalf("store coding-plan fixture: %v", err)
	}
	return job, claim, plan
}

func codingPlanExecutableLeaf(
	t *testing.T,
	statement string,
	decision model.CodingPlanDecision,
	originGeneration int64,
) CodingPlanLeafWrite {
	t.Helper()
	id, err := model.NewCodingPlanLeafID()
	if err != nil {
		t.Fatalf("construct coding-plan leaf identity: %v", err)
	}
	receipt := codingPlanAcceptedReceipt(t, statement)
	return CodingPlanLeafWrite{
		Leaf: model.CodingPlanLeaf{
			ID: id, Statement: statement,
			Decision: decision,
		},
		DecisionOriginGeneration: originGeneration,
		ResultRelation:           &receipt,
	}
}

func codingPlanAcceptedReceipt(t *testing.T, statement string) assemblyline.ApplicationRequirementCandidateResultRelationResult {
	t.Helper()
	result := assemblyline.ApplicationRequirementCandidateResultRelationResult{
		Schema:   assemblyline.ApplicationRequirementCandidateResultRelationSchemaV1,
		Relation: assemblyline.ApplicationRequirementNoDerivedResult,
	}
	if err := result.ValidateAcceptedFor(statement); err != nil {
		t.Fatal(err)
	}
	return result
}

func codingPlanOperationID(t *testing.T, label string, jobID int64) model.LifecycleOperationID {
	t.Helper()
	id, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatalf("construct coding-plan operation ID: %v", err)
	}
	return id
}

func codingPlanWorkspaceAuthority(t *testing.T, job model.Job) (string, string) {
	t.Helper()
	root, identity, err := lifecycleJobWorkspaceBinding(job)
	if err != nil {
		t.Fatalf("load coding-plan workspace authority: %v", err)
	}
	return root, identity
}

func codingPlanTestNonce(t *testing.T) string {
	t.Helper()
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("generate coding-plan test identity: %v", err)
	}
	return hex.EncodeToString(value[:])
}

func acceptCodingLedgerTaskFixture(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	task codingagent.Task,
) {
	t.Helper()
	ctx := context.Background()
	configuration, err := repository.FrozenCodingLedgerConfiguration(ctx, authority)
	if err != nil {
		t.Fatal(err)
	}
	store := repository.CodingAgentEvidence(authority)
	executionReads := codingAgentFixtureFiles(task.ReadFiles, task.WriteFiles, task.DeleteFiles)
	execution := &codingagent.Assignment{
		JobID: authority.JobID, Generation: authority.Generation, Role: codingagent.Execute,
		Task: &task, Root: "/workspace", ReadFiles: executionReads,
		WriteFiles: append([]string(nil), task.WriteFiles...), DeleteFiles: append([]string(nil), task.DeleteFiles...),
		Route: configuration.Executor,
	}
	executeCall, err := store.Append(ctx, codingagent.Entry{
		Generation: authority.Generation, Phase: "call_opened", Role: codingagent.Execute,
		TaskID: task.ID, Assignment: execution,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, codingagent.Entry{
		Generation: authority.Generation, Phase: "native_event", Role: codingagent.Execute,
		TaskID: task.ID, CallID: executeCall, Frame: []byte(`{"fixture":"execution"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, codingagent.Entry{
		Generation: authority.Generation, Phase: "call_completed", Role: codingagent.Execute,
		TaskID: task.ID, CallID: executeCall, Result: &codingagent.Result{
			CallID: executeCall, Provider: configuration.Executor.Provider, Model: configuration.Executor.Model,
			ThreadID: "execution-" + task.ID, TurnID: "execution-turn-" + task.ID,
			Completed: true, Content: []byte(`{"questions":[]}`),
		},
	}); err != nil {
		t.Fatal(err)
	}
	review := &codingagent.Assignment{
		JobID: authority.JobID, Generation: authority.Generation, Role: codingagent.Review,
		Task: &task, Root: "/workspace", ReadFiles: codingAgentFixtureFiles(task.ReadFiles, task.WriteFiles),
		Route: configuration.Reviewer,
	}
	reviewCall, err := store.Append(ctx, codingagent.Entry{
		Generation: authority.Generation, Phase: "call_opened", Role: codingagent.Review,
		TaskID: task.ID, Assignment: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, codingagent.Entry{
		Generation: authority.Generation, Phase: "native_event", Role: codingagent.Review,
		TaskID: task.ID, CallID: reviewCall, Frame: []byte(`{"fixture":"review"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, codingagent.Entry{
		Generation: authority.Generation, Phase: "call_completed", Role: codingagent.Review,
		TaskID: task.ID, CallID: reviewCall, Result: &codingagent.Result{
			CallID: reviewCall, Provider: configuration.Reviewer.Provider, Model: configuration.Reviewer.Model,
			ThreadID: "review-" + task.ID, TurnID: "review-turn-" + task.ID,
			Completed: true, Content: []byte(`{"findings":[],"questions":[]}`),
		},
	}); err != nil {
		t.Fatal(err)
	}
	checks := make([]codingagent.CheckResult, len(task.Checks))
	for index, check := range task.Checks {
		checks[index] = codingagent.CheckResult{ID: check.ID}
	}
	if _, err := store.Append(ctx, codingagent.Entry{
		Generation: authority.Generation, Phase: "task_accepted", TaskID: task.ID,
		CallID: reviewCall, Checks: checks,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AcceptCodingLedgerTask(ctx, authority, task.ID); err != nil {
		t.Fatal(err)
	}
}

func codingAgentFixtureFiles(groups ...[]string) []string {
	seen := make(map[string]bool)
	var files []string
	for _, group := range groups {
		for _, file := range group {
			if !seen[file] {
				seen[file] = true
				files = append(files, file)
			}
		}
	}
	sort.Strings(files)
	return files
}

func sameCodingPlanProjection(left, right model.CodingPlan) bool {
	if left.JobID != right.JobID || left.Generation != right.Generation ||
		left.Revision != right.Revision || left.State != right.State ||
		len(left.Leaves) != len(right.Leaves) || !left.CreatedAt.Equal(right.CreatedAt) ||
		!left.UpdatedAt.Equal(right.UpdatedAt) {
		return false
	}
	if (left.FrozenAt == nil) != (right.FrozenAt == nil) ||
		(left.FrozenAt != nil && !left.FrozenAt.Equal(*right.FrozenAt)) {
		return false
	}
	for index := range left.Leaves {
		if left.Leaves[index] != right.Leaves[index] {
			return false
		}
	}
	return true
}
