package cognitiongauntlet

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// This is contaminated world-validation machinery: the private witness policy
// proves real scale execution wiring and is never model-competence evidence.
func TestPostgresFullCognitionScaleExecutesGeneratedHundredFoldWorlds(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	base, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	sizes := []int{64, 6_400}
	generated, _, err := labyrinth.GenerateScaleFamily(base.spec.Generator, sizes)
	if err != nil {
		t.Fatal(err)
	}
	generation := mustRatGeneration(t)
	request := FullCognitionScaleRequest{WorldSizes: sizes}
	for index, size := range sizes {
		claim := claimScaleStep(t, repository, base.spec.Budget.WorkingSetBytes, index)
		episodeDirectory, evaluationDirectory := t.TempDir(), t.TempDir()
		request.Cases = append(request.Cases, FullCognitionScaleCaseRequest{
			WorldSize: size,
			Runs: []FullCognitionRunRequest{{
				Surface: SurfaceSymbolic, RatGeneration: generation,
				RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
				Attempt: claim, Pool: pool, HostStore: hostStore,
				Client: &witnessPolicyClient{
					model:        generation.Fixed.Brain.Model,
					witness:      generated[index].PrivateOracle().Witness,
					evidenceUses: generated[index].PrivateOracle().EvidenceUses,
				},
				EpisodeSealPath:     filepath.Join(episodeDirectory, "episode.json"),
				EvaluationPath:      filepath.Join(evaluationDirectory, "evaluation.json"),
				LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
				ProjectionPolicyVersion: "context-projection.v1",
			}},
		})
	}
	result, err := RunFullCognitionScale(ctx, base, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Report.GateInput.WorldMultiplier != 100 || len(result.Cases) != 2 ||
		result.Report.Measurements[1].WorldSize != 6_400 {
		t.Fatalf("actual scale report=%+v", result.Report)
	}
}

func claimScaleStep(
	t *testing.T,
	repository *queue.Repository,
	workingSetBytes int,
	index int,
) model.StepAttemptAuthority {
	t.Helper()
	ctx := t.Context()
	job, err := repository.EnqueueJob(
		ctx, fmt.Sprintf("gauntlet-scale-world-%d-%d", index, time.Now().UnixNano()),
		model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, fmt.Sprintf("gauntlet-scale-worker-%d", index))
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claimed scale step=%+v, want job %d", claim, job.ID)
	}
	budget, err := fullCognitionWorkingSetBudget(workingSetBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCurrentWorkingSet(ctx, claim.Authority, budget); err != nil {
		t.Fatal(err)
	}
	return claim.Authority
}
