package queue

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresClaimWritesRejectStaleAndCrossJobAuthority(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("claim-authority-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	retiredStepID := taskGenerationStepID(t, ctx, pool, job.ID, 1)
	claimExpectedJobStep(t, ctx, repository, job.ID, retiredStepID, marker+"-retired-worker")
	retiredEvidenceID := generationClaimEvidence(t, ctx, repository, pool, job.ID, retiredStepID, marker+"-retired")
	retiredClaimID := generationClaim(t, ctx, repository, job.ID, retiredStepID, marker+" retired claim")
	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "claim-writes", "Advance claim authority to a fresh generation.")); err != nil {
		t.Fatal(err)
	}
	currentStepID := taskGenerationStepID(t, ctx, pool, job.ID, 2)
	claimExpectedJobStep(t, ctx, repository, job.ID, currentStepID, marker+"-current-worker")
	currentEvidenceID := generationClaimEvidence(t, ctx, repository, pool, job.ID, currentStepID, marker+"-current")
	currentClaimID := generationClaim(t, ctx, repository, job.ID, currentStepID, marker+" current claim")

	if _, err := repository.WriteClaims(ctx, []model.ClaimRecord{{
		JobID: job.ID, StepID: retiredStepID, Text: "Stale claim", NormalizedText: "stale claim",
		Status: claimStatusSupported, Confidence: 1,
	}}); !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatalf("retired claim write error=%v", err)
	}

	otherJob, err := repository.EnqueueJob(ctx, marker+"-other", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	otherStepID := taskGenerationStepID(t, ctx, pool, otherJob.ID, 1)
	claimExpectedJobStep(t, ctx, repository, otherJob.ID, otherStepID, marker+"-other-worker")
	otherEvidenceID := generationClaimEvidence(t, ctx, repository, pool, otherJob.ID, otherStepID, marker+"-other")

	invalidBatches := []struct {
		name     string
		supports []model.ClaimSupportRecord
		stale    bool
	}{
		{
			name: "malformed zero identity",
			supports: []model.ClaimSupportRecord{{
				ClaimID: currentClaimID, SupportScore: 1, Rationale: "Missing evidence identity.",
			}},
		},
		{
			name: "missing positive identity",
			supports: []model.ClaimSupportRecord{{
				ClaimID: currentClaimID, EvidenceID: math.MaxInt64,
				SupportScore: 1, Rationale: "Evidence does not exist.",
			}},
		},
		{
			name: "cross job evidence",
			supports: []model.ClaimSupportRecord{{
				ClaimID: currentClaimID, EvidenceID: otherEvidenceID,
				SupportScore: 1, Rationale: "Cross-job evidence is forbidden.",
			}},
		},
		{
			name: "retired evidence",
			supports: []model.ClaimSupportRecord{{
				ClaimID: currentClaimID, EvidenceID: retiredEvidenceID,
				SupportScore: 1, Rationale: "Retired evidence is stale.",
			}}, stale: true,
		},
		{
			name: "retired claim",
			supports: []model.ClaimSupportRecord{{
				ClaimID: retiredClaimID, EvidenceID: currentEvidenceID,
				SupportScore: 1, Rationale: "Retired claim is stale.",
			}}, stale: true,
		},
	}
	for _, testCase := range invalidBatches {
		t.Run(testCase.name, func(t *testing.T) {
			err := repository.WriteClaimSupports(ctx, testCase.supports)
			if err == nil {
				t.Fatal("invalid claim support was accepted")
			}
			if testCase.stale && !errors.Is(err, ErrStaleJobGeneration) {
				t.Fatalf("stale claim support error=%v", err)
			}
		})
	}

	if err := repository.WriteClaimSupports(ctx, []model.ClaimSupportRecord{
		{
			ClaimID: currentClaimID, EvidenceID: currentEvidenceID,
			SupportScore: 0.8, Rationale: "This insert must roll back with the batch.",
		},
		{
			ClaimID: currentClaimID, EvidenceID: retiredEvidenceID,
			SupportScore: 0.2, Rationale: "This retired link must fail the batch.",
		},
	}); !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatalf("partial stale batch error=%v", err)
	}
	if count := generationClaimSupportCount(t, ctx, pool, job.ID); count != 0 {
		t.Fatalf("failed support batches persisted %d rows", count)
	}
}

func TestPostgresDuplicateClaimSupportIsNotUpserted(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("claim-duplicate-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	stepID := taskGenerationStepID(t, ctx, pool, job.ID, 1)
	claimExpectedJobStep(t, ctx, repository, job.ID, stepID, marker+"-worker")
	claimID := generationClaim(t, ctx, repository, job.ID, stepID, marker+" claim")
	evidenceID := generationClaimEvidence(t, ctx, repository, pool, job.ID, stepID, marker+"-evidence")

	first := model.ClaimSupportRecord{
		ClaimID: claimID, EvidenceID: evidenceID,
		SupportScore: 0.25, Rationale: "Original support remains authoritative.",
	}
	if err := repository.WriteClaimSupports(ctx, []model.ClaimSupportRecord{first}); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SupportScore = 0.95
	second.Rationale = "A duplicate must not overwrite the original."
	if err := repository.WriteClaimSupports(ctx, []model.ClaimSupportRecord{second}); err == nil {
		t.Fatal("duplicate claim support was silently upserted")
	}

	var count int64
	var score float64
	var rationale string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MIN(support_score), MIN(rationale)
		FROM claim_support
		WHERE job_id=$1 AND claim_id=$2 AND evidence_id=$3
	`, job.ID, claimID, evidenceID).Scan(&count, &score, &rationale); err != nil {
		t.Fatal(err)
	}
	if count != 1 || score != first.SupportScore || rationale != first.Rationale {
		t.Fatalf("duplicate changed support: count=%d score=%v rationale=%q", count, score, rationale)
	}
}

func claimExpectedJobStep(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	jobID, stepID int64,
	workerID string,
) {
	t.Helper()
	claimed, err := repository.ClaimNextStep(ctx, workerID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Job.ID != jobID || claimed.Step.ID != stepID {
		t.Fatalf("claimed step=%+v, want job %d step %d", claimed, jobID, stepID)
	}
}

func generationClaim(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	jobID, stepID int64,
	text string,
) int64 {
	t.Helper()
	saved, err := repository.WriteClaims(ctx, []model.ClaimRecord{{
		JobID: jobID, StepID: stepID, Text: text, NormalizedText: text,
		Status: claimStatusSupported, Confidence: 0.75,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].ID <= 0 {
		t.Fatalf("saved claims=%+v", saved)
	}
	return saved[0].ID
}

func generationClaimEvidence(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	pool *pgxpool.Pool,
	jobID, stepID int64,
	sourceRef string,
) int64 {
	t.Helper()
	if err := repository.WriteEvidence(ctx, evidence.Record{
		JobID: jobID, StepID: stepID, Kind: evidence.KindModelJudgment,
		SourceType: "generation_authority_test", SourceRef: sourceRef, Summary: "Bounded test evidence.",
	}); err != nil {
		t.Fatal(err)
	}
	var evidenceID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM evidence
		WHERE job_id=$1 AND step_id=$2 AND source_ref=$3
		ORDER BY id DESC LIMIT 1
	`, jobID, stepID, sourceRef).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	return evidenceID
}

func generationClaimSupportCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID int64,
) int64 {
	t.Helper()
	var count int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM claim_support WHERE job_id=$1`, jobID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
