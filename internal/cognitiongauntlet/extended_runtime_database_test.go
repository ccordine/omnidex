package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresExtendedSuitesUseProductionRuntimeAndReplayExactSeal(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	for _, suite := range []labyrinth.Suite{
		labyrinth.SuiteTraverse, labyrinth.SuiteBind,
		labyrinth.SuiteRevise, labyrinth.SuiteOrder,
	} {
		suite := suite
		t.Run(string(suite), func(t *testing.T) {
			generated, err := labyrinth.GenerateExtended(labyrinth.ExtendedGeneratorConfig{
				Suite: suite, Seed: 91_000 + uint64(len(suite)),
				GeneratorVersion: labyrinth.ExtendedGeneratorVersionV1,
				GrammarVersion:   labyrinth.ExtendedGrammarVersionV1,
			})
			if err != nil {
				t.Fatal(err)
			}
			job, err := repository.EnqueueJob(
				ctx, fmt.Sprintf("extended-%s-%d", suite, time.Now().UnixNano()),
				model.PipelineAssistant, []byte(`{}`),
			)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, fmt.Sprintf("extended-worker-%d", job.ID))
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v error=%v", claim, err)
			}
			setBudget, err := fullCognitionWorkingSetBudget(extendedRuntimeBudget().WorkingSetBytes)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repository.CreateCurrentWorkingSet(ctx, claim.Authority, setBudget); err != nil {
				t.Fatal(err)
			}
			generation := mustRatGeneration(t)
			client := &extendedWitnessPolicyClient{
				witnessPolicyClient: &witnessPolicyClient{
					model:        generation.Fixed.Brain.Model,
					witness:      generated.PrivateOracle().Witness,
					evidenceUses: generated.PrivateOracle().EvidenceUses,
				},
				suite: suite,
			}
			fingerprint := transferTestFingerprint()
			fingerprint.ProductionSourceSHA256 = fullCognitionTestDigest(job.Instruction)
			request := ExtendedRuntimeRunRequest{
				Surface: SurfaceSymbolic, RatGeneration: generation,
				RuntimeFingerprint: fingerprint, Repetition: 1,
				Attempt: claim.Authority, Pool: pool, Client: client, HostStore: hostStore,
			}
			receipt, err := RunExtendedRuntime(ctx, generated, request)
			if err != nil {
				t.Fatalf("run after %d policy decisions: %v", client.calls(), err)
			}
			if err := receipt.Validate(); err != nil {
				t.Fatal(err)
			}
			if receipt.PromotionEligible || receipt.EvidenceClass != ExtendedEvidenceStructuralWitness {
				t.Fatalf("structural witness was misclassified: %#v", receipt)
			}
			before := extendedDurableCounts(t, pool, receipt.EpisodeID)
			calls := client.calls()
			replayed, err := RunExtendedRuntime(ctx, generated, request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(replayed, receipt) || client.calls() != calls ||
				extendedDurableCounts(t, pool, receipt.EpisodeID) != before {
				t.Fatalf("terminal replay changed receipt, inference, or durable state")
			}
		})
	}
}

func TestPostgresExtendedRuntimeSealsRegisteredPolicyFailureAndReplays(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	generated, err := labyrinth.GenerateExtended(labyrinth.ExtendedGeneratorConfig{
		Suite: labyrinth.SuiteTraverse, Seed: 92_001,
		GeneratorVersion: labyrinth.ExtendedGeneratorVersionV1,
		GrammarVersion:   labyrinth.ExtendedGrammarVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		ctx, fmt.Sprintf("extended-failure-%d", time.Now().UnixNano()),
		model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, fmt.Sprintf("extended-failure-worker-%d", job.ID))
	if err != nil || claim == nil {
		t.Fatalf("claim=%#v error=%v", claim, err)
	}
	budget, err := fullCognitionWorkingSetBudget(extendedRuntimeBudget().WorkingSetBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCurrentWorkingSet(ctx, claim.Authority, budget); err != nil {
		t.Fatal(err)
	}
	generation := mustRatGeneration(t)
	client := &terminalPolicyClient{
		witnessPolicyClient: &witnessPolicyClient{model: generation.Fixed.Brain.Model},
		response:            `{"invalid":true}`,
	}
	fingerprint := transferTestFingerprint()
	fingerprint.ProductionSourceSHA256 = fullCognitionTestDigest(job.Instruction)
	request := ExtendedRuntimeRunRequest{
		Surface: SurfaceSymbolic, RatGeneration: generation,
		RuntimeFingerprint: fingerprint, Repetition: 1,
		Attempt: claim.Authority, Pool: pool, Client: client, HostStore: hostStore,
	}
	receipt, err := RunExtendedRuntime(ctx, generated, request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Seal.Outcome != queue.CognitionEpisodeCanceled ||
		receipt.CancellationCode != cognitionruntime.CancellationPolicyFailure ||
		receipt.PolicyCalls != 1 || receipt.EnvironmentActions != 0 ||
		receipt.PromotionEligible || receipt.EvidenceClass != ExtendedEvidenceInProcessRuntime {
		t.Fatalf("canceled receipt=%#v", receipt)
	}
	before := extendedDurableCounts(t, pool, receipt.EpisodeID)
	calls := client.calls()
	replayed, err := RunExtendedRuntime(ctx, generated, request)
	if err != nil || !reflect.DeepEqual(replayed, receipt) || client.calls() != calls ||
		extendedDurableCounts(t, pool, receipt.EpisodeID) != before {
		t.Fatalf("canceled replay=%#v error=%v", replayed, err)
	}
}

type extendedCounts struct{ actions, calls, seals int }

func extendedDurableCounts(
	t *testing.T,
	pool *pgxpool.Pool,
	episode string,
) extendedCounts {
	t.Helper()
	var value extendedCounts
	if err := pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=$1)
	`, episode).Scan(&value.actions, &value.calls, &value.seals); err != nil {
		t.Fatal(err)
	}
	return value
}
