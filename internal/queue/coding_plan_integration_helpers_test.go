package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
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

func codingAgentPlanReviewFixture(t *testing.T, repository *Repository, planner *model.ClaimedStep, goal string) (StoreCodingPlanReviewCommand, CodingPlanLeafWrite) {
	t.Helper()
	workload := &codingagent.Workload{Tasks: []codingagent.Task{{ID: "task", Goal: goal, Acceptance: []string{goal}, WriteFiles: []string{"agent-fixture.txt"}, RequirementIDs: []string{"objective"}, Checks: []codingagent.Check{{ID: "task-check", Argv: []string{"true"}}}}}, Checks: []codingagent.Check{{ID: "overall-check", Argv: []string{"true"}}}}
	statement, err := codingagent.PlanTaskStatement(workload.Tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	leaf := codingAgentPlanLeaf(t, statement, model.CodingPlanDecisionPending, planner.Authority.Generation)
	store := repository.CodingAgentEvidence(planner.Authority)
	configuration, err := codingagent.ConfigurationFromMetadata(planner.Job.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	assignment := &codingagent.Assignment{
		JobID: planner.Job.ID, Generation: planner.Authority.Generation, Role: codingagent.Plan,
		Route:        configuration.Planner,
		Requirements: []codingagent.Requirement{{ID: "objective", Outcome: planner.Job.Instruction}},
		Objective:    &codingagent.Objective{Instruction: planner.Job.Instruction},
	}
	callID, err := store.Append(context.Background(), codingagent.Entry{Generation: planner.Authority.Generation, Phase: "call_opened", Role: codingagent.Plan, Assignment: assignment})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), codingagent.Entry{Generation: planner.Authority.Generation, Phase: "native_event", Role: codingagent.Plan, CallID: callID, Frame: []byte(`{"fixture":"native-plan"}`)}); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(workload)
	if err != nil {
		t.Fatal(err)
	}
	result := &codingagent.Result{CallID: callID, Provider: configuration.Planner.Provider, Model: configuration.Planner.Model, ThreadID: "fixture-plan-thread", TurnID: "fixture-plan-turn", Completed: true, Content: content}
	if _, err := store.Append(context.Background(), codingagent.Entry{Generation: planner.Authority.Generation, Phase: "call_completed", Role: codingagent.Plan, CallID: callID, Result: result}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), codingagent.Entry{Generation: planner.Authority.Generation, Phase: "plan_proposed", CallID: callID, Workload: workload}); err != nil {
		t.Fatal(err)
	}
	intake := &codingagent.IntakeContext{RedactedRequest: goal, ArtifactBindings: []codingagent.ArtifactBinding{}}
	return StoreCodingPlanReviewCommand{Authority: planner.Authority, Leaves: []CodingPlanLeafWrite{leaf}, IntakeContext: intake, AgentWorkload: workload}, leaf
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

func freezeCodingAgentPlanFixture(t *testing.T, repository *Repository, job model.Job, command StoreCodingPlanReviewCommand) model.CodingPlan {
	t.Helper()
	ctx := context.Background()
	plan, err := repository.StoreCodingPlanReview(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	decided, err := repository.ApplyCodingPlanDecisions(ctx, ApplyCodingPlanDecisionsCommand{OperationID: codingPlanOperationID(t, "approve-agent-plan", job.ID), JobID: job.ID, Generation: plan.Generation, Revision: plan.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity, Decisions: []CodingPlanDecisionChange{{LeafID: plan.Leaves[0].ID, Decision: model.CodingPlanDecisionApproved}}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := repository.FreezeCodingPlan(ctx, FreezeCodingPlanCommand{OperationID: codingPlanOperationID(t, "freeze-agent-plan", job.ID), JobID: job.ID, Generation: plan.Generation, Revision: decided.Plan.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity})
	if err != nil {
		t.Fatal(err)
	}
	return frozen.Plan
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
