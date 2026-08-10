package cognitiongauntlet

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPublicAblationRunnerExecutesSealsAndEvaluatesRegisteredVariants(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []Variant{
		VariantRawObservation, VariantLedgerProjection, VariantOracleEvidence, VariantRawShell,
	} {
		t.Run(string(variant), func(t *testing.T) {
			surface := SurfaceSymbolic
			if variant == VariantRawShell {
				surface = SurfaceFilesystem
			}
			generation := mustRatGeneration(t)
			paired, err := fixture.PairedAuthority(surface, generation, 1, transferTestFingerprint())
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := NewVariantPublicInferenceBundle(fixture, paired, variant)
			if err != nil {
				t.Fatal(err)
			}
			episode, err := PublicVariantEpisodeRef(bundle.Authority)
			if err != nil {
				t.Fatal(err)
			}
			actor := cognition.AttemptRef{
				JobID: 601, Generation: 2, StepID: 17, Attempt: 1,
				WorkerID: "isolated-variant-runner",
			}
			environment, closeEnvironment, err := newBenchmarkEnvironment(
				fixture, episode, actor, surface,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if closeErr := closeEnvironment(); closeErr != nil {
					t.Errorf("close public ablation environment: %v", closeErr)
				}
			}()
			oracle := fixture.generated.PrivateOracle()
			request := PublicAblationRunRequest{
				Actor: actor,
				Client: &witnessPolicyClient{
					model: generation.Fixed.Brain.Model, witness: oracle.Witness,
					evidenceUses: oracle.EvidenceUses,
				},
				Environment:         environment,
				Completion:          localRuntimeCompletion{evaluator: environment.(ablationGoalEvaluator)},
				EpisodeSealPath:     filepath.Join(t.TempDir(), "episode.json"),
				LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
				ProjectionPolicyVersion: ablationProjectionPolicyVersionV1,
			}
			if variant == VariantOracleEvidence {
				request.ContaminatedOracle = &oracle
			}
			public, err := RunPublicAblation(t.Context(), bundle, request)
			if err != nil {
				t.Fatal(err)
			}
			result, err := EvaluatePublicAblation(
				filepath.Join(t.TempDir(), "evaluation.json"), fixture, paired,
				bundle.Authority, public.Episode,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Variant.Variant != variant || result.Episode.Manifest.Resources.ModelCalls == 0 {
				t.Fatalf("public ablation result=%+v", result)
			}
			if result.PromotionEligible || result.EvidenceClass == AblationIsolatedEvidence {
				t.Fatalf("direct in-process evaluation claimed serious isolation: %+v", result)
			}
		})
	}
}

type localRuntimeCompletion struct {
	evaluator ablationGoalEvaluator
}

func (authority localRuntimeCompletion) Evaluate(
	ctx context.Context,
	request cognitionruntime.CompletionRequest,
) (cognition.CompletionResult, error) {
	satisfied, err := authority.evaluator.EvaluateGoal(
		ctx, request.Binding.Episode, request.Revision, request.Obligation.Desired,
	)
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	outcome := cognition.CompletionUnsatisfied
	evidence := []cognition.EvidenceRef{}
	if satisfied {
		outcome = cognition.CompletionSatisfied
		evidence = append(evidence, request.EvidenceRefs...)
	}
	return cognition.NewCompletionResult(
		request.Obligation.ID, request.Obligation.CompletionCheck,
		request.Revision, outcome, evidence,
	)
}
