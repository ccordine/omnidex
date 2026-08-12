package cognitiongauntlet

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestExtendedScenarioUsesTheSamePublicRunAndPrivateEvaluatorPath(t *testing.T) {
	generation := mustRatGeneration(t)
	spec, err := ResolveOfflineScenarioSpecV1(
		SuiteTraverse, 94_001, offlineExecutableScenarioTestBudget(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := generateOfflineScenario(spec)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := generated.pairedAuthority(
		SurfaceSymbolic, generation, 1, transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := newScenarioPublicInferenceBundle(
		generated.scenario, paired, VariantOracleEvidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		t.Fatal(err)
	}
	actor := cognition.AttemptRef{
		JobID: 801, Generation: 2, StepID: 23, Attempt: 1,
		WorkerID: "extended-public-runner",
	}
	environment, closeEnvironment, err := newScenarioEnvironmentWithAuthorizer(
		generated.scenario, episode,
		func(_ context.Context, candidate cognition.AttemptRef) error {
			if candidate != actor {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
		SurfaceSymbolic,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closeEnvironment(); err != nil {
			t.Error(err)
		}
	})
	oracle := generated.extended.PrivateOracle()
	packet, err := contaminatedEvidenceFor(generated)
	if err != nil {
		t.Fatal(err)
	}
	episodeDirectory, evidenceDirectory := t.TempDir(), t.TempDir()
	public, err := RunPublicAblation(t.Context(), bundle, PublicAblationRunRequest{
		Actor: actor,
		Client: &witnessPolicyClient{
			model: generation.Fixed.Brain.Model, witness: oracle.Witness,
			evidenceUses: oracle.EvidenceUses,
		},
		Environment:             environment,
		Completion:              localRuntimeCompletion{evaluator: environment.(ablationGoalEvaluator)},
		ContaminatedEvidence:    &packet,
		EpisodeSealPath:         filepath.Join(episodeDirectory, "episode.json"),
		EvidenceSealPath:        filepath.Join(evidenceDirectory, "ablation-evidence.json"),
		LedgerSchemaVersion:     "task-ledger.v1",
		WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: ablationProjectionPolicyVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluateGeneratedPublicAblation(
		filepath.Join(t.TempDir(), "evaluation.json"), generated, paired,
		bundle.Authority, public.Episode,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Evaluation.GoalSuccess || result.Authority.Suite != SuiteTraverse ||
		result.Evaluation.TaskArchetype != oracle.TaskArchetype {
		t.Fatalf("extended public/evaluator result=%+v", result)
	}
	if result.CausalAcquisition.RequiredEvidence != 0 ||
		result.CausalAcquisition.AcquiredEvidence != 0 ||
		result.CausalAcquisition.AcquisitionTraceRefs == nil {
		t.Fatalf("zero-evidence traverse causal report=%+v", result.CausalAcquisition)
	}
}
