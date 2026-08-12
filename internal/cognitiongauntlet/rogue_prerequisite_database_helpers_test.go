package cognitiongauntlet

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func buildRoguePrerequisites(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	hostStore *labyrinthhost.Store,
	generation RatGeneration,
) RoguePrerequisiteBundle {
	t.Helper()
	receipts := make([]RoguePrerequisiteReceipt, 0, len(roguePrerequisites()))
	for _, suite := range []labyrinth.Suite{
		labyrinth.SuiteTraverse, labyrinth.SuiteBind,
		labyrinth.SuiteRevise, labyrinth.SuiteOrder,
	} {
		run := runRogueExtendedPrerequisite(
			t, ctx, pool, repository, hostStore, generation, suite,
		)
		receipt, err := NewRogueExtendedPrerequisite(run)
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, receipt)
	}
	resume, err := NewRogueResumePrerequisite(
		runRogueResumePrerequisite(t, ctx, pool, repository, hostStore, generation),
	)
	if err != nil {
		t.Fatal(err)
	}
	receipts = append(receipts, resume)
	scale, err := NewRogueScalePrerequisite(
		runRogueScalePrerequisite(t, ctx, pool, repository, hostStore, generation),
	)
	if err != nil {
		t.Fatal(err)
	}
	receipts = append(receipts, scale)
	transfer, err := NewRogueTransferPrerequisite(
		runRogueTransferPrerequisite(t, ctx, pool, repository, hostStore, generation),
	)
	if err != nil {
		t.Fatal(err)
	}
	receipts = append(receipts, transfer)
	bundle, err := NewRoguePrerequisiteBundle(receipts)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func runRogueResumePrerequisite(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	hostStore *labyrinthhost.Store,
	generation RatGeneration,
) FullCognitionRunResult {
	t.Helper()
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	claim := claimFailureStep(t, repository, fixture.Spec().Budget.WorkingSetBytes)
	client := &witnessPolicyClient{
		model:        generation.Fixed.Brain.Model,
		witness:      fixture.generated.PrivateOracle().Witness,
		evidenceUses: fixture.generated.PrivateOracle().EvidenceUses,
	}
	request := failureRunRequest(t, pool, hostStore, claim, generation, client)
	request.RestartAfterCycles = []uint32{3}
	result, err := RunFullCognition(ctx, fixture, request)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runRogueScalePrerequisite(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	hostStore *labyrinthhost.Store,
	generation RatGeneration,
) FullCognitionScaleResult {
	t.Helper()
	base, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	sizes := []int{40, 4_000}
	generated, _, err := labyrinth.GenerateScaleFamily(base.spec.Generator, sizes)
	if err != nil {
		t.Fatal(err)
	}
	request := FullCognitionScaleRequest{WorldSizes: sizes}
	for index, size := range sizes {
		claim := claimScaleStep(t, repository, base.spec.Budget.WorkingSetBytes, index+10)
		run := rogueFullCognitionRequest(t, pool, hostStore, claim, generation, SurfaceSymbolic)
		run.Client = &witnessPolicyClient{
			model:        generation.Fixed.Brain.Model,
			witness:      generated[index].PrivateOracle().Witness,
			evidenceUses: generated[index].PrivateOracle().EvidenceUses,
		}
		request.Cases = append(request.Cases, FullCognitionScaleCaseRequest{
			WorldSize: size, Runs: []FullCognitionRunRequest{run},
		})
	}
	result, err := RunFullCognitionScale(ctx, base, request)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func rogueFullCognitionRequest(
	t *testing.T,
	pool *pgxpool.Pool,
	hostStore *labyrinthhost.Store,
	claim model.StepAttemptAuthority,
	generation RatGeneration,
	surface Surface,
) FullCognitionRunRequest {
	t.Helper()
	return FullCognitionRunRequest{
		Surface: surface, RatGeneration: generation,
		RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
		Attempt: claim, Pool: pool, HostStore: hostStore,
		EpisodeSealPath:     filepath.Join(t.TempDir(), "episode.json"),
		EvaluationPath:      filepath.Join(t.TempDir(), "evaluation.json"),
		LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "context-projection.v1",
	}
}
