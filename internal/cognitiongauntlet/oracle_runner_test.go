package cognitiongauntlet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestDeterministicOracleBaselineSealsAllFiveMicrogauntlets(t *testing.T) {
	fixtures, err := GenerateInitialMicrogauntletsV1()
	if err != nil {
		t.Fatal(err)
	}
	for index, fixture := range fixtures {
		t.Run(string(fixture.Spec().Generator.Suite), func(t *testing.T) {
			request := oracleTestRequest(t, SurfaceSymbolic, index+1)
			result, err := RunOracleBaseline(context.Background(), fixture, request)
			if err != nil {
				t.Fatal(err)
			}
			if err := result.Validate(); err != nil {
				t.Fatal(err)
			}
			loadedEpisode, err := LoadSealedEpisode(request.EpisodeSealPath)
			if err != nil {
				t.Fatal(err)
			}
			loadedEvaluation, err := LoadEvaluation(request.EvaluationPath)
			if err != nil {
				t.Fatal(err)
			}
			if loadedEpisode.SealSHA256 != result.Episode.SealSHA256 ||
				loadedEvaluation.EpisodeSealSHA256 != result.Episode.SealSHA256 ||
				loadedEvaluation.CausalAcquisition == nil {
				t.Fatal("oracle result files do not bind the returned episode")
			}
			if result.Episode.Manifest.Resources.EnvironmentActions !=
				fixture.Spec().Generator.Difficulty.SolutionDepth {
				t.Fatalf("actions=%d depth=%d",
					result.Episode.Manifest.Resources.EnvironmentActions,
					fixture.Spec().Generator.Difficulty.SolutionDepth)
			}
			if result.Episode.Manifest.Resources.ModelCalls != 0 {
				t.Fatal("deterministic oracle baseline claimed a model call")
			}
			if result.Purpose != BaselineWorldValidation {
				t.Fatal("deterministic oracle baseline was presented as model competence")
			}
			if result.CausalAcquisition.AcquiredEvidence !=
				fixture.generated.PublicArtifact().World.Descriptor.Difficulty.EvidenceArtifacts {
				t.Fatalf("causal acquisition=%+v", result.CausalAcquisition)
			}
		})
	}
}

func TestOracleBaselineExecutesSameCombinedCaseAcrossThreeSurfaces(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	var referenceCost int64
	for index, surface := range []Surface{SurfaceSymbolic, SurfaceFilesystem, SurfaceRecord} {
		request := oracleTestRequest(t, surface, index+1)
		result, err := RunOracleBaseline(context.Background(), fixture, request)
		if err != nil {
			t.Fatalf("surface %s: %v", surface, err)
		}
		if index == 0 {
			referenceCost = result.Evaluation.ActualDecisionCost
		} else if result.Evaluation.ActualDecisionCost != referenceCost {
			t.Fatalf("surface %s cost=%d want=%d", surface, result.Evaluation.ActualDecisionCost, referenceCost)
		}
		if result.Authority.SurfaceVersion == "" || !result.Evaluation.GoalSuccess {
			t.Fatalf("surface %s result=%+v", surface, result)
		}
		if result.CausalAcquisition.SurfaceVersion != result.Authority.SurfaceVersion {
			t.Fatalf("surface %s causal report=%+v", surface, result.CausalAcquisition)
		}
	}
}

func TestOracleBaselineRejectsUnsealedOrAmbiguousRunAuthority(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	request := oracleTestRequest(t, SurfaceSymbolic, 1)
	request.EvaluationPath = request.EpisodeSealPath
	if _, err := RunOracleBaseline(context.Background(), fixture, request); err == nil {
		t.Fatal("oracle baseline accepted one path for public trace and private evaluation")
	}
	if _, err := RunOracleBaseline(nil, fixture, oracleTestRequest(t, SurfaceSymbolic, 1)); err == nil {
		t.Fatal("oracle baseline accepted a nil context")
	}
}

func TestOracleBaselineResultRejectsChangedPrivateGeneratorAuthority(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunOracleBaseline(
		context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	result.Oracle.Seed++
	if err := result.Validate(); err == nil {
		t.Fatal("oracle baseline result accepted changed private seed metadata")
	}
	result, err = RunOracleBaseline(
		context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 2),
	)
	if err != nil {
		t.Fatal(err)
	}
	result.CausalAcquisition.AcquiredEvidence--
	if err := result.Validate(); err == nil {
		t.Fatal("oracle baseline result accepted incomplete causal acquisition")
	}
}

func TestVariantEpisodeCannotBeReboundToChangedRunAuthority(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunOracleBaseline(
		context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	changed := result.Authority
	changed.Seed++
	if _, err := BindVariantResult(
		changed, VariantDeterministicOracle, result.Episode, result.Evaluation,
	); err == nil {
		t.Fatal("sealed episode was rebound to a different seed authority")
	}
}

func oracleTestRequest(t *testing.T, surface Surface, repetition int) OracleRunRequest {
	t.Helper()
	root := t.TempDir()
	episodeDirectory := filepath.Join(root, "episode")
	evaluationDirectory := filepath.Join(root, "evaluation")
	if err := os.Mkdir(episodeDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(evaluationDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	return OracleRunRequest{
		Surface: surface, RatGeneration: mustRatGeneration(t),
		RuntimeFingerprint: transferTestFingerprint(), Repetition: repetition,
		Actor: cognition.AttemptRef{
			JobID: 91, Generation: 3, StepID: 7, Attempt: 2, WorkerID: "oracle-runner",
		},
		EpisodeSealPath:         filepath.Join(episodeDirectory, "episode.json"),
		EvaluationPath:          filepath.Join(evaluationDirectory, "evaluation.json"),
		LedgerSchemaVersion:     "task-ledger.v1",
		WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "context-projection.v1",
	}
}
