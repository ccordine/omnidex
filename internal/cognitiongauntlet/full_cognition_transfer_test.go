package cognitiongauntlet

import (
	"path/filepath"
	"strings"
	"testing"

	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFullCognitionTransferFreezesRuntimeAcrossDistinctSurfaces(t *testing.T) {
	generation := mustRatGeneration(t)
	requests := transferValidationRequests(t, generation)
	if err := validateFullCognitionTransferRequests(
		[]Surface{SurfaceFilesystem, SurfaceRecord}, requests,
	); err != nil {
		t.Fatal(err)
	}
	requests[1].RuntimeFingerprint.PromptSHA256 = strings.Repeat("f", 64)
	if err := validateFullCognitionTransferRequests(
		[]Surface{SurfaceFilesystem, SurfaceRecord}, requests,
	); err == nil {
		t.Fatal("full cognition transfer accepted a changed prompt authority")
	}
}

func TestFullCognitionTransferRejectsDuplicateOrMissingSurface(t *testing.T) {
	generation := mustRatGeneration(t)
	requests := transferValidationRequests(t, generation)
	requests[1].Surface = SurfaceFilesystem
	if err := validateFullCognitionTransferRequests(
		[]Surface{SurfaceFilesystem, SurfaceRecord}, requests,
	); err == nil {
		t.Fatal("full cognition transfer accepted a duplicate surface")
	}
	if err := validateFullCognitionTransferRequests(
		[]Surface{SurfaceFilesystem, SurfaceRecord}, requests[:1],
	); err == nil {
		t.Fatal("full cognition transfer accepted an omitted surface")
	}
}

func transferValidationRequests(t *testing.T, generation RatGeneration) []FullCognitionRunRequest {
	t.Helper()
	requests := make([]FullCognitionRunRequest, 2)
	for index, surface := range []Surface{SurfaceFilesystem, SurfaceRecord} {
		episodeDirectory, evaluationDirectory := t.TempDir(), t.TempDir()
		requests[index] = FullCognitionRunRequest{
			Surface: surface, RatGeneration: generation,
			RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
			Pool: &pgxpool.Pool{}, Client: &witnessPolicyClient{}, HostStore: &labyrinthhost.Store{},
			Attempt: model.StepAttemptAuthority{
				JobID: int64(index + 1), Generation: 1, StepID: int64(index + 1),
				Attempt: 1, WorkerID: "transfer-validation-worker",
			},
			EpisodeSealPath:         filepath.Join(episodeDirectory, "episode.json"),
			EvaluationPath:          filepath.Join(evaluationDirectory, "evaluation.json"),
			LedgerSchemaVersion:     "task-ledger.v1",
			WorkingSetPolicyVersion: "working-set.v1",
			ProjectionPolicyVersion: "context-projection.v1",
		}
	}
	return requests
}
