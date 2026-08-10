package cognitiongauntlet

import (
	"path/filepath"
	"testing"

	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFullCognitionScalePreparesRealHundredFoldWorlds(t *testing.T) {
	base, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	request := scaleValidationRequest(t, []int{64, 6_400})
	fixtures, family, authority, err := prepareFullCognitionScale(base, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 2 || family.Cases[1].WorldSize != 6_400 ||
		authority.RelevantSurfaceSHA256 != family.RelevantSurfaceSHA256 {
		t.Fatalf("scale family=%+v authority=%+v", family, authority)
	}
	baseManifest, err := fixtures[0].PublicManifest(SurfaceSymbolic)
	if err != nil {
		t.Fatal(err)
	}
	largeManifest, err := fixtures[1].PublicManifest(SurfaceSymbolic)
	if err != nil {
		t.Fatal(err)
	}
	if baseManifest.Difficulty.WorldSize != 64 || largeManifest.Difficulty.WorldSize != 6_400 {
		t.Fatalf("public scale sizes=%d,%d", baseManifest.Difficulty.WorldSize, largeManifest.Difficulty.WorldSize)
	}
	baseBytes, err := labyrinthRelevantSurfaceBytes(fixtures[0])
	if err != nil {
		t.Fatal(err)
	}
	largeBytes, err := labyrinthRelevantSurfaceBytes(fixtures[1])
	if err != nil {
		t.Fatal(err)
	}
	if baseBytes != largeBytes {
		t.Fatalf("relevant surface changed from %d to %d bytes", baseBytes, largeBytes)
	}
}

func TestFullCognitionScaleRejectsChangedRatOrCallerWorldSize(t *testing.T) {
	base, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	request := scaleValidationRequest(t, []int{64, 6_400})
	request.Cases[1].WorldSize++
	if _, _, _, err := prepareFullCognitionScale(base, request); err == nil {
		t.Fatal("scale runner accepted a caller world size that differs from the generated family")
	}
	request = scaleValidationRequest(t, []int{64, 6_400})
	request.Cases[1].Runs[0].RatGeneration.Runtime.Version = "changed-runtime.v1"
	if _, _, _, err := prepareFullCognitionScale(base, request); err == nil {
		t.Fatal("scale runner accepted a changed Rat generation")
	}
}

func scaleValidationRequest(t *testing.T, sizes []int) FullCognitionScaleRequest {
	t.Helper()
	generation := mustRatGeneration(t)
	result := FullCognitionScaleRequest{WorldSizes: append([]int(nil), sizes...)}
	for index, size := range sizes {
		episodeDirectory, evaluationDirectory := t.TempDir(), t.TempDir()
		result.Cases = append(result.Cases, FullCognitionScaleCaseRequest{
			WorldSize: size,
			Runs: []FullCognitionRunRequest{{
				Surface: SurfaceSymbolic, RatGeneration: generation,
				RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
				Pool: &pgxpool.Pool{}, Client: &witnessPolicyClient{}, HostStore: &labyrinthhost.Store{},
				Attempt: model.StepAttemptAuthority{
					JobID: int64(index + 1), Generation: 1, StepID: int64(index + 1),
					Attempt: 1, WorkerID: "scale-validation-worker",
				},
				EpisodeSealPath:     filepath.Join(episodeDirectory, "episode.json"),
				EvaluationPath:      filepath.Join(evaluationDirectory, "evaluation.json"),
				LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
				ProjectionPolicyVersion: "context-projection.v1",
			}},
		})
	}
	return result
}
