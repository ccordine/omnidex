package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

var (
	_ int64  = model.Job{}.CurrentGeneration
	_ int64  = model.Step{}.Generation
	_ *int64 = model.Step{}.SupersededAtGeneration
)

func TestGenerationProjectionShapesMatchSharedScanners(t *testing.T) {
	jobProjection := "SELECT id, instruction, pipeline, status, result, error, metadata, current_generation, created_at, updated_at, completed_at"
	stepSelectProjection := "SELECT id, job_id, action, sort_index, status, generation, superseded_at_generation, worker_id, output, error, started_at, finished_at, created_at, updated_at"
	stepReturningProjection := "RETURNING id, job_id, action, sort_index, status, generation, superseded_at_generation, worker_id, output, error, started_at, finished_at, created_at, updated_at"
	claimProjection := "s.id, s.job_id, s.action, s.sort_index, s.status, s.generation, s.superseded_at_generation, s.worker_id, s.output, s.error, s.started_at, s.finished_at, s.created_at, s.updated_at, j.id, j.instruction, j.pipeline, j.status, j.result, j.error, j.metadata, j.current_generation, j.created_at, j.updated_at, j.completed_at"

	jobCallFiles := map[string]int{
		"repository_job_lock.go":      1,
		"repository_cancel.go":        1,
		"repository_replan_commit.go": 1,
		"repository_step_input.go":    1,
		"repository_claims.go":        3,
	}
	for path, expected := range jobCallFiles {
		source := normalizedGenerationSource(t, path)
		if calls := strings.Count(source, "scanJob("); calls != expected {
			t.Fatalf("%s has %d scanJob call sites; expected %d", path, calls, expected)
		}
		if projections := strings.Count(source, jobProjection); projections != expected {
			t.Fatalf("%s has %d complete job projections; expected %d", path, projections, expected)
		}
	}

	claimsSource := normalizedGenerationSource(t, "repository_claims.go")
	stepClaimSource := normalizedGenerationSource(t, "repository_step_claim.go")
	delegatedSource := normalizedGenerationSource(t, "repository_delegated_steps.go")
	stepSources := claimsSource + " " + delegatedSource
	if calls := strings.Count(stepSources, "scanStep("); calls != 2 {
		t.Fatalf("queue claim/delegation writers have %d scanStep call sites; expected 2", calls)
	}
	if projections := strings.Count(stepSources, stepSelectProjection) + strings.Count(stepSources, stepReturningProjection); projections != 2 {
		t.Fatalf("queue claim/delegation writers have %d complete step projections; expected 2", projections)
	}
	if calls := strings.Count(stepClaimSource, "scanClaim("); calls != 1 {
		t.Fatalf("repository_step_claim.go has %d scanClaim call sites; expected 1", calls)
	}
	if projections := strings.Count(stepClaimSource, claimProjection); projections != 1 {
		t.Fatalf("repository_step_claim.go has %d complete claim projections; expected 1", projections)
	}

	scanSource := normalizedGenerationSource(t, "repository_scan.go")
	for required, expected := range map[string]int{
		"&job.Metadata, &job.CurrentGeneration, &job.CreatedAt":                     2,
		"&step.Status, &step.Generation, &step.SupersededAtGeneration, &workerID":   1,
		"&step.Status, &step.Generation, &step.SupersededAtGeneration, &stepWorker": 1,
	} {
		if count := strings.Count(scanSource, required); count != expected {
			t.Fatalf("generation scanner projection contains %q %d times; expected %d", required, count, expected)
		}
	}
}

func normalizedGenerationSource(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(raw)), " ")
}
