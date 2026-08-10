package cognitiongauntlet

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/labyrinth"
)

func TestPostgresRogueRequiresSealedPrerequisitesAndReplaysExactTerminalReceipt(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	generation := mustRatGeneration(t)
	prerequisites := buildRoguePrerequisites(
		t, ctx, pool, repository, hostStore, generation,
	)
	generated, err := labyrinth.GenerateExtended(labyrinth.ExtendedGeneratorConfig{
		Suite: labyrinth.SuiteRogue, Seed: 94_001,
		GeneratorVersion: labyrinth.ExtendedGeneratorVersionV1,
		GrammarVersion:   labyrinth.ExtendedGrammarVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, instruction := claimRogueStep(
		t, repository, extendedRuntimeBudget().WorkingSetBytes, "composition",
	)
	client := &extendedWitnessPolicyClient{
		witnessPolicyClient: &witnessPolicyClient{
			model:        generation.Fixed.Brain.Model,
			witness:      generated.PrivateOracle().Witness,
			evidenceUses: generated.PrivateOracle().EvidenceUses,
		},
		suite: labyrinth.SuiteRogue,
	}
	fingerprint := transferTestFingerprint()
	fingerprint.ProductionSourceSHA256 = fullCognitionTestDigest(instruction)
	request := ExtendedRuntimeRunRequest{
		Surface: SurfaceSymbolic, RatGeneration: generation,
		RuntimeFingerprint: fingerprint, Repetition: 1,
		Attempt: claim, Pool: pool, Client: client, HostStore: hostStore,
	}
	if _, err := RunExtendedRuntime(ctx, generated, request); err == nil {
		t.Fatal("ordinary extended runner accepted Rogue without prerequisite authority")
	}
	receipt, err := RunRogueRuntime(ctx, generated, request, prerequisites)
	if err != nil {
		t.Fatalf("Rogue run after %d policy decisions: %v", client.calls(), err)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	if receipt.PrerequisiteBundleSHA256 != prerequisites.BundleSHA256 ||
		receipt.RevisionTraceSHA256 == "" || receipt.PromotionEligible {
		t.Fatalf("Rogue receipt lacks sealed composition authority: %#v", receipt)
	}
	before := extendedDurableCounts(t, pool, receipt.EpisodeID)
	calls := client.calls()
	replayed, err := RunRogueRuntime(ctx, generated, request, prerequisites)
	if err != nil || !reflect.DeepEqual(replayed, receipt) || client.calls() != calls ||
		extendedDurableCounts(t, pool, receipt.EpisodeID) != before {
		t.Fatalf("Rogue replay changed receipt, inference, or durable state: %#v error=%v", replayed, err)
	}
}
