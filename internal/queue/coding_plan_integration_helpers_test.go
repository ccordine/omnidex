package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/assemblyline"
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
	return pool, New(pool, authority, model.CodingScopeModeNormal)
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
		Authority: claim.Authority, ScopeMode: model.CodingScopeModeNormal,
		RequestSHA256: assemblyline.ExactObjectiveContextSHA(job.Instruction),
		Leaves:        leaves,
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
	id, err := model.NewCodingPlanLeafID(statement)
	if err != nil {
		t.Fatalf("construct coding-plan leaf identity: %v", err)
	}
	receipt := codingPlanAcceptedReceipt(t, statement)
	return CodingPlanLeafWrite{
		Leaf: model.CodingPlanLeaf{
			ID: id, Statement: statement, Annotation: model.CodingPlanAnnotationGrounded,
			Decision: decision,
		},
		DecisionOriginGeneration: originGeneration,
		ResultRelation:           &receipt,
	}
}

func codingPlanConflictLeaf(t *testing.T, statement string, originGeneration int64) CodingPlanLeafWrite {
	t.Helper()
	write := codingPlanExecutableLeaf(t, statement, model.CodingPlanDecisionPending, originGeneration)
	write.Leaf.Annotation = model.CodingPlanAnnotationConcreteConflict
	return write
}

func codingPlanAcceptedReceipt(t *testing.T, statement string) CodingPlanResultRelationReceipt {
	t.Helper()
	candidateSHA := assemblyline.ExactObjectiveContextSHA(statement)
	kind := assemblyline.ApplicationRequirementCandidateKindResult{
		Schema:          assemblyline.ApplicationRequirementCandidateKindSchemaV1,
		CandidateSHA256: candidateSHA, Relation: assemblyline.ApplicationRequirementCandidateTaskLocal,
	}
	cardinality := assemblyline.ApplicationRequirementCandidateCardinalityResult{
		Schema:          assemblyline.ApplicationRequirementCandidateCardinalitySchemaV1,
		CandidateSHA256: candidateSHA, Relation: assemblyline.ApplicationRequirementOneRuntimeOutcome,
	}
	kindJSON, err := json.Marshal(kind)
	if err != nil {
		t.Fatalf("encode kind receipt: %v", err)
	}
	cardinalityJSON, err := json.Marshal(cardinality)
	if err != nil {
		t.Fatalf("encode cardinality receipt: %v", err)
	}
	return CodingPlanResultRelationReceipt{
		Schema:                   assemblyline.ApplicationRequirementCandidateResultRelationSchemaV1,
		CandidateSHA256:          candidateSHA,
		KindReceiptSHA256:        assemblyline.ExactObjectiveContextSHA(string(kindJSON)),
		CardinalityReceiptSHA256: assemblyline.ExactObjectiveContextSHA(string(cardinalityJSON)),
		Relation:                 assemblyline.ApplicationRequirementNoDerivedResult,
	}
}

func codingPlanOperationID(t *testing.T, label string, jobID int64) LifecycleOperationID {
	t.Helper()
	id, err := NewLifecycleOperationID(
		"coding-plan-integration", label, fmt.Sprintf("%d", jobID), codingPlanTestNonce(t),
	)
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
		left.ScopeMode != right.ScopeMode || left.RequestSHA256 != right.RequestSHA256 ||
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
