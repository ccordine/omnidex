package cognitiongauntlet

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestAllDeclaredNonFullAblationsExecuteAndSealRealEvidence(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	variants := []Variant{
		VariantRawObservation, VariantFullTranscript, VariantTranscriptCompacted,
		VariantTaskLedger, VariantLedgerWorkingSet, VariantLedgerProjection,
		VariantOracleEvidence, VariantRawShell,
	}
	results := make(map[Variant]AblationRunResult, len(variants))
	for _, variant := range variants {
		variant := variant
		t.Run(string(variant), func(t *testing.T) {
			surface := SurfaceSymbolic
			if variant == VariantRawShell {
				surface = SurfaceFilesystem
			}
			oracle := fixture.generated.PrivateOracle()
			client := &witnessPolicyClient{
				model:   mustRatGeneration(t).Fixed.Brain.Model,
				witness: oracle.Witness, evidenceUses: oracle.EvidenceUses,
			}
			request := ablationTestRequest(t, variant, surface, 1, client)
			result, err := RunAblation(context.Background(), fixture, request)
			if err != nil {
				t.Fatal(err)
			}
			if err := result.Validate(); err != nil {
				t.Fatal(err)
			}
			projections, calls, actions := 0, 0, 0
			for _, entry := range result.Episode.Manifest.Trace {
				switch entry.Kind {
				case TraceProjection:
					projections++
				case TraceModelCall:
					calls++
				case TraceAction:
					actions++
				}
			}
			if calls == 0 || projections != calls || actions == 0 ||
				result.Episode.Manifest.Resources.ModelCalls != calls ||
				result.Episode.Manifest.Resources.EnvironmentActions != actions {
				t.Fatalf("variant %s emitted projection/call/action=%d/%d/%d resources=%+v",
					variant, projections, calls, actions, result.Episode.Manifest.Resources)
			}
			if result.PromotionEligible {
				t.Fatal("in-process ablation was marked promotion eligible")
			}
			if variant == VariantOracleEvidence && result.EvidenceClass != AblationOracleContaminated {
				t.Fatal("oracle-evidence runner omitted its contamination label")
			}
			assertNeutralAblationPrompts(t, variant, client.renderedPrompts())
			results[variant] = result
		})
	}
	left, right := results[VariantRawObservation].Variant, results[VariantFullTranscript].Variant
	if err := RequirePairedVariants(left, right); err != nil {
		t.Fatalf("same seed/brain/budgets were not paired: %v", err)
	}
}

func assertNeutralAblationPrompts(t *testing.T, variant Variant, prompts []string) {
	t.Helper()
	if len(prompts) == 0 {
		t.Fatal("variant emitted no exact rendered prompts")
	}
	for index, prompt := range prompts {
		lower := strings.ToLower(prompt)
		for _, forbidden := range []string{
			"benchmark", "gauntlet", "oracle", "witness", "suite", "private",
			"ablation", "task-ledger", "ledger-working-set", "oracle-evidence", "raw-shell",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("variant %s prompt %d exposes control-plane label %q", variant, index+1, forbidden)
			}
		}
		if variant == VariantOracleEvidence && !strings.Contains(lower, "contaminated") {
			t.Fatalf("oracle ceiling prompt %d omitted its explicit contamination marker", index+1)
		}
	}
}

func TestRawShellRoundTripsEveryRegisteredWitnessRequest(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	catalog := fixture.SealedEnvironmentScenario().Catalog()
	for _, witness := range fixture.generated.PrivateOracle().Witness {
		command, err := rawShellCommand(witness.Request)
		if err != nil {
			t.Fatalf("render %s: %v", witness.ID, err)
		}
		shell, err := cognition.NewActionRequest("shell", []cognition.ActionArgument{{
			Name: rawShellArgument, Value: command,
		}})
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := parseRawShellDecision(shell, catalog)
		if err != nil {
			t.Fatalf("parse %s: %v", witness.ID, err)
		}
		if !reflect.DeepEqual(decoded, witness.Request) {
			t.Fatalf("request %s round trip changed\nwant=%#v\n got=%#v", witness.ID, witness.Request, decoded)
		}
	}
}

func TestAblationRunnerRejectsMetadataOnlyAndUnsafeShellVariants(t *testing.T) {
	request := ablationTestRequest(t, VariantRawObservation, SurfaceSymbolic, 1, &witnessPolicyClient{})
	request.Variant = VariantFullCognition
	if err := request.Validate(); err == nil {
		t.Fatal("full cognition was accepted by the benchmark-only ablation runner")
	}
	request.Variant = VariantRawShell
	if err := request.Validate(); err == nil {
		t.Fatal("raw shell was accepted outside the isolated filesystem surface")
	}
}

func ablationTestRequest(
	t *testing.T,
	variant Variant,
	surface Surface,
	repetition int,
	client *witnessPolicyClient,
) AblationRunRequest {
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
	return AblationRunRequest{
		Variant: variant, Surface: surface, RatGeneration: mustRatGeneration(t),
		RuntimeFingerprint: transferTestFingerprint(), Repetition: repetition,
		Actor: cognition.AttemptRef{
			JobID: 191, Generation: 3, StepID: 17, Attempt: 2, WorkerID: "ablation-runner",
		},
		Client: client, EpisodeSealPath: filepath.Join(episodeDirectory, "episode.json"),
		EvaluationPath:      filepath.Join(evaluationDirectory, "evaluation.json"),
		LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: ablationProjectionPolicyVersionV1,
	}
}
