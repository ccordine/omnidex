package cognitiongauntlet

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runRogueExtendedPrerequisite(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	hostStore *labyrinthhost.Store,
	generation RatGeneration,
	suite labyrinth.Suite,
) ExtendedRuntimeReceipt {
	t.Helper()
	generated, err := labyrinth.GenerateExtended(labyrinth.ExtendedGeneratorConfig{
		Suite: suite, Seed: 93_000 + uint64(len(suite)),
		GeneratorVersion: labyrinth.ExtendedGeneratorVersionV1,
		GrammarVersion:   labyrinth.ExtendedGrammarVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, instruction := claimRogueStep(t, repository, extendedRuntimeBudget().WorkingSetBytes, string(suite))
	fingerprint := transferTestFingerprint()
	fingerprint.ProductionSourceSHA256 = fullCognitionTestDigest(instruction)
	result, err := RunExtendedRuntime(ctx, generated, ExtendedRuntimeRunRequest{
		Surface: SurfaceSymbolic, RatGeneration: generation,
		RuntimeFingerprint: fingerprint, Repetition: 1,
		Attempt: claim, Pool: pool, HostStore: hostStore,
		Client: &extendedWitnessPolicyClient{
			witnessPolicyClient: &witnessPolicyClient{
				model:        generation.Fixed.Brain.Model,
				witness:      generated.PrivateOracle().Witness,
				evidenceUses: generated.PrivateOracle().EvidenceUses,
			},
			suite: suite,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runRogueTransferPrerequisite(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	hostStore *labyrinthhost.Store,
	generation RatGeneration,
) FullCognitionTransferResult {
	t.Helper()
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	surfaces := []Surface{SurfaceFilesystem, SurfaceRecord}
	requests := make([]FullCognitionRunRequest, len(surfaces))
	for index, surface := range surfaces {
		claim := claimScaleStep(t, repository, fixture.spec.Budget.WorkingSetBytes, index+20)
		requests[index] = rogueFullCognitionRequest(t, pool, hostStore, claim, generation, surface)
		requests[index].Client = &witnessPolicyClient{
			model:        generation.Fixed.Brain.Model,
			witness:      fixture.generated.PrivateOracle().Witness,
			evidenceUses: fixture.generated.PrivateOracle().EvidenceUses,
		}
	}
	result, err := RunFullCognitionTransfer(ctx, fixture, surfaces, requests)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func claimRogueStep(
	t *testing.T,
	repository *queue.Repository,
	workingSetBytes int,
	label string,
) (model.StepAttemptAuthority, string) {
	t.Helper()
	instruction := fmt.Sprintf("Rogue prerequisite %s %d", label, time.Now().UnixNano())
	job, err := repository.EnqueueJob(
		t.Context(), instruction, model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), fmt.Sprintf("rogue-worker-%d", job.ID))
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v error=%v", claim, err)
	}
	budget, err := fullCognitionWorkingSetBudget(workingSetBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCurrentWorkingSet(t.Context(), claim.Authority, budget); err != nil {
		t.Fatal(err)
	}
	return claim.Authority, instruction
}
